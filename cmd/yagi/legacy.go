package main

import (
	"log/slog"
	"os"
)

// legacyConfigPath is the settings file's name before the project was renamed.
const legacyConfigPath = "antennarr.json"

// withLegacyConfig keeps an existing install working after the rename: with no
// -config given, no yagi.json yet, and an antennarr.json where it was started,
// the old file is used (and a line says to rename it) instead of starting over
// with empty settings. Nothing on disk is renamed for the owner.
func withLegacyConfig(path string, explicit bool, log *slog.Logger) string {
	if explicit || path != defaultConfigPath {
		return path
	}
	if _, err := os.Stat(path); err == nil {
		return path
	}
	if _, err := os.Stat(legacyConfigPath); err != nil {
		return path
	}
	log.Warn("using the settings file from before the rename; rename it to "+defaultConfigPath+" (and update any -config you pass) to stop seeing this", "file", legacyConfigPath)
	return legacyConfigPath
}
