// Package gateway defines SetFree's normalized view of an LLM
// endpoint — the thing every CLI adapter translates into its own
// vendor-specific environment. Adapters never read config or secrets
// directly; they only ever see a Gateway.
package gateway

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/mindsdb/setfree/internal/config"
	"github.com/mindsdb/setfree/internal/secrets"
)

// Gateway is a normalized LLM endpoint: a base URL and the credential used
// to authenticate against it.
type Gateway struct {
	Name    string
	BaseURL string
	APIKey  string
}

// Env var overrides, documented in README/help. These always win over saved
// config so SetFree behaves predictably in scripts and CI.
const (
	EnvBaseURL = "SETFREE_BASE_URL"
	EnvAPIKey  = "SETFREE_API_KEY"
	EnvGateway = "SETFREE_GATEWAY"
	EnvModel   = "SETFREE_MODEL"
)

// Resolver combines saved settings, stored secrets, and environment
// variable overrides into the Gateway + model a launch should use.
type Resolver struct {
	Settings *config.Settings
	Secrets  secrets.Store
	// Getenv defaults to os.Getenv; tests override it.
	Getenv func(string) string
}

// NewResolver returns a Resolver ready for real use.
func NewResolver(s *config.Settings, store secrets.Store) *Resolver {
	return &Resolver{Settings: s, Secrets: store, Getenv: os.Getenv}
}

func (r *Resolver) getenv(key string) string {
	if r.Getenv != nil {
		return r.Getenv(key)
	}
	return os.Getenv(key)
}

// Resolved is what an adapter needs to build a child environment.
type Resolved struct {
	Gateway Gateway
	Model   string // empty means "let the CLI use its own default"
	// ModelDiscovery means SetFree has confirmed the gateway serves this
	// CLI's models via a listing endpoint, so adapters should turn on the
	// CLI's own gateway-backed model picker where one exists (e.g. Claude
	// Code's CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY).
	ModelDiscovery bool
}

// Resolve computes the gateway + model to use for cliName, applying the
// documented precedence: SetFree env vars, then saved config. It never
// triggers interactive setup — callers decide whether to fall back to that
// when Configured() is false.
func (r *Resolver) Resolve(cliName string) (Resolved, error) {
	name, gw, configured := r.selectGateway()

	if envURL := strings.TrimSpace(r.getenv(EnvBaseURL)); envURL != "" {
		gw.BaseURL = envURL
		configured = true
	}
	if envKey := r.getenv(EnvAPIKey); envKey != "" {
		gw.APIKey = envKey
		configured = true
	}

	model := ""
	discovery := false
	if r.Settings != nil {
		if cliCfg, ok := r.Settings.CLI[cliName]; ok {
			model = cliCfg.Model
			discovery = cliCfg.ModelDiscovery
		}
	}
	if envModel := strings.TrimSpace(r.getenv(EnvModel)); envModel != "" {
		model = envModel
	}

	if !configured {
		return Resolved{}, errNotConfigured
	}
	if gw.BaseURL == "" {
		return Resolved{}, fmt.Errorf("gateway %q has no base URL configured", name)
	}

	gw.Name = name
	return Resolved{Gateway: gw, Model: model, ModelDiscovery: discovery}, nil
}

// selectGateway picks the named gateway (via SETFREE_GATEWAY, if set and
// present in config) or the configured default, without applying env
// overrides yet. configured reports whether a usable entry was found.
func (r *Resolver) selectGateway() (name string, gw Gateway, configured bool) {
	if r.Settings == nil {
		return "default", Gateway{}, false
	}

	if want := strings.TrimSpace(r.getenv(EnvGateway)); want != "" {
		if setting, ok := r.Settings.Gateways[want]; ok {
			return want, r.withKey(want, setting), true
		}
		// Named gateway not found in config; fall through to default so a
		// stray env var doesn't hard-fail a launch that would otherwise work.
	}

	setting, name, ok := r.Settings.Default()
	if !ok {
		return "default", Gateway{}, false
	}
	return name, r.withKey(name, setting), true
}

func (r *Resolver) withKey(name string, setting config.GatewaySetting) Gateway {
	gw := Gateway{Name: name, BaseURL: setting.BaseURL}
	if r.Secrets != nil {
		if key, ok, _ := r.Secrets.Get(name); ok {
			gw.APIKey = key
		}
	}
	return gw
}

// Configured reports whether any gateway is currently usable: either saved
// in config or supplied entirely via SETFREE_BASE_URL.
func (r *Resolver) Configured() bool {
	if _, _, ok := r.selectGateway(); ok {
		return true
	}
	return strings.TrimSpace(r.getenv(EnvBaseURL)) != ""
}

var errNotConfigured = fmt.Errorf("no gateway configured")

// ErrNotConfigured reports whether err indicates that no gateway is set up
// yet, as opposed to some other resolution failure.
func ErrNotConfigured(err error) bool {
	return err == errNotConfigured
}

// ValidateBaseURL rejects obviously invalid base URLs while staying
// permissive about scheme/host combinations SetFree can't fully vet ahead
// of time (private gateways, internal DNS names, non-standard ports).
func ValidateBaseURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("base URL is required")
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("%q is not a valid URL", trimmed)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("base URL must start with http:// or https://")
	}
	if u.Host == "" {
		return "", fmt.Errorf("base URL must include a host")
	}
	return trimmed, nil
}
