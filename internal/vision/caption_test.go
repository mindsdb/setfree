package vision

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func imgBlock(media, data string) map[string]any {
	return map[string]any{
		"type": "image",
		"source": map[string]any{
			"type":       "base64",
			"media_type": media,
			"data":       data,
		},
	}
}

func textBlock(t string) map[string]any { return map[string]any{"type": "text", "text": t} }

func TestIsImageBlock(t *testing.T) {
	if !isImageBlock(imgBlock("image/png", "abcd")) {
		t.Error("image block not detected")
	}
	if isImageBlock(textBlock("hi")) {
		t.Error("text block misdetected as image")
	}
	// URL-sourced images aren't base64 and aren't handled — they must not match.
	if isImageBlock(map[string]any{"type": "image", "source": map[string]any{"type": "url"}}) {
		t.Error("url image block should not be handled")
	}
}

func TestCollectImageBlocks_TopLevel(t *testing.T) {
	msgs := []any{
		map[string]any{"role": "user", "content": []any{
			textBlock("look"), imgBlock("image/png", "AAAA"), imgBlock("image/png", "AAAA"),
		}},
	}
	imgs := collectImageBlocks(msgs)
	// Deduped by key: the two identical images collapse to one.
	if len(imgs) != 1 {
		t.Fatalf("got %d images, want 1 (deduped)", len(imgs))
	}
}

func TestCollectImageBlocks_NestedInToolResult(t *testing.T) {
	// An image returned by a tool sits inside a tool_result block's content
	// array. Missing it sends a raw image to the text model — the exact bug
	// this bridge exists to fix.
	msgs := []any{
		map[string]any{"role": "user", "content": []any{
			map[string]any{
				"type":        "tool_result",
				"tool_use_id": "tool_1",
				"content":     []any{imgBlock("image/png", "BBBB")},
			},
		}},
	}
	imgs := collectImageBlocks(msgs)
	if len(imgs) != 1 {
		t.Fatalf("nested image not collected: got %d, want 1", len(imgs))
	}
	if imgs[0].Source.Data != "BBBB" {
		t.Errorf("collected wrong image: %q", imgs[0].Source.Data)
	}
}

func TestCaptionTextBlock(t *testing.T) {
	if b := captionTextBlock("a red square", "qwen3.5"); b["type"] != "text" ||
		!strings.Contains(b["text"].(string), "a red square") {
		t.Errorf("success block wrong: %+v", b)
	}
	if b := captionTextBlock("", "qwen3.5"); b["type"] != "text" ||
		!strings.Contains(b["text"].(string), "unavailable") {
		t.Errorf("failure block wrong: %+v", b)
	}
}

// fakeVisionServer answers caption calls with a fixed text block, recording
// how many times it was hit so the cache can be tested.
func fakeVisionServer(t *testing.T, reply string) (*httptest.Server, *int) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		body, _ := io.ReadAll(r.Body)
		var parsed map[string]any
		_ = json.Unmarshal(body, &parsed)
		// Confirm the vLLM passthrough that keeps Qwen3.5 from burning its
		// budget on thinking is present.
		if kwargs, ok := parsed["chat_template_kwargs"].(map[string]any); !ok || kwargs["enable_thinking"] != false {
			t.Errorf("vision call missing chat_template_kwargs.enable_thinking=false: %v", parsed["chat_template_kwargs"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{{"type": "text", "text": reply}},
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestCaption_CachesAcrossCalls(t *testing.T) {
	srv, hits := fakeVisionServer(t, "a red square")
	cfg := Config{Enabled: true, Model: "qwen3.5", BaseURL: srv.URL, APIKey: "k",
		GatewayBaseURL: srv.URL, Concurrency: 1, IdleTimeout: 0,
		CacheDir: t.TempDir()}
	c := newCaptioner(cfg)
	ctx := context.Background()
	img := imgBlock("image/png", "ZZZZZZ")

	first := c.caption(ctx, toImageBlock(img))
	if first != "a red square" {
		t.Fatalf("first caption = %q", first)
	}
	second := c.caption(ctx, toImageBlock(img))
	if second != "a red square" {
		t.Fatalf("second caption = %q", second)
	}
	if *hits != 1 {
		t.Errorf("vision endpoint hit %d times, want 1 (cached)", *hits)
	}
}

func TestCaption_FailureFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	cfg := Config{Enabled: true, Model: "qwen3.5", BaseURL: srv.URL, APIKey: "k",
		GatewayBaseURL: srv.URL, Concurrency: 1, CacheDir: t.TempDir()}
	c := newCaptioner(cfg)
	desc := c.caption(context.Background(), toImageBlock(imgBlock("image/png", "YYYY")))
	if desc != "" {
		t.Errorf("failed transcription should yield empty desc, got %q", desc)
	}
}

func TestRewriteMessages_ImageFreePassthrough(t *testing.T) {
	srv, _ := fakeVisionServer(t, "should-not-be-called")
	cfg := Config{Enabled: true, Model: "qwen3.5", BaseURL: srv.URL, APIKey: "k",
		GatewayBaseURL: srv.URL, Concurrency: 1, CacheDir: t.TempDir()}
	c := newCaptioner(cfg)
	msgs := []any{
		map[string]any{"role": "user", "content": []any{textBlock("just text")}},
	}
	out := c.rewriteMessages(context.Background(), msgs)
	// No images → same value (rewriteBlocks returns nil, original kept, not
	// copied). Compare by pointer identity since maps aren't comparable with ==.
	if reflect.ValueOf(out[0]).Pointer() != reflect.ValueOf(msgs[0]).Pointer() {
		t.Error("image-free message should be kept by identity, not copied")
	}
}

func TestRewriteMessages_TopLevelImage(t *testing.T) {
	srv, _ := fakeVisionServer(t, "a blue circle")
	cfg := Config{Enabled: true, Model: "qwen3.5", BaseURL: srv.URL, APIKey: "k",
		GatewayBaseURL: srv.URL, Concurrency: 1, CacheDir: t.TempDir()}
	c := newCaptioner(cfg)
	msgs := []any{
		map[string]any{"role": "user", "content": []any{imgBlock("image/png", "CCCC")}},
	}
	out := c.rewriteMessages(context.Background(), msgs)
	content := out[0].(map[string]any)["content"].([]any)
	blk := content[0].(map[string]any)
	if blk["type"] != "text" || !strings.Contains(blk["text"].(string), "a blue circle") {
		t.Errorf("image not replaced by caption: %+v", blk)
	}
}

func TestRewriteMessages_NestedToolResultImage(t *testing.T) {
	srv, _ := fakeVisionServer(t, "a screenshot")
	cfg := Config{Enabled: true, Model: "qwen3.5", BaseURL: srv.URL, APIKey: "k",
		GatewayBaseURL: srv.URL, Concurrency: 1, CacheDir: t.TempDir()}
	c := newCaptioner(cfg)
	msgs := []any{
		map[string]any{"role": "user", "content": []any{
			map[string]any{
				"type":        "tool_result",
				"tool_use_id": "t1",
				"content":     []any{imgBlock("image/png", "DDDD")},
			},
		}},
	}
	out := c.rewriteMessages(context.Background(), msgs)
	tr := out[0].(map[string]any)["content"].([]any)[0].(map[string]any)
	if tr["type"] != "tool_result" {
		t.Fatalf("tool_result block not preserved: %+v", tr)
	}
	inner := tr["content"].([]any)[0].(map[string]any)
	if inner["type"] != "text" || !strings.Contains(inner["text"].(string), "a screenshot") {
		t.Errorf("nested image not captioned: %+v", inner)
	}
}
