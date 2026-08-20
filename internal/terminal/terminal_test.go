package terminal

import (
	"bufio"
	"io"
	"strings"
	"testing"
)

func TestColors_DisabledPassesThroughPlainText(t *testing.T) {
	c := Colors{Enabled: false}
	if c.Bold("x") != "x" || c.Dim("x") != "x" || c.Green("x") != "x" || c.Red("x") != "x" {
		t.Error("disabled Colors must not add ANSI codes")
	}
}

func TestColors_EnabledWrapsWithANSI(t *testing.T) {
	c := Colors{Enabled: true}
	got := c.Bold("x")
	if !strings.Contains(got, "x") || got == "x" {
		t.Errorf("Bold(%q) = %q, want ANSI-wrapped", "x", got)
	}
}

func TestDetect_NoColorEnvDisablesColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if Detect().Enabled {
		t.Error("NO_COLOR=1 must disable color regardless of TTY state")
	}
}

// fakeReadLiner lets ReadLine's line-splitting logic be exercised without a
// real terminal.
func newBufReader(s string) *bufio.Reader { return bufio.NewReader(strings.NewReader(s)) }

func TestReader_ReadLine(t *testing.T) {
	r := &Reader{buf: newBufReader("hello\nworld\r\n")}

	line, err := r.ReadLine()
	if err != nil || line != "hello" {
		t.Fatalf("ReadLine() = %q, %v", line, err)
	}
	line, err = r.ReadLine()
	if err != nil || line != "world" {
		t.Fatalf("ReadLine() = %q, %v", line, err)
	}
}

func TestReader_ReadLine_EOF(t *testing.T) {
	r := &Reader{buf: newBufReader("")}
	_, err := r.ReadLine()
	if err != io.EOF {
		t.Fatalf("ReadLine() err = %v, want io.EOF", err)
	}
}
