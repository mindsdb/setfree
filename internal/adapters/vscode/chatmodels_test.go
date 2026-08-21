package vscode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mindsdb/setfree/internal/catalog"
)

func boolPtr(b bool) *bool { return &b }

func TestBuildGroup_FiltersAndShapes(t *testing.T) {
	models := []catalog.Model{
		{ID: "mindshub_air", Label: "MindsHub Air"},
		{ID: "kimi", Label: "Kimi K3"},
		{ID: "embed-small", Label: "Embeddings", Embedding: true},
		{ID: "off-model", Enabled: boolPtr(false)},
		{ID: "sonnet"}, // no label: falls back to the id
	}

	g := buildGroup("https://api.example.com", "mdb_key", models)

	if g.Vendor != "customendpoint" || g.APIType != "chat-completions" {
		t.Errorf("group = %+v", g)
	}
	if g.APIKey != "mdb_key" {
		t.Errorf("APIKey = %q", g.APIKey)
	}
	if len(g.Models) != 3 {
		t.Fatalf("got %d models, want 3 (embedding and disabled models skipped): %+v", len(g.Models), g.Models)
	}
	for _, m := range g.Models {
		if m.URL != "https://api.example.com/v1/chat/completions" {
			t.Errorf("%s: url = %q", m.ID, m.URL)
		}
		if !m.ToolCalling {
			t.Errorf("%s: toolCalling must be true or the model is hidden in agent mode", m.ID)
		}
		if m.ContextWindow == 0 || m.MaxOutputTokens == 0 {
			t.Errorf("%s: token budgets must be set — VS Code plans against them", m.ID)
		}
	}
	if g.Models[0].Name != "MindsHub Air" {
		t.Errorf("label should become the display name, got %q", g.Models[0].Name)
	}
	if g.Models[2].Name != "sonnet" {
		t.Errorf("a model without a label should display its id, got %q", g.Models[2].Name)
	}
}

// Kimi rejects VS Code's default temperature with a 400; the entry has to
// carry the documented override. Other models must not get one.
func TestBuildGroup_KimiTemperature(t *testing.T) {
	g := buildGroup("https://api.example.com", "k", []catalog.Model{{ID: "kimi"}, {ID: "sonnet"}})
	if g.Models[0].ModelOptions["temperature"] != 1 {
		t.Errorf("kimi modelOptions = %v, want temperature 1", g.Models[0].ModelOptions)
	}
	if g.Models[1].ModelOptions != nil {
		t.Errorf("sonnet should carry no modelOptions, got %v", g.Models[1].ModelOptions)
	}
}

func TestBuildGroup_BaseAlreadyEndingInV1(t *testing.T) {
	g := buildGroup("https://api.example.com/v1", "k", []catalog.Model{{ID: "m"}})
	if got := g.Models[0].URL; got != "https://api.example.com/v1/chat/completions" {
		t.Errorf("url = %q, want /v1 exactly once", got)
	}
}

func TestUpsertChatModels_FreshFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "User", "chatLanguageModels.json")

	if err := upsertChatModels(path, buildGroup("https://api.example.com", "mdb_k", []catalog.Model{{ID: "m"}})); err != nil {
		t.Fatalf("upsertChatModels: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var groups []chatModelGroup
	if err := json.Unmarshal(data, &groups); err != nil {
		t.Fatalf("output isn't valid JSON: %v", err)
	}
	if len(groups) != 1 || groups[0].Name != groupName {
		t.Errorf("groups = %+v", groups)
	}

	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600 — the file holds the API key in plain text", info.Mode().Perm())
	}
}

// The file is shared with providers the user added by hand or via the
// wizard; SetFree must only ever replace its own entry.
func TestUpsertChatModels_PreservesOtherProvidersAndReplacesOwn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chatLanguageModels.json")
	existing := `[
	  {"name": "Ollama Local", "vendor": "customendpoint", "apiKey": "x", "apiType": "chat-completions", "models": []},
	  {"name": "SetFree", "vendor": "customendpoint", "apiKey": "old", "apiType": "chat-completions", "models": []}
	]`
	if err := os.WriteFile(path, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := upsertChatModels(path, buildGroup("https://api.example.com", "mdb_new", []catalog.Model{{ID: "m"}})); err != nil {
		t.Fatalf("upsertChatModels: %v", err)
	}

	data, _ := os.ReadFile(path)
	var groups []chatModelGroup
	if err := json.Unmarshal(data, &groups); err != nil {
		t.Fatalf("output isn't valid JSON: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want the user's plus SetFree's: %s", len(groups), data)
	}
	if groups[0].Name != "Ollama Local" {
		t.Errorf("the user's own provider entry was disturbed: %+v", groups[0])
	}
	if groups[1].Name != groupName || groups[1].APIKey != "mdb_new" {
		t.Errorf("SetFree's entry wasn't replaced: %+v", groups[1])
	}
	if strings.Contains(string(data), `"old"`) {
		t.Error("the stale SetFree entry survived the upsert")
	}
}

// A file that exists but doesn't parse may be a hand-edit in progress;
// clobbering it would destroy the user's work.
func TestUpsertChatModels_RefusesToOverwriteCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chatLanguageModels.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := upsertChatModels(path, buildGroup("https://api.example.com", "k", []catalog.Model{{ID: "m"}}))
	if err == nil {
		t.Fatal("expected a corrupt existing file to be an error, not overwritten")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "{not json" {
		t.Error("the corrupt file was modified despite the error")
	}
}
