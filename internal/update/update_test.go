package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func fakeEnv(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestDue_TrueWhenNeverChecked(t *testing.T) {
	c := &Checker{Dir: t.TempDir(), Getenv: fakeEnv(nil)}
	if !c.Due(time.Now()) {
		t.Error("expected Due() to be true before any check has run")
	}
}

func TestDue_FalseRightAfterChecking(t *testing.T) {
	c := &Checker{Dir: t.TempDir(), Getenv: fakeEnv(nil)}
	now := time.Now()
	if err := c.MarkChecked(now); err != nil {
		t.Fatalf("MarkChecked: %v", err)
	}
	if c.Due(now.Add(time.Hour)) {
		t.Error("expected Due() to be false within the check interval")
	}
	if !c.Due(now.Add(25 * time.Hour)) {
		t.Error("expected Due() to be true after the check interval has passed")
	}
}

func TestDue_FalseWhenDisabledByEnv(t *testing.T) {
	c := &Checker{Dir: t.TempDir(), Getenv: fakeEnv(map[string]string{EnvDisable: "1"})}
	if c.Due(time.Now()) {
		t.Error("expected Due() to be false when SETFREE_NO_AUTOUPDATE is set")
	}
}

func TestAsset_KnownPlatforms(t *testing.T) {
	cases := []struct {
		goos, goarch, want string
	}{
		{"darwin", "arm64", "setfree_Darwin_arm64.tar.gz"},
		{"darwin", "amd64", "setfree_Darwin_x86_64.tar.gz"},
		{"linux", "arm64", "setfree_Linux_arm64.tar.gz"},
		{"linux", "amd64", "setfree_Linux_x86_64.tar.gz"},
		{"windows", "amd64", "setfree_Windows_x86_64.zip"},
		{"windows", "arm64", "setfree_Windows_arm64.zip"},
	}
	for _, tc := range cases {
		got, err := Asset(tc.goos, tc.goarch)
		if err != nil || got != tc.want {
			t.Errorf("Asset(%q, %q) = %q, %v, want %q", tc.goos, tc.goarch, got, err, tc.want)
		}
	}
}

func TestAsset_UnknownPlatform(t *testing.T) {
	if _, err := Asset("plan9", "amd64"); err == nil {
		t.Error("expected an error for an unsupported OS")
	}
	if _, err := Asset("linux", "riscv64"); err == nil {
		t.Error("expected an error for an unsupported architecture")
	}
}

func TestVerifyChecksum(t *testing.T) {
	data := []byte("pretend this is a tarball")
	sum := sha256.Sum256(data)
	hexSum := hex.EncodeToString(sum[:])
	checksums := []byte(hexSum + "  setfree_Linux_x86_64.tar.gz\nabc123  some_other_file.zip\n")

	if err := verifyChecksum(data, "setfree_Linux_x86_64.tar.gz", checksums); err != nil {
		t.Errorf("expected checksum to verify, got %v", err)
	}
	if err := verifyChecksum([]byte("tampered"), "setfree_Linux_x86_64.tar.gz", checksums); err == nil {
		t.Error("expected a mismatch error for tampered data")
	}
	if err := verifyChecksum(data, "not_listed.tar.gz", checksums); err == nil {
		t.Error("expected an error when the asset isn't listed in checksums.txt")
	}
}

func buildTarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(content)), Mode: 0o755}); err != nil {
		t.Fatalf("tar header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("tar write: %v", err)
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func buildZip(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatalf("zip create: %v", err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	zw.Close()
	return buf.Bytes()
}

func TestExtract_TarGz(t *testing.T) {
	want := []byte("fake binary contents")
	archive := buildTarGz(t, "setfree", want)
	got, err := extract(archive, "setfree_Linux_x86_64.tar.gz", "setfree")
	if err != nil || !bytes.Equal(got, want) {
		t.Errorf("extract() = %q, %v, want %q", got, err, want)
	}
}

func TestExtract_Zip(t *testing.T) {
	want := []byte("fake windows binary")
	archive := buildZip(t, "setfree.exe", want)
	got, err := extract(archive, "setfree_Windows_x86_64.zip", "setfree.exe")
	if err != nil || !bytes.Equal(got, want) {
		t.Errorf("extract() = %q, %v, want %q", got, err, want)
	}
}

func TestExtract_BinaryNotFound(t *testing.T) {
	archive := buildTarGz(t, "some_other_name", []byte("x"))
	if _, err := extract(archive, "setfree_Linux_x86_64.tar.gz", "setfree"); err == nil {
		t.Error("expected an error when the named binary isn't in the archive")
	}
}

func TestReplaceSelf_SwapsContentAndSetsExecutablePermission(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "setfree")
	if err := os.WriteFile(path, []byte("old content"), 0o755); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	if err := replaceSelf(path, []byte("new content")); err != nil {
		t.Fatalf("replaceSelf: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil || string(got) != "new content" {
		t.Fatalf("ReadFile = %q, %v", got, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Error("expected the replaced binary to be executable")
	}

	// No leftover temp files.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("dir contents = %v, want only the final binary", names)
	}
}

func TestLatestCommit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/commit.txt" {
			w.Write([]byte("deadbeefcafefeed\n"))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := &Checker{BaseURL: srv.URL, Client: srv.Client()}
	got, err := c.LatestCommit(context.Background())
	if err != nil || got != "deadbeefcafefeed" {
		t.Fatalf("LatestCommit() = %q, %v", got, err)
	}
}

func TestApply_DownloadsVerifiesAndInstalls(t *testing.T) {
	asset, err := Asset(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatalf("Asset: %v", err)
	}
	binaryName := "setfree"
	if runtime.GOOS == "windows" {
		binaryName = "setfree.exe"
	}

	want := []byte("new fake binary contents")
	var archive []byte
	if runtime.GOOS == "windows" {
		archive = buildZip(t, binaryName, want)
	} else {
		archive = buildTarGz(t, binaryName, want)
	}
	sum := sha256.Sum256(archive)
	checksums := hex.EncodeToString(sum[:]) + "  " + asset + "\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + asset:
			w.Write(archive)
		case "/checksums.txt":
			w.Write([]byte(checksums))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	exePath := filepath.Join(dir, "setfree")
	if err := os.WriteFile(exePath, []byte("old binary"), 0o755); err != nil {
		t.Fatalf("seed exe: %v", err)
	}

	c := &Checker{BaseURL: srv.URL, Client: srv.Client()}
	if err := c.Apply(context.Background(), exePath); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	got, err := os.ReadFile(exePath)
	if err != nil || !bytes.Equal(got, want) {
		t.Errorf("installed binary = %q, %v, want %q", got, err, want)
	}
}

func TestApply_RejectsTamperedDownload(t *testing.T) {
	asset, err := Asset(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatalf("Asset: %v", err)
	}
	real := buildTarGz(t, "setfree", []byte("real"))
	sum := sha256.Sum256(real)
	checksums := hex.EncodeToString(sum[:]) + "  " + asset + "\n"
	tampered := buildTarGz(t, "setfree", []byte("tampered"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + asset:
			w.Write(tampered) // served bytes don't match checksums.txt
		case "/checksums.txt":
			w.Write([]byte(checksums))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	exePath := filepath.Join(dir, "setfree")
	original := []byte("untouched")
	if err := os.WriteFile(exePath, original, 0o755); err != nil {
		t.Fatalf("seed exe: %v", err)
	}

	c := &Checker{BaseURL: srv.URL, Client: srv.Client()}
	if err := c.Apply(context.Background(), exePath); err == nil {
		t.Fatal("expected Apply to reject a checksum mismatch")
	}

	got, _ := os.ReadFile(exePath)
	if !bytes.Equal(got, original) {
		t.Error("the original binary must be left untouched when verification fails")
	}
}
