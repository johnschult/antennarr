package main

import "testing"

func TestParseTuner(t *testing.T) {
	good := map[string]string{
		"192.168.1.132":          "http://192.168.1.132",
		"192.168.1.132:80":       "http://192.168.1.132:80",
		"http://192.168.1.132":   "http://192.168.1.132",
		"hdhr.example.ts.net":    "http://hdhr.example.ts.net",
		"https://tuner.internal": "https://tuner.internal",
	}
	for in, want := range good {
		u, err := parseTuner(in)
		if err != nil {
			t.Errorf("parseTuner(%q): %v", in, err)
			continue
		}
		if u.String() != want {
			t.Errorf("parseTuner(%q) = %q, want %q", in, u, want)
		}
	}

	for _, in := range []string{"", "http://", "://x", "   "} {
		if _, err := parseTuner(in); err == nil {
			t.Errorf("parseTuner(%q) succeeded, want an error", in)
		}
	}
}
