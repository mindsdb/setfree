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
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
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

// successRedirectDelaySeconds is how long the success page waits before
// navigating the browser to SuccessRedirect. Long enough that the "switch
// back to your terminal" message actually gets read before the tab moves
// out from under the user.
const successRedirectDelaySeconds = 10

// successPageTemplate is the post-sign-in page. Placeholders ({{URL}},
// {{HOST}}, {{SECONDS}}) are filled by writeSuccessPage via a string
// replacer rather than fmt verbs — with this much CSS, every literal "%"
// would otherwise need escaping and one missed verb silently corrupts the
// page. The redirect URL reaches the script through a body data attribute,
// so it only ever needs HTML-attribute escaping, never JS-string escaping.
const successPageTemplate = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>You’re signed in</title>
<meta http-equiv="refresh" content="{{SECONDS}};url={{URL}}">
<style>
:root{color-scheme:dark}
*{box-sizing:border-box}
body{margin:0;min-height:100vh;display:grid;place-items:center;font:16px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI",system-ui,sans-serif;color:#e7e9ee;background:#08090b;background-image:radial-gradient(ellipse 90% 55% at 50% -15%,rgba(56,189,248,.14),transparent 70%),radial-gradient(ellipse 60% 40% at 50% 115%,rgba(52,211,153,.07),transparent 70%)}
main{width:min(25rem,calc(100vw - 2.5rem));padding:2.75rem 2.25rem 2rem;text-align:center;border-radius:20px;border:1px solid rgba(255,255,255,.09);background:linear-gradient(180deg,rgba(255,255,255,.055),rgba(255,255,255,.015));box-shadow:0 24px 70px rgba(0,0,0,.55),inset 0 1px 0 rgba(255,255,255,.06);animation:rise .6s cubic-bezier(.22,1,.36,1) both}
.check{width:76px;height:76px;margin-bottom:1.5rem;filter:drop-shadow(0 0 18px rgba(52,211,153,.35))}
.check circle,.check path{fill:none;stroke:#34d399;stroke-width:3;stroke-linecap:round;stroke-linejoin:round}
.check circle{stroke-dasharray:151;animation:draw .7s cubic-bezier(.65,0,.45,1) .1s both}
.check path{stroke-dasharray:36;animation:draw .35s cubic-bezier(.65,0,.45,1) .75s both}
h1{margin:0 0 .6rem;font-size:1.45rem;font-weight:650;letter-spacing:-.01em}
p{margin:.4rem 0;color:#8b93a3}
.lead{color:#c8cdd8}
kbd{font:.85em ui-monospace,SFMono-Regular,Menlo,monospace;color:#c8cdd8;background:rgba(255,255,255,.07);border:1px solid rgba(255,255,255,.1);border-bottom-width:2px;border-radius:6px;padding:.1em .45em}
hr{border:0;height:1px;margin:1.4rem 0;background:linear-gradient(90deg,transparent,rgba(255,255,255,.12),transparent)}
.manage{font-size:.92rem}
a{color:#7dd3fc;text-decoration:none}
a:hover{text-decoration:underline}
.cta{display:inline-flex;align-items:center;gap:.6rem;margin-top:1.1rem;padding:.6rem 1.15rem;border-radius:999px;font-weight:550;color:#08090b;background:linear-gradient(180deg,#5eead4,#34d399);box-shadow:0 6px 20px rgba(52,211,153,.28),inset 0 1px 0 rgba(255,255,255,.4);transition:transform .15s ease,box-shadow .15s ease}
.cta:hover{text-decoration:none;transform:translateY(-1px);box-shadow:0 10px 26px rgba(52,211,153,.36),inset 0 1px 0 rgba(255,255,255,.4)}
.countdown{display:flex;align-items:center;justify-content:center;gap:.5rem;margin-top:1rem;font-size:.85rem;color:#6d7585}
.ring{width:18px;height:18px;transform:rotate(-90deg)}
.ring .track{fill:none;stroke:rgba(255,255,255,.12);stroke-width:2.5}
.ring .bar{fill:none;stroke:#7dd3fc;stroke-width:2.5;stroke-linecap:round;stroke-dasharray:50.27;animation:drain {{SECONDS}}s linear both}
#count{color:#c8cdd8;font-variant-numeric:tabular-nums}
.sig{margin-top:1.6rem;font:.8rem ui-monospace,SFMono-Regular,Menlo,monospace;color:#525a68;letter-spacing:.02em}
@keyframes rise{from{opacity:0;transform:translateY(14px) scale(.985)}}
@keyframes draw{from{stroke-dashoffset:151}to{stroke-dashoffset:0}}
@keyframes drain{from{stroke-dashoffset:0}to{stroke-dashoffset:50.27}}
@media (prefers-reduced-motion:reduce){main,.check circle,.check path,.ring .bar{animation:none}}
</style></head>
<body data-redirect="{{URL}}">
<main>
<svg class="check" viewBox="0 0 52 52" aria-hidden="true"><circle cx="26" cy="26" r="24"/><path d="M15.5 27.2l7.3 7.3L36.5 20"/></svg>
<h1>You’re signed in</h1>
<p class="lead">You can switch back to your terminal now — SetFree will pick up from there.</p>
<hr>
<p class="manage">Manage your plan and credits anytime at<br><a href="{{URL}}">{{HOST}}</a></p>
<a class="cta" href="{{URL}}">Open the console →</a>
<div class="countdown"><svg class="ring" viewBox="0 0 20 20" aria-hidden="true"><circle class="track" cx="10" cy="10" r="8"/><circle class="bar" cx="10" cy="10" r="8"/></svg><span>Redirecting in <span id="count">{{SECONDS}}</span>s</span></div>
<p class="sig">[•‿•]&ensp;SetFree</p>
</main>
<script>
(function () {
  var n = {{SECONDS}}, el = document.getElementById('count');
  var to = document.body.getAttribute('data-redirect');
  var t = setInterval(function () {
    n--;
    if (el) el.textContent = n;
    if (n <= 0) { clearInterval(t); location.replace(to); }
  }, 1000);
})();
</script>
</body></html>`

// writeSuccessPage renders the page shown right after sign-in completes.
// With no redirect configured it's the plain "you can close this tab"
// message; with one, it makes clear the user can already go back to their
// terminal, and navigates the browser to redirect after a visible
// countdown — for MindsHub, its console's billing page, so credits/plan
// management is a click away instead of leaving the user on a bare
// confirmation page.
func writeSuccessPage(w http.ResponseWriter, redirect string) {
	if redirect == "" {
		writeBrowserPage(w, "You're signed in", "You can close this tab and return to your terminal.")
		return
	}
	host := redirect
	if u, err := url.Parse(redirect); err == nil && u.Host != "" {
		host = u.Host
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(w, strings.NewReplacer(
		"{{URL}}", html.EscapeString(redirect),
		"{{HOST}}", html.EscapeString(host),
		"{{SECONDS}}", strconv.Itoa(successRedirectDelaySeconds),
	).Replace(successPageTemplate))
}

// DefaultTimeout bounds a whole sign-in, including time spent waiting on
// the user in the browser.
const DefaultTimeout = 3 * time.Minute
