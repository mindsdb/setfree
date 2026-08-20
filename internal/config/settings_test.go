package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_MissingFileReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	s, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !s.Empty() {
		t.Error("expected Empty() on fresh settings")
	}
	if s.Version != CurrentVersion {
		t.Errorf("Version = %d, want %d", s.Version, CurrentVersion)
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := &Settings{Version: CurrentVersion, DefaultGateway: "default"}
	s.SetGatewayBaseURL("default", "https://gw.example.com")
	s.CLI = map[string]CLISetting{"claude": {Model: "kimi-k2.5"}}

	if err := Save(dir, s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	gw, name, ok := loaded.Default()
	if !ok || name != "default" || gw.BaseURL != "https://gw.example.com" {
		t.Errorf("Default() = %+v, %q, %v", gw, name, ok)
	}
	if loaded.CLI["claude"].Model != "kimi-k2.5" {
		t.Errorf("CLI[claude].Model = %q", loaded.CLI["claude"].Model)
	}
}

func TestLoad_RejectsNewerSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	future := &Settings{Version: CurrentVersion + 1}
	if err := Save(dir, future); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := Load(dir); err == nil {
		t.Error("expected Load to reject a config from a newer schema version")
	}
}

func TestReset_RemovesFileButNotDirectory(t *testing.T) {
	dir := t.TempDir()
	s := &Settings{}
	s.SetGatewayBaseURL("default", "https://gw.example.com")
	if err := Save(dir, s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := Reset(dir); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if _, err := os.Stat(SettingsPath(dir)); !os.IsNotExist(err) {
		t.Errorf("expected config file to be removed, stat err = %v", err)
	}

	// Reset on an already-clean dir must not error.
	if err := Reset(dir); err != nil {
		t.Fatalf("Reset on missing file: %v", err)
	}
}

func TestSave_DoesNotLeaveTempFiles(t *testing.T) {
	dir := t.TempDir()
	s := &Settings{}
	s.SetGatewayBaseURL("default", "https://gw.example.com")
	if err := Save(dir, s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(SettingsPath(dir)) {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("dir contents = %v, want only config.toml", names)
	}
}

func TestSettings_DefaultGatewayNaming(t *testing.T) {
	s := &Settings{}
	s.SetGatewayBaseURL(DefaultGatewayName, "https://gw.example.com")
	if s.DefaultGateway != DefaultGatewayName {
		t.Errorf("DefaultGateway = %q, want %q", s.DefaultGateway, DefaultGatewayName)
	}

	// A second gateway must not steal the default.
	s.SetGatewayBaseURL("work", "https://work.example.com")
	if s.DefaultGateway != DefaultGatewayName {
		t.Errorf("DefaultGateway changed to %q after adding a second gateway", s.DefaultGateway)
	}
}

func TestCLISetting_ModelDiscoveryFields_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := &Settings{}
	s.SetGatewayBaseURL(DefaultGatewayName, "https://gw.example.com")
	s.CLI = map[string]CLISetting{
		"claude": {ModelDiscovery: true, ModelResolved: true},
		"codex":  {Model: "gpt-5", ModelResolved: true},
	}
	if err := Save(dir, s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := loaded.CLI["claude"]; !got.ModelDiscovery || !got.ModelResolved || got.Model != "" {
		t.Errorf("claude setting = %+v", got)
	}
	if got := loaded.CLI["codex"]; got.Model != "gpt-5" || !got.ModelResolved || got.ModelDiscovery {
		t.Errorf("codex setting = %+v", got)
	}
}

func TestSetGatewayBaseURL_ChangingURLClearsModelChoices(t *testing.T) {
	s := &Settings{}
	s.SetGatewayBaseURL(DefaultGatewayName, "https://old.example.com")
	s.CLI = map[string]CLISetting{
		"claude": {ModelDiscovery: true, ModelResolved: true},
		"codex":  {Model: "gpt-5", ModelResolved: true},
	}

	// Setting the SAME url again must not wipe out what was just learned.
	s.SetGatewayBaseURL(DefaultGatewayName, "https://old.example.com")
	if len(s.CLI) != 2 {
		t.Fatalf("re-setting the same base URL should not clear model choices, CLI = %+v", s.CLI)
	}

	// Changing to a DIFFERENT url must invalidate every CLI's model choice:
	// a different endpoint may serve entirely different models.
	s.SetGatewayBaseURL(DefaultGatewayName, "https://new.example.com")
	if len(s.CLI) != 0 {
		t.Errorf("changing base URL should clear model choices, CLI = %+v", s.CLI)
	}
}
