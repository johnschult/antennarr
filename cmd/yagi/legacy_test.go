package main

import (
	"io"
	"log/slog"
	"os"
	"testing"
)

func TestAnOldSettingsFileIsStillUsed(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	t.Chdir(t.TempDir())

	if got := withLegacyConfig(defaultConfigPath, false, log); got != defaultConfigPath {
		t.Errorf("with neither file = %q, want the new default", got)
	}
	if err := os.WriteFile(legacyConfigPath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := withLegacyConfig(defaultConfigPath, false, log); got != legacyConfigPath {
		t.Errorf("with only the old file = %q, want it used", got)
	}
	if got := withLegacyConfig(defaultConfigPath, true, log); got != defaultConfigPath {
		t.Errorf("an explicit -config = %q, must be left alone", got)
	}
	if got := withLegacyConfig("/etc/other.json", false, log); got != "/etc/other.json" {
		t.Errorf("another path = %q, must be left alone", got)
	}
	if err := os.WriteFile(defaultConfigPath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := withLegacyConfig(defaultConfigPath, false, log); got != defaultConfigPath {
		t.Errorf("with both files = %q, the new one wins", got)
	}
}
