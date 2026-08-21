// Package vscode launches Visual Studio Code with SetFree's gateway
// environment in place, so the Claude Code extension inside it routes
// through the configured gateway. The extension works by spawning the same
// `claude` binary the claude adapter launches directly, and that binary
// reads its configuration from the environment it inherits — from VS Code,
// which inherits it from here. The env this adapter builds is therefore
// exactly the claude adapter's (claude.Env), not a variant.
//
// One real caveat: the `code` CLI only starts a fresh VS Code process when
// none is running. If an instance is already open, it just tells that
// instance to open a window over IPC and exits — and the existing process
// keeps whatever environment it started with, gateway or not. There's no
// clean way to force a fresh process without switching profiles
// (--user-data-dir), so instead of silently half-working, the launch
// prints a one-line note about it.
package vscode

import (
	"context"
	"fmt"

	"github.com/mindsdb/setfree/internal/adapters"
	"github.com/mindsdb/setfree/internal/adapters/claude"
	"github.com/mindsdb/setfree/internal/catalog"
	"github.com/mindsdb/setfree/internal/detect"
	"github.com/mindsdb/setfree/internal/gateway"
)

type adapter struct{}

func init() {
	adapters.Register(adapter{})
	adapters.RegisterAlias("vscode", "code")
}

func (adapter) Name() string        { return "code" }
func (adapter) DisplayName() string { return "VS Code" }

// BinaryNames prefers the PATH names but also searches the macOS app
// bundle directly (see detect.VSCodeBinaries): installing VS Code on a Mac
// doesn't put `code` on PATH, and requiring that extra manual step would
// make an installed VS Code look missing.
func (adapter) BinaryNames() []string { return detect.VSCodeBinaries() }

func (adapter) Build(baseEnv []string, resolved gateway.Resolved) (adapters.Build, error) {
	return adapters.Build{
		Env:  claude.Env(baseEnv, resolved),
		Note: "If VS Code is already running, quit it fully first — the gateway only applies to a freshly started VS Code.",
	}, nil
}

func (adapter) DiscoverModels(ctx context.Context, gw gateway.Gateway) adapters.Discovery {
	// The thing talking to the gateway is the claude binary the extension
	// spawns, so the probe must be Claude Code's, not something editor-
	// flavored.
	return claude.Discover(ctx, gw)
}

// Prepare configures VS Code's own chat to call the gateway directly —
// no extension in the middle: it fetches the gateway's model catalog and
// writes a Custom Endpoint provider entry into chatLanguageModels.json,
// so every catalog model appears in the native chat model picker posting
// straight to the gateway's chat-completions endpoint. Runs on every
// launch so the picker tracks the catalog; the upsert only ever touches
// SetFree's own entry.
func (adapter) Prepare(ctx context.Context, resolved gateway.Resolved) (string, error) {
	models, err := catalog.Fetch(ctx, resolved.Gateway.BaseURL, resolved.Gateway.APIKey)
	if err != nil {
		return "", fmt.Errorf("fetching the gateway's model catalog: %w", err)
	}

	group := buildGroup(resolved.Gateway.BaseURL, resolved.Gateway.APIKey, models)
	if len(group.Models) == 0 {
		return "", fmt.Errorf("the gateway listed no chat models")
	}

	path, err := chatModelsPath()
	if err != nil {
		return "", err
	}
	if err := upsertChatModels(path, group); err != nil {
		return "", err
	}
	return fmt.Sprintf("✓ %d gateway models configured in VS Code's chat model picker", len(group.Models)), nil
}
