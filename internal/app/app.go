// Package app wires together config, gateway resolution, adapters, and the
// launcher into SetFree's command-line behavior. It's the only package
// that touches os.Stdin/Stdout/Stderr directly outside of main.
package app

import (
	"fmt"
	"os"

	_ "github.com/mindsdb/setfree/internal/adapters/claude"
	_ "github.com/mindsdb/setfree/internal/adapters/codex"
	"github.com/mindsdb/setfree/internal/config"
	"github.com/mindsdb/setfree/internal/gateway"
	"github.com/mindsdb/setfree/internal/secrets"
	"github.com/mindsdb/setfree/internal/terminal"
	"github.com/mindsdb/setfree/internal/ui"
	"github.com/mindsdb/setfree/internal/version"
)

// env holds everything a command needs, resolved once per run.
type env struct {
	dir      string
	settings *config.Settings
	store    secrets.Store
	colors   terminal.Colors
}

func newEnv() (*env, error) {
	dir, err := config.Dir()
	if err != nil {
		return nil, fmt.Errorf("finding SetFree's config directory: %w", err)
	}
	settings, err := config.Load(dir)
	if err != nil {
		return nil, err
	}
	return &env{
		dir:      dir,
		settings: settings,
		store:    secrets.NewFileStore(dir),
		colors:   terminal.Detect(),
	}, nil
}

func (e *env) resolver() *gateway.Resolver {
	return gateway.NewResolver(e.settings, e.store)
}

// Run is SetFree's entire command dispatch. It returns the process exit
// code; main() is responsible for os.Exit.
func Run(args []string) int {
	if len(args) == 0 {
		maybeSelfUpdate()
		return cmdLanding()
	}

	switch args[0] {
	case "help", "-h", "--help":
		fmt.Print(ui.HelpText)
		return 0
	case "version", "--version", "-v":
		fmt.Printf("setfree %s\n", version.Display())
		return 0
	case "config":
		maybeSelfUpdate()
		return cmdConfig(args[1:])
	default:
		maybeSelfUpdate()
		return cmdLaunch(args[0], args[1:])
	}
}

func cmdLanding() int {
	e, err := newEnv()
	if err != nil {
		return fail(err)
	}
	ui.Landing(os.Stdout, e.colors, e.resolver(), terminal.IsTTY(os.Stdout))
	return 0
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, err)
	return 1
}

func debugf(format string, args ...any) {
	if os.Getenv("SETFREE_DEBUG") == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "debug: "+format+"\n", args...)
}
