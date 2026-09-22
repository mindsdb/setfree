//go:build unix

package launcher

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

func launch(opts Options) (int, error) {
	// syscall.Exec replaces this process's image with the target binary.
	// stdin/stdout/stderr, the controlling terminal, the process group, and
	// the working directory all carry over automatically because there is
	// no new process — this one just becomes the child. Signals (including
	// Ctrl+C) go straight to it with no forwarding logic required.
	err := syscall.Exec(opts.Path, opts.Args, opts.Env)
	// Reached only on failure; on success the process image is gone.
	return 0, err
}

// run spawns opts as a foreground child in the same process group and waits
// for it, forwarding termination signals so the child behaves like a direct
// invocation. The child shares the foreground process group, so terminal-
// generated signals (Ctrl-C in cooked mode, SIGWINCH on resize) reach it
// directly; this parent only needs to (a) not die on Ctrl-C before the child
// reports its exit code, and (b) forward externally-sent signals (SIGTERM,
// SIGHUP, SIGQUIT from `kill`) that are delivered to this process alone.
func run(opts Options) (int, error) {
	cmd := exec.Command(opts.Path)
	if len(opts.Args) > 0 {
		cmd.Args = opts.Args
	}
	cmd.Env = opts.Env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return 0, err
	}

	// SIGINT (Ctrl-C) is delivered to the whole foreground process group, so
	// the child already receives it; draining it here just keeps this parent
	// alive to report the child's real exit code. Forward the rest.
	sigCh := make(chan os.Signal, 4)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case s := <-sigCh:
				if s == os.Interrupt {
					continue // child got it directly via the shared pgroup
				}
				_ = cmd.Process.Signal(s)
			case <-done:
				return
			}
		}
	}()

	err := cmd.Wait()
	signal.Stop(sigCh)
	close(done)

	if err == nil {
		return 0, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), nil
	}
	return 0, err
}
