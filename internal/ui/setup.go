package ui

import (
	"errors"
	"fmt"
	"io"

	"github.com/mindsdb/setfree/internal/config"
	"github.com/mindsdb/setfree/internal/gateway"
	"github.com/mindsdb/setfree/internal/terminal"
)

// LineReader reads one line of plain input.
type LineReader interface {
	ReadLine() (string, error)
}

// PasswordReader reads one line of input with local echo disabled.
type PasswordReader interface {
	ReadPassword() (string, error)
}

// ErrSetupCancelled is returned when the user aborts setup (Ctrl+D/EOF).
var ErrSetupCancelled = errors.New("setup cancelled")

// RunSetup walks the user through connecting their first gateway. It
// intentionally does not attempt to "test the connection": SetFree can't
// know what a successful response looks like for an arbitrary gateway, and
// a fake-looking check would be worse than none. It only validates that the
// base URL is well-formed.
func RunSetup(w io.Writer, c terminal.Colors, lines LineReader, passwords PasswordReader) (baseURL, apiKey string, provider config.Provider, err error) {
	fmt.Fprintln(w, Robot)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Welcome to SetFree.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "No LLM gateway is configured yet. Let's connect one.")
	fmt.Fprintln(w)

	provider, err = PromptProvider(w, lines, c)
	if err != nil {
		return "", "", config.Provider{}, err
	}

	baseURL, apiKey, err = ConnectProvider(w, c, provider, lines, passwords)
	if err != nil {
		return "", "", config.Provider{}, err
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "%s Settings saved\n", c.Green(terminal.Check))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Saved as your default gateway.")
	fmt.Fprintln(w)

	return baseURL, apiKey, provider, nil
}

// PromptBaseURL prompts for and validates a base URL, retrying on invalid
// input until one is accepted or the user cancels (Ctrl+D/EOF).
func PromptBaseURL(w io.Writer, lines LineReader) (string, error) {
	for {
		fmt.Fprint(w, "Base URL\n> ")
		raw, err := lines.ReadLine()
		if err != nil {
			return "", ErrSetupCancelled
		}
		valid, verr := gateway.ValidateBaseURL(raw)
		if verr == nil {
			return valid, nil
		}
		fmt.Fprintf(w, "  %s\n\n", verr)
	}
}

// PromptAPIKey prompts for a new API key with echo disabled.
func PromptAPIKey(w io.Writer, passwords PasswordReader) (string, error) {
	fmt.Fprint(w, "API key\n> ")
	key, err := passwords.ReadPassword()
	if err != nil {
		return "", ErrSetupCancelled
	}
	return key, nil
}
