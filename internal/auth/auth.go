// Package auth implements the browser-based sign-in SetFree uses for
// gateways that authenticate with an identity provider rather than a
// pasted API key.
//
// The flow is OAuth 2.0 authorization code with PKCE (RFC 7636) over a
// loopback redirect (RFC 8252 §7.3): SetFree listens on a random port on
// 127.0.0.1, opens the system browser at the provider's authorize endpoint,
// and receives the code on that local callback. No client secret is
// involved — a CLI can't keep one — which is exactly what PKCE exists to
// make safe.
//
// The access token this yields is deliberately short-lived, so callers are
// expected to trade it immediately for a long-lived API key (see
// CreateAPIKey) and store only that. SetFree therefore never has to manage
// refresh tokens or token expiry at launch time.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Config describes one OpenID Connect provider well enough to sign in.
type Config struct {
	// Issuer is the provider's base URL, e.g.
	// "https://auth.example.com/auth" for a Keycloak deployment served
	// under /auth.
	Issuer string
	// Realm is the Keycloak realm name.
	Realm string
	// ClientID is the public client registered for this CLI.
	ClientID string
	// Scopes requested at authorize time.
	Scopes []string
	// SuccessRedirect, if set, is where the browser is sent after a
	// successful sign-in — a product console, say — instead of staying on
	// the plain "you can close this tab" page.
	SuccessRedirect string
}

func (c Config) endpoint(name string) string {
	return fmt.Sprintf("%s/realms/%s/protocol/openid-connect/%s",
		strings.TrimRight(c.Issuer, "/"), c.Realm, name)
}

// Login runs the loopback PKCE flow and returns an access token.
//
// notify receives human-readable progress (the URL to open, mainly) so the
// caller decides how to present it. The context bounds the whole flow,
// including how long it waits for the user to finish in the browser.
func Login(ctx context.Context, cfg Config, notify func(string)) (string, error) {
	verifier, challenge, err := pkcePair()
	if err != nil {
		return "", err
	}
	state, err := randomString(24)
	if err != nil {
		return "", err
	}

	// Bind before building the URL: the redirect_uri has to name the exact
	// port, and only the kernel can tell us which one is free.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("opening a local callback port: %w", err)
	}
	defer listener.Close()
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", listener.Addr().(*net.TCPAddr).Port)

	authURL := cfg.endpoint("auth") + "?" + url.Values{
		"response_type":         {"code"},
		"client_id":             {cfg.ClientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {strings.Join(cfg.Scopes, " ")},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}.Encode()

	type result struct {
		code string
		err  error
	}
	results := make(chan result, 1)

	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/callback" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		if e := q.Get("error"); e != "" {
			writeBrowserPage(w, "Sign-in failed", q.Get("error_description"))
			results <- result{err: fmt.Errorf("provider reported %q", e)}
			return
		}
		// A mismatched state means this callback didn't originate from
		// the request we made — the exact case CSRF protection exists
		// for, so refuse the code rather than redeeming it.
		if q.Get("state") != state {
			writeBrowserPage(w, "Sign-in failed", "The sign-in response didn't match this request.")
			results <- result{err: fmt.Errorf("state mismatch")}
			return
		}
		code := q.Get("code")
		if code == "" {
			writeBrowserPage(w, "Sign-in failed", "No authorization code was returned.")
			results <- result{err: fmt.Errorf("no authorization code in callback")}
			return
		}
		go activateTerminal() // best-effort; osascript's latency shouldn't hold up the response
		writeSuccessPage(w, cfg.SuccessRedirect)
		results <- result{code: code}
	})}
	go server.Serve(listener)
	defer server.Close()

	notify(authURL)
	_ = openBrowser(authURL) // best-effort: the URL was printed either way

	select {
	case <-ctx.Done():
		return "", fmt.Errorf("timed out waiting for sign-in to finish")
	case res := <-results:
		if res.err != nil {
			return "", res.err
		}
		return exchangeCode(ctx, cfg, res.code, redirectURI, verifier)
	}
}

func exchangeCode(ctx context.Context, cfg Config, code, redirectURI, verifier string) (string, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {cfg.ClientID},
		"code_verifier": {verifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.endpoint("token"),
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("exchanging the authorization code: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned %s", resp.Status)
	}

	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", fmt.Errorf("parsing the token response: %w", err)
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("the token response contained no access token")
	}
	return tok.AccessToken, nil
}

