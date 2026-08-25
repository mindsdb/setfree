package vision

// caption.go ports the captioning core of claude-local's visionBridge.ts to
// Go. It operates on decoded Anthropic /v1/messages request bodies
// (map[string]any): it finds base64 image content blocks, replaces each with
// a text caption produced by a separate vision model, and caches captions
// to an append-only NDJSON file so each image is transcribed once ever.
//
// Image blocks occur both at the top level of a message's content array and
// nested one level inside a tool_result block's content array. The walk
// recurses one level into any block carrying a content array, exactly as the
// reference does — missing the nested case sends a raw image to the text
// model and reproduces the "not multimodal" error this bridge exists to fix.
//
// The vision call forces chat_template_kwargs.enable_thinking=false: the
// default vision model (Qwen3.5) is a reasoning model that otherwise spends
// its whole token budget in <think> and returns empty content. That's a
// vLLM passthrough; it's harmless to endpoints that ignore it.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// imageBlock is the Anthropic base64 image content block shape.
//
//	{ "type": "image", "source": { "type": "base64", "media_type": "...", "data": "..." } }
type imageBlock struct {
	Type   string `json:"type"`
	Source struct {
		Type      string `json:"type"`
		MediaType string `json:"media_type"`
		Data      string `json:"data"`
	} `json:"source"`
}

// defaultSystemPrompt is the system prompt for the captioning call, carried
// verbatim from the reference: it asks for a complete, literal description so
// a text-only model can reason about the image from the caption alone.
const defaultSystemPrompt = "You are a precise vision system. Describe the image completely and literally: " +
	"all visible text transcribed verbatim, UI elements and their labels/positions, " +
	"shapes, colors, and layout. Be factual and specific; do not speculate."

const captionPrompt = "Describe this attached image in full so a text-only assistant can work with it."

// captioner coordinates image captioning: the in-memory + on-disk caption
// cache and the concurrent prewarm of unique images before a sequential
// rewrite (each caption is an ~8s round-trip, so N images done serially is
// N×8s and looks like a hang).
type captioner struct {
	cfg     Config
	httpc   *http.Client
	cacheMu sync.RWMutex
	cache   map[string]string

	diskOnce    sync.Once
	cachePath   string
	diskLoadErr error
}

func newCaptioner(cfg Config) *captioner {
	return &captioner{
		cfg:   cfg,
		httpc: &http.Client{Timeout: 5 * time.Minute},
		cache: make(map[string]string),
	}
}

// imageKey is a stable cache key for an image: media type + data length +
// head/tail of the base64 data. Carried from the reference; good enough to
// dedup without hashing the whole payload.
func imageKey(img imageBlock) string {
	d := img.Source.Data
	head, tail := 64, 64
	if len(d) < head {
		head = len(d)
	}
	if len(d) < tail {
		tail = len(d)
	}
	return fmt.Sprintf("%s:%d:%s:%s", img.Source.MediaType, len(d), d[:head], d[len(d)-tail:])
}

// caption returns the caption for img, using the cache when available. It
// never returns an error: a failed transcription yields a fallback string so
// a flaky vision endpoint never breaks the request — the text model gets a
// "[description unavailable]" marker instead of a raw image it can't handle.
func (c *captioner) caption(ctx context.Context, img imageBlock) string {
	key := imageKey(img)
	if cached := c.cacheGet(key); cached != "" {
		return cached
	}
	desc := c.transcribe(ctx, img)
	if desc != "" {
		c.cacheSet(key, desc)
	}
	return desc
}

func (c *captioner) cacheGet(key string) string {
	c.cacheMu.RLock()
	v := c.cache[key]
	c.cacheMu.RUnlock()
	if v != "" {
		return v
	}
	// A miss in memory may be a hit on disk (a prior session). Load the disk
	// cache once, lazily, then re-check.
	c.diskOnce.Do(c.loadDiskCache)
	c.cacheMu.RLock()
	v = c.cache[key]
	c.cacheMu.RUnlock()
	return v
}

func (c *captioner) cacheSet(key, desc string) {
	c.cacheMu.Lock()
	c.cache[key] = desc
	c.cacheMu.Unlock()
	c.persistCaption(key, desc)
}

