package app

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/mindsdb/setfree/internal/adapters"
	"github.com/mindsdb/setfree/internal/config"
	"github.com/mindsdb/setfree/internal/detect"
	"github.com/mindsdb/setfree/internal/gateway"
	"github.com/mindsdb/setfree/internal/launcher"
	"github.com/mindsdb/setfree/internal/terminal"
	"github.com/mindsdb/setfree/internal/ui"
)

// cmdLaunch is `setfree <cli> [args...]`.
func cmdLaunch(name string, passthrough []string) int {
	adapter, hasAdapter := adapters.Find(name)
	if !hasAdapter {
		if known, ok := detect.Find(name); ok {
			return fail(ui.UnsupportedError(known.DisplayName, known.Key))
		}
		return fail(ui.UnknownCLIError(name))
	}

	path := findBinary(adapter.BinaryNames())
	if path == "" {
		return fail(ui.NotInstalledError(adapter.DisplayName(), adapter.Name()))
	}

	e, err := newEnv()
	if err != nil {
		return fail(err)
	}

	resolved, err := e.resolver().Resolve(adapter.Name())
	justConfigured := false
	if err != nil {
		if !gateway.ErrNotConfigured(err) {
			return fail(err)
		}
		resolved, err = runFirstTimeSetup(e, adapter.Name())
		if err != nil {
			return fail(err)
		}
		justConfigured = true
	}

	build, err := adapter.Build(os.Environ(), resolved)
	if err != nil {
		return fail(err)
	}

	if justConfigured {
		// Only the first run breaks the "feels like running the real CLI"
		// illusion (it has to, to collect a gateway) — say so once. Every
		// subsequent launch stays silent and goes straight to exec.
		fmt.Printf("Launching %s...\n\n", adapter.DisplayName())
	}

	argv := buildArgv(path, build.Args, passthrough)

	code, err := launcher.Launch(launcher.Options{Path: path, Args: argv, Env: build.Env})
	if err != nil {
		debugf("exec of %s failed: %v", path, err)
		return fail(fmt.Errorf("couldn't launch %s: %w", adapter.DisplayName(), err))
	}
	return code
}

// buildArgv assembles the child's argv: the resolved binary path as argv[0],
// then any adapter-owned config flags (e.g. Codex's `-c` overrides), then
// the user's own arguments untouched and in order.
func buildArgv(path string, adapterArgs, passthrough []string) []string {
	argv := make([]string, 0, 1+len(adapterArgs)+len(passthrough))
	argv = append(argv, path)
	argv = append(argv, adapterArgs...)
	argv = append(argv, passthrough...)
	return argv
}

func findBinary(names []string) string {
	for _, name := range names {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

func runFirstTimeSetup(e *env, cliName string) (gateway.Resolved, error) {
	if !terminal.IsTTY(os.Stdin) {
		return gateway.Resolved{}, ui.NoGatewayError()
	}

	reader := terminal.NewReader(os.Stdin)
	baseURL, apiKey, err := ui.RunSetup(os.Stdout, e.colors, reader, reader)
	if err != nil {
		return gateway.Resolved{}, err
	}

	e.settings.SetGatewayBaseURL(config.DefaultGatewayName, baseURL)
	if err := config.Save(e.dir, e.settings); err != nil {
		return gateway.Resolved{}, fmt.Errorf("saving settings: %w", err)
	}
	if apiKey != "" {
		if err := e.store.Set(config.DefaultGatewayName, apiKey); err != nil {
			return gateway.Resolved{}, fmt.Errorf("saving API key: %w", err)
		}
	}

	return e.resolver().Resolve(cliName)
}
