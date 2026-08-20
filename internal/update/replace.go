package update

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// replaceSelf atomically swaps the file at path for content. It writes to a
// temp file in the same directory (so the rename is same-filesystem, hence
// atomic) and renames over path.
//
// This is safe even though path is very likely the binary currently
// executing this code: on Unix, renaming over a running executable's file
// never disturbs the running process, since it's already mapped by inode,
// not by name — only the *next* invocation sees the new content. Modern
// Windows generally allows the same, but for older Windows behavior, where
// replacing a running exe's file can fail, we fall back to moving the
// current binary aside first and then moving the new one into its place.
func replaceSelf(path string, content []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".setfree-update-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once the rename below succeeds

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("writing new binary: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing new binary: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return fmt.Errorf("setting permissions: %w", err)
	}

	renameErr := os.Rename(tmpPath, path)
	if renameErr == nil {
		return nil
	}
	if runtime.GOOS != "windows" {
		return fmt.Errorf("installing new binary: %w", renameErr)
	}

	// Windows fallback: move the running exe aside, then move the new one
	// into place. Best-effort cleanup of a stale .old from an interrupted
	// previous update.
	old := path + ".old"
	os.Remove(old)
	if err := os.Rename(path, old); err != nil {
		return fmt.Errorf("installing new binary: %w", renameErr)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Rename(old, path) // best-effort restore
		return fmt.Errorf("installing new binary: %w", err)
	}
	return nil
}
