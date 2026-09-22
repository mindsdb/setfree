package vision

// launch.go starts the vision-bridge proxy as a sidecar of the SetFree
// process that's about to run the CLI. The proxy is the SetFree binary
// itself invoked as `setfree vision-proxy`, so there's no second artifact.
// The parent learns the proxy's address from a "ready <addr>" line on its
// stdout, points the CLI at it, runs the CLI (as a child, via launcher.Run),
// and kills the proxy when the CLI exits.
//
// Only the real gateway's base URL is passed on argv (it's not secret); the
// vision model/endpoint/key and everything else are resolved inside the
// proxy from the same config + env the parent already uses. The real
// gateway's API key is never held by the proxy: it forwards the CLI's own
// Authorization header to the gateway unchanged.

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/mindsdb/setfree/internal/gateway"
)

// ProxySubcommand is the argv the parent uses to relaunch itself as the
// proxy. Kept here so the dispatch in internal/app and this launcher can't
// drift apart.
const ProxySubcommand = "vision-proxy"

// Result is the outcome of MaybeStart: the URL the CLI should be pointed at
// (empty when the bridge is off), and a Stop function the parent must call
// when the CLI exits to tear the proxy down. Stop is safe to call exactly
// once.
type Result struct {
	// ProxyURL replaces the CLI's gateway base URL when the bridge is on;
	// empty when it's off, so callers can run MaybeStart unconditionally and
	// branch on a single non-empty check.
	ProxyURL string
	Stop     func()
}

// MaybeStart launches the vision proxy when the bridge is enabled and
// returns a Result whose ProxyURL replaces the CLI's gateway base URL. When
// disabled it returns an empty ProxyURL and a no-op Stop, so callers can run
// it unconditionally without branching on whether vision is configured.
//
// resolved is the main gateway the launch is using; the proxy forwards to
// its BaseURL. cfg is the resolved vision config.
func MaybeStart(ctx context.Context, resolved gateway.Resolved, cfg Config) (Result, error) {
	if !cfg.Enabled {
		return Result{Stop: func() {}}, nil
	}

	exe, err := os.Executable()
	if err != nil {
		return Result{}, fmt.Errorf("vision bridge: locating the setfree binary: %w", err)
	}

	// Pass the real gateway URL (non-secret) on argv; the vision model, base
	// URL, and key come from the proxy's own config/env resolution. The idle
	// timeout is passed through so a manually-started proxy respects it too.
	cmd := exec.CommandContext(ctx, exe, ProxySubcommand,
		"--target", resolved.Gateway.BaseURL,
		"--idle-timeout", strconv.FormatInt(int64(cfg.IdleTimeout.Seconds()), 10),
	)
	cmd.Env = os.Environ()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, fmt.Errorf("vision bridge: starting proxy: %w", err)
	}
	// Keep the proxy detached from this process group's terminal lifecycle so
	// it doesn't receive terminal signals twice (the parent forwards what's
	// needed; the proxy has no tty of its own). On Unix this is a new session.
	setDetach(cmd)

	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("vision bridge: starting proxy: %w", err)
	}

	// Wait for "ready <addr>" or give up — the proxy failing to start must not
	// hang the launch.
	ready := make(chan string, 1)
	go readReady(stdout, ready)
	addr := ""
	select {
	case addr = <-ready:
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		return Result{}, fmt.Errorf("vision bridge: proxy did not announce readiness in time")
	}
	if addr == "" {
		_ = cmd.Process.Kill()
		return Result{}, fmt.Errorf("vision bridge: proxy exited without becoming ready")
	}

	stop := func() {
		_ = cmd.Process.Signal(sigTerm)
		// Don't reap forever; if it ignores the signal, force it.
		done := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}

	return Result{ProxyURL: "http://" + addr, Stop: stop}, nil
}

// readReady scans the proxy's stdout for the "ready <addr>" line and delivers
// it (or "" on EOF/error) to ch.
func readReady(r io.Reader, ch chan<- string) {
	sc := bufio.NewScanner(r)
	ch <- readyLine(sc)
	// Drain any further output so the child doesn't block on a full pipe if it
	// logs after readiness.
	io.Copy(io.Discard, r)
}