// transcribe sends one image to the vision model and returns its text. The
// call forces enable_thinking=false (see the package doc). Returns "" on any
// failure.
func (c *captioner) transcribe(ctx context.Context, img imageBlock) string {
	base := c.cfg.BaseURL
	if base == "" {
		base = c.cfg.GatewayBaseURL
	}
	base = trimTrailingSlash(base)
	// The reference probes /v1/models then /models; the caption call posts to
	// the messages endpoint. Claude Code-shaped gateways serve it at
	// <base>/v1/messages; a base already ending in /v1 serves it at /messages.
	url := base + "/v1/messages"
	if endsWithV1(base) {
		url = base + "/messages"
	}

	key := c.cfg.APIKey
	if key == "" {
		key = c.cfg.GatewayAPIKey
	}

	// chat_template_kwargs is a vLLM passthrough not part of the Anthropic
	// schema, so the body is built as a generic map rather than a typed struct.
	body := map[string]any{
		"model":      c.cfg.Model,
		"max_tokens": 1024,
		"system":     defaultSystemPrompt,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "text", "text": captionPrompt},
					map[string]any{
						"type": "image",
						"source": map[string]any{
							"type":       "base64",
							"media_type": img.Source.MediaType,
							"data":       img.Source.Data,
						},
					},
				},
			},
		},
		"chat_template_kwargs": map[string]any{"enable_thinking": false},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return ""
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	// Hedge like the claude adapter: some Anthropic-style gateways expect
	// x-api-key; harmless to endpoints that don't.
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpc.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var respBody struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&respBody); err != nil {
		return ""
	}
	var sb bytes.Buffer
	for _, b := range respBody.Content {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	return strings.TrimSpace(sb.String())
}

// prewarm transcribes every unique image concurrently before the sequential
// rewrite, so the rewrite's serial await loop is all cache hits. Without
// this, N images cost N×~8s serially. Concurrency is bounded by cfg.
func (c *captioner) prewarm(ctx context.Context, imgs []imageBlock) {
	if len(imgs) <= 1 {
		for _, img := range imgs {
			_ = c.caption(ctx, img)
		}
		return
	}
	limit := c.cfg.Concurrency
	if limit < 1 {
		limit = 1
	}
	if limit > len(imgs) {
		limit = len(imgs)
	}
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for _, img := range imgs {
		wg.Add(1)
		sem <- struct{}{}
		go func(img imageBlock) {
			defer wg.Done()
			defer func() { <-sem }()
			_ = c.caption(ctx, img)
		}(img)
	}
	wg.Wait()
}

// ---- caption cache (in-memory + append-only NDJSON on disk) ----

func (c *captioner) loadDiskCache() {
	path := c.cacheFilePath()
	c.cachePath = path
	data, err := os.ReadFile(path)
	if err != nil {
		// No file yet, or unreadable — start empty. Don't clobber c.diskLoadErr
		// for a benign missing file.
		return
	}
	c.cacheMu.Lock()
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var entry [2]string
		if err := json.Unmarshal(line, &entry); err != nil {
			continue // skip a torn/partial line
		}
		if entry[0] != "" && entry[1] != "" {
			c.cache[entry[0]] = entry[1]
		}
	}
	c.cacheMu.Unlock()
}

func (c *captioner) persistCaption(key, desc string) {
	path := c.cacheFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	line, err := json.Marshal([2]string{key, desc})
	if err != nil {
		return
	}
	line = append(line, '\n')
	// Append is the only write, so concurrent writers (the prewarm) and other
	// live sessions never clobber each other; a read-modify-write blob would
	// lose entries. Best-effort: a failed write never breaks the request.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(line)
}

func (c *captioner) cacheFilePath() string {
	if c.cfg.CacheDir != "" {
		return filepath.Join(c.cfg.CacheDir, "vision-captions.ndjson")
	}
	return defaultCacheFilePath()
}

// ---- block walking (operates on decoded map[string]any) ----

// isImageBlock reports whether v is an Anthropic base64 image content block.
func isImageBlock(v any) bool {
	m, ok := v.(map[string]any)
	if !ok || m["type"] != "image" {
		return false
	}
	src, ok := m["source"].(map[string]any)
	if !ok {
		return false
	}
	return src["type"] == "base64"
}

