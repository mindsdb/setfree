// Package adapters defines how SetFree translates a normalized Gateway
// into the environment a specific coding CLI expects, and holds the
// registry of CLIs SetFree can actually launch. Everything vendor-specific
// (env var names, config flags, wire formats) lives inside one adapter;
// nothing outside this package should need to know about them.
package adapters

import "github.com/mindsdb/setfree/internal/gateway"

// Installation describes a coding CLI found on the user's PATH.
type Installation struct {
	Path string
}

// Build is the environment and extra argv an adapter wants for one launch.
// Args are prepended before the user's own passthrough arguments.
type Build struct {
	Env  []string
	Args []string
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
}

// registry holds every implemented adapter, keyed by Name().
var registry = map[string]Adapter{}

// Register adds an adapter. Called from each adapter package's init().
func Register(a Adapter) {
	registry[a.Name()] = a
}

// Find returns the adapter for name, if SetFree can launch it.
func Find(name string) (Adapter, bool) {
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
