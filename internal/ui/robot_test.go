package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// Every frame has to occupy the same block of screen, or redrawing in place
// leaves fragments of the previous frame behind.
func TestFrames_AllExactlyRobotHeight(t *testing.T) {
	for name, frames := range map[string][]string{"idle": idleFrames, "hatch": hatchFrames} {
		for i, f := range frames {
			if n := len(frameLines(f)); n != robotHeight {
				t.Errorf("%s frame %d has %d lines, want %d", name, i, n, robotHeight)
			}
		}
	}
}

// Idling shouldn't bob the robot up and down — only the deliberate jump in
// the hatch sequence moves it off its resting line.
func TestIdleFrames_FaceStaysOnOneLine(t *testing.T) {
	for i, f := range idleFrames {
		if !strings.Contains(frameLines(f)[2], "[") {
			t.Errorf("idle frame %d: face is not on line 2:\n%s", i, f)
		}
	}
}

func TestShowRobot_IdlesUntilTaskFinishesThenHatches(t *testing.T) {
	var buf bytes.Buffer
	slow := 500 * time.Millisecond // outlasts one idle cycle (5 * frameDelay)

	ShowRobot(&buf, true, func(status func(string)) {
		status("Updating SetFree...")
		time.Sleep(slow)
		status("✓ Updated to abc1234")
	})

	out := buf.String()
	if !strings.Contains(out, "Updating SetFree...") {
		t.Error("in-progress status was never rendered")
	}
	if !strings.Contains(out, "✓ Updated to abc1234") {
		t.Error("final status was never rendered")
	}
	if !strings.Contains(out, "[•‿•]") {
		t.Error("hatch sequence never played, so the task's completion never released it")
	}
	if n := strings.Count(out, "[^_^]"); n < 2 {
		t.Errorf("idle cycled %d time(s); expected it to keep looping while the task ran", n)
	}
}

func TestShowRobot_NilTaskStillHatches(t *testing.T) {
	var buf bytes.Buffer
	ShowRobot(&buf, true, nil)
	if !strings.Contains(buf.String(), "[•‿•]") {
		t.Error("expected the hatch sequence to play with no task to wait on")
	}
}

// Piped output must stay free of cursor-movement escapes, which would
// otherwise land as noise in a log file.
func TestShowRobot_StaticPathIsPlainText(t *testing.T) {
	var buf bytes.Buffer
	ShowRobot(&buf, false, func(status func(string)) {
		status("Updating SetFree...")
		status("✓ Updated to abc1234")
	})

	out := buf.String()
	if strings.Contains(out, "\x1b[") {
		t.Errorf("static path emitted escape codes: %q", out)
	}
	for _, want := range []string{"Updating SetFree...", "✓ Updated to abc1234", "[•‿•]"} {
		if !strings.Contains(out, want) {
			t.Errorf("static output is missing %q:\n%s", want, out)
		}
	}
}

// An empty status must not print a stray blank line on the static path.
func TestShowRobot_StaticPathSkipsEmptyStatus(t *testing.T) {
	var buf bytes.Buffer
	ShowRobot(&buf, false, func(status func(string)) { status("") })
	if got := buf.String(); got != Robot+"\n\n" {
		t.Errorf("output = %q, want just the resting robot", got)
	}
}
