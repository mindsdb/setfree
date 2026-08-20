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
	interacted := false
	if err != nil {
		if !gateway.ErrNotConfigured(err) {
			return fail(err)
		}
		resolved, err = runFirstTimeSetup(e, adapter.Name())
		if err != nil {
			return fail(err)
		}
		interacted = true
	}

	// A model choice hasn't been made for this CLI against this gateway
	// yet, and nothing (env var or existing config) already pins one:
	// probe for one now, once, and remember the outcome either way.
	if setting := e.settings.CLI[adapter.Name()]; resolved.Model == "" && !setting.ModelResolved && terminal.IsTTY(os.Stdin) {
		modelInteracted, modelErr := resolveModel(e, adapter, resolved.Gateway)
		if modelErr != nil {
			debugf("model discovery: %v", modelErr)
		} else if modelInteracted {
			interacted = true
			resolved, err = e.resolver().Resolve(adapter.Name())
			if err != nil {
				return fail(err)
			}
		}
	}

	build, err := adapter.Build(os.Environ(), resolved)
	if err != nil {
		return fail(err)
	}

	if interacted {
		// Only a run that just asked the user something breaks the
		// "feels like running the real CLI" illusion — say so once.
		// Every fully silent run goes straight to exec.
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
	baseURL, apiKey, err := ui.RunSetup(os.Stdout, os.Stdin, e.colors, reader, reader)
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
