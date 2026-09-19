package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/mindsdb/setfree/internal/config"
	"github.com/mindsdb/setfree/internal/usage"
)

func cmdUsage(args []string) int {
	rangeValue := "period"
	jsonOutput := false
	for _, arg := range args {
		switch arg {
		case "--lifetime":
			rangeValue = "lifetime"
		case "--json":
			jsonOutput = true
		default:
			return fail(fmt.Errorf("unknown flag %q\n\nUsage: setfree usage [--lifetime] [--json]", arg))
		}
	}

	e, err := newEnv()
	if err != nil {
		return fail(err)
	}
	resolved, err := e.resolver().Resolve("codex")
	if err != nil {
		return fail(err)
	}
	if !isMindsHubGateway(resolved.Gateway.BaseURL) {
		return fail(fmt.Errorf("usage checks are currently supported only for the MindsHub gateway"))
	}
	if resolved.Gateway.APIKey == "" {
		return fail(fmt.Errorf("no API key is configured for the selected gateway"))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	summary, err := usage.Fetch(ctx, config.MindsHubUsageAPI(), resolved.Gateway.APIKey, rangeValue)
	cancel()
	if err != nil {
		return fail(err)
	}
	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(summary); err != nil {
			return fail(fmt.Errorf("encoding usage output: %w", err))
		}
		return 0
	}

	fmt.Printf("MindsHub usage (%s)\n", summary.Range.Requested)
	for _, row := range summary.Results {
		name := row.Dimensions.Label
		if name == "" {
			name = row.Dimensions.Alias
		}
		cost := "unavailable"
		if row.Cost != nil {
			cost = "$" + row.Cost.TotalUSD
		}
		fmt.Printf("  %-24s %d input / %d output tokens  %s\n", name, row.Usage.InputTokens, row.Usage.OutputTokens, cost)
	}
	return 0
}

func isMindsHubGateway(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	provider, ok := config.FindProvider("mindshub")
	if !ok {
		return false
	}
	providerURL, err := url.Parse(provider.BaseURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Scheme, providerURL.Scheme) && strings.EqualFold(parsed.Host, providerURL.Host)
}
