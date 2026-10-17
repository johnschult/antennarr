package version

import (
	"strings"
	"testing"
)

func TestGetReturnsTheBuiltVersion(t *testing.T) {
	old := Version
	defer func() { Version = old }()
	Version = "2026.10.0"
	if got := Get(); got != "2026.10.0" {
		t.Errorf("Get() = %q", got)
	}
}

func TestADevBuildSaysDev(t *testing.T) {
	old := Version
	defer func() { Version = old }()
	for _, v := range []string{"dev", ""} {
		Version = v
		if got := Get(); got != "dev" && !strings.HasPrefix(got, "dev+") {
			t.Errorf("Version=%q: Get() = %q, want dev or dev+<commit>", v, got)
		}
	}
}

func TestWhichNumbersAreReleases(t *testing.T) {
	for _, v := range []string{"2026.10.0", "2026.1.12", "2026.01.3", "2027.12.0"} {
		if !IsRelease(v) {
			t.Errorf("%q should be a release", v)
		}
	}
	for _, v := range []string{"", "dev", "dev+abc1234", "2026.13.0", "2026.0.1", "2026.10", "v2026.10.0", "2026.10.0-rc1", "1.2.3", "2026.10.0\n; rm"} {
		if IsRelease(v) {
			t.Errorf("%q should not be a release", v)
		}
	}
}

func TestURLPointsAtTheReleaseOrTheProject(t *testing.T) {
	if got := URL("2026.10.0"); got != Repo+"/releases/tag/v2026.10.0" {
		t.Errorf("release URL = %q", got)
	}
	for _, v := range []string{"dev", "dev+abc1234", ""} {
		if got := URL(v); got != Repo {
			t.Errorf("URL(%q) = %q, want the project page", v, got)
		}
	}
}
