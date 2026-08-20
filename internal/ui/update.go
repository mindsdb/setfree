package ui

import "fmt"

// SelfUpdateStarted is printed right before SetFree downloads and installs
// a newer build of itself. It's the one moment self-updating is allowed to
// say anything at all — silent otherwise, including when it checks and
// finds nothing new.
const SelfUpdateStarted = "Updating SetFree..."

// SelfUpdateFinished reports the commit SetFree just updated to.
func SelfUpdateFinished(commit string) string {
	short := commit
	if len(short) > 7 {
		short = short[:7]
	}
	return fmt.Sprintf("✓ Updated to %s\n\n", short)
}
