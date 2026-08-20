// Package update lets SetFree keep itself current with the latest commit
// on main, rather than waiting on manually-cut releases. A GitHub Actions
// workflow rebuilds and republishes a rolling "latest" release on every
// push to main, alongside a commit.txt naming the exact commit it was
// built from. SetFree compares that against its own embedded commit
// (internal/version.Commit) and, if it's behind, replaces its own binary
// in place.
//
// The check is throttled to once a day and uses a short timeout, so a slow
// or unreachable network never adds noticeable latency to a normal launch.
// Set SETFREE_NO_AUTOUPDATE to turn it off entirely.
package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// EnvDisable, when set to any non-empty value, turns self-updating off.
const EnvDisable = "SETFREE_NO_AUTOUPDATE"

// releaseTag is the one release SetFree's self-updater ever looks at: a
// rolling release that always tracks main's current HEAD. It's deliberately
// distinct from any future tagged (`vX.Y.Z`) releases, which install.sh /
// install.ps1 resolve dynamically instead.
const releaseTag = "latest"

const checkInterval = 24 * time.Hour

// ThrottleChecks gates the once-a-day limit on update checks. It's off
// while SetFree is still moving fast: a fix pushed to main should reach
// people on their next launch, not up to a day later. Flip it back to true
// once releases settle down, so normal use stops making a network request
// on every single command.
//
// SETFREE_NO_AUTOUPDATE still disables checking entirely, regardless.
const ThrottleChecks = false

const repo = "mindsdb/setfree"

const defaultBaseURL = "https://github.com/" + repo + "/releases/download/" + releaseTag

// Checker holds what's needed to check for and apply an update.
type Checker struct {
	// Dir is SetFree's config directory, used only to remember when we
	// last checked.
	Dir string
	// Client defaults to http.DefaultClient.
	Client *http.Client
	// Getenv defaults to os.Getenv; tests override it.
	Getenv func(string) string
	// BaseURL defaults to the real rolling release on GitHub; tests point
	// it at a local server instead.
	BaseURL string
}

func (c *Checker) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return defaultBaseURL
}

func (c *Checker) assetURL(name string) string {
	return c.baseURL() + "/" + name
}

func (c *Checker) getenv(key string) string {
	if c.Getenv != nil {
		return c.Getenv(key)
	}
	return os.Getenv(key)
}

func (c *Checker) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return http.DefaultClient
}

func (c *Checker) statePath() string {
	return filepath.Join(c.Dir, "last-update-check")
}

// Due reports whether it's time to check again: never when disabled by env
// var, always when ThrottleChecks is off, and otherwise when there's no
// record of a previous check or the last one was over checkInterval ago.
func (c *Checker) Due(now time.Time) bool {
	if c.getenv(EnvDisable) != "" {
		return false
	}
	if !ThrottleChecks {
		return true
	}
	data, err := os.ReadFile(c.statePath())
	if err != nil {
		return true
	}
	last, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
	if err != nil {
		return true
	}
	return now.Sub(last) >= checkInterval
}

// MarkChecked records now as the last check time, so Due won't fire again
// until the next interval. Called whether or not the check succeeded — a
// broken network shouldn't turn into a retry on every single launch.
func (c *Checker) MarkChecked(now time.Time) error {
	return os.WriteFile(c.statePath(), []byte(now.UTC().Format(time.RFC3339)), 0o644)
}

func (c *Checker) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

// LatestCommit fetches the commit SHA the rolling release was built from.
func (c *Checker) LatestCommit(ctx context.Context) (string, error) {
	body, err := c.get(ctx, c.assetURL("commit.txt"), 1<<20)
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(string(body))
	if sha == "" {
		return "", fmt.Errorf("commit.txt was empty")
	}
	return sha, nil
}

// Asset returns the release asset name for goos/goarch, matching the naming
// release.yml and install.sh/install.ps1 already agree on.
func Asset(goos, goarch string) (string, error) {
	var os_ string
	switch goos {
	case "darwin":
		os_ = "Darwin"
	case "linux":
		os_ = "Linux"
	case "windows":
		os_ = "Windows"
	default:
		return "", fmt.Errorf("unsupported OS: %s", goos)
	}

	var arch string
	switch goarch {
	case "arm64":
		arch = "arm64"
	case "amd64":
		arch = "x86_64"
	default:
		return "", fmt.Errorf("unsupported architecture: %s", goarch)
	}

	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("setfree_%s_%s.%s", os_, arch, ext), nil
}

// Apply downloads the current platform's build from the rolling release,
// verifies it against checksums.txt, and replaces the binary at exePath.
func (c *Checker) Apply(ctx context.Context, exePath string) error {
	asset, err := Asset(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}

	archive, err := c.get(ctx, c.assetURL(asset), 200<<20)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", asset, err)
	}

	sums, err := c.get(ctx, c.assetURL("checksums.txt"), 1<<20)
	if err != nil {
		return fmt.Errorf("downloading checksums.txt: %w", err)
	}
	if err := verifyChecksum(archive, asset, sums); err != nil {
		return err
	}

	binaryName := "setfree"
	if runtime.GOOS == "windows" {
		binaryName = "setfree.exe"
	}
	binary, err := extract(archive, asset, binaryName)
	if err != nil {
		return err
	}

	return replaceSelf(exePath, binary)
}

func verifyChecksum(archive []byte, asset string, checksumsTxt []byte) error {
	var expected string
	for _, line := range strings.Split(string(checksumsTxt), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == asset {
			expected = fields[0]
			break
		}
	}
	if expected == "" {
		return fmt.Errorf("%s isn't listed in checksums.txt", asset)
	}
	sum := sha256.Sum256(archive)
	got := hex.EncodeToString(sum[:])
	if got != expected {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", asset, expected, got)
	}
	return nil
}

func extract(archive []byte, asset, binaryName string) ([]byte, error) {
	if strings.HasSuffix(asset, ".zip") {
		return extractZip(archive, binaryName)
	}
	return extractTarGz(archive, binaryName)
}

func extractTarGz(archive []byte, binaryName string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Name == binaryName {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("%s not found in archive", binaryName)
}

func extractZip(archive []byte, binaryName string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if f.Name == binaryName {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("%s not found in archive", binaryName)
}
