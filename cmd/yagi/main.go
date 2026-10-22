package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/johnschult/yagi/internal/access"
	"github.com/johnschult/yagi/internal/banner"
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

// deprecatedFlags are the ones that describe a device beyond its address. The admin page adds a tuner, learns its
// DeviceID and lets its port be changed, and the settings file holds the rest, so a flag for each is one way too
// many. They still work for now, and say so. -tuner (and YAGI_TUNER) stay, for a start with no settings to write.
var deprecatedFlags = []struct{ name, instead string }{
	{"listen", `add the tuner on the admin page, or set "listen" on it in the settings file`},
	{"tuner-id", `add the tuner on the admin page, which learns its DeviceID`},
	{"stream-port", `set "streamPort" on the tuner in the settings file`},
	{"advertise", `set "advertise" on the tuner in the settings file`},
	{"report-id", `set "reportId" on the tuner in the settings file`},
}

// warnDeprecated says, once for each, which of the flags that were given are on their way out.
func warnDeprecated(log *slog.Logger, set map[string]bool) {
	for _, d := range deprecatedFlags {
		if set[d.name] {
			log.Warn("-"+d.name+" is deprecated and will be removed in a future release: "+d.instead, "flag", "-"+d.name)
		}
	}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "discover" {
		os.Exit(runDiscover(os.Args[2:]))
	}

	var f deviceFlags
	flag.StringVar(&f.listen, "listen", ":5004", "DEPRECATED: address to listen on; add the tuner on the admin page, or set \"listen\" in the settings file")
	flag.StringVar(&f.tuner, "tuner", os.Getenv("YAGI_TUNER"), "tuner host or URL, e.g. 192.168.1.132 (also set by YAGI_TUNER)")
	flag.StringVar(&f.id, "tuner-id", "", "DEPRECATED: add the tuner on the admin page, which learns its DeviceID")
	flag.StringVar(&f.streamPort, "stream-port", "5004", "DEPRECATED: set \"streamPort\" in the settings file")
	flag.StringVar(&f.advertise, "advertise", "", "DEPRECATED: set \"advertise\" in the settings file")
	flag.StringVar(&f.reportID, "report-id", "", "DEPRECATED: set \"reportId\" in the settings file")
	adminListen := flag.String("admin-listen", os.Getenv("YAGI_ADMIN_LISTEN"), "where the admin pages are served, on a port of their own that no tuner uses (default \":9244\"; also set by YAGI_ADMIN_LISTEN or \"adminListen\" in the settings file)")
	allowLAN := flag.Bool("allow-lan", os.Getenv("YAGI_ALLOW_LAN") != "", "keep the LAN ports open even if \"tailnet only\" is on, to get back in if it left you locked out (also set by YAGI_ALLOW_LAN)")
	poll := flag.Duration("poll", 5*time.Second, "how often to check the tuners")
	firmwareURL := flag.String("firmware-url", "", "read the firmware changelog from here instead of SiliconDust's page (for a mirror, or testing)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	configPath := flag.String("config", defaultConfigPath, "settings file (devices, viewers, limits); created with defaults if it doesn't exist, reloaded on SIGHUP")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: yagi [flags]\n       yagi discover [-wait 3s] [host ...]\n\n"+
			"With no tuner named, Yagi starts without one; the admin page lists the HDHomeRun tuners it can see, to add.\n"+
			"Name a tuner with -tuner or -tuner-id, or declare devices under \"devices\" in the settings file.\n\n")
		flag.PrintDefaults()
	}
	hintAtMistypedFlags(os.Args[1:])
	flag.Parse()
	if *showVersion {
		fmt.Println("yagi", version.Get())
		return
	}

	set := map[string]bool{}
	flag.Visit(func(fl *flag.Flag) { set[fl.Name] = true })

	// What is logged goes to standard error as before, and the last few thousand lines are kept in memory for the
	// Logs page. The level starts at info and follows the log level setting once the settings are read.
	// Started by hand in a terminal, say who this is first. Never in a log: only a terminal gets it.
	banner.Print(os.Stderr, banner.ModeFor(isTerminal(os.Stderr), os.Getenv), version.Get())
	level := new(slog.LevelVar)
	logs := logbuf.New(2000)
	log := slog.New(logbuf.Tee(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}), logs))
	log.Info("starting", "version", version.Get())
	warnDeprecated(log, set)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	settings, savePath, err := loadOrCreateSettings(withLegacyConfig(*configPath, set["config"], log), set["config"], log)
	if err != nil {
		fail(err)
	}
	// With no tuner given anywhere, start without one. Tuners that Yagi can see are shown on the admin page, and
	// the person adds them: nothing is added without being asked.
	if len(settings.Devices) == 0 && f.tuner == "" && f.id == "" {
		log.Info("no tuner is set up yet. Open the admin page to add one; it lists the HDHomeRun tuners it can see. A tuner's own port (5004, …) opens only once one is added. If yours is on another network, or Yagi is in Docker without host networking, add it by its address (or start with -tuner ADDRESS or YAGI_TUNER)", "listen", f.listen)
	}
	devices, err := devicesFor(settings, f, set)
	if err != nil {
		fail(err)
	}

	// The flag and the environment win over the settings file, which wins over the default.
	admin := settings.AdminListen
	if *adminListen != "" {
		admin = *adminListen
	}
	if admin, err = access.ParseAdminListen(admin); err != nil {
		fail(err)
	}
	for _, d := range devices {
		if samePort(admin, d.Listen) {
			fail(fmt.Errorf("the admin pages and tuner %q are both on port %s: tuners never share the admin port, so give one of them another (-admin-listen, or -listen / \"listen\")", d.Name, portOf(admin)))
		}
	}
	srv, err := server.New(ctx, server.Config{
		AllowLAN: *allowLAN, Settings: settings, Devices: devices, ConfigPath: savePath, AdminListen: admin, TailscaleAuthKey: os.Getenv("TS_AUTHKEY"), Poll: *poll, Log: log, LogLevel: level, Logs: logs, FirmwareURL: *firmwareURL,
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

// hintAtMistypedFlags stops with a plain message for "-admin-listen:9244", which Go reports only as an
// unknown flag: a flag and its value go apart by a space or an equals sign.
func hintAtMistypedFlags(args []string) {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") || strings.Contains(a, "=") {
			continue
		}
		name, value, found := strings.Cut(strings.TrimLeft(a, "-"), ":")
		if found && name != "" && flag.Lookup(name) != nil {
			fail(fmt.Errorf("%s: put a space or an equals sign between the flag and its value, for example -%s :%s or -%s=:%s", a, name, value, name, value))
		}
	}
}

// isTerminal is whether f is a terminal rather than a file or a pipe (`docker logs` and systemd are pipes).
func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

func portOf(addr string) string {
	_, p, _ := net.SplitHostPort(addr)
	return p
}

func samePort(a, b string) bool { return portOf(a) != "" && portOf(a) == portOf(b) }

// defaultConfigPath is where the settings live when -config isn't given: next
// to wherever Yagi is run from. (The Docker image runs from /config.)
const defaultConfigPath = "yagi.json"

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
