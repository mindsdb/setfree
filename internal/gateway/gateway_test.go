package gateway

import (
	"testing"

	"github.com/mindsdb/setfree/internal/config"
)

type fakeStore struct {
	keys map[string]string
}

func newFakeStore() *fakeStore { return &fakeStore{keys: map[string]string{}} }

func (f *fakeStore) Get(gw string) (string, bool, error) {
	k, ok := f.keys[gw]
	return k, ok, nil
}
func (f *fakeStore) Set(gw, key string) error { f.keys[gw] = key; return nil }
func (f *fakeStore) Delete(gw string) error   { delete(f.keys, gw); return nil }
func (f *fakeStore) Reset() error             { f.keys = map[string]string{}; return nil }

func fakeEnv(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestResolve_NotConfigured(t *testing.T) {
	r := &Resolver{Settings: &config.Settings{}, Secrets: newFakeStore(), Getenv: fakeEnv(nil)}
	_, err := r.Resolve("claude")
	if err == nil {
		t.Fatal("expected an error when nothing is configured")
	}
	if !ErrNotConfigured(err) {
		t.Fatalf("expected ErrNotConfigured(err) to be true, got err=%v", err)
	}
	if r.Configured() {
		t.Fatal("expected Configured() to be false")
	}
}

func TestResolve_FromSavedConfig(t *testing.T) {
	settings := &config.Settings{DefaultGateway: "default"}
	settings.SetGatewayBaseURL("default", "https://gw.example.com")
	store := newFakeStore()
	store.Set("default", "sk-saved")

	r := &Resolver{Settings: settings, Secrets: store, Getenv: fakeEnv(nil)}
	resolved, err := r.Resolve("claude")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.Gateway.BaseURL != "https://gw.example.com" {
		t.Errorf("BaseURL = %q", resolved.Gateway.BaseURL)
	}
	if resolved.Gateway.APIKey != "sk-saved" {
		t.Errorf("APIKey = %q", resolved.Gateway.APIKey)
	}
	if !r.Configured() {
		t.Error("expected Configured() to be true")
	}
}

func TestResolve_EnvOverridesWinOverSavedConfig(t *testing.T) {
	settings := &config.Settings{DefaultGateway: "default"}
	settings.SetGatewayBaseURL("default", "https://saved.example.com")
	store := newFakeStore()
	store.Set("default", "sk-saved")

	r := &Resolver{
		Settings: settings,
		Secrets:  store,
		Getenv: fakeEnv(map[string]string{
			EnvBaseURL: "https://override.example.com",
			EnvAPIKey:  "sk-override",
		}),
	}
	resolved, err := r.Resolve("claude")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.Gateway.BaseURL != "https://override.example.com" {
		t.Errorf("BaseURL = %q, want env override", resolved.Gateway.BaseURL)
	}
	if resolved.Gateway.APIKey != "sk-override" {
		t.Errorf("APIKey = %q, want env override", resolved.Gateway.APIKey)
	}
}

func TestResolve_EnvAloneIsSufficientWithNoSavedConfig(t *testing.T) {
	r := &Resolver{
		Settings: &config.Settings{},
		Secrets:  newFakeStore(),
		Getenv: fakeEnv(map[string]string{
			EnvBaseURL: "https://ci.example.com",
			EnvAPIKey:  "sk-ci",
		}),
	}
	resolved, err := r.Resolve("codex")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.Gateway.BaseURL != "https://ci.example.com" {
		t.Errorf("BaseURL = %q", resolved.Gateway.BaseURL)
	}
}

func TestResolve_NamedGatewayViaEnv(t *testing.T) {
	settings := &config.Settings{DefaultGateway: "default"}
	settings.SetGatewayBaseURL("default", "https://default.example.com")
	settings.SetGatewayBaseURL("work", "https://work.example.com")
	store := newFakeStore()
	store.Set("work", "sk-work")

	r := &Resolver{
		Settings: settings,
		Secrets:  store,
		Getenv:   fakeEnv(map[string]string{EnvGateway: "work"}),
	}
	resolved, err := r.Resolve("claude")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.Gateway.Name != "work" || resolved.Gateway.BaseURL != "https://work.example.com" {
		t.Errorf("got %+v, want the 'work' gateway", resolved.Gateway)
	}
}

func TestResolve_UnknownNamedGatewayFallsBackToDefault(t *testing.T) {
	settings := &config.Settings{DefaultGateway: "default"}
	settings.SetGatewayBaseURL("default", "https://default.example.com")

	r := &Resolver{
		Settings: settings,
		Secrets:  newFakeStore(),
		Getenv:   fakeEnv(map[string]string{EnvGateway: "does-not-exist"}),
	}
	resolved, err := r.Resolve("claude")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.Gateway.Name != "default" {
		t.Errorf("Gateway.Name = %q, want fallback to default", resolved.Gateway.Name)
	}
}

func TestResolve_Model(t *testing.T) {
	settings := &config.Settings{DefaultGateway: "default"}
	settings.SetGatewayBaseURL("default", "https://gw.example.com")
	settings.CLI = map[string]config.CLISetting{"claude": {Model: "kimi-k2.5"}}

	r := &Resolver{Settings: settings, Secrets: newFakeStore(), Getenv: fakeEnv(nil)}
	resolved, err := r.Resolve("claude")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.Model != "kimi-k2.5" {
		t.Errorf("Model = %q, want config value", resolved.Model)
	}

	r.Getenv = fakeEnv(map[string]string{EnvModel: "gpt-5"})
	resolved, err = r.Resolve("claude")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.Model != "gpt-5" {
		t.Errorf("Model = %q, want env override to win", resolved.Model)
	}
}

func TestValidateBaseURL(t *testing.T) {
	valid := []string{
		"https://api.example.com",
		"http://localhost:8080",
		"  https://padded.example.com  ",
		"https://internal.corp:9443/v1",
	}
	for _, in := range valid {
		if _, err := ValidateBaseURL(in); err != nil {
			t.Errorf("ValidateBaseURL(%q) = %v, want valid", in, err)
		}
	}

	invalid := []string{
		"",
		"   ",
		"not a url",
		"ftp://example.com",
		"example.com",
		"https://",
	}
	for _, in := range invalid {
		if _, err := ValidateBaseURL(in); err == nil {
			t.Errorf("ValidateBaseURL(%q) = nil, want an error", in)
		}
	}
}
