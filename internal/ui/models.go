package ui

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/mindsdb/setfree/internal/terminal"
)

// NativeModelDiscovery reports that the gateway serves displayName's own
// models directly, so its built-in model picker can be trusted.
func NativeModelDiscovery(c terminal.Colors, displayName string) string {
	return fmt.Sprintf("%s This gateway serves %s models directly. Its own model picker will work as usual.\n", c.Green(terminal.Check), displayName)
}

// PromptModelChoice shows models found on a gateway that doesn't look
// native to displayName's usual provider, and asks which one to pin.
// Blank input skips pinning one at all, leaving the CLI's own default in
// place. It loops on unrecognized input rather than guessing.
func PromptModelChoice(w io.Writer, lines LineReader, displayName string, models []string) (string, error) {
	fmt.Fprintf(w, "This gateway doesn't look like a native %s endpoint. It lists these models:\n\n", displayName)
	for i, m := range models {
		fmt.Fprintf(w, "  %d. %s\n", i+1, m)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Which one should %s use? Enter a number, or press enter to skip.\n> ", displayName)

	for {
		line, err := lines.ReadLine()
		if err != nil {
			return "", ErrSetupCancelled
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return "", nil
		}
		if n, convErr := strconv.Atoi(line); convErr == nil && n >= 1 && n <= len(models) {
			return models[n-1], nil
		}
		for _, m := range models {
			if strings.EqualFold(m, line) {
				return m, nil
			}
		}
		fmt.Fprintf(w, "  Enter a number from 1-%d, the exact model name, or press enter to skip.\n> ", len(models))
	}
}
