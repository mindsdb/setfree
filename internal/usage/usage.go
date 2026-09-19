// Package usage reads the authenticated gateway's usage summary.
package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type Summary struct {
	Range   SummaryRange `json:"range"`
	Results []ModelUsage `json:"results"`
	Totals  Totals       `json:"totals"`
}

type SummaryRange struct {
	Requested string `json:"requested"`
}

type ModelUsage struct {
	Dimensions ModelDimensions `json:"dimensions"`
	Usage      TokenUsage      `json:"usage"`
	Cost       *Cost           `json:"cost"`
}

type ModelDimensions struct {
	Alias string `json:"model_alias"`
	Label string `json:"model_label"`
}

type TokenUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

type Cost struct {
	TotalUSD string `json:"total_usd"`
}

type Totals struct {
	Usage TokenUsage `json:"usage"`
	Cost  *Cost      `json:"cost"`
}

// Fetch retrieves a usage summary without exposing the API key in output or errors.
func Fetch(ctx context.Context, endpoint, apiKey, rangeValue string) (Summary, error) {
	separator := "?"
	if strings.Contains(endpoint, "?") {
		separator = "&"
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		endpoint+separator+"range="+rangeValue+"&group_by=model", nil)
	if err != nil {
		return Summary{}, fmt.Errorf("creating usage request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("Accept", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return Summary{}, fmt.Errorf("requesting usage: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return Summary{}, fmt.Errorf("usage endpoint returned %s", response.Status)
	}

	var summary Summary
	if err := json.NewDecoder(response.Body).Decode(&summary); err != nil {
		return Summary{}, fmt.Errorf("decoding usage response: %w", err)
	}
	return summary, nil
}
