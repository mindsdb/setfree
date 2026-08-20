package ui

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/mindsdb/setfree/internal/auth"
	"github.com/mindsdb/setfree/internal/config"
	"github.com/mindsdb/setfree/internal/terminal"
)

// PromptProvider asks which gateway to connect, as an arrow-key list when
// stdin is a terminal and a numbered prompt otherwise (a pipe, a CI job).
// Both paths accept the same digits, so scripted input works either way.
func PromptProvider(w io.Writer, in *os.File, c terminal.Colors, lines LineReader) (config.Provider, error) {
	providers := config.Providers()

	fmt.Fprintln(w, "Which gateway do you want to use?")
	fmt.Fprintln(w)

	choices := make([]terminal.Choice, len(providers))
	for i, p := range providers {
		choices[i] = terminal.Choice{Label: p.DisplayName, Detail: p.Detail}
	}

	idx, err := terminal.Select(w, in, c, choices, 0)
	switch {
	case err == nil:
		fmt.Fprintln(w)
		return providers[idx], nil
	case err == terminal.ErrSelectCancelled:
		return config.Provider{}, ErrSetupCancelled
	case err != terminal.ErrNotInteractive:
		return config.Provider{}, err
	}

	// No terminal to drive: fall back to a plain numbered prompt.
	for i, p := range providers {
		fmt.Fprintf(w, "  %d. %s  (%s)\n", i+1, p.DisplayName, p.Detail)
	}
	fmt.Fprintln(w)
	for {
		fmt.Fprint(w, "> ")
		raw, err := lines.ReadLine()
		if err != nil {
			return config.Provider{}, ErrSetupCancelled
		}
		raw = strings.TrimSpace(raw)
		if n, convErr := strconv.Atoi(raw); convErr == nil && n >= 1 && n <= len(providers) {
			fmt.Fprintln(w)
			return providers[n-1], nil
		}
		for _, p := range providers {
			if strings.EqualFold(p.Key, raw) || strings.EqualFold(p.DisplayName, raw) {
				fmt.Fprintln(w)
				return p, nil
			}
		}
		fmt.Fprintf(w, "  Enter a number from 1-%d.\n", len(providers))
	}
}

// ConnectProvider takes a chosen preset through to a usable base URL and
// credential: prompting for an address when the provider is self-hosted,
// then either signing in through the browser or taking a pasted key.
func ConnectProvider(w io.Writer, c terminal.Colors, p config.Provider, lines LineReader, passwords PasswordReader) (baseURL, apiKey string, err error) {
	baseURL = p.BaseURL
	if p.NeedsBaseURL() {
		if p.Key == "litellm" {
			fmt.Fprintf(w, "%s\n\n", c.Dim("LiteLLM is self-hosted, so enter the address of your proxy."))
		}
		baseURL, err = PromptBaseURL(w, lines)
		if err != nil {
			return "", "", err
		}
		fmt.Fprintln(w)
	} else {
		fmt.Fprintf(w, "Gateway: %s\n\n", baseURL)
	}

	if p.SSO {
		key, ssoErr := signIn(w, c, p)
		if ssoErr == nil {
			return baseURL, key, nil
		}
		// Sign-in is the happy path, not the only path: a headless box,
		// a browser that won't open, or a provider hiccup all leave the
		// user perfectly able to paste a key they already have.
		fmt.Fprintf(w, "\n%s\n", c.Dim("Couldn't complete browser sign-in: "+ssoErr.Error()))
		fmt.Fprintf(w, "%s\n\n", c.Dim("Paste an API key instead."))
	}

	apiKey, err = PromptAPIKey(w, passwords)
	if err != nil {
		return "", "", err
	}
	return baseURL, apiKey, nil
}

// signIn runs the browser flow and trades the resulting session for a
// long-lived API key, which is all SetFree keeps.
func signIn(w io.Writer, c terminal.Colors, p config.Provider) (string, error) {
	if p.Key != "mindshub" {
		return "", fmt.Errorf("no sign-in configured for %s", p.DisplayName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), auth.DefaultTimeout)
	defer cancel()

	fmt.Fprintln(w, "Opening your browser to sign in...")
	token, err := Login(ctx, config.MindsHubOIDC(), func(url string) {
		fmt.Fprintf(w, "\n%s\n%s\n\n", c.Dim("If it doesn't open, visit:"), url)
	})
	if err != nil {
		return "", err
	}

	fmt.Fprintf(w, "%s Signed in\n", c.Green(terminal.Check))
	key, err := CreateAPIKey(ctx, config.MindsHubAuthAPI(), token, "setfree")
	if err != nil {
		return "", err
	}
	fmt.Fprintf(w, "%s API key issued\n", c.Green(terminal.Check))
	return key, nil
}

// Login and CreateAPIKey are indirected through variables so tests can
// substitute them without standing up an identity provider.
var (
	Login        = auth.Login
	CreateAPIKey = auth.CreateAPIKey
)
