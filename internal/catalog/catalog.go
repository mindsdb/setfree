// Package catalog fetches a gateway's model list with the metadata SetFree
// needs to configure editors — labels and type flags, not just ids. It's
// separate from the adapters' model-listing probe (adapters.ProbeModels),
// which answers a different question ("does this endpoint speak my
// protocol, and do the ids look native?") and deliberately reads nothing
// but ids.
package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Model is one catalog entry, in the {"data":[...]} shape MindsHub and
// other OpenAI-compatible gateways serve from their models endpoint.
type Model struct {
	ID string `json:"id"`
	// Label is a human-readable display name (e.g. "MindsHub Air").
	// Gateways that don't send one leave it empty; callers fall back to ID.
	Label string `json:"label"`
	// Embedding marks models that only produce embeddings — not usable in
	// a chat picker.
	Embedding bool `json:"embedding"`
	// Enabled is sent by gateways that list entitlement per model. The
	// field defaults true so gateways that omit it entirely (sending only
	// usable models) aren't misread as all-disabled.
	Enabled *bool `json:"enabled"`
}

// Usable reports whether the model belongs in a chat model picker.
func (m Model) Usable() bool {
	if m.ID == "" || m.Embedding {
		return false
	}
	return m.Enabled == nil || *m.Enabled
}

// DisplayName returns the label, or the id when no label was sent.
func (m Model) DisplayName() string {
	if m.Label != "" {
		return m.Label
	}
	return m.ID
}

// Fetch lists baseURL's model catalog, trying the same two path
// conventions the adapters' probe uses: "/v1/models" appended to a bare
// host, "/models" when the base already ends in /v1. Auth is a bearer
// token only — MindsHub rejects x-api-key outright, and every other
// OpenAI-compatible gateway takes bearer.
func Fetch(ctx context.Context, baseURL, apiKey string) ([]Model, error) {
	trimmed := strings.TrimRight(baseURL, "/")
	paths := []string{"/v1/models", "/models"}
	if strings.HasSuffix(strings.ToLower(trimmed), "/v1") {
		paths = []string{"/models", "/v1/models"}
	}

	var lastErr error
	for _, path := range paths {
		models, err := fetchOnce(ctx, trimmed+path, apiKey)
		if err == nil {
			return models, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func fetchOnce(ctx context.Context, url, apiKey string) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}

	var body struct {
		Data []Model `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("parsing the models list: %w", err)
	}
	if len(body.Data) == 0 {
		return nil, fmt.Errorf("GET %s: the models list was empty", url)
	}
	return body.Data, nil
}
