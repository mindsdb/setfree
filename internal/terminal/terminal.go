// Package terminal provides the small set of terminal primitives SetFree's
// UI needs: TTY detection, NO_COLOR-aware styling, and hidden password
// input. It deliberately avoids pulling in a TUI framework.
package terminal

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// IsTTY reports whether f is an interactive terminal.
func IsTTY(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// Colors controls whether output should be colorized: only when stdout is a
// TTY and the user hasn't set NO_COLOR (https://no-color.org).
type Colors struct {
	Enabled bool
}

// Detect returns the color policy for the current process.
func Detect() Colors {
	if os.Getenv("NO_COLOR") != "" {
		return Colors{Enabled: false}
	}
	return Colors{Enabled: IsTTY(os.Stdout)}
}

const (
	ansiReset = "\x1b[0m"
	ansiDim   = "\x1b[2m"
	ansiBold  = "\x1b[1m"
	ansiGreen = "\x1b[32m"
	ansiRed   = "\x1b[31m"

	ansiHideCursor = "\x1b[?25l"
	ansiShowCursor = "\x1b[?25h"
)

func (c Colors) wrap(code, s string) string {
	if !c.Enabled {
		return s
	}
	return code + s + ansiReset
}

func (c Colors) Dim(s string) string   { return c.wrap(ansiDim, s) }
func (c Colors) Bold(s string) string  { return c.wrap(ansiBold, s) }
func (c Colors) Green(s string) string { return c.wrap(ansiGreen, s) }
func (c Colors) Red(s string) string   { return c.wrap(ansiRed, s) }

// Check and Cross are the status glyphs used across SetFree's UI. They're
// plain ASCII-adjacent Unicode that renders correctly in every terminal
// SetFree targets, with no locale detection required.
const (
	Check = "✓" // ✓
	Dash  = "–" // –
)

// Reader reads interactive input from a terminal: plain lines and, for
// secrets, input with local echo disabled.
type Reader struct {
	in  *os.File
	buf *bufio.Reader
}

// NewReader wraps in (typically os.Stdin) for line and password reads.
func NewReader(in *os.File) *Reader {
	return &Reader{in: in, buf: bufio.NewReader(in)}
}

// ReadLine reads a single line, with the trailing newline trimmed. It
// returns io.EOF if the input is closed before any data arrives (e.g. Ctrl+D
// or a closed pipe), so callers can distinguish that from an empty line.
func (r *Reader) ReadLine() (string, error) {
	line, err := r.buf.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" && err == io.EOF {
		return "", io.EOF
	}
	return line, nil
}

// ReadPassword reads a line with local terminal echo disabled, for secrets.
// It requires in to be a real terminal.
func (r *Reader) ReadPassword() (string, error) {
	fd := int(r.in.Fd())
	data, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stdout)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
