package vscode

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/mindsdb/setfree/internal/adapters"
	"github.com/mindsdb/setfree/internal/adapters/claude"
	"github.com/mindsdb/setfree/internal/detect"
	"github.com/mindsdb/setfree/internal/gateway"
)

func TestVSCodeAdapter_IsRegisteredUnderBothNames(t *testing.T) {
	a, ok := adapters.Find("code")
	if !ok {
		t.Fatal("code adapter is not registered")
	}
	if a.DisplayName() != "VS Code" {
		t.Errorf("DisplayName() = %q", a.DisplayName())
	}

	alias, ok := adapters.Find("vscode")
	if !ok {
		t.Fatal("the vscode alias does not resolve")
	}
	if alias.Name() != "code" {
		t.Errorf("alias resolved to %q, want the canonical code adapter", alias.Name())
	}
}

// The adapter and detect must search the exact same places, or the
// landing screen could say "installed" while launch says "not found".
func TestBinaryNames_MatchDetect(t *testing.T) {
	got := adapter{}.BinaryNames()
	want := detect.VSCodeBinaries()
	if len(got) != len(want) {
		t.Fatalf("BinaryNames() has %d entries, detect.VSCodeBinaries() has %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("BinaryNames()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// Aliases are lookup-only; if they leaked into All(), per-CLI settings
// like the provider default model would be written under two names.
func TestVSCodeAlias_NotDuplicatedInAll(t *testing.T) {
	count := 0
	for _, a := range adapters.All() {
		if a.Name() == "code" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("code adapter appears %d times in All(), want 1", count)
	}
}

// The whole point of the vscode adapter is that VS Code's Claude Code
// extension spawns the same claude binary, so its environment must be
// exactly what the claude adapter would build — drift between the two
// means "works in the terminal, broken in the editor".
func TestBuild_EnvIsExactlyClaudesEnv(t *testing.T) {
	a := adapter{}
	baseEnv := []string{"PATH=/usr/bin", "ANTHROPIC_API_KEY=stale-shell-key"}
	resolved := gateway.Resolved{
		Gateway:        gateway.Gateway{BaseURL: "https://gw.example.com", APIKey: "sk-gateway"},
		Model:          "mindshub_air",
		ModelDiscovery: true,
	}

	build, err := a.Build(baseEnv, resolved)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	want := claude.Env(baseEnv, resolved)
	if len(build.Env) != len(want) {
		t.Fatalf("env length %d, want %d", len(build.Env), len(want))
	}
	for i := range want {
		if build.Env[i] != want[i] {
			t.Errorf("env[%d] = %q, want %q", i, build.Env[i], want[i])
		}
	}
}

// A silent launch would hide the one real gotcha: an already-running
// VS Code keeps its old environment, gateway or not.
func TestBuild_WarnsAboutAlreadyRunningVSCode(t *testing.T) {
	build, err := adapter{}.Build(nil, gateway.Resolved{
		Gateway: gateway.Gateway{BaseURL: "https://gw.example.com"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !strings.Contains(build.Note, "already running") {
		t.Errorf("Note = %q, want it to warn about an already-running instance", build.Note)
	}
}

// Prepare, end to end: catalog fetched from the gateway, provider entry
// written where VS Code reads it. HOME is overridden so chatModelsPath
// resolves into the test dir.
func TestPrepare_WritesChatModelsFromGatewayCatalog(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[{"id":"mindshub_air","label":"MindsHub Air"},{"id":"kimi","label":"Kimi K3"}]}`))
	}))
	defer srv.Close()

	home := t.TempDir()
	t.Setenv("HOME", home)

	note, err := adapter{}.Prepare(context.Background(), gateway.Resolved{
		Gateway: gateway.Gateway{BaseURL: srv.URL, APIKey: "mdb_k"},
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if !strings.Contains(note, "2 gateway models") {
		t.Errorf("note = %q", note)
	}

	path, err := chatModelsPath()
	if err != nil {
		t.Fatalf("chatModelsPath: %v", err)
	}
	if !strings.HasPrefix(path, home) {
		t.Fatalf("path %q escaped the test HOME %q", path, home)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the config file wasn't written: %v", err)
	}
	for _, want := range []string{"mindshub_air", "Kimi K3", "customendpoint", srv.URL + "/v1/chat/completions"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("config missing %q:\n%s", want, data)
		}
	}
}

func TestPrepare_UnreachableGatewayIsAnErrorNotAPanic(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, err := adapter{}.Prepare(context.Background(), gateway.Resolved{
		Gateway: gateway.Gateway{BaseURL: "http://127.0.0.1:1", APIKey: "k"},
	})
	if err == nil {
		t.Fatal("expected an unreachable gateway to be reported")
	}
}

// Discovery must be Claude Code's probe — the gateway's catalog rewriting
// keys off Claude Code's client headers, and the thing that will actually
// talk to the gateway is the claude binary the extension spawns.
func TestDiscoverModels_UsesClaudeCodeProbe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "claude-cli/") {
			t.Errorf("User-Agent = %q, want Claude Code's", r.Header.Get("User-Agent"))
		}
		w.Write([]byte(`{"data":[{"id":"claude-3-5-sonnet"}]}`))
	}))
	defer srv.Close()

	d := adapter{}.DiscoverModels(context.Background(), gateway.Gateway{BaseURL: srv.URL, APIKey: "sk"})
	if !d.Supported || !d.Native {
		t.Errorf("Discovery = %+v, want Supported and Native", d)
	}
}
