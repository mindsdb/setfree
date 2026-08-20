// Package config manages SetFree's own non-secret settings file. It never
// reads or writes configuration belonging to the coding CLIs it launches.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// CurrentVersion is the schema version written by this build of SetFree.
const CurrentVersion = 1

// Settings is SetFree's normalized, non-secret configuration. API keys are
// never stored here; see the secrets package.
type Settings struct {
	Version        int                       `toml:"version"`
	DefaultGateway string                    `toml:"default_gateway,omitempty"`
	Gateways       map[string]GatewaySetting `toml:"gateways,omitempty"`
	CLI            map[string]CLISetting     `toml:"cli,omitempty"`
}

// GatewaySetting holds the non-secret half of a configured gateway.
type GatewaySetting struct {
	BaseURL string `toml:"base_url"`
}

// CLISetting holds per-coding-CLI preferences, such as a pinned model.
type CLISetting struct {
	Model string `toml:"model,omitempty"`
}

// DefaultGatewayName is the gateway name SetFree uses until multi-gateway
// commands (`gateway add`, `--gateway <name>`) exist.
const DefaultGatewayName = "default"

// Empty reports whether no gateway has been configured yet.
func (s *Settings) Empty() bool {
	return s == nil || len(s.Gateways) == 0
}

// Default returns the settings' default gateway, if any is configured.
func (s *Settings) Default() (GatewaySetting, string, bool) {
	if s == nil || len(s.Gateways) == 0 {
		return GatewaySetting{}, "", false
	}
	name := s.DefaultGateway
	if name == "" {
		name = "default"
	}
	gw, ok := s.Gateways[name]
	return gw, name, ok
}

// SetGatewayBaseURL records base_url for the named gateway, creating it if
// needed, and marks it as the default gateway if none is set yet.
func (s *Settings) SetGatewayBaseURL(name, baseURL string) {
	if s.Gateways == nil {
		s.Gateways = map[string]GatewaySetting{}
	}
	s.Gateways[name] = GatewaySetting{BaseURL: baseURL}
	if s.DefaultGateway == "" {
		s.DefaultGateway = name
	}
}

// Load reads settings from dir. A missing file is not an error: it returns
// fresh, empty settings so callers can distinguish "not configured yet"
// from a corrupt file.
func Load(dir string) (*Settings, error) {
	path := SettingsPath(dir)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Settings{Version: CurrentVersion}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var s Settings
	if err := toml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if s.Version > CurrentVersion {
		return nil, fmt.Errorf("%s was written by a newer version of SetFree (schema v%d, this build supports up to v%d) — please upgrade SetFree", path, s.Version, CurrentVersion)
	}
	if s.Version == 0 {
		s.Version = CurrentVersion
	}
	return &s, nil
}

// Save writes settings to dir atomically, replacing any existing file.
func Save(dir string, s *Settings) error {
	if s.Version == 0 {
		s.Version = CurrentVersion
	}
	path := SettingsPath(dir)

	data, err := toml.Marshal(s)
	if err != nil {
		return fmt.Errorf("encoding settings: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".config-*.toml.tmp")
	if err != nil {
		return fmt.Errorf("writing settings: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("writing settings: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing settings: %w", err)
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return fmt.Errorf("writing settings: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("writing settings: %w", err)
	}
	return nil
}

// Reset removes SetFree's settings file. It never touches configuration
// belonging to the coding CLIs SetFree launches.
func Reset(dir string) error {
	err := os.Remove(SettingsPath(dir))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Path is a small convenience for callers (e.g. `config show`) that want to
// display where settings live without reaching into paths.go directly.
func Path(dir string) string {
	return filepath.Clean(SettingsPath(dir))
}
