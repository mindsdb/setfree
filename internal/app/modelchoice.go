package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/mindsdb/setfree/internal/adapters"
	"github.com/mindsdb/setfree/internal/config"
	"github.com/mindsdb/setfree/internal/gateway"
	"github.com/mindsdb/setfree/internal/terminal"
	"github.com/mindsdb/setfree/internal/ui"
)

// resolveModel figures out, once per CLI per gateway, whether that CLI's
// own model picker can be trusted against the configured gateway or
// whether the user should pin one from whatever the gateway actually
// offers. It probes the gateway's model-listing endpoint using the
// adapter's own protocol, then either:
//
//   - the models found all look native to that CLI's usual provider: say
//     so and leave the model unset, trusting the CLI's own picker.
//   - they don't: show the list and ask which one to use.
//   - the gateway doesn't support listing at all: the CLI's own model
//     picker can't be trusted either (it relies on that same endpoint),
//     so ask for a model name directly instead of launching with
//     whatever the CLI/gateway happens to default to.
//
// The outcome is remembered so this never runs again for the same CLI and
// gateway — except when the endpoint doesn't support listing, which asks
// again on every launch, since that endpoint being down today says
// nothing about tomorrow. It reports whether it actually interacted with
// the user (so the caller can decide whether to break launch's usual
// silence).
func resolveModel(e *env, a adapters.Adapter, gw gateway.Gateway) (interacted bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	discovery := a.DiscoverModels(ctx, gw)

	setting := e.settings.CLI[a.Name()]

	switch {
	case !discovery.Supported:
		// No list to offer, and the CLI's own picker needs that same
		// endpoint, so it can't be trusted either. Ask directly instead
		// of silently launching with whatever default applies. This
		// deliberately does not set ModelResolved: an endpoint that's
		// down today might work tomorrow, so it asks again next launch
		// rather than settling on a permanent verdict.
		reader := terminal.NewReader(os.Stdin)
		choice, promptErr := ui.PromptModelName(os.Stdout, reader, a.DisplayName())
		if promptErr != nil {
			return false, promptErr
		}
		setting.Model = choice
		interacted = true

	case discovery.Native:
		setting.ModelDiscovery = true
		setting.ModelResolved = true
		fmt.Print(ui.NativeModelDiscovery(e.colors, a.DisplayName()))
		interacted = true

	default:
		reader := terminal.NewReader(os.Stdin)
		choice, promptErr := ui.PromptModelChoice(os.Stdout, reader, a.DisplayName(), discovery.Models)
		if promptErr != nil {
			return false, promptErr
		}
		setting.Model = choice
		setting.ModelResolved = true
		interacted = true
	}

	if e.settings.CLI == nil {
		e.settings.CLI = map[string]config.CLISetting{}
	}
	e.settings.CLI[a.Name()] = setting
	if err := config.Save(e.dir, e.settings); err != nil {
		return interacted, fmt.Errorf("saving model choice: %w", err)
	}
	return interacted, nil
}
