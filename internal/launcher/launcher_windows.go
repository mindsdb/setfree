//go:build windows

package launcher

import (
	"os"
	"os/exec"
	"os/signal"
)

func launch(opts Options) (int, error) {
	cmd := exec.Command(opts.Path)
	if len(opts.Args) > 0 {
		cmd.Args = opts.Args
	}
	cmd.Env = opts.Env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Windows delivers a console Ctrl+C event to every process attached to
	// the same console, including this one and the child we're about to
	// spawn. Without a signal.Notify registration, Go's default behavior on
	// receiving that event is to terminate this process immediately — which
	// would kill the wrapper (and orphan/race the child) before it can
	// report the child's real exit code. Registering a handler that simply
	// drains the channel keeps this process alive so it can wait for the
	// child, which receives and handles the same Ctrl+C event on its own.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)
	go func() {
		for range sig {
		}
	}()

	if err := cmd.Start(); err != nil {
		return 0, err
	}
	err := cmd.Wait()
	if err == nil {
		return 0, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), nil
	}
	return 0, err
}
