package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/johnschult/yagi/internal/access"
	"github.com/johnschult/yagi/internal/discovery"
	"github.com/johnschult/yagi/internal/logbuf"
	"github.com/johnschult/yagi/internal/server"
	"github.com/johnschult/yagi/internal/version"
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
	adminListen := flag.String("admin-listen", os.Getenv("YAGI_ADMIN_LISTEN"), "serve the admin pages on their own address, e.g. \":5000\", and only there; the tuner ports then carry nothing but the tuners (also set by YAGI_ADMIN_LISTEN)")
	poll := flag.Duration("poll", 5*time.Second, "how often to check the tuners")
	firmwareURL := flag.String("firmware-url", "", "read the firmware changelog from here instead of SiliconDust's page (for a mirror, or testing)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	configPath := flag.String("config", defaultConfigPath, "settings file (devices, viewers, limits); created with defaults if it doesn't exist, reloaded on SIGHUP")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: yagi [flags]\n       yagi discover [-wait 3s] [host ...]\n\n"+
			"With no -tuner, Yagi looks for HDHomeRun tuners on the network and remembers them in the settings file.\n"+
			"Name a tuner with -tuner or -tuner-id, or declare devices under \"devices\" in the settings file.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println("yagi", version.Get())
		return
	}

	set := map[string]bool{}
	flag.Visit(func(fl *flag.Flag) { set[fl.Name] = true })

	// What is logged goes to standard error as before, and the last few thousand lines are kept in memory for the
	// Logs page. The level starts at info and follows the log level setting once the settings are read.
	level := new(slog.LevelVar)
	logs := logbuf.New(2000)
	log := slog.New(logbuf.Tee(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}), logs))
	log.Info("starting", "version", version.Get())
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	settings, savePath, err := loadOrCreateSettings(withLegacyConfig(*configPath, set["config"], log), set["config"], log)
	if err != nil {
		fail(err)
	}
	// With no tuner given anywhere, find one: nobody should have to write a
	// config file before the first start. What is found is saved to the settings
	// file, so it is the same tuner next time and can be edited from then on.
	if wantsAutoDiscovery(settings, set) {
		// Look for a short while. If nothing answers, start anyway with no tuner:
		// the admin page is there, and a tuner can be added from it (or found
		// again with its Find button) once it is plugged in or reachable.
		sctx, cancel := context.WithTimeout(ctx, startupSearch)
		found, err := findTuners(sctx, f.listen, func(ctx context.Context, wait time.Duration) ([]discovery.Device, error) {
			return discovery.Discover(ctx, wait)
		}, discoverRetry, log)
		cancel()
		switch {
		case err == nil:
			settings.Devices = found
			for _, d := range found {
				log.Info("found a tuner", "device", d.Name, "id", d.ID, "at", d.Tuner, "listen", d.Listen)
			}
			if savePath != "" {
				if err := access.SaveSettings(savePath, settings); err != nil {
					log.Warn("couldn't save the tuners that were found; they will be looked for again next start", "path", savePath, "err", err)
				} else {
					log.Info("saved them to the settings file", "path", savePath)
				}
			}
		case ctx.Err() != nil:
			return // asked to stop while still looking
		case errors.Is(err, context.DeadlineExceeded):
			log.Info("no tuner found, so starting without one. Open the admin page to add one; if yours is on another network, add it by its address", "listen", f.listen)
		default:
			fail(err)
		}
	}
	devices, err := devicesFor(settings, f, set)
	if err != nil {
		fail(err)
	}

	admin := f.listen
	if *adminListen != "" {
		if admin, err = adminAddress(*adminListen); err != nil {
			fail(err)
		}
		if _, ap, _ := net.SplitHostPort(admin); ap != "" {
			if _, lp, _ := net.SplitHostPort(f.listen); ap == lp {
				fail(fmt.Errorf("-admin-listen and -listen are both port %s: -listen is where the tuners start, so the admin pages need another", ap))
			}
		}
	}
	srv, err := server.New(ctx, server.Config{
		Settings: settings, Devices: devices, ConfigPath: savePath, AdminListen: admin, AdminSeparate: *adminListen != "", Poll: *poll, Log: log, LogLevel: level, Logs: logs, FirmwareURL: *firmwareURL,
	})
	if err != nil {
		fail(err)
	}
	go reloadOnHUP(ctx, savePath, srv, log)

	if err := srv.Run(ctx); err != nil {
		log.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

// adminAddress reads -admin-listen: a port (":5000" or "5000") or an address ("127.0.0.1:5000").
func adminAddress(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v != "" && strings.Trim(v, "0123456789") == "" {
		v = ":" + v
	}
	if _, port, err := net.SplitHostPort(v); err != nil {
		return "", fmt.Errorf("-admin-listen %q must look like \":5000\" or \"127.0.0.1:5000\"", v)
	} else if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return "", fmt.Errorf("-admin-listen %q has a bad port", v)
	}
	return v, nil
}

// defaultConfigPath is where the settings live when -config isn't given: next
// to wherever Yagi is run from. (The Docker image runs from /config.)
const defaultConfigPath = "yagi.json"

// startupSearch is how long a start with no tuner named looks for one before
// giving up and starting without.
const startupSearch = 10 * time.Second

// loadOrCreateSettings reads the settings file, and writes one with the
// defaults if there is none yet, so there is always a file to edit and the
// admin page can always save. It returns the settings and the path they are
// kept at.
//
// If the file can't be created, a -config the user asked for by name is an
// error. The default path is not: Yagi then runs as it always did without
// a file, with edits turned off (savePath is empty), and says so.
func loadOrCreateSettings(path string, explicit bool, log *slog.Logger) (s access.Settings, savePath string, err error) {
	s, exists, err := access.LoadSettings(path)
	if err != nil {
		return s, "", err
	}
	if exists {
		return s, path, nil
	}
	if err := access.SaveSettings(path, s); err != nil {
		if explicit {
			return s, "", fmt.Errorf("creating the settings file: %w", err)
		}
		log.Warn("couldn't create a settings file; running with the defaults, and settings can't be saved. Use -config to say where it should go", "path", path, "err", err)
		return s, "", nil
	}
	log.Info("created a settings file with the defaults", "path", path)
	return s, path, nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "yagi:", err)
	os.Exit(2)
}

// devicesFor decides which devices to run: those declared in the settings
// file, or else the single one described by the flags, or none at all (tuners
// can be added from the admin page). Mixing the first two would leave it
// unclear which wins, so that is an error.
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
		// None named and none found: start without one. Tuners are added from the
		// admin page.
		return nil, nil
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
		fmt.Fprintln(os.Stderr, "yagi discover:", err)
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
