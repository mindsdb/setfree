package adapters

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeModels_PrefersV1ModelsWhenBaseHasNoV1Suffix(t *testing.T) {
	var hit string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = r.URL.Path
		if r.URL.Path == "/v1/models" {
			w.Write([]byte(`{"data":[{"id":"claude-3-5-sonnet"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	d := ProbeModels(context.Background(), srv.URL, nil, []string{"claude"})
	if !d.Supported {
		t.Fatalf("Discovery = %+v, want Supported", d)
	}
	if hit != "/v1/models" {
		t.Errorf("hit path %q, want /v1/models", hit)
	}
}

func TestProbeModels_DoesNotDuplicateV1WhenBaseAlreadyEndsInIt(t *testing.T) {
	// baseURL already ends in "/v1", so the request path on the wire
	// (relative to the server root) must be "/v1/models" — appending the
	// default "/v1/models" suffix on top would wrongly produce
	// ".../v1/v1/models".
	var hit string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = r.URL.Path
		if r.URL.Path == "/v1/models" {
			w.Write([]byte(`{"data":[{"id":"gpt-4o"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	d := ProbeModels(context.Background(), srv.URL+"/v1", nil, []string{"gpt"})
	if !d.Supported {
		t.Fatalf("Discovery = %+v, want Supported", d)
	}
	if hit != "/v1/models" {
		t.Errorf("hit path %q, want /v1/models (not /v1/v1/models)", hit)
	}
}

func TestProbeModels_FallsBackToSecondPath(t *testing.T) {
	// A base URL with no /v1 suffix tries /v1/models first; if that 404s,
	// it should still try /models before giving up.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.Write([]byte(`{"data":[{"id":"claude-3-5-sonnet"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	d := ProbeModels(context.Background(), srv.URL, nil, []string{"claude"})
	if !d.Supported {
		t.Fatalf("Discovery = %+v, want Supported via fallback path", d)
	}
}

func TestProbeModels_HeadersArePassedThrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "yes" {
			http.Error(w, "missing header", http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"data":[{"id":"claude-3-5-sonnet"}]}`))
	}))
	defer srv.Close()

	d := ProbeModels(context.Background(), srv.URL, map[string]string{"X-Custom": "yes"}, []string{"claude"})
	if !d.Supported {
		t.Fatalf("Discovery = %+v, want Supported", d)
	}
}

func TestProbeModels_EmptyModelListIsNotSupported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	d := ProbeModels(context.Background(), srv.URL, nil, []string{"claude"})
	if d.Supported {
		t.Error("an empty models list should not be reported as Supported")
	}
}

func TestAllNative(t *testing.T) {
	if !allNative([]string{"claude-3", "anthropic-foo"}, []string{"claude", "anthropic"}) {
		t.Error("expected an all-native list to match")
	}
	if allNative([]string{"claude-3", "gpt-4o"}, []string{"claude", "anthropic"}) {
		t.Error("expected a mixed list to not match")
	}
	if allNative(nil, []string{"claude"}) != true {
		t.Error("an empty list is vacuously all-native (callers guard on len(models)==0 separately)")
	}
}
