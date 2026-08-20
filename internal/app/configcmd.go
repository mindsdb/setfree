package app

import (
	"fmt"
	"os"
	"strings"

	"github.com/mindsdb/setfree/internal/config"
	"github.com/mindsdb/setfree/internal/gateway"
	"github.com/mindsdb/setfree/internal/secrets"
	"github.com/mindsdb/setfree/internal/terminal"
	"github.com/mindsdb/setfree/internal/ui"
)

// cmdConfig is `setfree config [show|set|reset]`.
func cmdConfig(args []string) int {
	if len(args) == 0 {
		return cmdConfigMenu()
	}
	switch args[0] {
	case "show":
		return cmdConfigShow()
	case "set":
		return cmdConfigSet(args[1:])
	case "reset":
		return cmdConfigReset(args[1:])
	default:
		return fail(fmt.Errorf("unknown 'config' subcommand %q\n\nUsage: setfree config [show|set|reset]", args[0]))
	}
}

func statusOf(e *env) ui.Status {
	st := ui.Status{ConfigPath: config.Path(e.dir)}
	setting, name, ok := e.settings.Default()
	if !ok {
		return st
	}
	st.BaseURL = setting.BaseURL
	if _, has, _ := e.store.Get(name); has {
		st.HasAPIKey = true
	}
	return st
}

func currentGatewayName(e *env) string {
	if _, name, ok := e.settings.Default(); ok {
		return name
	}
	return config.DefaultGatewayName
}

func cmdConfigShow() int {
	e, err := newEnv()
	if err != nil {
		return fail(err)
	}
	ui.ShowStatus(os.Stdout, statusOf(e))
	return 0
}

// cmdConfigSet is the non-interactive, scriptable path: setfree config set
// --base-url <url> [--api-key <key>]. With no flags it points the user at
// either flag usage or `setfree config` for interactive editing, rather
// than guessing which field they meant to change.
func cmdConfigSet(args []string) int {
	var baseURL, apiKey string
	var haveBaseURL, haveAPIKey bool

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--base-url":
			i++
			if i >= len(args) {
				return fail(fmt.Errorf("--base-url requires a value"))
			}
			baseURL, haveBaseURL = args[i], true
		case "--api-key":
			i++
			if i >= len(args) {
				return fail(fmt.Errorf("--api-key requires a value"))
			}
			apiKey, haveAPIKey = args[i], true
		default:
			return fail(fmt.Errorf("unknown flag %q\n\nUsage: setfree config set --base-url <url> [--api-key <key>]", args[i]))
		}
	}

	if !haveBaseURL && !haveAPIKey {
		return fail(fmt.Errorf("nothing to set.\n\nUsage:\n  setfree config set --base-url <url> [--api-key <key>]\n\nFor interactive editing, run:\n  setfree config"))
	}

	e, err := newEnv()
	if err != nil {
		return fail(err)
	}
	name := currentGatewayName(e)

	if haveBaseURL {
		valid, verr := gateway.ValidateBaseURL(baseURL)
		if verr != nil {
			return fail(verr)
		}
		e.settings.SetGatewayBaseURL(name, valid)
		if err := config.Save(e.dir, e.settings); err != nil {
			return fail(err)
		}
	}
	if haveAPIKey {
		if err := e.store.Set(name, apiKey); err != nil {
			return fail(err)
		}
	}

	fmt.Println("✓ Settings saved")
	return 0
}

func cmdConfigReset(args []string) int {
	force := false
	for _, a := range args {
		switch a {
		case "--force", "-y":
			force = true
		default:
			return fail(fmt.Errorf("unknown flag %q for 'config reset'", a))
		}
	}

	if !force {
		if !terminal.IsTTY(os.Stdin) {
			return fail(fmt.Errorf("config reset requires confirmation.\n\nRun with --force to reset non-interactively."))
		}
		reader := terminal.NewReader(os.Stdin)
		fmt.Print(ui.ConfirmResetPrompt)
		answer, err := reader.ReadLine()
		if err != nil || !isYes(answer) {
			fmt.Println("Cancelled.")
			return 0
		}
	}

	dir, err := config.Dir()
	if err != nil {
		return fail(err)
	}
	if err := config.Reset(dir); err != nil {
		return fail(err)
	}
	if err := secrets.NewFileStore(dir).Reset(); err != nil {
		return fail(err)
	}

	fmt.Println("✓ Configuration reset")
	return 0
}

// cmdConfigMenu is bare `setfree config`: a small interactive loop over the
// same status view, letting the user edit one field at a time.
func cmdConfigMenu() int {
	if !terminal.IsTTY(os.Stdin) {
		return fail(fmt.Errorf("`setfree config` needs an interactive terminal.\n\nUse:\n  setfree config show\n  setfree config set --base-url <url> [--api-key <key>]\n  setfree config reset --force"))
	}

	e, err := newEnv()
	if err != nil {
		return fail(err)
	}
	reader := terminal.NewReader(os.Stdin)

	for {
		ui.MenuHeader(os.Stdout, e.colors, statusOf(e))
		fmt.Print(ui.MenuOptions)
		fmt.Print("> ")

		choice, err := reader.ReadLine()
		if err != nil {
			fmt.Println()
			return 0
		}

		switch strings.TrimSpace(choice) {
		case "1":
			name := currentGatewayName(e)
			baseURL, err := ui.PromptBaseURL(os.Stdout, reader)
			if err != nil {
				return 0
			}
			e.settings.SetGatewayBaseURL(name, baseURL)
			if err := config.Save(e.dir, e.settings); err != nil {
				return fail(err)
			}
		case "2":
			name := currentGatewayName(e)
			key, err := ui.PromptAPIKey(os.Stdout, reader)
			if err != nil {
				return 0
			}
			if err := e.store.Set(name, key); err != nil {
				return fail(err)
			}
		case "3":
			fmt.Print(ui.ConfirmResetPrompt)
			answer, _ := reader.ReadLine()
			if isYes(answer) {
				if err := config.Reset(e.dir); err != nil {
					return fail(err)
				}
				if err := e.store.Reset(); err != nil {
					return fail(err)
				}
				e.settings = &config.Settings{Version: config.CurrentVersion}
				fmt.Println("✓ Configuration reset")
			}
		case "4", "":
			return 0
		default:
			fmt.Println("Please choose 1-4.")
		}
		fmt.Println()
	}
}

func isYes(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}
