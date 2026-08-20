package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// EnvNoAnimation, when set to any non-empty value, keeps the mascot static.
const EnvNoAnimation = "SETFREE_NO_ANIMATION"

// robotHeight is the fixed line count every frame is padded to. The
// animation redraws in place by moving the cursor up robotHeight+1 lines —
// the frame itself plus the status line underneath it.
const robotHeight = 5

// Robot is SetFree's tiny mascot at rest: hatched out of its shell, on its
// own legs, smiling. This is the animation's final frame, and the whole of
// it wherever animating isn't possible (piped output, SETFREE_NO_ANIMATION).
const Robot = `    [•‿•]
     |‖|
     / \`

// idleFrames are the mascot waiting in its shell: eyes and mouth shifting,
// the blocks either side of the box riding up and down like hands. They
// loop for as long as the background task takes, so the wait reads as the
// robot biding its time rather than the screen being stuck.
var idleFrames = []string{
	// Hands level with the face.
	`

 ▖▖▖▖▖▖▖
▖▖[•_•]▖▖
 ▖▖▖▖▖▖▖`,
	// Hands up, mouth open.
	`

▖▖▖▖▖▖▖▖▖
 ▖[◦o◦]▖
 ▖▖▖▖▖▖▖`,
	// A blink.
	`

 ▖▖▖▖▖▖▖
▖▖[-_-]▖▖
 ▖▖▖▖▖▖▖`,
	// Hands down.
	`

 ▖▖▖▖▖▖▖
 ▖[•~•]▖
▖▖▖▖▖▖▖▖▖`,
	// Hands up again, pleased.
	`

▖▖▖▖▖▖▖▖▖
 ▖[^_^]▖
 ▖▖▖▖▖▖▖`,
}

// hatchFrames are the payoff, played once the background task is done: the
// shell rattles, cracks, bursts apart, legs appear, and the robot jumps.
var hatchFrames = []string{
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

// frameDelay is how long each frame holds.
const frameDelay = 85 * time.Millisecond

// ShowRobot draws the mascot to w, ending with one trailing line that
// carries task's last status message (or is blank if there was none).
//
// When animate is true the mascot idles in its shell while task runs in the
// background, and only hatches once task returns — so slow work is covered
// by the animation instead of stalling in front of it, and the payoff never
// plays over something still in progress. task reports progress through the
// status callback, rendered on the line below the robot; it may be called
// from the background goroutine at any time. task may be nil.
//
// When animate is false, task runs to completion first and its status
// messages print as plain lines, since the cursor movement the animation
// relies on would land as escape-code noise in a pipe or log file.
func ShowRobot(w io.Writer, animate bool, task func(status func(string))) {
	if !animate || os.Getenv(EnvNoAnimation) != "" {
		if task != nil {
			task(func(msg string) {
				if msg != "" {
					fmt.Fprintln(w, msg)
				}
			})
		}
		fmt.Fprintln(w, Robot)
		fmt.Fprintln(w)
		return
	}

	var mu sync.Mutex
	var status string
	setStatus := func(msg string) {
		mu.Lock()
		status = msg
		mu.Unlock()
	}
	currentStatus := func() string {
		mu.Lock()
		defer mu.Unlock()
		return status
	}

	done := make(chan struct{})
	if task == nil {
		close(done)
	} else {
		go func() {
			defer close(done)
			task(setStatus)
		}()
	}

	fmt.Fprint(w, ansiHideCursor)
	defer fmt.Fprint(w, ansiShowCursor)

	drawn := false
	draw := func(frame string) {
		if drawn {
			// Rewind over the frame and status line just drawn.
			fmt.Fprintf(w, "\x1b[%dF", robotHeight+1)
		}
		drawn = true
		for _, line := range frameLines(frame) {
			fmt.Fprintf(w, "%s\x1b[K\n", line)
		}
		fmt.Fprintf(w, "%s\x1b[K\n", currentStatus())
		time.Sleep(frameDelay)
	}

	// Always play one full idle cycle, then keep looping until the task
	// finishes. Checking only between cycles keeps the hand-wave from
	// being cut off mid-motion.
idle:
	for {
		for _, frame := range idleFrames {
			draw(frame)
		}
		select {
		case <-done:
			break idle
		default:
		}
	}

	for _, frame := range hatchFrames {
		draw(frame)
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
