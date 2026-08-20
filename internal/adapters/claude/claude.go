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
	"github.com/mindsdb/setfree/internal/adapters"
	"github.com/mindsdb/setfree/internal/envutil"
	"github.com/mindsdb/setfree/internal/gateway"
)

const (
	envBaseURL   = "ANTHROPIC_BASE_URL"
	envAuthToken = "ANTHROPIC_AUTH_TOKEN"
	envAPIKey    = "ANTHROPIC_API_KEY"
	envModel     = "ANTHROPIC_MODEL"
)

type adapter struct{}

func init() {
	adapters.Register(adapter{})
}

func (adapter) Name() string          { return "claude" }
func (adapter) DisplayName() string   { return "Claude Code" }
func (adapter) BinaryNames() []string { return []string{"claude"} }

func (adapter) Build(baseEnv []string, resolved gateway.Resolved) (adapters.Build, error) {
	env := envutil.Unset(baseEnv, envAPIKey)
	env = envutil.Set(env, envBaseURL, resolved.Gateway.BaseURL)
	env = envutil.Set(env, envAuthToken, resolved.Gateway.APIKey)
	if resolved.Model != "" {
		env = envutil.Set(env, envModel, resolved.Model)
	}
	return adapters.Build{Env: env}, nil
}
