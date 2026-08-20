package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// stubBrowser replaces the real browser launch for the duration of a test,
// optionally handing the authorize URL to visit back to the caller.
func stubBrowser(t *testing.T, visit func(authURL string)) {
	t.Helper()
	orig := openBrowser
	openBrowser = func(target string) error {
		if visit != nil {
			go visit(target)
		}
		return nil
	}
	t.Cleanup(func() { openBrowser = orig })
}

// fakeIDP stands in for Keycloak: it records the authorize parameters and
// redeems the code only if the PKCE verifier actually matches the
// challenge that was sent.
type fakeIDP struct {
	server    *httptest.Server
	challenge string
	verifier  string // captured at token time
	issued    string
}

func newFakeIDP(t *testing.T, accessToken string) *fakeIDP {
	t.Helper()
	idp := &fakeIDP{issued: accessToken}
	mux := http.NewServeMux()
	mux.HandleFunc("/realms/test/protocol/openid-connect/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		idp.verifier = r.Form.Get("code_verifier")
		sum := sha256.Sum256([]byte(idp.verifier))
		if base64.RawURLEncoding.EncodeToString(sum[:]) != idp.challenge {
			http.Error(w, "PKCE verification failed", http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"access_token": idp.issued})
	})
	idp.server = httptest.NewServer(mux)
	t.Cleanup(idp.server.Close)
	return idp
}

func (f *fakeIDP) config() Config {
	return Config{Issuer: f.server.URL, Realm: "test", ClientID: "cli", Scopes: []string{"openid"}}
}

func TestLogin_HappyPath(t *testing.T) {
	idp := newFakeIDP(t, "the-access-token")

	stubBrowser(t, func(authURL string) {
		u, err := url.Parse(authURL)
		if err != nil {
			t.Errorf("authorize URL is unparseable: %v", err)
			return
		}
		q := u.Query()
		idp.challenge = q.Get("code_challenge")

		if got := q.Get("code_challenge_method"); got != "S256" {
			t.Errorf("code_challenge_method = %q, want S256", got)
		}
		if q.Get("response_type") != "code" {
			t.Errorf("response_type = %q", q.Get("response_type"))
		}
		redirect := q.Get("redirect_uri")
		if !strings.HasPrefix(redirect, "http://127.0.0.1:") {
			t.Errorf("redirect_uri = %q, want a 127.0.0.1 loopback", redirect)
		}

		// Play the part of the browser returning to the callback.
		http.Get(redirect + "?code=abc123&state=" + url.QueryEscape(q.Get("state")))
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	token, err := Login(ctx, idp.config(), func(string) {})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if token != "the-access-token" {
		t.Errorf("token = %q", token)
	}
	if idp.verifier == "" {
		t.Error("no code_verifier reached the token endpoint")
	}
}

// A callback carrying the wrong state is exactly the CSRF case the state
// parameter exists to catch, so the code must not be redeemed.
func TestLogin_RejectsStateMismatch(t *testing.T) {
	idp := newFakeIDP(t, "should-never-be-issued")

	stubBrowser(t, func(authURL string) {
		u, _ := url.Parse(authURL)
		redirect := u.Query().Get("redirect_uri")
		http.Get(redirect + "?code=abc123&state=not-the-state-we-sent")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := Login(ctx, idp.config(), func(string) {}); err == nil {
		t.Fatal("expected a state mismatch to fail the sign-in")
	}
}

func TestLogin_ProviderReturnedError(t *testing.T) {
	idp := newFakeIDP(t, "unused")

	stubBrowser(t, func(authURL string) {
		u, _ := url.Parse(authURL)
		redirect := u.Query().Get("redirect_uri")
		http.Get(redirect + "?error=access_denied&error_description=nope")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := Login(ctx, idp.config(), func(string) {})
	if err == nil || !strings.Contains(err.Error(), "access_denied") {
		t.Fatalf("err = %v, want it to name the provider's error", err)
	}
}

func TestLogin_ContextCancelledWhileWaiting(t *testing.T) {
	idp := newFakeIDP(t, "unused")
	stubBrowser(t, nil) // nobody ever visits the callback

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	if _, err := Login(ctx, idp.config(), func(string) {}); err == nil {
		t.Fatal("expected Login to give up when the context expires")
	}
}

func TestPKCEPair_ChallengeIsS256OfVerifier(t *testing.T) {
	verifier, challenge, err := pkcePair()
	if err != nil {
		t.Fatalf("pkcePair: %v", err)
	}
	sum := sha256.Sum256([]byte(verifier))
	if want := base64.RawURLEncoding.EncodeToString(sum[:]); challenge != want {
		t.Errorf("challenge = %q, want %q", challenge, want)
	}
	// RFC 7636 requires 43-128 characters.
	if len(verifier) < 43 || len(verifier) > 128 {
		t.Errorf("verifier length %d is outside the RFC 7636 range", len(verifier))
	}
	if strings.ContainsAny(challenge, "+/=") {
		t.Errorf("challenge %q must be base64url with no padding", challenge)
	}
}

func TestPKCEPair_IsFreshEachTime(t *testing.T) {
	a, _, _ := pkcePair()
	b, _, _ := pkcePair()
	if a == b {
		t.Error("two sign-ins must not share a code verifier")
	}
}

func TestCreateAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api-keys/" {
			t.Errorf("path = %q, want /api-keys/", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer jwt-token" {
			t.Errorf("Authorization = %q", got)
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["name"] == "" {
			t.Error("expected the request to name the key")
		}
		json.NewEncoder(w).Encode(map[string]string{"key": "mdb_abc123"})
	}))
	defer srv.Close()

	key, err := CreateAPIKey(context.Background(), srv.URL, "jwt-token", "setfree")
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if key != "mdb_abc123" {
		t.Errorf("key = %q", key)
	}
}

func TestCreateAPIKey_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	if _, err := CreateAPIKey(context.Background(), srv.URL, "jwt", "setfree"); err == nil {
		t.Fatal("expected a non-2xx response to be an error")
	}
}
