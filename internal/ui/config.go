package ui

import (
	"fmt"
	"io"

	"github.com/mindsdb/setfree/internal/terminal"
)

// Status is the display-ready state of the default gateway.
type Status struct {
	ConfigPath string
	BaseURL    string // empty if unset
	HasAPIKey  bool
}

// ShowStatus renders `setfree config show`-style output.
func ShowStatus(w io.Writer, s Status) {
	fmt.Fprintf(w, "Config file: %s\n", s.ConfigPath)
	if s.BaseURL == "" {
		fmt.Fprintln(w, "\nNo gateway configured yet. Run `setfree config` or `setfree claude` to set one up.")
		return
	}
	fmt.Fprintf(w, "Base URL:    %s\n", s.BaseURL)
	key := "not configured"
	if s.HasAPIKey {
		key = "configured"
	}
	fmt.Fprintf(w, "API key:     %s\n", key)
}

// MenuHeader renders the small settings screen shown before the gateway
// picker in `setfree config`.
func MenuHeader(w io.Writer, c terminal.Colors, s Status) {
	fmt.Fprintln(w, Robot)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%s\n", c.Bold("SetFree settings"))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Base URL")
	if s.BaseURL == "" {
		fmt.Fprintf(w, "  %s\n", c.Dim("not configured"))
	} else {
		fmt.Fprintf(w, "  %s\n", s.BaseURL)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "API key")
	if s.HasAPIKey {
		fmt.Fprintln(w, "  configured")
	} else {
		fmt.Fprintf(w, "  %s\n", c.Dim("not configured"))
	}
	fmt.Fprintln(w)
}

// ConfirmReset renders the reset confirmation prompt.
const ConfirmResetPrompt = "Reset SetFree configuration? This removes your saved gateway and API key. [y/N] "

// ConnectionSuccess renders the confirmation shown once `setfree config`
// connects a gateway: which coding CLIs SetFree found installed and can
// now launch against it, so the user knows exactly what to type next
// instead of being left to guess. This is meant as the last thing printed
// before the process exits — connecting is a natural stopping point, not
// a reason to loop back to the picker.
func ConnectionSuccess(w io.Writer, c terminal.Colors, gatewayName string, installedKeys []string) {
	fmt.Fprintf(w, "%s %s\n", c.Green(terminal.Check), c.Bold(gatewayName+" gateway successfully connected"))
	fmt.Fprintln(w)

	switch len(installedKeys) {
	case 0:
		fmt.Fprintln(w, "Once Claude Code or Codex is installed, run it through SetFree from any folder:")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  setfree claude")
		fmt.Fprintln(w, "  setfree codex")
	case 1:
		fmt.Fprintln(w, "You're set. Run this from any folder to get started:")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "  setfree %s\n", installedKeys[0])
	default:
		fmt.Fprintln(w, "You're set. Run one of these from any folder to get started:")
		fmt.Fprintln(w)
		for _, key := range installedKeys {
			fmt.Fprintf(w, "  setfree %s\n", key)
		}
	}
	fmt.Fprintln(w)
}
