package vision

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// recordingGateway is the fake "real gateway" the proxy forwards to. It
// echoes the received body so a test can assert what actually reached the
// gateway, and captures the Authorization header.
func recordingGateway(t *testing.T) (*httptest.Server, *struct {
	body []byte
	auth string
}) {
	got := &struct {
		body []byte
		auth string
	}{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.body, _ = io.ReadAll(r.Body)
		got.auth = r.Header.Get("Authorization")
		w.Header().Set("X-Gateway", "yes")
		// Echo the body so the proxy's response streaming path can be checked
		// too. SSE isn't needed to prove forwarding correctness.
		_, _ = w.Write(got.body)
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func newTestProxy(t *testing.T, targetURL, visionURL string) *proxy {
	cfg := Config{Enabled: true, Model: "qwen3.5", BaseURL: visionURL, APIKey: "vk",
		GatewayBaseURL: targetURL, GatewayAPIKey: "gk", Concurrency: 1, CacheDir: t.TempDir()}
	return newProxy(targetURL, cfg)
}

func do(t *testing.T, p *proxy, method, path string, body []byte, auth string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, bytes.NewReader(body))
	if auth != "" {
		r.Header.Set("Authorization", auth)
	}
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	p.handle(rec, r)
	return rec
}

func TestProxy_PassthroughNoImages(t *testing.T) {
	gw, got := recordingGateway(t)
	p := newTestProxy(t, gw.URL, "http://unreachable-vision")
	body := []byte(`{"model":"glm-5.2","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)
	rec := do(t, p, http.MethodPost, "/v1/messages", body, "Bearer realkey")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !bytes.Equal(got.body, body) {
		t.Errorf("body changed en route to gateway:\nwant %s\ngot  %s", body, got.body)
	}
	if got.auth != "Bearer realkey" {
		t.Errorf("Authorization not forwarded: got %q", got.auth)
	}
	if rec.Header().Get("X-Gateway") != "yes" {
		t.Error("response headers not proxied back")
	}
}

func TestProxy_RewritesImages(t *testing.T) {
	gw, got := recordingGateway(t)
	srv, _ := fakeVisionServer(t, "a green triangle")
	p := newTestProxy(t, gw.URL, srv.URL)
	body := []byte(`{"model":"glm-5.2","messages":[{"role":"user","content":[{"type":"text","text":"see"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"EEEE"}}]}]}`)
	rec := do(t, p, http.MethodPost, "/v1/messages", body, "Bearer realkey")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var forwarded map[string]any
	if err := json.Unmarshal(got.body, &forwarded); err != nil {
		t.Fatalf("forwarded body not JSON: %v (%s)", err, got.body)
	}
	msgs := forwarded["messages"].([]any)
	content := msgs[0].(map[string]any)["content"].([]any)
	// First block (text) unchanged; second block (image) replaced by caption.
	if content[0].(map[string]any)["text"] != "see" {
		t.Errorf("non-image block changed: %+v", content[0])
	}
	if content[1].(map[string]any)["type"] != "text" {
		t.Fatalf("image not replaced: %+v", content[1])
	}
	if !bytes.Contains([]byte(content[1].(map[string]any)["text"].(string)), []byte("a green triangle")) {
		t.Errorf("caption not in forwarded body: %+v", content[1])
	}
	if got.auth != "Bearer realkey" {
		t.Errorf("Authorization not forwarded: %q", got.auth)
	}
}

func TestProxy_NonMessagesPathUntouched(t *testing.T) {
	gw, got := recordingGateway(t)
	p := newTestProxy(t, gw.URL, "http://unreachable-vision")
	body := []byte(`{"models":[]}`)
	rec := do(t, p, http.MethodGet, "/v1/models", body, "Bearer realkey")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !bytes.Equal(got.body, body) {
		t.Errorf("/v1/models body should pass through unchanged")
	}
}

func TestProxy_InvalidJSONPassthrough(t *testing.T) {
	gw, got := recordingGateway(t)
	p := newTestProxy(t, gw.URL, "http://unreachable-vision")
	body := []byte(`not json at all`)
	rec := do(t, p, http.MethodPost, "/v1/messages", body, "Bearer realkey")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !bytes.Equal(got.body, body) {
		t.Error("unparseable body should pass through unchanged, not error")
	}
}
