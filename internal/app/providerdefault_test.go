package app

import (
	"testing"

	"github.com/mindsdb/setfree/internal/adapters"
	"github.com/mindsdb/setfree/internal/config"
)

func TestApplyProviderDefaultModel_PinsModelForEveryAdapter(t *testing.T) {
	e := newTestEnv(t)
	p, ok := config.FindProvider("mindshub")
	if !ok {
		t.Fatal("mindshub provider is missing")
	}

	if err := applyProviderDefaultModel(e, p); err != nil {
		t.Fatalf("applyProviderDefaultModel: %v", err)
	}

	if len(adapters.All()) == 0 {
		t.Fatal("no adapters registered -- nothing for this test to check")
	}
	for _, a := range adapters.All() {
		got := e.settings.CLI[a.Name()]
		if got.Model != "mindshub_air" {
			t.Errorf("%s: Model = %q, want mindshub_air", a.Name(), got.Model)
		}
		if !got.ModelResolved {
			t.Errorf("%s: ModelResolved = false, want true so the discovery probe doesn't second-guess it", a.Name())
		}
	}

	reloaded, err := config.Load(e.dir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if reloaded.CLI["claude"].Model != "mindshub_air" {
		t.Error("the default model should be persisted to config.toml, not just held in memory")
	}
}

func TestApplyProviderDefaultModel_NoopWhenProviderHasNoDefault(t *testing.T) {
	e := newTestEnv(t)
	p, ok := config.FindProvider("openrouter")
	if !ok {
		t.Fatal("openrouter provider is missing")
	}

	if err := applyProviderDefaultModel(e, p); err != nil {
		t.Fatalf("applyProviderDefaultModel: %v", err)
	}
	if len(e.settings.CLI) != 0 {
		t.Errorf("expected no CLI settings to be touched, got %+v", e.settings.CLI)
	}
}
