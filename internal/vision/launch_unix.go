//go:build unix

package vision

import (
	"context"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// setDetach puts the proxy in its own session so it doesn't share the
// terminal's signal group with the parent. The parent owns the proxy's
// lifecycle (it kills the proxy when the CLI exits), so detaching from the
// terminal just avoids the proxy receiving signals meant for the CLI twice.
func setDetach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// sigTerm is the polite signal used to ask the proxy to exit before forcing it.
var sigTerm = syscall.SIGTERM

// watchSignals cancels ctx on a terminating signal, so a manually-started
// proxy responds to Ctrl-C. The parent also kills the proxy directly on CLI
// exit; this is for the human-started case.
func watchSignals(ctx context.Context, cancel context.CancelFunc) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	select {
	case <-ch:
		cancel()
	case <-ctx.Done():
	}
}
