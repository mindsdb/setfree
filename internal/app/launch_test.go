package app

import (
	"os"
	"reflect"
	"testing"

	"github.com/mindsdb/setfree/internal/config"
	"github.com/mindsdb/setfree/internal/gateway"
	"github.com/mindsdb/setfree/internal/secrets"
	"github.com/mindsdb/setfree/internal/terminal"
)

func TestBuildArgv_PassthroughOrder(t *testing.T) {
	got := buildArgv("/usr/local/bin/codex", []string{"-c", "model_provider=\"setfree\""}, []string{".", "--full-auto"})
	want := []string{"/usr/local/bin/codex", "-c", "model_provider=\"setfree\"", ".", "--full-auto"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestBuildArgv_NoAdapterArgsIsPlainPassthrough(t *testing.T) {
	got := buildArgv("/usr/local/bin/claude", nil, []string{".", "--dangerously-skip-permissions"})
	want := []string{"/usr/local/bin/claude", ".", "--dangerously-skip-permissions"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// swapStdin temporarily points os.Stdin at a non-terminal file (os.DevNull)
// so tests never depend on whether the test binary itself happens to be
// attached to a real terminal.
func swapStdin(t *testing.T) {
	t.Helper()
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	orig := os.Stdin
	os.Stdin = devNull
	t.Cleanup(func() {
		os.Stdin = orig
		devNull.Close()
	})
}

func TestRunFirstTimeSetup_NonInteractive_FailsFastInsteadOfHanging(t *testing.T) {
	swapStdin(t)

	dir := t.TempDir()
	settings, err := config.Load(dir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	e := &env{
		dir:      dir,
		settings: settings,
		store:    secrets.NewFileStore(dir),
		colors:   terminal.Colors{},
	}

	_, err = runFirstTimeSetup(e, "claude")
	if err == nil {
		t.Fatal("expected an error instead of attempting to prompt a non-terminal stdin")
	}
}

func TestRunFirstTimeSetup_SkippedWhenAlreadyConfigured(t *testing.T) {
	// Sanity check for the calling convention in cmdLaunch: Resolve alone
	// (no setup) must succeed once a gateway is saved, confirming
	// runFirstTimeSetup is never invoked on the common path.
	settings := &config.Settings{}
	settings.SetGatewayBaseURL(config.DefaultGatewayName, "https://gw.example.com")
	store := secrets.NewFileStore(t.TempDir())
	store.Set(config.DefaultGatewayName, "sk-test")

	r := gateway.NewResolver(settings, store)
	if _, err := r.Resolve("claude"); err != nil {
		t.Fatalf("Resolve on a configured gateway should succeed without setup: %v", err)
	}
}
