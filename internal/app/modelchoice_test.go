package app

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/mindsdb/setfree/internal/adapters"
	"github.com/mindsdb/setfree/internal/config"
	"github.com/mindsdb/setfree/internal/gateway"
	"github.com/mindsdb/setfree/internal/secrets"
	"github.com/mindsdb/setfree/internal/terminal"
)

// fakeAdapter is a minimal adapters.Adapter double for testing resolveModel
// without a real coding CLI or a real gateway.
type fakeAdapter struct {
	name      string
	discovery adapters.Discovery
}

func (f fakeAdapter) Name() string          { return f.name }
func (f fakeAdapter) DisplayName() string   { return "Fake CLI" }
func (f fakeAdapter) BinaryNames() []string { return []string{f.name} }
func (f fakeAdapter) Build(baseEnv []string, resolved gateway.Resolved) (adapters.Build, error) {
	return adapters.Build{Env: baseEnv}, nil
}
func (f fakeAdapter) DiscoverModels(ctx context.Context, gw gateway.Gateway) adapters.Discovery {
	return f.discovery
}

func newTestEnv(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	settings := &config.Settings{}
	settings.SetGatewayBaseURL(config.DefaultGatewayName, "https://gw.example.com")
	return &env{
		dir:      dir,
		settings: settings,
		store:    secrets.NewFileStore(dir),
		colors:   terminal.Colors{},
	}
}

func TestResolveModel_UnsupportedEndpoint_PromptsForModelName(t *testing.T) {
	e := newTestEnv(t)
	a := fakeAdapter{name: "claude", discovery: adapters.Discovery{Supported: false}}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig })
	go func() {
		io.WriteString(w, "custom-model\n")
		w.Close()
	}()

	interacted, err := resolveModel(e, a, gateway.Gateway{BaseURL: "https://gw.example.com"})
	if err != nil {
		t.Fatalf("resolveModel: %v", err)
	}
	if !interacted {
		t.Error("prompting for a model name should count as user interaction")
	}
	got := e.settings.CLI["claude"]
	if got.Model != "custom-model" {
		t.Errorf("Model = %q, want %q", got.Model, "custom-model")
	}
	if got.ModelResolved {
		t.Error("an unsupported endpoint should not be marked resolved, so it's asked again next launch")
	}
}

func TestResolveModel_UnsupportedEndpoint_SkipLeavesModelEmpty(t *testing.T) {
	e := newTestEnv(t)
	a := fakeAdapter{name: "claude", discovery: adapters.Discovery{Supported: false}}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig })
	go func() {
		io.WriteString(w, "\n") // blank: skip pinning a model
		w.Close()
	}()

	interacted, err := resolveModel(e, a, gateway.Gateway{BaseURL: "https://gw.example.com"})
	if err != nil {
		t.Fatalf("resolveModel: %v", err)
	}
	if !interacted {
		t.Error("even a skipped prompt should count as user interaction")
	}
	got := e.settings.CLI["claude"]
	if got.Model != "" || got.ModelResolved {
		t.Errorf("claude setting = %+v, want Model empty and ModelResolved false", got)
	}
}

func TestResolveModel_NativeModels_SetsDiscoveryFlagNoPrompt(t *testing.T) {
	e := newTestEnv(t)
	a := fakeAdapter{name: "claude", discovery: adapters.Discovery{
		Supported: true, Native: true, Models: []string{"claude-3-5-sonnet"},
	}}

	interacted, err := resolveModel(e, a, gateway.Gateway{BaseURL: "https://gw.example.com"})
	if err != nil {
		t.Fatalf("resolveModel: %v", err)
	}
	if !interacted {
		t.Error("expected native discovery to count as interaction (it prints a line)")
	}
	got := e.settings.CLI["claude"]
	if !got.ModelDiscovery || !got.ModelResolved || got.Model != "" {
		t.Errorf("claude setting = %+v", got)
	}

	// Persisted to disk, not just in memory.
	reloaded, err := config.Load(e.dir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if !reloaded.CLI["claude"].ModelDiscovery {
		t.Error("expected ModelDiscovery to be saved to config.toml")
	}
}

func TestResolveModel_ForeignModels_PromptsAndPersistsChoice(t *testing.T) {
	e := newTestEnv(t)
	a := fakeAdapter{name: "codex", discovery: adapters.Discovery{
		Supported: true, Native: false, Models: []string{"model-a", "model-b"},
	}}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig })
	go func() {
		io.WriteString(w, "2\n")
		w.Close()
	}()

	interacted, err := resolveModel(e, a, gateway.Gateway{BaseURL: "https://gw.example.com"})
	if err != nil {
		t.Fatalf("resolveModel: %v", err)
	}
	if !interacted {
		t.Error("expected prompting the user to count as interaction")
	}
	got := e.settings.CLI["codex"]
	if got.Model != "model-b" || !got.ModelResolved || got.ModelDiscovery {
		t.Errorf("codex setting = %+v, want Model=model-b", got)
	}
}

func TestResolveModel_ForeignModels_SkipStillMarksResolved(t *testing.T) {
	e := newTestEnv(t)
	a := fakeAdapter{name: "codex", discovery: adapters.Discovery{
		Supported: true, Native: false, Models: []string{"model-a", "model-b"},
	}}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig })
	go func() {
		io.WriteString(w, "\n") // blank: skip pinning a model
		w.Close()
	}()

	_, err = resolveModel(e, a, gateway.Gateway{BaseURL: "https://gw.example.com"})
	if err != nil {
		t.Fatalf("resolveModel: %v", err)
	}
	got := e.settings.CLI["codex"]
	if got.Model != "" || !got.ModelResolved {
		t.Errorf("codex setting = %+v, want Model empty but ModelResolved true", got)
	}
}
