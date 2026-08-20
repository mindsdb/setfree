package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/mindsdb/setfree/internal/config"
	"github.com/mindsdb/setfree/internal/ui"
	"github.com/mindsdb/setfree/internal/update"
	"github.com/mindsdb/setfree/internal/version"
)

// selfUpdateTask checks whether main has moved past the commit this binary
// was built from and installs the newer build in place if so, reporting
// progress through status rather than printing directly. That lets the
// landing screen run it behind the mascot animation, showing the message
// underneath, while other commands just print it.
//
// This never blocks or fails a command: a dev build (no embedded commit),
// a missing config dir, a network error, or a write-permission error all
// just mean "skip it" — visible only with SETFREE_DEBUG. The one thing it
// does say out loud is that it's actually replacing the binary, since doing
// that silently would be the wrong kind of surprising.
func selfUpdateTask(status func(string)) {
	if version.Commit == "" {
		return // dev build: nothing to compare against
	}

	dir, err := config.Dir()
	if err != nil {
		debugf("self-update: %v", err)
		return
	}

	checker := &update.Checker{Dir: dir}
	now := time.Now()
	if !checker.Due(now) {
		return
	}

	checkCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	latest, err := checker.LatestCommit(checkCtx)
	cancel()
	_ = checker.MarkChecked(now) // recorded regardless of outcome, see doc comment
	if err != nil {
		debugf("self-update check: %v", err)
		return
	}
	if latest == version.Commit {
		return
	}

	exePath, err := os.Executable()
	if err != nil {
		debugf("self-update: locating the running binary: %v", err)
		return
	}

	status(ui.SelfUpdateStarted)
	applyCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := checker.Apply(applyCtx, exePath); err != nil {
		debugf("self-update: %v", err)
		// Take the "Updating..." line back down: it didn't happen, and
		// leaving it up would claim otherwise.
		status("")
		return
	}
	status(ui.SelfUpdateFinished(latest))
}

// maybeSelfUpdate runs selfUpdateTask for commands with no animation to
// hide it behind, printing its progress as plain lines. It's called for
// every command except help/version, which stay instant and offline.
func maybeSelfUpdate() {
	printed := false
	selfUpdateTask(func(msg string) {
		if msg == "" {
			return
		}
		fmt.Println(msg)
		printed = true
	})
	if printed {
		fmt.Println()
	}
}
