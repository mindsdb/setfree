package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetch_ParsesCatalogAndSendsBearer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer mdb_key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"data":[
			{"id":"mindshub_air","label":"MindsHub Air","enabled":true},
			{"id":"embed-small","label":"Embeddings","enabled":true,"embedding":true},
			{"id":"off","enabled":false}
		]}`))
	}))
	defer srv.Close()

	models, err := Fetch(context.Background(), srv.URL, "mdb_key")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(models) != 3 {
		t.Fatalf("got %d models", len(models))
	}
	if !models[0].Usable() || models[0].DisplayName() != "MindsHub Air" {
		t.Errorf("model[0] = %+v", models[0])
	}
	if models[1].Usable() {
		t.Error("embedding models are not usable in a chat picker")
	}
	if models[2].Usable() {
		t.Error("disabled models are not usable")
	}
}

// Gateways that don't send an enabled field at all list only usable
// models; absence must not read as disabled.
func TestModel_EnabledDefaultsTrue(t *testing.T) {
	m := Model{ID: "m"}
	if !m.Usable() {
		t.Error("a model with no enabled field should be usable")
	}
}

func TestFetch_BaseAlreadyEndingInV1(t *testing.T) {
	var hit string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = r.URL.Path
		w.Write([]byte(`{"data":[{"id":"m"}]}`))
	}))
	defer srv.Close()

	if _, err := Fetch(context.Background(), srv.URL+"/v1", "k"); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if hit != "/v1/models" {
		t.Errorf("hit %q, want /v1/models (not /v1/v1/models)", hit)
	}
}

func TestFetch_ErrorStatusIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer srv.Close()

	if _, err := Fetch(context.Background(), srv.URL, "k"); err == nil {
		t.Fatal("expected a non-200 to be an error")
	}
}
