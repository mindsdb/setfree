package ui

import (
	"fmt"
	"io"

	"github.com/mindsdb/setfree/internal/detect"
	"github.com/mindsdb/setfree/internal/gateway"
	"github.com/mindsdb/setfree/internal/terminal"
)

// Landing renders the branded screen shown when `setfree` is run with no
// arguments: current gateway status, which coding CLIs are installed, and a
// couple of next steps. Kept compact — this isn't a dashboard.
//
// animate plays the mascot's hatching sequence; callers pass false when
// output isn't an interactive terminal. It's only used here, not on the
// screens that redraw in a loop (`setfree config`), where replaying it on
// every pass would wear thin fast.
func Landing(w io.Writer, c terminal.Colors, resolver *gateway.Resolver, animate bool) {
	ShowRobot(w, animate)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s\n", c.Bold("SETFREE"))
	fmt.Fprintf(w, "  %s\n", c.Dim("Any CLI. Any gateway. Any model."))
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Configured gateway:")
	if resolved, err := resolver.Resolve(""); err == nil {
		fmt.Fprintf(w, "  %s\n", resolved.Gateway.BaseURL)
	} else {
		fmt.Fprintf(w, "  %s\n", c.Dim("not configured yet — run `setfree config`"))
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Coding CLIs found:")
	for _, k := range detect.List {
		mark, label := c.Dim(terminal.Dash), k.DisplayName
		if k.Installed() {
			mark = c.Green(terminal.Check)
		}
		fmt.Fprintf(w, "  %s %s\n", mark, label)
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Try:")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  setfree claude")
	fmt.Fprintln(w, "  setfree codex")
	fmt.Fprintln(w, "  setfree config")
	fmt.Fprintln(w)
}
