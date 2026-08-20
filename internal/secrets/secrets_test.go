package secrets

import (
	"os"
	"runtime"
	"testing"
)

func TestRedact_NeverLeaksTheKey(t *testing.T) {
	cases := []string{"", "sk-ant-api03-abcdefgh", "a"}
	for _, key := range cases {
		got := Redact(key)
		if got != "configured" && got != "not configured" {
			t.Errorf("Redact(%q) = %q, want a fixed status string", key, got)
		}
		if key != "" && got == key {
			t.Errorf("Redact(%q) returned the key itself", key)
		}
	}
}

func TestFileStore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	if _, ok, _ := store.Get("default"); ok {
		t.Fatal("expected no key before Set")
	}

	if err := store.Set("default", "sk-test"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	key, ok, err := store.Get("default")
	if err != nil || !ok || key != "sk-test" {
		t.Fatalf("Get after Set = %q, %v, %v", key, ok, err)
	}

	if err := store.Delete("default"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok, _ := store.Get("default"); ok {
		t.Fatal("expected key to be gone after Delete")
	}
}

func TestFileStore_Reset(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	store.Set("default", "sk-a")
	store.Set("work", "sk-b")

	if err := store.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	for _, gw := range []string{"default", "work"} {
		if _, ok, _ := store.Get(gw); ok {
			t.Errorf("gateway %q still has a key after Reset", gw)
		}
	}

	// Reset on an already-clean store must not error.
	if err := store.Reset(); err != nil {
		t.Fatalf("Reset on missing file: %v", err)
	}
}

func TestFileStore_FilePermissionsAreRestrictive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits don't apply on Windows")
	}
	dir := t.TempDir()
	store := NewFileStore(dir)
	if err := store.Set("default", "sk-test"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	info, err := os.Stat(store.path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("credentials file mode = %o, want 0600", perm)
	}
}