// CreateAPIKey trades a short-lived access token for a long-lived API key,
// which is what SetFree actually stores and sends on every launch. baseURL
// is the auth service's versioned API root, e.g.
// "https://auth.example.com/v1".
func CreateAPIKey(ctx context.Context, baseURL, accessToken, name string) (string, error) {
	payload, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		return "", err
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/api-keys/"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(payload)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting an API key: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("the API key endpoint returned %s", resp.Status)
	}

	var created struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		return "", fmt.Errorf("parsing the API key response: %w", err)
	}
	if created.Key == "" {
		return "", fmt.Errorf("the API key response contained no key")
	}
	return created.Key, nil
}

// pkcePair returns a fresh code verifier and its S256 challenge.
func pkcePair() (verifier, challenge string, err error) {
	verifier, err = randomString(32)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// openBrowser launches the system browser. Failure is not fatal: callers
// print the URL too, so the user can always open it themselves. It's a
// variable so tests can stub it out — otherwise running them would open
// real browser windows.
var openBrowser = func(target string) error {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler"}
	default:
		cmd = "xdg-open"
	}
	return exec.Command(cmd, append(args, target)...).Start()
}

// terminalBundleIDs maps a terminal emulator's TERM_PROGRAM value to its
// macOS application bundle ID, for activateTerminal. Bundle IDs survive an
// app being renamed; matching on the app's display name wouldn't.
var terminalBundleIDs = map[string]string{
	"Apple_Terminal": "com.apple.Terminal",
	"iTerm.app":      "com.googlecode.iterm2",
	"WezTerm":        "com.github.wez.wezterm",
	"Hyper":          "co.zeit.hyper",
	"WarpTerminal":   "dev.warp.Warp-Stable",
	"vscode":         "com.microsoft.VSCode",
}

// activateTerminal makes a best-effort attempt to bring the terminal that
// launched SetFree back to the foreground once sign-in completes, so the
// user isn't left looking at the browser tab. It only knows how to do this
// on macOS, and only for a terminal it recognizes via TERM_PROGRAM — an
// unset or unrecognized value means it does nothing rather than guess and
// activate the wrong application.
//
// This activates the whole app, not the specific window or tab that ran
// setfree — the best a single osascript call can do without much deeper,
// per-terminal window scripting. Good enough when only one window is open,
// which is the common case for a fresh sign-in.
var activateTerminal = func() {
	if runtime.GOOS != "darwin" {
		return
	}
	bundleID, ok := terminalBundleID(os.Getenv("TERM_PROGRAM"))
	if !ok {
		return
	}
	exec.Command("osascript", "-e", fmt.Sprintf(`tell application id %q to activate`, bundleID)).Run()
}

func terminalBundleID(termProgram string) (string, bool) {
	id, ok := terminalBundleIDs[termProgram]
	return id, ok
}

func writeBrowserPage(w http.ResponseWriter, title, detail string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>%s</title>
<style>body{font:16px system-ui,sans-serif;display:grid;place-items:center;height:100vh;margin:0;background:#0f0f10;color:#eee}
div{text-align:center}p{color:#999}</style>
<div><h1>%s</h1><p>%s</p></div>`, title, title, detail)
}

// writeSuccessPage renders the page shown right after sign-in completes.
// With no redirect configured it's the plain "you can close this tab"
// message; with one, it also navigates the browser there after a brief
// pause, so a provider with its own web console (MindsHub's, say) lands
// the user there instead of leaving them on a bare confirmation page.
func writeSuccessPage(w http.ResponseWriter, redirect string) {
	if redirect == "" {
		writeBrowserPage(w, "You're signed in", "You can close this tab and return to your terminal.")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>You're signed in</title>
<meta http-equiv="refresh" content="1;url=%s">
<style>body{font:16px system-ui,sans-serif;display:grid;place-items:center;height:100vh;margin:0;background:#0f0f10;color:#eee}
div{text-align:center}p{color:#999}</style>
<div><h1>You're signed in</h1><p>Taking you to the console&hellip;</p></div>
<script>setTimeout(function(){location.replace(%q)},900)</script>`, redirect, redirect)
}

// DefaultTimeout bounds a whole sign-in, including time spent waiting on
// the user in the browser.
const DefaultTimeout = 3 * time.Minute
