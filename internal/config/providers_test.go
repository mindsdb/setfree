package config

import (
	"strings"
	"testing"
)

func TestProviders_ShapeAndOrder(t *testing.T) {
	got := Providers()
	want := []string{"mindshub", "openrouter", "litellm", "custom"}
	if len(got) != len(want) {
		t.Fatalf("got %d providers, want %d", len(got), len(want))
	}
	for i, key := range want {
		if got[i].Key != key {
			t.Errorf("provider %d = %q, want %q", i, got[i].Key, key)
		}
	}
}

// Hosted providers can skip straight to auth; self-hosted ones have no
// canonical address, so setup has to ask.
func TestProviders_NeedsBaseURL(t *testing.T) {
	cases := map[string]bool{
		"mindshub":   false,
		"openrouter": false,
		"litellm":    true,
		"custom":     true,
	}
	for key, wantPrompt := range cases {
		p, ok := FindProvider(key)
		if !ok {
			t.Fatalf("provider %q is missing", key)
		}
		if p.NeedsBaseURL() != wantPrompt {
			t.Errorf("%s: NeedsBaseURL() = %v, want %v", key, p.NeedsBaseURL(), wantPrompt)
		}
	}
}

func TestProviders_OnlyMindsHubUsesSSO(t *testing.T) {
	for _, p := range Providers() {
		if want := p.Key == "mindshub"; p.SSO != want {
			t.Errorf("%s: SSO = %v, want %v", p.Key, p.SSO, want)
		}
	}
}

// All three MindsHub hosts derive from one domain, so pointing at staging
// can't leave auth and inference disagreeing about which deployment
// they're talking to.
func TestMindsHubDomain_OverrideMovesEveryHostTogether(t *testing.T) {
	t.Setenv(EnvMindsHubDomain, "staging.mindshub.ai")

	p, _ := FindProvider("mindshub")
	if p.BaseURL != "https://api.staging.mindshub.ai" {
		t.Errorf("gateway = %q", p.BaseURL)
	}
	if got := MindsHubOIDC().Issuer; got != "https://auth.staging.mindshub.ai/auth" {
		t.Errorf("issuer = %q", got)
	}
	if got := MindsHubAuthAPI(); got != "https://auth.staging.mindshub.ai/v1" {
		t.Errorf("auth API = %q", got)
	}
}

func TestMindsHubDefaultsToProduction(t *testing.T) {
	t.Setenv(EnvMindsHubDomain, "")

	p, _ := FindProvider("mindshub")
	for _, s := range []string{p.BaseURL, MindsHubOIDC().Issuer, MindsHubAuthAPI(), MindsHubConsoleURL()} {
		if strings.Contains(s, "staging") {
			t.Errorf("%q should point at production by default", s)
		}
	}
	if got := MindsHubOIDC().ClientID; got != "anton-desktop" {
		t.Errorf("client id = %q", got)
	}
}

func TestMindsHubConsoleURL_FollowsDomainOverride(t *testing.T) {
	t.Setenv(EnvMindsHubDomain, "staging.mindshub.ai")
	if got := MindsHubConsoleURL(); got != "https://console.staging.mindshub.ai" {
		t.Errorf("console URL = %q", got)
	}

	t.Setenv(EnvMindsHubDomain, "")
	if got := MindsHubConsoleURL(); got != "https://console.mindshub.ai" {
		t.Errorf("console URL = %q", got)
	}
}

func TestMindsHubBillingURL_FollowsDomainOverride(t *testing.T) {
	t.Setenv(EnvMindsHubDomain, "staging.mindshub.ai")
	if got := MindsHubBillingURL(); got != "https://console.staging.mindshub.ai/settings/organization/billing" {
		t.Errorf("billing URL = %q", got)
	}

	t.Setenv(EnvMindsHubDomain, "")
	if got := MindsHubBillingURL(); got != "https://console.mindshub.ai/settings/organization/billing" {
		t.Errorf("billing URL = %q", got)
	}
}

// The OIDC config is what carries the redirect into the sign-in flow, so
// it has to actually point at the billing page, not be left empty.
func TestMindsHubOIDC_RedirectsToBilling(t *testing.T) {
	t.Setenv(EnvMindsHubDomain, "")
	if got := MindsHubOIDC().SuccessRedirect; got != "https://console.mindshub.ai/settings/organization/billing" {
		t.Errorf("SuccessRedirect = %q", got)
	}
}

// The picker should steer people toward exactly one preset.
func TestProviders_OnlyOneRecommended(t *testing.T) {
	count := 0
	for _, p := range Providers() {
		if p.Recommended {
			count++
			if p.Key != "mindshub" {
				t.Errorf("unexpected recommended provider %q", p.Key)
			}
		}
	}
	if count != 1 {
		t.Errorf("%d providers marked recommended, want exactly 1", count)
	}
}

func TestProviders_OnlyMindsHubHasADefaultModel(t *testing.T) {
	for _, p := range Providers() {
		want := ""
		if p.Key == "mindshub" {
			want = "mindshub_air"
		}
		if p.DefaultModel != want {
			t.Errorf("%s: DefaultModel = %q, want %q", p.Key, p.DefaultModel, want)
		}
	}
}

// MindsHub rewrites its catalog into each CLI's native shape, so the
// CLIs' built-in model pickers work against it without probing first.
// No other preset promises that.
func TestProviders_OnlyMindsHubEnablesModelDiscovery(t *testing.T) {
	for _, p := range Providers() {
		if want := p.Key == "mindshub"; p.ModelDiscovery != want {
			t.Errorf("%s: ModelDiscovery = %v, want %v", p.Key, p.ModelDiscovery, want)
		}
	}
}

func TestFindProvider_Unknown(t *testing.T) {
	if _, ok := FindProvider("nope"); ok {
		t.Error("expected an unknown key to report not-found")
	}
}