// toImageBlock extracts the image block fields from v (known to be an image
// block by a prior isImageBlock check).
func toImageBlock(v any) imageBlock {
	m := v.(map[string]any)
	src, _ := m["source"].(map[string]any)
	ib := imageBlock{Type: "image"}
	ib.Source.Type, _ = src["type"].(string)
	ib.Source.MediaType, _ = src["media_type"].(string)
	ib.Source.Data, _ = src["data"].(string)
	return ib
}

// collectImageBlocks walks a /v1/messages body and returns every base64 image
// block, deduped by cache key. It recurses one level into any block carrying
// a content array (the tool_result case).
func collectImageBlocks(messages []any) []imageBlock {
	seen := map[string]struct{}{}
	var out []imageBlock
	var scan func(v any)
	scan = func(v any) {
		arr, ok := v.([]any)
		if ok {
			for _, b := range arr {
				scan(b)
			}
			return
		}
		if isImageBlock(v) {
			ib := toImageBlock(v)
			k := imageKey(ib)
			if _, ok := seen[k]; !ok {
				seen[k] = struct{}{}
				out = append(out, ib)
			}
			return
		}
		if m, ok := v.(map[string]any); ok {
			if content, ok := m["content"].([]any); ok {
				scan(content)
			}
		}
	}
	for _, m := range messages {
		if mm, ok := m.(map[string]any); ok {
			if content, ok := mm["content"].([]any); ok {
				scan(content)
			}
		}
	}
	return out
}

// rewriteMessages returns a copy of messages with every image content block
// (top-level or nested one level in a tool_result) replaced by a text block
// holding the caption. It never mutates the input. When the bridge is off or
// there are no images, the caller passes through untouched.
func (c *captioner) rewriteMessages(ctx context.Context, messages []any) []any {
	out := make([]any, 0, len(messages))
	for _, m := range messages {
		mm, ok := m.(map[string]any)
		if !ok {
			out = append(out, m)
			continue
		}
		content, ok := mm["content"].([]any)
		if !ok {
			out = append(out, m)
			continue
		}
		rewritten := c.rewriteBlocks(ctx, content)
		if rewritten == nil {
			// No images in this message; keep the original to avoid a needless
			// shallow copy (byte-identical passthrough for image-free turns).
			out = append(out, m)
			continue
		}
		copy := shallowCopyMap(mm)
		copy["content"] = rewritten
		out = append(out, copy)
	}
	return out
}

// rewriteBlocks returns a new block array with images replaced by captions,
// or nil if no image was found (so the caller can keep the original). It
// recurses one level into blocks carrying a content array.
func (c *captioner) rewriteBlocks(ctx context.Context, blocks []any) []any {
	var out []any
	changed := false
	for _, b := range blocks {
		if isImageBlock(b) {
			changed = true
			ib := toImageBlock(b)
			desc := c.caption(ctx, ib)
			out = append(out, captionTextBlock(desc, c.cfg.Model))
			continue
		}
		if m, ok := b.(map[string]any); ok {
			if content, ok := m["content"].([]any); ok {
				inner := c.rewriteBlocks(ctx, content)
				if inner != nil {
					changed = true
					cp := shallowCopyMap(m)
					cp["content"] = inner
					out = append(out, cp)
					continue
				}
			}
		}
		out = append(out, b)
	}
	if !changed {
		return nil
	}
	return out
}

// captionTextBlock builds the text block substituted for an image: the
// vision model's caption, or an explicit "unavailable" marker on failure so
// the text model knows there was an image it can't see.
func captionTextBlock(desc, model string) map[string]any {
	if desc == "" {
		return map[string]any{
			"type": "text",
			"text": "[Attached image — vision description unavailable.]",
		}
	}
	return map[string]any{
		"type": "text",
		"text": fmt.Sprintf(
			"[Attached image — described by the vision model (%s):]\n%s",
			model, desc),
	}
}

func shallowCopyMap(m map[string]any) map[string]any {
	cp := make(map[string]any, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}

func trimTrailingSlash(s string) string {
	return strings.TrimRight(s, "/")
}

func endsWithV1(s string) bool {
	return strings.HasSuffix(strings.ToLower(s), "/v1")
}
