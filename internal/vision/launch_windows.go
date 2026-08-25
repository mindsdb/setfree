//go:build windows

package vision

import (
	"context"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// setDetach is a no-op on Windows: there's no session/process-group concept
// to detach from, and the parent owns the proxy's lifecycle directly.
func setDetach(cmd *exec.Cmd) {}

// sigTerm on Windows maps to the TerminateProcess signal. os/exec's
// cmd.Process.Signal supports a small set on Windows; syscall.SIGTERM is
// handled by killing the process, which is what we'd fall back to anyway.
var sigTerm = syscall.SIGTERM

// watchSignals cancels ctx on a console Ctrl event so a manually-started
// proxy responds to Ctrl-C.
func watchSignals(ctx context.Context, cancel context.CancelFunc) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	select {
	case <-ch:
		cancel()
	case <-ctx.Done():
	}
}
