package claude

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mindsdb/setfree/internal/adapters"
	"github.com/mindsdb/setfree/internal/gateway"
)

func lookup(t *testing.T, env []string, key string) (string, bool) {
	t.Helper()
	prefix := key + "="
	for _, kv := range env {
		if len(kv) > len(prefix) && kv[:len(prefix)] == prefix {
			return kv[len(prefix):], true
		}
	}
	return "", false
}

func TestClaudeAdapter_IsRegistered(t *testing.T) {
	a, ok := adapters.Find("claude")
	if !ok {
		t.Fatal("claude adapter is not registered")
	}
	if a.DisplayName() != "Claude Code" {
		t.Errorf("DisplayName() = %q", a.DisplayName())
	}
}

func TestBuild_SetsAuthTokenNotAPIKey(t *testing.T) {
	a := adapter{}
	baseEnv := []string{"PATH=/usr/bin", envAPIKey + "=stale-key-from-shell"}

	build, err := a.Build(baseEnv, gateway.Resolved{
		Gateway: gateway.Gateway{BaseURL: "https://gw.example.com", APIKey: "sk-gateway"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if v, ok := lookup(t, build.Env, envBaseURL); !ok || v != "https://gw.example.com" {
		t.Errorf("%s = %q, %v", envBaseURL, v, ok)
	}
	if v, ok := lookup(t, build.Env, envAuthToken); !ok || v != "sk-gateway" {
		t.Errorf("%s = %q, %v", envAuthToken, v, ok)
	}
	if _, ok := lookup(t, build.Env, envAPIKey); ok {
		t.Errorf("%s should be cleared, not inherited from the parent shell", envAPIKey)
	}
	if v, ok := lookup(t, build.Env, "PATH"); !ok || v != "/usr/bin" {
		t.Errorf("PATH should be preserved from baseEnv, got %q, %v", v, ok)
	}
}

func TestBuild_ModelOverride(t *testing.T) {
	a := adapter{}

	build, err := a.Build(nil, gateway.Resolved{
		Gateway: gateway.Gateway{BaseURL: "https://gw.example.com", APIKey: "sk-gateway"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, ok := lookup(t, build.Env, envModel); ok {
		t.Error("ANTHROPIC_MODEL should be unset when no model override is given")
	}

	build, err = a.Build(nil, gateway.Resolved{
		Gateway: gateway.Gateway{BaseURL: "https://gw.example.com", APIKey: "sk-gateway"},
		Model:   "kimi-k2.5",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if v, ok := lookup(t, build.Env, envModel); !ok || v != "kimi-k2.5" {
		t.Errorf("%s = %q, %v", envModel, v, ok)
	}
}

func TestBuild_ModelDiscoveryEnablesGatewayPicker(t *testing.T) {
	a := adapter{}

	build, err := a.Build(nil, gateway.Resolved{
		Gateway: gateway.Gateway{BaseURL: "https://gw.example.com", APIKey: "sk-gateway"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, ok := lookup(t, build.Env, envModelDiscovery); ok {
		t.Errorf("%s should be unset when discovery wasn't confirmed", envModelDiscovery)
	}

	build, err = a.Build(nil, gateway.Resolved{
		Gateway:        gateway.Gateway{BaseURL: "https://gw.example.com", APIKey: "sk-gateway"},
		ModelDiscovery: true,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if v, ok := lookup(t, build.Env, envModelDiscovery); !ok || v != "1" {
		t.Errorf("%s = %q, %v; want \"1\" so /model lists the gateway's models", envModelDiscovery, v, ok)
	}
}

func TestDiscoverModels_NativeAnthropicModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-gateway" {
			t.Errorf("missing/wrong Authorization header: %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("anthropic-version") == "" {
			t.Error("missing anthropic-version header")
		}
		w.Write([]byte(`{"data":[{"id":"claude-3-5-sonnet-20241022"},{"id":"claude-3-opus-20240229"}]}`))
	}))
	defer srv.Close()

	a := adapter{}
	d := a.DiscoverModels(context.Background(), gatewayAt(srv.URL, "sk-gateway"))
	if !d.Supported || !d.Native {
		t.Fatalf("Discovery = %+v, want Supported and Native", d)
	}
	if len(d.Models) != 2 {
		t.Errorf("Models = %v", d.Models)
	}
}

func TestDiscoverModels_SendsClaudeCodeClientHeaders(t *testing.T) {
	// Some gateways only surface a claude-prefixed catalog (so Claude
	// Code's own /model picker can show non-Anthropic models) when the
	// caller looks like the real Claude Code client, not a generic HTTP
	// client. The probe must send the same header shape or it'll see the
	// unrewritten catalog and wrongly conclude the models aren't native.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "claude-cli/") {
			t.Errorf("User-Agent = %q, want claude-cli/ prefix", r.Header.Get("User-Agent"))
		}
		if r.Header.Get("x-app") != "cli" {
			t.Errorf("x-app = %q, want \"cli\"", r.Header.Get("x-app"))
		}
		if !strings.Contains(r.Header.Get("anthropic-beta"), "claude-code-") {
			t.Errorf("anthropic-beta = %q, want it to contain \"claude-code-\"", r.Header.Get("anthropic-beta"))
		}
		w.Write([]byte(`{"data":[{"id":"claude-3-5-sonnet"}]}`))
	}))
	defer srv.Close()

	a := adapter{}
	d := a.DiscoverModels(context.Background(), gatewayAt(srv.URL, "sk-gateway"))
	if !d.Supported {
		t.Fatalf("Discovery = %+v, want Supported", d)
	}
}

func TestDiscoverModels_ForeignModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[{"id":"claude-3-5-sonnet"},{"id":"gpt-4o"},{"id":"llama-3-70b"}]}`))
	}))
	defer srv.Close()

	a := adapter{}
	d := a.DiscoverModels(context.Background(), gatewayAt(srv.URL, "sk-gateway"))
	if !d.Supported {
		t.Fatal("expected the models list to be Supported")
	}
	if d.Native {
		t.Error("a mixed multi-provider models list should not be reported as Native")
	}
	if len(d.Models) != 3 {
		t.Errorf("Models = %v", d.Models)
	}
}

func TestDiscoverModels_UnsupportedEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	a := adapter{}
	d := a.DiscoverModels(context.Background(), gatewayAt(srv.URL, "sk-gateway"))
	if d.Supported {
		t.Errorf("expected an unsupported endpoint to report Supported=false, got %+v", d)
	}
}

func gatewayAt(baseURL, apiKey string) gateway.Gateway {
	return gateway.Gateway{BaseURL: baseURL, APIKey: apiKey}
}

func TestBuild_DoesNotMutateBaseEnv(t *testing.T) {
	a := adapter{}
	baseEnv := []string{"PATH=/usr/bin"}
	_, err := a.Build(baseEnv, gateway.Resolved{Gateway: gateway.Gateway{BaseURL: "https://gw.example.com"}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(baseEnv) != 1 || baseEnv[0] != "PATH=/usr/bin" {
		t.Errorf("baseEnv was mutated: %v", baseEnv)
	}
}
