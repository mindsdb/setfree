package codex

import (
	"strings"
	"testing"

	"github.com/mindsdb/setfree/internal/adapters"
	"github.com/mindsdb/setfree/internal/gateway"
)

func TestCodexAdapter_IsRegistered(t *testing.T) {
	a, ok := adapters.Find("codex")
	if !ok {
		t.Fatal("codex adapter is not registered")
	}
	if a.DisplayName() != "Codex" {
		t.Errorf("DisplayName() = %q", a.DisplayName())
	}
}

// argValue returns the value passed to a "-c key=..." flag pair for key.
func argValue(t *testing.T, args []string, key string) (string, bool) {
	t.Helper()
	for i := 0; i < len(args)-1; i++ {
		if args[i] != "-c" {
			continue
		}
		prefix := key + "="
		if strings.HasPrefix(args[i+1], prefix) {
			return strings.TrimPrefix(args[i+1], prefix), true
		}
	}
	return "", false
}

func TestBuild_UsesResponsesWireAPIAndEnvKeyIndirection(t *testing.T) {
	a := adapter{}
	build, err := a.Build(nil, gateway.Resolved{
		Gateway: gateway.Gateway{BaseURL: "https://gw.example.com", APIKey: "sk-gateway"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	for _, key := range []string{
		"model_provider",
		"model_providers.setfree.name",
		"model_providers.setfree.base_url",
		"model_providers.setfree.wire_api",
		"model_providers.setfree.env_key",
	} {
		if _, ok := argValue(t, build.Args, key); !ok {
			t.Errorf("missing -c %s=... in %v", key, build.Args)
		}
	}

	if v, _ := argValue(t, build.Args, "model_providers.setfree.wire_api"); v != `"responses"` {
		t.Errorf("wire_api = %s, want a quoted TOML string \"responses\"", v)
	}
	if v, _ := argValue(t, build.Args, "model_providers.setfree.base_url"); v != `"https://gw.example.com"` {
		t.Errorf("base_url = %s", v)
	}

	// The raw API key must never appear on argv.
	for _, arg := range build.Args {
		if strings.Contains(arg, "sk-gateway") {
			t.Errorf("API key leaked into argv: %q", arg)
		}
	}

	// It must instead be reachable via the env_key indirection.
	envKeyVal, _ := argValue(t, build.Args, "model_providers.setfree.env_key")
	envKeyVal = strings.Trim(envKeyVal, `"`)
	found := false
	for _, kv := range build.Env {
		if kv == envKeyVal+"=sk-gateway" {
			found = true
		}
	}
	if !found {
		t.Errorf("env_key %q not set to the API key in build.Env: %v", envKeyVal, build.Env)
	}
}

func TestBuild_ModelOverrideIsOptional(t *testing.T) {
	a := adapter{}

	build, _ := a.Build(nil, gateway.Resolved{Gateway: gateway.Gateway{BaseURL: "https://gw.example.com"}})
	if _, ok := argValue(t, build.Args, "model"); ok {
		t.Error("-c model=... should be omitted with no model override")
	}

	build, _ = a.Build(nil, gateway.Resolved{
		Gateway: gateway.Gateway{BaseURL: "https://gw.example.com"},
		Model:   "gpt-5",
	})
	if v, ok := argValue(t, build.Args, "model"); !ok || v != `"gpt-5"` {
		t.Errorf("model = %s, %v", v, ok)
	}
}
