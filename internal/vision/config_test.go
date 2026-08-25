package vision

import (
	"testing"
	"time"

	"github.com/mindsdb/setfree/internal/config"
	"github.com/mindsdb/setfree/internal/secrets"
)

type memStore struct {
	m map[string]string
}

func (s *memStore) Get(g string) (string, bool, error) { k, ok := s.m[g]; return k, ok, nil }
func (s *memStore) Set(g, k string) error              { s.m[g] = k; return nil }
func (s *memStore) Delete(g string) error              { delete(s.m, g); return nil }
func (s *memStore) Reset() error                       { s.m = map[string]string{}; return nil }

func newMemStore() *memStore { return &memStore{m: map[string]string{}} }

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestResolve_DisabledByDefault(t *testing.T) {
	cfg := Resolve(&config.Settings{}, newMemStore(), envOf(nil), config.GatewaySetting{BaseURL: "https://gw"}, "mainkey")
	if cfg.Enabled {
		t.Fatal("bridge should be inert with no vision model")
	}
	if cfg.Model != "" {
		t.Errorf("Model = %q, want empty", cfg.Model)
	}
}

func TestResolve_EnabledBySavedModel(t *testing.T) {
	s := &config.Settings{Vision: config.VisionSetting{Model: "qwen3.5"}}
	cfg := Resolve(s, newMemStore(), envOf(nil), config.GatewaySetting{BaseURL: "https://gw"}, "mainkey")
	if !cfg.Enabled {
		t.Fatal("saved model should enable the bridge")
	}
	if cfg.Model != "qwen3.5" {
		t.Errorf("Model = %q", cfg.Model)
	}
	// Defaults: vision endpoint/key fall back to the main gateway.
	if cfg.BaseURL != "" || cfg.APIKey != "" {
		t.Errorf("unset vision endpoint/key should default to empty (→ main), got BaseURL=%q APIKey=%q", cfg.BaseURL, cfg.APIKey)
	}
	if cfg.GatewayBaseURL != "https://gw" || cfg.GatewayAPIKey != "mainkey" {
		t.Errorf("main gateway defaults not carried: %+v", cfg)
	}
}

func TestResolve_EnvOverridesSaved(t *testing.T) {
	s := &config.Settings{Vision: config.VisionSetting{Model: "saved-model", BaseURL: "https://saved"}}
	env := envOf(map[string]string{
		EnvModel:   "env-model",
		EnvBaseURL: "https://env-vision",
		EnvAPIKey:  "env-key",
	})
	cfg := Resolve(s, newMemStore(), env, config.GatewaySetting{BaseURL: "https://gw"}, "mainkey")
	if cfg.Model != "env-model" || cfg.BaseURL != "https://env-vision" || cfg.APIKey != "env-key" {
		t.Errorf("env should win: %+v", cfg)
	}
}

func TestResolve_SecretKeyFallback(t *testing.T) {
	store := newMemStore()
	store.m[SecretName] = "secret-vision-key"
	s := &config.Settings{Vision: config.VisionSetting{Model: "qwen3.5"}}
	cfg := Resolve(s, store, envOf(nil), config.GatewaySetting{BaseURL: "https://gw"}, "mainkey")
	if cfg.APIKey != "secret-vision-key" {
		t.Errorf("vision key from secrets store: got %q", cfg.APIKey)
	}
}

func TestResolve_OffWins(t *testing.T) {
	s := &config.Settings{Vision: config.VisionSetting{Model: "qwen3.5"}}
	env := envOf(map[string]string{EnvOff: "1", EnvModel: "env-model"})
	cfg := Resolve(s, newMemStore(), env, config.GatewaySetting{BaseURL: "https://gw"}, "mainkey")
	if cfg.Enabled {
		t.Fatal("SETFREE_VISION_OFF must hard-disable even with a model set")
	}
	if cfg.Model != "" {
		t.Errorf("Model = %q, want empty when off", cfg.Model)
	}
}

func TestResolve_ConcurrencyAndTimeout(t *testing.T) {
	env := envOf(map[string]string{
		EnvModel:       "qwen3.5",
		EnvConcurrency: "3",
		EnvIdleTimeout: "90",
	})
	cfg := Resolve(&config.Settings{}, newMemStore(), env, config.GatewaySetting{BaseURL: "https://gw"}, "k")
	if cfg.Concurrency != 3 {
		t.Errorf("Concurrency = %d, want 3", cfg.Concurrency)
	}
	if cfg.IdleTimeout != 90*time.Second {
		t.Errorf("IdleTimeout = %v, want 90s", cfg.IdleTimeout)
	}
}

func TestResolve_Defaults(t *testing.T) {
	cfg := Resolve(&config.Settings{}, newMemStore(), envOf(nil), config.GatewaySetting{BaseURL: "https://gw"}, "k")
	if cfg.Concurrency != defaultConcurrency {
		t.Errorf("default Concurrency = %d, want %d", cfg.Concurrency, defaultConcurrency)
	}
	if cfg.IdleTimeout != defaultIdleTimeout {
		t.Errorf("default IdleTimeout = %v, want %v", cfg.IdleTimeout, defaultIdleTimeout)
	}
}

// silence unused linter for the store interface methods not exercised here
var _ secrets.Store = (*memStore)(nil)
