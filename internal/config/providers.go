package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/mindsdb/setfree/internal/auth"
)

// EnvMindsHubDomain overrides which MindsHub deployment the preset points
// at. It names the bare domain — "staging.mindshub.ai" for staging — from
// which the auth, API-key, and inference hosts are all derived, mirroring
// how the web console derives them from its own origin.
const EnvMindsHubDomain = "SETFREE_MINDSHUB_DOMAIN"

const defaultMindsHubDomain = "mindshub.ai"

// Provider is a preset gateway offered during setup, so the common cases
// don't require knowing a base URL by heart.
type Provider struct {
	// Key is the stable identifier saved in config.
	Key string
	// DisplayName is what the picker shows.
	DisplayName string
	// Detail is a short dimmed hint shown beside the name.
	Detail string
	// BaseURL is the gateway endpoint. Empty means SetFree has to ask,
	// because the provider is self-hosted and has no canonical address.
	BaseURL string
	// SSO reports whether this provider signs in through a browser rather
	// than a pasted API key.
	SSO bool
	// Recommended marks the preset the picker should steer people toward.
	// At most one provider should set this.
	Recommended bool
	// DefaultModel, if set, is pinned as the starting model for every
	// known CLI as soon as this provider is connected — rather than left
	// unset for that CLI's own hardcoded default, which usually names a
	// specific vendor's model this gateway may not even serve.
	DefaultModel string
	// ModelDiscovery marks a provider known to serve each CLI's model
	// catalog in that CLI's own native shape (MindsHub rewrites its
	// catalog for Claude Code callers), so the CLI's built-in model
	// picker can be enabled against it without probing first.
	ModelDiscovery bool
}

// NeedsBaseURL reports whether setup must prompt for an address.
func (p Provider) NeedsBaseURL() bool { return p.BaseURL == "" }

// mindsHubDomain returns the deployment domain the MindsHub preset targets.
func mindsHubDomain() string {
	if d := strings.TrimSpace(os.Getenv(EnvMindsHubDomain)); d != "" {
		return d
	}
	return defaultMindsHubDomain
}

// MindsHubOIDC returns the sign-in configuration for the MindsHub preset.
//
// It reuses the `anton-desktop` public client, which is already registered
// in the realm for exactly this shape of flow: a native client doing
// authorization-code + PKCE against a loopback callback. A dedicated
// `setfree-cli` client would be preferable — it would let CLI sessions be
// revoked or rate-limited on their own — and swapping to one is a change to
// this constant plus a realm entry, nothing more.
func MindsHubOIDC() auth.Config {
	return auth.Config{
		Issuer:          fmt.Sprintf("https://auth.%s/auth", mindsHubDomain()),
		Realm:           "mindsdb",
		ClientID:        "anton-desktop",
		Scopes:          []string{"openid", "profile", "email"},
		SuccessRedirect: MindsHubBillingURL(),
	}
}

// MindsHubAuthAPI returns the auth service's versioned API root, where a
// signed-in session is traded for a long-lived API key.
func MindsHubAuthAPI() string {
	return fmt.Sprintf("https://auth.%s/v1", mindsHubDomain())
}

// MindsHubConsoleURL returns the web console's address.
func MindsHubConsoleURL() string {
	return fmt.Sprintf("https://console.%s", mindsHubDomain())
}

// MindsHubBillingURL returns the console's organization billing page, so
// the browser can be sent there once sign-in completes — putting credit
// and plan management one click away — rather than left on a bare
// confirmation page.
func MindsHubBillingURL() string {
	return MindsHubConsoleURL() + "/settings/organization/billing"
}

// Providers are the gateway presets offered at setup, in display order.
func Providers() []Provider {
	return []Provider{
		{
			Key:            "mindshub",
			DisplayName:    "MindsHub",
			Detail:         "all models in one place, open and closed — sign in with your browser",
			BaseURL:        fmt.Sprintf("https://api.%s", mindsHubDomain()),
			SSO:            true,
			Recommended:    true,
			DefaultModel:   "mindshub_air",
			ModelDiscovery: true,
		},
		{
			Key:         "openrouter",
			DisplayName: "OpenRouter",
			Detail:      "hosted, bring an API key",
			BaseURL:     "https://openrouter.ai/api/v1",
		},
		{
			Key:         "litellm",
			DisplayName: "LiteLLM",
			Detail:      "your own proxy",
		},
		{
			Key:         "custom",
			DisplayName: "Custom",
			Detail:      "any OpenAI- or Anthropic-compatible endpoint",
		},
	}
}

// FindProvider returns the preset with the given key.
func FindProvider(key string) (Provider, bool) {
	for _, p := range Providers() {
		if p.Key == key {
			return p, true
		}
	}
	return Provider{}, false
}
