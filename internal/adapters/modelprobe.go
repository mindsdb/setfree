package adapters

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// ProbeModels issues a models-list GET request against baseURL and
// classifies whatever it finds against nativePrefixes. It tries the two
// conventions gateways actually use for where "/models" lives: appended
// after "/v1" if baseURL doesn't already end in one, otherwise appended
// directly.
//
// It expects the common {"data": [{"id": "..."}]} response shape that
// both OpenAI's and Anthropic's real models-list endpoints use, which is
// also what most compatible gateways (LiteLLM, OpenRouter, etc.) mirror.
// Anything else (network error, non-200, unexpected shape, empty list) is
// reported as Discovery{} — not supported, nothing more to say.
func ProbeModels(ctx context.Context, baseURL string, headers map[string]string, nativePrefixes []string) Discovery {
	trimmed := strings.TrimRight(baseURL, "/")
	paths := []string{"/v1/models", "/models"}
	if strings.HasSuffix(strings.ToLower(trimmed), "/v1") {
		paths = []string{"/models", "/v1/models"}
	}

	for _, path := range paths {
		if d := probeOnce(ctx, trimmed+path, headers, nativePrefixes); d.Supported {
			return d
		}
	}
	return Discovery{}
}

func probeOnce(ctx context.Context, url string, headers map[string]string, nativePrefixes []string) Discovery {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Discovery{}
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Discovery{}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Discovery{}
	}

	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return Discovery{}
	}

	var models []string
	for _, m := range body.Data {
		if m.ID != "" {
			models = append(models, m.ID)
		}
	}
	if len(models) == 0 {
		return Discovery{}
	}

	return Discovery{Supported: true, Native: allNative(models, nativePrefixes), Models: models}
}

func allNative(models []string, prefixes []string) bool {
	for _, m := range models {
		lower := strings.ToLower(m)
		matched := false
		for _, p := range prefixes {
			if strings.HasPrefix(lower, p) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}
