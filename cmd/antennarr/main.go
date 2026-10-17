package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/johnschult/antennarr/internal/access"
	"github.com/johnschult/antennarr/internal/discovery"
	"github.com/johnschult/antennarr/internal/server"
	"github.com/johnschult/antennarr/internal/version"
)

// deviceFlags are the command-line way to describe a single device.
type deviceFlags struct {
	listen, tuner, id, streamPort, advertise, reportID string
}

// flagNames lists the flags that describe a device, for spotting conflicts.
var flagNames = []string{"listen", "tuner", "tuner-id", "stream-port", "advertise", "report-id"}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "discover" {
		os.Exit(runDiscover(os.Args[2:]))
	}

	var f deviceFlags
	flag.StringVar(&f.listen, "listen", ":5004", "address to listen on")
	flag.StringVar(&f.tuner, "tuner", "", "tuner host or URL, e.g. 192.168.1.132")
	flag.StringVar(&f.id, "tuner-id", "", "DeviceID of the tuner to follow if its address changes (learned from -tuner if omitted)")
	flag.StringVar(&f.streamPort, "stream-port", "5004", "tuner's streaming port")
	flag.StringVar(&f.advertise, "advertise", "", "base URL to hand to clients (default: the host they connected with)")
	flag.StringVar(&f.reportID, "report-id", "", "report this DeviceID to clients instead of the tuner's (8 hex digits; for testing against a tuner your media server already knows)")
	poll := flag.Duration("poll", 5*time.Second, "how often to check the tuners")
	firmwareURL := flag.String("firmware-url", "", "read the firmware changelog from here instead of SiliconDust's page (for a mirror, or testing)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	configPath := flag.String("config", "", "settings file (devices, viewers, limits); reloaded on SIGHUP. Without one, anyone who can reach a port may use the tuner")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: antennarr [flags]\n       antennarr discover [-wait 3s] [host ...]\n\n"+
			"Run one device with -tuner or -tuner-id, or declare several under \"devices\" in the -config file.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println("antennarr", version.Get())
		return
	}

	set := map[string]bool{}
	flag.Visit(func(fl *flag.Flag) { set[fl.Name] = true })

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	log.Info("starting", "version", version.Get())
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	settings := access.DefaultSettings()
	if *configPath != "" {
		var exists bool
		var err error
		if settings, exists, err = access.LoadSettings(*configPath); err != nil {
			fail(err)
		}
		if !exists {
			log.Warn("settings file not found, using defaults", "path", *configPath)
		}
	}
	devices, err := devicesFor(settings, f, set)
	if err != nil {
		fail(err)
	}

	srv, err := server.New(ctx, server.Config{
		Settings: settings, Devices: devices, ConfigPath: *configPath, Poll: *poll, Log: log, FirmwareURL: *firmwareURL,
	})
	if err != nil {
		fail(err)
	}
	go reloadOnHUP(ctx, *configPath, srv, log)

	if err := srv.Run(ctx); err != nil {
		log.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "antennarr:", err)
	os.Exit(2)
}

// devicesFor decides which devices to run: those declared in the settings
// file, or else the single one described by the flags. Mixing the two would
// leave it unclear which wins, so that is an error.
func devicesFor(s access.Settings, f deviceFlags, set map[string]bool) ([]access.Device, error) {
	if len(s.Devices) > 0 {
		var given []string
		for _, n := range flagNames {
			if set[n] {
				given = append(given, "-"+n)
			}
		}
		if len(given) > 0 {
			return nil, fmt.Errorf("%s can't be combined with devices declared in the settings file; set them there instead", strings.Join(given, ", "))
		}
		return s.Devices, nil
	}
	if f.tuner == "" && f.id == "" {
		return nil, errors.New("give -tuner (an address) or -tuner-id (a DeviceID to search for), or declare devices in a -config file")
	}
	d := access.Device{
		Name: "tuner", Listen: f.listen, Tuner: f.tuner, ID: f.id,
		StreamPort: f.streamPort, Advertise: f.advertise, ReportID: f.reportID,
	}
	if err := d.Validate(); err != nil {
		return nil, err
	}
	return []access.Device{d}, nil
}

// reloadOnHUP re-reads the settings file when the process gets SIGHUP. A bad
// file is reported and the running settings are kept.
func reloadOnHUP(ctx context.Context, path string, srv *server.Server, log *slog.Logger) {
	if path == "" {
		return
	}
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	defer signal.Stop(hup)
	for {
		select {
		case <-ctx.Done():
			return
		case <-hup:
			if err := srv.ReloadFile(path); err != nil {
				log.Error("settings reload failed, keeping the current ones", "err", err)
			}
		}
	}
}

func runDiscover(args []string) int {
	fs := flag.NewFlagSet("discover", flag.ExitOnError)
	wait := fs.Duration("wait", 3*time.Second, "how long to listen for answers")
	_ = fs.Parse(args)

	devs, err := discovery.Discover(context.Background(), *wait, fs.Args()...)
	if err != nil {
		fmt.Fprintln(os.Stderr, "antennarr discover:", err)
		return 1
	}
	if len(devs) == 0 {
		fmt.Println("no HDHomeRun tuners answered")
		return 1
	}
	for _, d := range devs {
		fmt.Printf("%s  %s  %d tuners\n", d.ID, d.BaseURL, d.TunerCount)
	}
	return 0
}
