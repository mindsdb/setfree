// Package launcher runs the real coding CLI binary as a faithful
// replacement for `setfree`: same stdin/stdout/stderr, working directory,
// arguments, exit code, and signal behavior a direct invocation would have.
package launcher

// Options describes the process to launch.
type Options struct {
	// Path is the resolved absolute path to the CLI binary.
	Path string
	// Args is the full argv, including Args[0] (conventionally the binary
	// name, matching what a shell would set).
	Args []string
	// Env is the complete child environment.
	Env []string
}

// Launch runs the process described by opts.
//
// On Unix, this replaces the current process image (execve) and never
// returns on success — there is no wrapper process left to forward signals
// through, which is exactly the point. It only returns if the exec call
// itself failed.
//
// On Windows, where processes can't be replaced in place, it spawns the
// child, waits for it, and returns its exit code.
func Launch(opts Options) (exitCode int, err error) {
	return launch(opts)
}

// Run spawns the process described by opts as a child and waits for it,
// returning its exit code. Unlike Launch (which exec-replaces on Unix), Run
// keeps the SetFree process alive as the parent — so a caller that started
// sidecar processes (e.g. the vision proxy) can tear them down when the child
// exits. Signals to the parent are forwarded to the child so the launched
// CLI behaves like it was invoked directly.
//
// This is the path the vision bridge uses: the proxy must live exactly as
// long as the CLI, which means SetFree has to stay around to kill it. Every
// other launch still uses Launch (exec-replace), so the common case is
// unchanged.
func Run(opts Options) (exitCode int, err error) {
	return run(opts)
}
