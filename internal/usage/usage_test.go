package usage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchRequestsModelGroupedUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("range") != "lifetime" || r.URL.Query().Get("group_by") != "model" {
			t.Fatalf("query = %q, want lifetime/model", r.URL.RawQuery)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"range":{"requested":"lifetime"},"results":[{"dimensions":{"model_alias":"air","model_label":"MindsHub Air"},"usage":{"input_tokens":10,"output_tokens":4},"cost":{"total_usd":"0.12"}}],"totals":{"usage":{"input_tokens":10,"output_tokens":4},"cost":{"total_usd":"0.12"}}}`))
	}))
	defer server.Close()

	summary, err := Fetch(context.Background(), server.URL, "test-key", "lifetime")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(summary.Results) != 1 || summary.Results[0].Dimensions.Alias != "air" {
		t.Fatalf("results = %+v", summary.Results)
	}
	if summary.Results[0].Cost == nil || summary.Results[0].Cost.TotalUSD != "0.12" {
		t.Fatalf("cost = %+v", summary.Results[0].Cost)
	}
}

func TestFetchDoesNotExposeAPIKeyOnHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := Fetch(context.Background(), server.URL, "secret-key", "period")
	if err == nil || err.Error() != "usage endpoint returned 401 Unauthorized" {
		t.Fatalf("error = %v", err)
	}
}
