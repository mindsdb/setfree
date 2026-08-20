// Package version holds SetFree's build-time version metadata.
package version

import "fmt"

// Version and Commit are set at build time via -ldflags:
//
//	-X .../internal/version.Version=... -X .../internal/version.Commit=...
//
// A local `go build` leaves both at their zero values: Version defaults to
// "dev", and Commit stays empty. Commit being empty is also how the
// self-updater (internal/update) recognizes a dev build and skips itself —
// there's nothing to compare a dev build against.
var (
	Version = "dev"
	Commit  = ""
)

// Display formats the version for `setfree version`: just Version for a
// dev build, or "Version (short commit)" for a real one.
func Display() string {
	if Commit == "" {
		return Version
	}
	short := Commit
	if len(short) > 7 {
		short = short[:7]
	}
	return fmt.Sprintf("%s (%s)", Version, short)
}
