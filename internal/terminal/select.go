package terminal

import (
	"errors"
	"fmt"
	"io"

	"golang.org/x/term"
)

// ErrNotInteractive means Select was asked to run somewhere it can't: the
// input isn't a terminal, so there are no keystrokes to read. Callers fall
// back to a numbered prompt.
var ErrNotInteractive = errors.New("not an interactive terminal")

// ErrSelectCancelled means the user backed out (Ctrl+C or q).
var ErrSelectCancelled = errors.New("selection cancelled")

// Choice is one row in a Select list.
type Choice struct {
	// Label is the primary text, shown highlighted when selected.
	Label string
	// Detail is optional dimmed text shown after the label.
	Detail string
}

// Selector is satisfied by anything that can drive an arrow-key list —
// namely *Reader. Callers that only have a LineReader (a test double, or
// any other line-oriented input) can type-assert for this to get the rich
// picker when it's available, and fall back to a plain prompt when it
// isn't.
type Selector interface {
	Select(out io.Writer, c Colors, choices []Choice, initial int) (int, error)
}

// Select renders an arrow-key-driven list and returns the chosen index.
//
// It puts the terminal in raw mode for the duration so keystrokes arrive
// unbuffered, and restores the previous state on every exit path. Up/down
// arrows and j/k move; enter picks; a digit picks that row directly; Ctrl+C
// or q cancels. Bare Escape is deliberately not a cancel key: telling it
// apart from the Escape that starts an arrow sequence needs a read timeout,
// and guessing wrong would swallow arrow presses.
//
// Keystrokes are read through r's own buffer rather than straight from the
// file descriptor. Reading the fd directly would miss anything the buffer
// already holds — a line the user typed ahead, or the rest of a piped
// script — which would otherwise look exactly like a premature EOF.
//
// This is intentionally the whole of SetFree's "TUI" — a redraw loop over
// one list, not a framework.
func (r *Reader) Select(out io.Writer, c Colors, choices []Choice, initial int) (int, error) {
	if len(choices) == 0 {
		return 0, errors.New("no choices to select from")
	}
	if !IsTTY(r.in) {
		return 0, ErrNotInteractive
	}

	fd := int(r.in.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return 0, ErrNotInteractive
	}
	defer term.Restore(fd, state)

	cur := initial
	if cur < 0 || cur >= len(choices) {
		cur = 0
	}

	fmt.Fprint(out, ansiHideCursor)
	defer fmt.Fprint(out, ansiShowCursor)

	drawn := false
	draw := func() {
		if drawn {
			fmt.Fprintf(out, "\x1b[%dF", len(choices))
		}
		drawn = true
		for i, ch := range choices {
			marker, label := "  ", ch.Label
			if i == cur {
				marker, label = c.Green("❯")+" ", c.Bold(ch.Label)
			}
			line := marker + label
			if ch.Detail != "" {
				line += "  " + c.Dim(ch.Detail)
			}
			// \r\n, not \n: raw mode disables the newline-to-CRLF
			// translation that would otherwise return the cursor to
			// column 0, leaving every line stair-stepped to the right.
			fmt.Fprintf(out, "%s\x1b[K\r\n", line)
		}
	}
	draw()

	for {
		b, err := r.buf.ReadByte()
		if err != nil {
			return 0, ErrSelectCancelled
		}

		switch {
		case b == 3, b == 'q': // Ctrl+C, q
			return 0, ErrSelectCancelled
		case b == '\r', b == '\n':
			return cur, nil
		case b == 'k':
			cur = (cur - 1 + len(choices)) % len(choices)
		case b == 'j':
			cur = (cur + 1) % len(choices)
		case b >= '1' && b <= '9':
			if i := int(b - '1'); i < len(choices) {
				return i, nil
			}
		case b == 27: // ESC: start of an arrow sequence
			if next, err := r.buf.ReadByte(); err != nil || next != '[' {
				continue
			}
			code, err := r.buf.ReadByte()
			if err != nil {
				return 0, ErrSelectCancelled
			}
			switch code {
			case 'A':
				cur = (cur - 1 + len(choices)) % len(choices)
			case 'B':
				cur = (cur + 1) % len(choices)
			default:
				continue
			}
		default:
			continue
		}
		draw()
	}
}
