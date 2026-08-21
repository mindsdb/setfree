// Package detect knows about coding CLIs SetFree is aware of — both the
// ones it can launch (see internal/adapters) and ones it merely recognizes
// for the landing screen and helpful error messages.
package detect

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

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
	{Key: "code", DisplayName: "VS Code", BinaryNames: VSCodeBinaries(), Supported: true},
	{Key: "gemini", DisplayName: "Gemini CLI", BinaryNames: []string{"gemini"}, Supported: false},
	{Key: "aider", DisplayName: "Aider", BinaryNames: []string{"aider"}, Supported: false},
}

// VSCodeBinaries lists everywhere the VS Code CLI actually lives: the PATH
// names first, then — on macOS — the CLI script inside the app bundle
// itself. Installing VS Code on a Mac doesn't put `code` on PATH; that
// takes a separate "Shell Command: Install 'code' command" step most
// people never run, so an installed VS Code would otherwise be reported as
// missing. exec.LookPath handles the absolute entries (a name containing a
// slash is tried directly), so callers just iterate this list as usual.
//
// Exported because the vscode adapter must search the exact same places —
// detect saying "installed" while launch says "not found" would be worse
// than either alone.
func VSCodeBinaries() []string {
	names := []string{"code", "code-insiders"}
	if runtime.GOOS != "darwin" {
		return names
	}
	appDirs := []string{"/Applications"}
	if home, err := os.UserHomeDir(); err == nil {
		appDirs = append(appDirs, filepath.Join(home, "Applications"))
	}
	for _, dir := range appDirs {
		names = append(names,
			filepath.Join(dir, "Visual Studio Code.app/Contents/Resources/app/bin/code"),
			filepath.Join(dir, "Visual Studio Code - Insiders.app/Contents/Resources/app/bin/code-insiders"),
		)
	}
	return names
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
