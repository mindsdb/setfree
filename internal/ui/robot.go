package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// EnvNoAnimation, when set to any non-empty value, keeps the mascot static.
const EnvNoAnimation = "SETFREE_NO_ANIMATION"

// robotHeight is the fixed line count every frame is padded to, so the
// animation can redraw in place by moving the cursor up exactly this far.
const robotHeight = 5

// Robot is SetFree's tiny mascot at rest: hatched out of its shell, on its
// own legs, smiling. This is the animation's final frame, and the whole of
// it wherever animating isn't possible (piped output, SETFREE_NO_ANIMATION).
const Robot = `    [•‿•]
     |‖|
     / \`

// robotFrames tells the mascot's story in place: it starts sealed in a
// shell, rattles, cracks, bursts apart, sprouts legs, jumps, and lands
// smiling. Frames are padded to robotHeight lines by frameLines.
var robotFrames = []string{
	// Sealed in its shell.
	`

▖▖▖▖▖▖▖
▖[•_•]▖
▖▖▖▖▖▖▖`,
	// Something inside is moving.
	`

 ▖▖▖▖▖▖▖
 ▖[•_•]▖
 ▖▖▖▖▖▖▖`,
	// First cracks.
	`
  ▖ ▖ ▖
▖▖  ▖▖▖
▖[•_•]
 ▖▖ ▖▖▖`,
	// It bursts.
	` ▖  ▖   ▖
   ▖  ▖
 ▖ [•_•] ▖
   ▖   ▖
  ▖  ▖  ▖`,
	// Shell fragments fly outward; legs appear underneath.
	`▖         ▖

    [•_•]
     |‖|
▖    / \    ▖`,
	// Standing free.
	`


    [•_•]
     |‖|
     / \`,
	// A crouch...
	`


    [•_•]
     |‖|
     /_\`,
	// ...and a jump: the whole robot lifts one line, feet tucked.
	`

    [•_•]
     |‖|
     ^ ^
   ‿     ‿`,
	// Landing.
	`


    [•_•]
     |‖|
     / \`,
	// Pleased with itself.
	`


    [•‿•]
     |‖|
     / \`,
}

// frameDelay is how long each frame holds. Deliberately brisk: the whole
// sequence is under a second, so it reads as a flourish rather than
// something the user has to sit through before the screen appears.
const frameDelay = 85 * time.Millisecond

// ShowRobot draws the mascot to w. When animate is true it plays the
// hatching sequence in place; otherwise it prints the resting Robot once.
// Callers pass animate=false whenever output isn't an interactive terminal,
// since the cursor movement the animation relies on would otherwise land as
// escape-code noise in a pipe or log file.
func ShowRobot(w io.Writer, animate bool) {
	if !animate || os.Getenv(EnvNoAnimation) != "" {
		fmt.Fprintln(w, Robot)
		return
	}

	fmt.Fprint(w, ansiHideCursor)
	defer fmt.Fprint(w, ansiShowCursor)

	for i, frame := range robotFrames {
		if i > 0 {
			// Rewind over the frame just drawn so this one replaces it.
			fmt.Fprintf(w, "\x1b[%dF", robotHeight)
		}
		for _, line := range frameLines(frame) {
			fmt.Fprintf(w, "%s\x1b[K\n", line)
		}
		time.Sleep(frameDelay)
	}
}

// frameLines splits a frame into exactly robotHeight lines, padding short
// frames with blanks so every frame occupies the same block of screen.
func frameLines(frame string) []string {
	lines := strings.Split(strings.TrimPrefix(frame, "\n"), "\n")
	for len(lines) < robotHeight {
		lines = append(lines, "")
	}
	return lines[:robotHeight]
}

const (
	ansiHideCursor = "\x1b[?25l"
	ansiShowCursor = "\x1b[?25h"
)
