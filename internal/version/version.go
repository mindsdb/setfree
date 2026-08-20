// Package version holds SetFree's build-time version metadata.
package version

// Version is set at build time via -ldflags "-X github.com/mindsdb/setfree/internal/version.Version=...".
// It defaults to "dev" for local/unreleased builds.
var Version = "dev"
