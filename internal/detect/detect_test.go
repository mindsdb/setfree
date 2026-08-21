package detect

import "testing"

func TestKnown_Installed(t *testing.T) {
	// "go" is guaranteed to be on PATH: it's what's running this test.
	goCLI := Known{Key: "go-test", DisplayName: "Go", BinaryNames: []string{"go"}}
	if !goCLI.Installed() {
		t.Error("expected the go binary running this test to be detected")
	}
	if path, ok := goCLI.Path(); !ok || path == "" {
		t.Errorf("Path() = %q, %v", path, ok)
	}

	missing := Known{Key: "nope", DisplayName: "Nope", BinaryNames: []string{"definitely-not-a-real-binary-xyz123"}}
	if missing.Installed() {
		t.Error("expected a nonexistent binary to be reported as not installed")
	}
}

func TestFind(t *testing.T) {
	if _, ok := Find("claude"); !ok {
		t.Error("expected \"claude\" in the known CLI list")
	}
	if _, ok := Find("not-a-known-cli"); ok {
		t.Error("expected an unknown key to not be found")
	}
}

func TestList_SupportedEntriesHaveAdapters(t *testing.T) {
	// This is a guard against List and the adapters registry drifting apart:
	// every entry marked Supported must correspond to a real adapter name.
	// (The adapters package itself isn't imported here to avoid a cycle;
	// this just checks the two supported keys by name.)
	wantSupported := map[string]bool{"claude": true, "codex": true, "code": true}
	for _, k := range List {
		if k.Supported != wantSupported[k.Key] {
			t.Errorf("%s: Supported = %v, want %v", k.Key, k.Supported, wantSupported[k.Key])
		}
	}
}
