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

// MenuHeader renders the small settings screen shown before the numbered
// edit menu in `setfree config`.
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

// MenuOptions are the numbered choices in the config menu.
const MenuOptions = `What would you like to change?

  1. Base URL
  2. API key
  3. Reset configuration
  4. Done
`

// ConfirmReset renders the reset confirmation prompt.
const ConfirmResetPrompt = "Reset SetFree configuration? This removes your saved gateway and API key. [y/N] "
