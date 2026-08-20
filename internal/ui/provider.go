package ui

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/mindsdb/setfree/internal/auth"
	"github.com/mindsdb/setfree/internal/config"
	"github.com/mindsdb/setfree/internal/terminal"
)

// PromptProvider asks which gateway to connect, as an arrow-key list when
// lines is backed by a real terminal (it satisfies terminal.Selector) and a
// numbered prompt otherwise (a pipe, a CI job, a test double). Both paths
// accept the same digits, so scripted input works either way.
func PromptProvider(w io.Writer, lines LineReader, c terminal.Colors) (config.Provider, error) {
	providers := config.Providers()
	fmt.Fprintln(w, "Which gateway do you want to use?")
	fmt.Fprintln(w)

	idx, err := pickFromList(w, lines, c, providerChoices(providers))
	if err != nil {
		return config.Provider{}, err
	}
	return providers[idx], nil
}

const resetChoiceLabel = "Reset configuration"

// PromptProviderOrReset is the entry point for `setfree config`: there's no
// settings menu above it — choosing a gateway to (re)connect and wiping
// SetFree's configuration entirely both live in the same list.
func PromptProviderOrReset(w io.Writer, lines LineReader, c terminal.Colors) (provider config.Provider, reset bool, err error) {
	providers := config.Providers()
	fmt.Fprintln(w, "Which gateway do you want to use?")
	fmt.Fprintln(w)

	choices := providerChoices(providers)
	choices = append(choices, choice{
		terminal.Choice{Label: resetChoiceLabel, Detail: "remove your saved gateway and API key"},
		[]string{"reset", resetChoiceLabel},
	})

	idx, err := pickFromList(w, lines, c, choices)
	if err != nil {
		return config.Provider{}, false, err
	}
	if idx == len(providers) {
		return config.Provider{}, true, nil
	}
	return providers[idx], false, nil
}

// choice pairs what a picker row displays with the extra strings (a
// provider's key, its full name) that also match it in the numbered
// fallback — so typing "litellm" works the same as typing its number.
type choice struct {
	terminal.Choice
	aliases []string
}

func providerChoices(providers []config.Provider) []choice {
	choices := make([]choice, len(providers))
	for i, p := range providers {
		choices[i] = choice{
			terminal.Choice{Label: p.DisplayName, Detail: p.Detail},
			[]string{p.Key, p.DisplayName},
		}
	}
	return choices
}

// pickFromList drives an arrow-key picker over choices when lines is backed
// by a real terminal (it satisfies terminal.Selector), falling back to a
// numbered prompt otherwise.
func pickFromList(w io.Writer, lines LineReader, c terminal.Colors, choices []choice) (int, error) {
	if selector, ok := lines.(terminal.Selector); ok {
		rows := make([]terminal.Choice, len(choices))
		for i, ch := range choices {
			rows[i] = ch.Choice
		}
		idx, err := selector.Select(w, c, rows, 0)
		switch {
		case err == nil:
			fmt.Fprintln(w)
			return idx, nil
		case err == terminal.ErrSelectCancelled:
			return 0, ErrSetupCancelled
		case err != terminal.ErrNotInteractive:
			return 0, err
		}
		// ErrNotInteractive: fall through to the numbered prompt below.
	}

	for i, ch := range choices {
		if ch.Detail != "" {
			fmt.Fprintf(w, "  %d. %s  (%s)\n", i+1, ch.Label, ch.Detail)
		} else {
			fmt.Fprintf(w, "  %d. %s\n", i+1, ch.Label)
		}
	}
	fmt.Fprintln(w)
	for {
		fmt.Fprint(w, "> ")
		raw, err := lines.ReadLine()
		if err != nil {
			return 0, ErrSetupCancelled
		}
		raw = strings.TrimSpace(raw)
		if n, convErr := strconv.Atoi(raw); convErr == nil && n >= 1 && n <= len(choices) {
			fmt.Fprintln(w)
			return n - 1, nil
		}
		for i, ch := range choices {
			for _, alias := range ch.aliases {
				if strings.EqualFold(alias, raw) {
					fmt.Fprintln(w)
					return i, nil
				}
			}
		}
		fmt.Fprintf(w, "  Enter a number from 1-%d.\n", len(choices))
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
