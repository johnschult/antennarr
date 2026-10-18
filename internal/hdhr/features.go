package hdhr

import "strings"

// Features is what a tuner says it can do (/sys/features): lines such as
//
//	channelmap: us-bcast us-cable us-hrc
//	modulation: atsc3 8vsb qam256 qam64
//	auto-modulation: auto auto6t auto6c qam
type Features struct {
	ChannelMaps []string
	Modulations []string
}

// ParseFeatures reads /sys/features. Lines and words it doesn't know are ignored, so a tuner that says
// more than this knows about still parses.
func ParseFeatures(s string) Features {
	var f Features
	for _, line := range strings.Split(s, "\n") {
		key, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "channelmap":
			f.ChannelMaps = strings.Fields(rest)
		case "modulation":
			f.Modulations = strings.Fields(rest)
		}
	}
	return f
}
