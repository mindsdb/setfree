package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mindsdb/setfree/internal/terminal"
)

func TestConnectionSuccess_NamesTheGateway(t *testing.T) {
	var buf bytes.Buffer
	ConnectionSuccess(&buf, terminal.Colors{}, "MindsHub", []string{"claude"})
	if !strings.Contains(buf.String(), "MindsHub gateway successfully connected") {
		t.Errorf("output = %q", buf.String())
	}
}

func TestConnectionSuccess_OneInstalledCLI(t *testing.T) {
	var buf bytes.Buffer
	ConnectionSuccess(&buf, terminal.Colors{}, "MindsHub", []string{"claude"})
	out := buf.String()
	if !strings.Contains(out, "setfree claude") {
		t.Errorf("expected a suggestion to run setfree claude:\n%s", out)
	}
	if strings.Contains(out, "setfree codex") {
		t.Errorf("codex isn't installed in this case, shouldn't be suggested:\n%s", out)
	}
}

func TestConnectionSuccess_MultipleInstalledCLIs(t *testing.T) {
	var buf bytes.Buffer
	ConnectionSuccess(&buf, terminal.Colors{}, "MindsHub", []string{"claude", "codex"})
	out := buf.String()
	if !strings.Contains(out, "setfree claude") || !strings.Contains(out, "setfree codex") {
		t.Errorf("expected both installed CLIs to be suggested:\n%s", out)
	}
}

// With nothing installed, the message must still give the user something
// concrete to do rather than leaving them with just a success checkmark.
func TestConnectionSuccess_NoneInstalled(t *testing.T) {
	var buf bytes.Buffer
	ConnectionSuccess(&buf, terminal.Colors{}, "MindsHub", nil)
	out := buf.String()
	if !strings.Contains(out, "setfree claude") || !strings.Contains(out, "setfree codex") {
		t.Errorf("expected both options to be suggested once something is installed:\n%s", out)
	}
	if !strings.Contains(out, "successfully connected") {
		t.Errorf("expected the success confirmation regardless:\n%s", out)
	}
}
