package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/johnschult/antennarr/internal/proxy"
)

func main() {
	listen := flag.String("listen", ":5004", "address to listen on")
	tuner := flag.String("tuner", "", "tuner host or URL, e.g. 192.168.1.132 (required)")
	streamPort := flag.String("stream-port", "5004", "tuner's streaming port")
	advertise := flag.String("advertise", "", "base URL to hand to clients (default: the host they connected with)")
	deviceID := flag.String("device-id", "", "report this DeviceID instead of the tuner's (8 hex digits; for testing against a tuner your media server already knows)")
	flag.Parse()

	if *tuner == "" {
		fmt.Fprintln(os.Stderr, "antennarr: -tuner is required")
		flag.Usage()
		os.Exit(2)
	}
	api, err := parseTuner(*tuner)
	if err != nil {
		fmt.Fprintln(os.Stderr, "antennarr:", err)
		os.Exit(2)
	}
	stream := *api
	stream.Host = net.JoinHostPort(api.Hostname(), *streamPort)

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	log.Info("starting", "listen", *listen, "api", api.String(), "stream", stream.String())

	h := proxy.New(proxy.Config{API: api, Stream: &stream, Advertise: *advertise, DeviceID: *deviceID, Logger: log})
	if err := http.ListenAndServe(*listen, h); err != nil {
		log.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func parseTuner(s string) (*url.URL, error) {
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("bad -tuner %q", s)
	}
	return u, nil
}
