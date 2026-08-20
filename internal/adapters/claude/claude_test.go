package claude

import (
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
