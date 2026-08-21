// Package codex adapts SetFree's normalized gateway into the per-invocation
// `-c` config overrides Codex (github.com/openai/codex, the `codex` binary)
// reads to talk to a custom model provider.
//
// Codex has no generic OPENAI_BASE_URL passthrough: a custom endpoint must
// be declared as a named entry under [model_providers.<id>], either in
// ~/.codex/config.toml or via repeated `-c key=value` flags on the command
// line (https://learn.chatgpt.com/docs/config-file/config-advanced). SetFree
// always uses the latter so it never rewrites the user's own config file.
//
// The API key itself is never passed on argv (where it could be visible via
// `ps`); instead it's placed in the child's environment under a SetFree-
// owned variable name, and the provider config points at that variable via
// env_key. As of the current Codex release, wire_api must be "responses" —
// "chat" was removed and now fails at startup.
package codex

import (
	"context"
	"fmt"
	"strings"

	"github.com/mindsdb/setfree/internal/adapters"
	"github.com/mindsdb/setfree/internal/envutil"
	"github.com/mindsdb/setfree/internal/gateway"
)

const (
	providerID   = "setfree"
	providerName = "SetFree Gateway"
	envAPIKeyVar = "SETFREE_CODEX_API_KEY"
)

// nativeModelPrefixes are how OpenAI names its own models. A gateway whose
// models list is entirely prefixed like this is one where Codex's own
// model picker can be trusted to work correctly.
var nativeModelPrefixes = []string{"gpt", "o1", "o3", "o4", "chatgpt", "davinci", "curie", "babbage", "ada", "text-embedding"}

type adapter struct{}

func init() {
	adapters.Register(adapter{})
}

func (adapter) Name() string          { return "codex" }
func (adapter) DisplayName() string   { return "Codex" }
func (adapter) BinaryNames() []string { return []string{"codex"} }

func (adapter) Build(baseEnv []string, resolved gateway.Resolved) (adapters.Build, error) {
	env := envutil.Set(baseEnv, envAPIKeyVar, resolved.Gateway.APIKey)

	// Codex appends "/responses" to base_url itself, so the URL must end
	// in the API version segment ("/v1") — the opposite of Claude Code,
	// which wants the bare host. Gateways get saved as bare hosts, so add
	// the segment here rather than hitting <host>/responses and 404ing.
	baseURL := strings.TrimRight(resolved.Gateway.BaseURL, "/")
	if !strings.HasSuffix(strings.ToLower(baseURL), "/v1") {
		baseURL += "/v1"
	}

	set := func(key, value string) string {
		return fmt.Sprintf("%s=%q", key, value)
	}
	args := []string{
		"-c", set("model_provider", providerID),
		"-c", set(fmt.Sprintf("model_providers.%s.name", providerID), providerName),
		"-c", set(fmt.Sprintf("model_providers.%s.base_url", providerID), baseURL),
		"-c", set(fmt.Sprintf("model_providers.%s.wire_api", providerID), "responses"),
		"-c", set(fmt.Sprintf("model_providers.%s.env_key", providerID), envAPIKeyVar),
	}
	if resolved.Model != "" {
		args = append(args, "-c", set("model", resolved.Model))
	}

	return adapters.Build{Env: env, Args: args}, nil
}

func (adapter) DiscoverModels(ctx context.Context, gw gateway.Gateway) adapters.Discovery {
	return adapters.ProbeModels(ctx, gw.BaseURL, map[string]string{
		"Authorization": "Bearer " + gw.APIKey,
	}, nativeModelPrefixes)
}
