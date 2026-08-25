package vision

// proxy.go is the request-path proxy the vision bridge runs as. It listens
// on a local ephemeral port, and for every request it forwards to the main
// gateway unchanged — except POST /v1/messages bodies carrying base64 image
// content blocks, which it rewrites to text captions first (see caption.go)
// so a text-only main model never receives a raw image.
//
// Lifecycle: the proxy is started by internal/app/launch.go as a child of the
// SetFree process that's about to run the CLI, and that parent kills it when
// the CLI exits. An idle-timeout (default 1h) is only a backstop for a proxy
// orphaned by an abnormal parent death (SIGKILL) — it must be generous enough
// to never cull a live session that's simply between turns.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// proxy is the running vision-bridge proxy.
type proxy struct {
	target    string // the real gateway base URL (no trailing slash)
	cap       *captioner
	transport *http.Transport

	inFlight     atomic.Int64
	lastActivity atomic.Int64
}

// newProxy constructs a proxy forwarding to target with the given vision cfg.
func newProxy(target string, cfg Config) *proxy {
	return &proxy{
		target:    trimTrailingSlash(target),
		cap:       newCaptioner(cfg),
		transport: &http.Transport{MaxIdleConnsPerHost: 4},
	}
}

// Serve starts the proxy on a local ephemeral port. It prints a single
// "ready <addr>" line to w (the parent reads this to learn where to point the
// CLI), then serves until ctx is cancelled or the idle backstop fires.
func Serve(ctx context.Context, w io.Writer, target string, cfg Config) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("vision proxy: listen: %w", err)
	}
	defer ln.Close()

	p := newProxy(target, cfg)
	p.touch()

	srv := &http.Server{
		Handler:      http.HandlerFunc(p.handle),
		ReadTimeout:  0, // requests can be large and streamed; don't cap reads
		WriteTimeout: 0, // responses are streamed (SSE); can't cap writes
	}

	// Announce readiness so the parent can wire the CLI at us.
	fmt.Fprintf(w, "ready %s\n", ln.Addr().String())

	// Idle backstop: exit only when no request is in flight AND nothing has
	// touched us for the configured timeout. A live session keeps lastActivity
	// fresh on every request, so this only reaps an orphaned proxy.
	if cfg.IdleTimeout > 0 {
		go p.idleWatcher(ctx, ln, cfg.IdleTimeout)
	}

	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()

	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (p *proxy) touch() {
	p.lastActivity.Store(time.Now().Unix())
}

func (p *proxy) idleWatcher(ctx context.Context, ln net.Listener, timeout time.Duration) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if p.inFlight.Load() == 0 {
				idle := time.Since(time.Unix(p.lastActivity.Load(), 0))
				if idle >= timeout {
					_ = ln.Close()
					return
				}
			}
		}
	}
}

func (p *proxy) handle(w http.ResponseWriter, r *http.Request) {
	p.inFlight.Add(1)
	defer p.inFlight.Add(-1)
	p.touch()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "vision proxy: reading request: "+err.Error(), http.StatusBadGateway)
		return
	}
	r.Body.Close()

	// Only the Anthropic messages endpoint carries image blocks we know how
	// to rewrite. Everything else passes through untouched.
	rewrite := r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/v1/messages")
	if rewrite {
		body = p.rewriteBody(r, body)
	}

	targetURL := p.target + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "vision proxy: building request: "+err.Error(), http.StatusBadGateway)
		return
	}
	copyHeaders(req.Header, r.Header)
	// The rewritten body may differ in length; drop the original and let the
	// transport recompute. Keep-alive on the loopback is fine.
	req.Header.Del("Content-Length")
	req.ContentLength = int64(len(body))

	resp, err := p.transport.RoundTrip(req)
	if err != nil {
		http.Error(w, "vision proxy: gateway unreachable: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Stream the response back, flushing so SSE streams reach the CLI
	// incrementally rather than buffering a whole turn.
	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	flushCopy(w, resp.Body)
}

// rewriteBody decodes a /v1/messages body, rewrites image blocks to captions,
// and re-marshals it. If the body isn't valid JSON or carries no images, it's
// returned untouched (the gateway sees exactly what the CLI sent).
func (p *proxy) rewriteBody(r *http.Request, body []byte) []byte {
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return body // not JSON or unexpected shape — pass through unchanged
	}
	messages, ok := parsed["messages"].([]any)
	if !ok || len(messages) == 0 {
		return body
	}
	imgs := collectImageBlocks(messages)
	if len(imgs) == 0 {
		return body // no images — byte-identical passthrough for this turn
	}

	// Warm the cache in parallel, then do the serial rewrite (all cache hits).
	ctx := r.Context()
	p.cap.prewarm(ctx, imgs)
	rewritten := p.cap.rewriteMessages(ctx, messages)

	out := shallowCopyMap(parsed)
	out["messages"] = rewritten
	newBody, err := json.Marshal(out)
	if err != nil {
		return body
	}
	return newBody
}

// flushCopy streams src to dst, flushing after each write so SSE responses
// reach the client as they arrive.
func flushCopy(dst http.ResponseWriter, src io.Reader) {
	flusher, _ := dst.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			_, _ = dst.Write(buf[:n])
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			if err == io.EOF {
				return
			}
			return
		}
	}
}

// hopBySideHeaders are stripped per RFC 7230; the proxy terminates them on the
// loopback hop.
var hopByHopHeaders = []string{
	"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization",
	"Te", "Trailer", "Transfer-Encoding", "Upgrade",
}

func copyHeaders(dst, src http.Header) {
	for _, h := range hopByHopHeaders {
		src.Del(h)
	}
	for k, vs := range src {
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}

// readyLine parses a "ready <addr>" line from a scanner over the proxy's
// stdout. Returns the address, or "" if no ready line was seen.
func readyLine(sc *bufio.Scanner) string {
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "ready ") {
			return strings.TrimPrefix(line, "ready ")
		}
	}
	return ""
}
