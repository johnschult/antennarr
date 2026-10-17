package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/johnschult/antennarr/internal/access"
	"github.com/johnschult/antennarr/internal/api"
	"github.com/johnschult/antennarr/internal/discovery"
	"github.com/johnschult/antennarr/internal/live"
	"github.com/johnschult/antennarr/internal/proxy"
	"github.com/johnschult/antennarr/internal/tuner"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "discover" {
		os.Exit(runDiscover(os.Args[2:]))
	}

	listen := flag.String("listen", ":5004", "address to listen on")
	tunerAddr := flag.String("tuner", "", "tuner host or URL, e.g. 192.168.1.132")
	tunerID := flag.String("tuner-id", "", "DeviceID of the tuner to follow if its address changes (learned from -tuner if omitted)")
	streamPort := flag.String("stream-port", "5004", "tuner's streaming port")
	advertise := flag.String("advertise", "", "base URL to hand to clients (default: the host they connected with)")
	reportID := flag.String("report-id", "", "report this DeviceID to clients instead of the tuner's (8 hex digits; for testing against a tuner your media server already knows)")
	poll := flag.Duration("poll", 5*time.Second, "how often to check the tuner")
	configPath := flag.String("config", "", "settings file (viewers, limits); reloaded on SIGHUP. Without one, anyone who can reach the port may use the tuner")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: antennarr [flags]\n       antennarr discover [-wait 3s] [host ...]\n\nneeds -tuner or -tuner-id.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	base, err := startAddress(ctx, *tunerAddr, *tunerID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "antennarr:", err)
		os.Exit(2)
	}

	hub := live.NewHub()
	mon := tuner.NewMonitor(tuner.Config{
		BaseURL: base, ID: *tunerID, StreamPort: *streamPort, Interval: *poll, OnChange: hub.Notify, Logger: log,
	})
	go mon.Run(ctx)

	settings := access.DefaultSettings()
	if *configPath != "" {
		var exists bool
		if settings, exists, err = access.LoadSettings(*configPath); err != nil {
			fmt.Fprintln(os.Stderr, "antennarr:", err)
			os.Exit(2)
		}
		if !exists {
			log.Warn("settings file not found, using defaults", "path", *configPath)
		}
	}
	policy, err := access.NewPolicy(settings, mon, mon, log)
	if err != nil {
		fmt.Fprintln(os.Stderr, "antennarr:", err)
		os.Exit(2)
	}
	if len(settings.Viewers) == 0 {
		log.Warn("no viewers configured: anyone who can reach this port can use the tuner", "ownerReserve", settings.OwnerReserve)
	}
	// A stream starting or ending changes the tuner's own status too: tell the
	// dashboards now, and have the monitor look again straight away.
	policy.SetOnChange(func() { hub.Notify(); mon.Kick() })
	go reloadOnHUP(ctx, *configPath, policy, log)

	mux := http.NewServeMux()
	mux.Handle("/antennarr/status", policy.AdminOnly(tuner.StatusHandler(mon)))
	mux.Handle("/antennarr/streams", policy.AdminOnly(policy.ActiveHandler()))
	mux.Handle(api.Prefix, policy.AdminOnly((&api.API{
		Mon: mon, Pol: policy, Store: access.NewStore(*configPath, policy), Hub: hub, Log: log,
	}).Handler()))
	mux.Handle("/", proxy.New(proxy.Config{Upstream: mon, Policy: policy, Advertise: *advertise, DeviceID: *reportID, Logger: log}))

	// No overall timeouts: streams run for hours. Only the header read is
	// bounded, so a stalled client can't hold a connection open for free.
	srv := &http.Server{Addr: *listen, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()

	log.Info("starting", "listen", *listen, "tuner", base, "tunerID", strings.ToUpper(*tunerID))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

// reloadOnHUP re-reads the settings file when the process gets SIGHUP. A bad
// file is reported and the running settings are kept.
func reloadOnHUP(ctx context.Context, path string, policy *access.Policy, log *slog.Logger) {
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
			s, _, err := access.LoadSettings(path)
			if err == nil {
				err = policy.Update(s)
			}
			if err != nil {
				log.Error("settings reload failed, keeping the current ones", "err", err)
				continue
			}
			log.Info("settings reloaded", "viewers", len(s.Viewers), "sharedCap", s.SharedCap, "ownerReserve", s.OwnerReserve)
		}
	}
}

// startAddress works out where to look for the tuner first: the -tuner flag,
// or failing that a network search for -tuner-id.
func startAddress(ctx context.Context, addr, id string) (string, error) {
	if addr != "" {
		u, err := parseTuner(addr)
		if err != nil {
			return "", err
		}
		return u.String(), nil
	}
	if id == "" {
		return "", fmt.Errorf("give -tuner (an address) or -tuner-id (a DeviceID to search for)")
	}
	fctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	devs, err := discovery.Discover(fctx, 3*time.Second)
	if err != nil && len(devs) == 0 {
		return "", fmt.Errorf("searching for tuner %s: %w", id, err)
	}
	for _, d := range devs {
		if strings.EqualFold(d.ID, id) {
			return d.BaseURL, nil
		}
	}
	return "", fmt.Errorf("tuner %s not found on the network; pass -tuner with its address", strings.ToUpper(id))
}

func parseTuner(s string) (*url.URL, error) {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("bad -tuner %q", s)
	}
	return u, nil
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
