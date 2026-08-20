package app

import (
	"runtime"
	"testing"

	"github.com/mindsdb/setfree/internal/config"
	"github.com/mindsdb/setfree/internal/secrets"
)

// setTempConfigHome points config.Dir() at a throwaway directory for the
// duration of the test, regardless of platform, so these tests never touch
// the real developer's SetFree configuration.
func setTempConfigHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("AppData", dir)
	case "darwin":
		t.Setenv("HOME", dir)
	default:
		t.Setenv("HOME", dir)
		t.Setenv("XDG_CONFIG_HOME", dir)
	}
}

func TestConfigSet_RequiresAtLeastOneFlag(t *testing.T) {
	setTempConfigHome(t)
	if code := cmdConfigSet(nil); code == 0 {
		t.Error("expected a nonzero exit code with no flags")
	}
}

func TestConfigSet_ThenShowThenReset(t *testing.T) {
	setTempConfigHome(t)

	if code := cmdConfigSet([]string{"--base-url", "https://gw.example.com", "--api-key", "sk-test"}); code != 0 {
		t.Fatalf("cmdConfigSet exit code = %d", code)
	}

	dir, err := config.Dir()
	if err != nil {
		t.Fatalf("config.Dir: %v", err)
	}
	settings, err := config.Load(dir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	gw, name, ok := settings.Default()
	if !ok || gw.BaseURL != "https://gw.example.com" {
		t.Fatalf("Default() = %+v, %q, %v", gw, name, ok)
	}
	key, ok, err := secrets.NewFileStore(dir).Get(name)
	if err != nil || !ok || key != "sk-test" {
		t.Fatalf("stored key = %q, %v, %v", key, ok, err)
	}

	if code := cmdConfigShow(); code != 0 {
		t.Fatalf("cmdConfigShow exit code = %d", code)
	}

	if code := cmdConfigReset([]string{"--force"}); code != 0 {
		t.Fatalf("cmdConfigReset exit code = %d", code)
	}
	after, err := config.Load(dir)
	if err != nil {
		t.Fatalf("config.Load after reset: %v", err)
	}
	if !after.Empty() {
		t.Error("expected reset to clear the configured gateway")
	}
	if _, ok, _ := secrets.NewFileStore(dir).Get(name); ok {
		t.Error("expected reset to clear the stored API key")
	}
}

func TestConfigReset_NonInteractiveWithoutForceFails(t *testing.T) {
	setTempConfigHome(t)
	swapStdin(t)

	if code := cmdConfigReset(nil); code == 0 {
		t.Error("expected 'config reset' to require --force when stdin isn't a terminal")
	}
}

func TestConfigMenu_NonInteractiveFailsInsteadOfHanging(t *testing.T) {
	setTempConfigHome(t)
	swapStdin(t)

	if code := cmdConfigMenu(); code == 0 {
		t.Error("expected the interactive menu to refuse a non-terminal stdin")
	}
}
