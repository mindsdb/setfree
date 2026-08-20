//go:build unix

package launcher

import "syscall"

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
