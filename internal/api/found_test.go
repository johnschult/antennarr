package api

import "testing"

func TestSuggestedNames(t *testing.T) {
	for _, c := range []struct{ friendly, id, want string }{
		{"HDHomeRun FLEX 4K", "10ABCC87", "flex-4k-87"},
		{"HDHomeRun CONNECT QUATRO", "1234ABCD", "connect-quatro-cd"},
		{"", "1234ABCD", "tuner-cd"},
		{"  ", "", "tuner"},
	} {
		if got := suggestName(c.friendly, c.id); got != c.want {
			t.Errorf("suggestName(%q, %q) = %q, want %q", c.friendly, c.id, got, c.want)
		}
	}
}
