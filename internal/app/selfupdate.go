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

// maybeSelfUpdate checks, at most once a day, whether main has moved past
// the commit this binary was built from, and installs the newer build in
// place if so. It's called for every command except help/version, which
// stay instant and offline no matter what.
//
// This never blocks or fails a command: a dev build (no embedded commit),
// a missing config dir, a network error, or a write-permission error all
// just mean "skip it" — visible only with SETFREE_DEBUG. The one thing it
// does say out loud is that it's actually replacing the binary, since doing
// that silently would be the wrong kind of surprising.
func maybeSelfUpdate() {
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

	fmt.Println(ui.SelfUpdateStarted)
	applyCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := checker.Apply(applyCtx, exePath); err != nil {
		debugf("self-update: %v", err)
		return
	}
	fmt.Print(ui.SelfUpdateFinished(latest))
}
