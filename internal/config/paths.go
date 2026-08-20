package config

import (
	"os"
	"path/filepath"
)

// Dir returns SetFree's per-user configuration directory, creating it if
// necessary. It uses the operating system's standard per-user config
// location (os.UserConfigDir): Application Support on macOS, XDG_CONFIG_HOME
// (or ~/.config) on Linux, and %AppData% on Windows.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "setfree")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// SettingsPath returns the path to SetFree's non-secret settings file.
func SettingsPath(dir string) string {
	return filepath.Join(dir, "config.toml")
}
