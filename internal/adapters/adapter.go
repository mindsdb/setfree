// Package adapters defines how SetFree translates a normalized Gateway
// into the environment a specific coding CLI expects, and holds the
// registry of CLIs SetFree can actually launch. Everything vendor-specific
// (env var names, config flags, wire formats) lives inside one adapter;
// nothing outside this package should need to know about them.
package adapters

import (
	"context"

	"github.com/mindsdb/setfree/internal/gateway"
)

// Installation describes a coding CLI found on the user's PATH.
type Installation struct {
	Path string
}

// Build is the environment and extra argv an adapter wants for one launch.
// Args are prepended before the user's own passthrough arguments.
type Build struct {
	Env  []string
	Args []string
	// Note, if non-empty, is a one-line caveat printed before launching —
	// for the rare CLI where a silent launch would hide a real gotcha
	// (e.g. VS Code ignoring the environment when an instance is already
	// running). Most adapters leave it empty; silence is the norm.
	Note string
}

// Adapter knows how one coding CLI receives configuration.
type Adapter interface {
	// Name is the key typed on the command line, e.g. "claude".
	Name() string
	// DisplayName is the human-readable product name, e.g. "Claude Code".
	DisplayName() string
	// BinaryNames are the executable names to search PATH for, in order.
	BinaryNames() []string
	// Build translates a resolved gateway + model into the environment
	// variables and extra arguments needed to route baseEnv (typically
	// os.Environ()) through it. It must not mutate baseEnv.
	Build(baseEnv []string, resolved gateway.Resolved) (Build, error)
	// DiscoverModels probes gw for a model-listing endpoint using this
	// CLI's usual protocol and auth headers. It's always best-effort: an
	// unreachable or non-conforming endpoint just reports Discovery{} —
	// that's an expected outcome, not a failure worth an error return.
	DiscoverModels(ctx context.Context, gw gateway.Gateway) Discovery
}

// Preparer is implemented by adapters whose target reads configuration
// from disk rather than (or in addition to) the environment — VS Code's
// chat model picker, say. Prepare runs on every launch, before Build, and
// must be idempotent. It returns a one-line confirmation to show the user
// (or "" for silence). A failure doesn't abort the launch: the caller
// reports it and launches anyway, since a stale model list beats no
// editor at all.
type Preparer interface {
	Prepare(ctx context.Context, resolved gateway.Resolved) (note string, err error)
}

// Discovery is what an adapter's model-listing probe found.
type Discovery struct {
	// Supported reports whether the endpoint answered with a usable list
	// of models at all.
	Supported bool
	// Native reports whether every model returned looks like it belongs
	// to this adapter's usual provider (e.g. "claude-"/"anthropic-" for
	// Claude Code) — meaning the CLI's own model picker can be trusted to
	// work correctly against this gateway, with no override needed.
	Native bool
	// Models holds whatever model ids were found, native or not.
	Models []string
}

// registry holds every implemented adapter, keyed by Name().
var registry = map[string]Adapter{}

// Register adds an adapter. Called from each adapter package's init().
func Register(a Adapter) {
	registry[a.Name()] = a
}

// aliases maps alternate command-line spellings to canonical adapter
// names, so `setfree vscode` works as well as `setfree code`. Aliases are
// lookup-only: they never appear in All(), so per-CLI settings and
// listings always use the one canonical name.
var aliases = map[string]string{}

// RegisterAlias makes name resolve to the same adapter as canonical.
func RegisterAlias(name, canonical string) {
	aliases[name] = canonical
}

// Find returns the adapter for name, if SetFree can launch it. Aliases
// resolve to their canonical adapter.
func Find(name string) (Adapter, bool) {
	if canonical, ok := aliases[name]; ok {
		name = canonical
	}
	a, ok := registry[name]
	return a, ok
}

// All returns every registered adapter.
func All() []Adapter {
	out := make([]Adapter, 0, len(registry))
	for _, a := range registry {
		out = append(out, a)
	}
	return out
}
