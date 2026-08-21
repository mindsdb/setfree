// Package claude adapts SetFree's normalized gateway into the environment
// variables Claude Code (github.com/anthropics/claude-code, the `claude`
// binary) reads to talk to a custom Anthropic-compatible endpoint.
//
// Current, documented variables (https://code.claude.com/docs/en/settings):
//
//	ANTHROPIC_BASE_URL    overrides the default api.anthropic.com endpoint.
//	ANTHROPIC_AUTH_TOKEN  sent as `Authorization: Bearer <token>`; the right
//	                      choice for third-party gateways and proxies, and
//	                      takes priority over ANTHROPIC_API_KEY if both are
//	                      set.
//	ANTHROPIC_API_KEY     sent as `x-api-key`; Anthropic's own key scheme.
//	ANTHROPIC_MODEL       overrides the model for this run.
//
// SetFree always authenticates via ANTHROPIC_AUTH_TOKEN, since a gateway
// speaking a custom base URL is far more likely to expect a bearer token
// than Anthropic's native key header. It also clears ANTHROPIC_API_KEY from
// the child environment so a key left over from the user's own shell can't
// silently take precedence or point at a different backend than base_url.
package claude

import (
	"context"

	"github.com/mindsdb/setfree/internal/adapters"
	"github.com/mindsdb/setfree/internal/envutil"
	"github.com/mindsdb/setfree/internal/gateway"
)

const (
	envBaseURL   = "ANTHROPIC_BASE_URL"
	envAuthToken = "ANTHROPIC_AUTH_TOKEN"
	envAPIKey    = "ANTHROPIC_API_KEY"
	envModel     = "ANTHROPIC_MODEL"
	// envModelDiscovery makes Claude Code's `/model` picker list what the
	// gateway's models endpoint actually serves, instead of only its own
	// hardcoded Anthropic lineup. Without it, `/model` shows Opus/Sonnet/
	// Haiku regardless of what the gateway offers.
	envModelDiscovery = "CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY"
)

// nativeModelPrefixes are how Anthropic names its own models. A gateway
// whose models list is entirely prefixed like this is one where Claude
// Code's own `/model` picker can be trusted to work correctly.
var nativeModelPrefixes = []string{"claude", "anthropic"}

// claudeCodeUserAgent and claudeCodeBetaMarker mimic the header shape the
// real Claude Code binary sends, so a models-list probe is treated by
// gateways the same way a genuine Claude Code launch would be. The exact
// version token doesn't matter to that check, only the prefix/substring.
const (
	claudeCodeUserAgent  = "claude-cli/1.0.0 (external, cli)"
	claudeCodeBetaMarker = "claude-code-20250219"
)

type adapter struct{}

func init() {
	adapters.Register(adapter{})
}

func (adapter) Name() string          { return "claude" }
func (adapter) DisplayName() string   { return "Claude Code" }
func (adapter) BinaryNames() []string { return []string{"claude"} }

func (adapter) Build(baseEnv []string, resolved gateway.Resolved) (adapters.Build, error) {
	return adapters.Build{Env: Env(baseEnv, resolved)}, nil
}

// Env builds the environment Claude Code needs to route through resolved's
// gateway. Exported because Claude Code isn't only launched directly: the
// vscode adapter launches editors whose Claude Code extension spawns the
// same `claude` binary, which reads this exact environment — so both
// adapters must agree on it, byte for byte.
func Env(baseEnv []string, resolved gateway.Resolved) []string {
	env := envutil.Unset(baseEnv, envAPIKey)
	env = envutil.Set(env, envBaseURL, resolved.Gateway.BaseURL)
	env = envutil.Set(env, envAuthToken, resolved.Gateway.APIKey)
	if resolved.Model != "" {
		env = envutil.Set(env, envModel, resolved.Model)
	}
	if resolved.ModelDiscovery {
		env = envutil.Set(env, envModelDiscovery, "1")
	}
	return env
}

func (adapter) DiscoverModels(ctx context.Context, gw gateway.Gateway) adapters.Discovery {
	return Discover(ctx, gw)
}

// Discover probes gw's models endpoint the way the Claude Code binary
// itself would. Exported for the same reason as Env: any adapter that
// ultimately runs Claude Code (the vscode adapter, via the extension)
// needs the identical probe, or the two would classify the same gateway
// differently.
func Discover(ctx context.Context, gw gateway.Gateway) adapters.Discovery {
	return adapters.ProbeModels(ctx, gw.BaseURL, map[string]string{
		"Authorization":     "Bearer " + gw.APIKey,
		"x-api-key":         gw.APIKey, // hedge: real Anthropic-style gateways expect this instead
		"anthropic-version": "2023-06-01",
		// Some gateways (e.g. mindsdb's inference gateway) only rewrite a
		// mixed-provider catalog into claude-prefixed ids — so Claude
		// Code's own /model picker can show them — when the caller looks
		// like the real Claude Code client. Without these, the probe
		// would see the unrewritten catalog, wrongly conclude the models
		// aren't native, and prompt the user even though the real binary
		// would have worked silently via its own picker.
		"User-Agent":     claudeCodeUserAgent,
		"x-app":          "cli",
		"anthropic-beta": claudeCodeBetaMarker,
	}, nativeModelPrefixes)
}
