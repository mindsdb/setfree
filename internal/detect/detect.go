// Package detect knows about coding CLIs SetFree is aware of — both the
// ones it can launch (see internal/adapters) and ones it merely recognizes
// for the landing screen and helpful error messages.
package detect

import "os/exec"

// Known describes one coding CLI SetFree recognizes.
type Known struct {
	// Key is the name typed on the command line, e.g. "claude".
	Key string
	// DisplayName is the human-readable product name, e.g. "Claude Code".
	DisplayName string
	// BinaryNames are the executable names to look for on PATH, tried in
	// order. Most CLIs have exactly one.
	BinaryNames []string
	// Supported reports whether SetFree has a launch adapter for this CLI.
	// Unsupported entries are still detected and listed, just not launched.
	Supported bool
}

// List enumerates every coding CLI SetFree knows about, in the order shown
// on the landing screen.
var List = []Known{
	{Key: "claude", DisplayName: "Claude Code", BinaryNames: []string{"claude"}, Supported: true},
	{Key: "codex", DisplayName: "Codex", BinaryNames: []string{"codex"}, Supported: true},
	{Key: "code", DisplayName: "VS Code", BinaryNames: []string{"code", "code-insiders"}, Supported: true},
	{Key: "gemini", DisplayName: "Gemini CLI", BinaryNames: []string{"gemini"}, Supported: false},
	{Key: "aider", DisplayName: "Aider", BinaryNames: []string{"aider"}, Supported: false},
}

// Find looks up key in List.
func Find(key string) (Known, bool) {
	for _, k := range List {
		if k.Key == key {
			return k, true
		}
	}
	return Known{}, false
}

// Path reports the resolved executable path for k, if it's installed.
func (k Known) Path() (string, bool) {
	for _, name := range k.BinaryNames {
		if p, err := exec.LookPath(name); err == nil {
			return p, true
		}
	}
	return "", false
}

// Installed reports whether k is found on PATH.
func (k Known) Installed() bool {
	_, ok := k.Path()
	return ok
}
