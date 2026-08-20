package ui

import (
	"fmt"

	"github.com/mindsdb/setfree/internal/terminal"
)

// SelfUpdateStarted is shown while SetFree downloads and installs a newer
// build of itself. It's the one moment self-updating says anything at all —
// silent otherwise, including when it checks and finds nothing new.
const SelfUpdateStarted = "Updating SetFree..."

// SelfUpdateFinished reports the commit SetFree just updated to. It's a
// single line with no trailing blank: callers own their own spacing, since
// this renders both as a printed line and as the animation's status line.
func SelfUpdateFinished(commit string) string {
	short := commit
	if len(short) > 7 {
		short = short[:7]
	}
	return fmt.Sprintf("%s Updated to %s", terminal.Check, short)
}
