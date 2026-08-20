package ui

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/mindsdb/setfree/internal/auth"
	"github.com/mindsdb/setfree/internal/config"
	"github.com/mindsdb/setfree/internal/terminal"
)

// scriptedReader replays canned lines, standing in for a user typing.
type scriptedReader struct {
	lines []string
	i     int
}

func (s *scriptedReader) ReadLine() (string, error) {
	if s.i >= len(s.lines) {
		return "", errors.New("no more input")
	}
	line := s.lines[s.i]
	s.i++
	return line, nil
}

func (s *scriptedReader) ReadPassword() (string, error) { return s.ReadLine() }

// stubSSO replaces the browser flow and key exchange for a test.
func stubSSO(t *testing.T, token, key string, loginErr, keyErr error) {
	t.Helper()
	origLogin, origCreate := Login, CreateAPIKey
	Login = func(ctx context.Context, cfg auth.Config, notify func(string)) (string, error) {
		notify("https://auth.example.com/authorize?...")
		return token, loginErr
	}
	CreateAPIKey = func(ctx context.Context, base, tok, name string) (string, error) {
		return key, keyErr
	}
	t.Cleanup(func() { Login, CreateAPIKey = origLogin, origCreate })
}

// With no terminal to drive, the picker degrades to a numbered prompt
// rather than failing outright — first-time setup has to work over a pipe.
func TestPromptProvider_FallsBackToNumberedPrompt(t *testing.T) {
	var buf bytes.Buffer
	r := &scriptedReader{lines: []string{"2"}}

	// os.Stdin here is not a TTY under `go test`, which is the case
	// being exercised.
	p, err := PromptProvider(&buf, os.Stdin, terminal.Colors{}, r)
	if err != nil {
		t.Fatalf("PromptProvider: %v", err)
	}
	if p.Key != "openrouter" {
		t.Errorf("picked %q, want openrouter", p.Key)
	}
	if !strings.Contains(buf.String(), "1. MindsHub") {
		t.Errorf("expected a numbered list in the fallback output:\n%s", buf.String())
	}
}

func TestPromptProvider_AcceptsNameNotJustNumber(t *testing.T) {
	var buf bytes.Buffer
	r := &scriptedReader{lines: []string{"litellm"}}

	p, err := PromptProvider(&buf, os.Stdin, terminal.Colors{}, r)
	if err != nil {
		t.Fatalf("PromptProvider: %v", err)
	}
	if p.Key != "litellm" {
		t.Errorf("picked %q, want litellm", p.Key)
	}
}

func TestPromptProvider_RetriesOnGarbage(t *testing.T) {
	var buf bytes.Buffer
	r := &scriptedReader{lines: []string{"banana", "99", "4"}}

	p, err := PromptProvider(&buf, os.Stdin, terminal.Colors{}, r)
	if err != nil {
		t.Fatalf("PromptProvider: %v", err)
	}
	if p.Key != "custom" {
		t.Errorf("picked %q, want custom", p.Key)
	}
}

// The SSO provider has a fixed address, so setup should never ask for one.
func TestConnectProvider_SSOSkipsBaseURLAndStoresIssuedKey(t *testing.T) {
	stubSSO(t, "jwt", "mdb_issued", nil, nil)

	var buf bytes.Buffer
	p, _ := config.FindProvider("mindshub")
	r := &scriptedReader{} // any read would mean it wrongly prompted

	baseURL, key, err := ConnectProvider(&buf, terminal.Colors{}, p, r, r)
	if err != nil {
		t.Fatalf("ConnectProvider: %v", err)
	}
	if !strings.HasPrefix(baseURL, "https://api.") {
		t.Errorf("baseURL = %q", baseURL)
	}
	if key != "mdb_issued" {
		t.Errorf("key = %q, want the key issued by the exchange", key)
	}
}

// A failed sign-in must not dead-end the user: they can still paste a key.
func TestConnectProvider_FallsBackToPasteWhenSignInFails(t *testing.T) {
	stubSSO(t, "", "", errors.New("no browser here"), nil)

	var buf bytes.Buffer
	p, _ := config.FindProvider("mindshub")
	r := &scriptedReader{lines: []string{"mdb_pasted"}}

	_, key, err := ConnectProvider(&buf, terminal.Colors{}, p, r, r)
	if err != nil {
		t.Fatalf("ConnectProvider: %v", err)
	}
	if key != "mdb_pasted" {
		t.Errorf("key = %q, want the pasted key", key)
	}
	if !strings.Contains(buf.String(), "Paste an API key instead") {
		t.Errorf("expected the fallback to be explained:\n%s", buf.String())
	}
}

// Signing in but failing to mint a key is still a failure to connect, and
// must fall back rather than returning an empty credential.
func TestConnectProvider_FallsBackWhenKeyExchangeFails(t *testing.T) {
	stubSSO(t, "jwt", "", nil, errors.New("quota exceeded"))

	var buf bytes.Buffer
	p, _ := config.FindProvider("mindshub")
	r := &scriptedReader{lines: []string{"mdb_pasted"}}

	_, key, err := ConnectProvider(&buf, terminal.Colors{}, p, r, r)
	if err != nil {
		t.Fatalf("ConnectProvider: %v", err)
	}
	if key != "mdb_pasted" {
		t.Errorf("key = %q", key)
	}
}

func TestConnectProvider_SelfHostedPromptsForAddress(t *testing.T) {
	var buf bytes.Buffer
	p, _ := config.FindProvider("litellm")
	r := &scriptedReader{lines: []string{"https://litellm.internal:4000", "sk-local"}}

	baseURL, key, err := ConnectProvider(&buf, terminal.Colors{}, p, r, r)
	if err != nil {
		t.Fatalf("ConnectProvider: %v", err)
	}
	if baseURL != "https://litellm.internal:4000" {
		t.Errorf("baseURL = %q", baseURL)
	}
	if key != "sk-local" {
		t.Errorf("key = %q", key)
	}
}

// A hosted, non-SSO provider should confirm the address it's using and go
// straight to the key prompt.
func TestConnectProvider_HostedSkipsAddressPrompt(t *testing.T) {
	var buf bytes.Buffer
	p, _ := config.FindProvider("openrouter")
	r := &scriptedReader{lines: []string{"sk-or-v1-xyz"}}

	baseURL, key, err := ConnectProvider(&buf, terminal.Colors{}, p, r, r)
	if err != nil {
		t.Fatalf("ConnectProvider: %v", err)
	}
	if baseURL != "https://openrouter.ai/api/v1" {
		t.Errorf("baseURL = %q", baseURL)
	}
	if key != "sk-or-v1-xyz" {
		t.Errorf("key = %q", key)
	}
}
