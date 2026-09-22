// Package vision implements SetFree's optional vision bridge: a local proxy
// that lets a text-only main model (e.g. GLM-5.2, chosen for a long context
// window) work in a multimodal session by captioning every image with a
// separate vision-language model before the request reaches the gateway.
//
// The bridge is inert unless a vision model is configured (saved in
// config.toml's [vision] table or set via SETFREE_VISION_MODEL). When it's
// on, internal/app/launch.go starts the proxy and points the CLI at it
// instead of the gateway; the proxy forwards everything untouched except
// image content blocks, which it replaces with text captions. The CLI binary
// itself is never modified — SetFree's hard rule — so the bridge is a
// request-path proxy, not an in-process hook.
package vision

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mindsdb/setfree/internal/config"
	"github.com/mindsdb/setfree/internal/secrets"
)

// Environment variable overrides for the vision bridge. These follow the same
// precedence idiom as the gateway resolver: env beats saved config. The
// presence of a model (env or saved) is what enables the bridge.
const (
	EnvModel       = "SETFREE_VISION_MODEL"
	EnvBaseURL     = "SETFREE_VISION_BASE_URL"
	EnvAPIKey      = "SETFREE_VISION_API_KEY"
	EnvOff         = "SETFREE_VISION_OFF"
	EnvConcurrency = "SETFREE_VISION_CONCURRENCY"
	EnvIdleTimeout = "SETFREE_VISION_IDLE_TIMEOUT"
	EnvCacheDir    = "SETFREE_VISION_CACHE_DIR"

	// SecretName is the key under which the optional vision API key is stored
	// in the secrets store, alongside gateway keys — never in config.toml.
	SecretName = "vision"
)

// Defaults.
const (
	defaultConcurrency = 8
	// defaultIdleTimeout is a backstop, not the primary lifecycle. The proxy
	// is normally killed by its parent (internal/app/launch.go) when the CLI
	// exits; this only reaps a proxy orphaned by an abnormal parent death
	// (SIGKILL), so it's deliberately generous to never cull a live session
	// that's simply between turns.
	defaultIdleTimeout = time.Hour
)

// Config is a fully-resolved vision-bridge configuration: concrete values,
// with empty BaseURL/APIKey meaning "use the main gateway's".
type Config struct {
	Enabled bool
	Model   string
	BaseURL string // "" → same as the main gateway
	APIKey  string // "" → same as the main gateway

	// GatewayBaseURL/GatewayAPIKey are the main gateway's resolved
	// credentials — where the proxy forwards requests and, when Vision.BaseURL
	// or the vision key are unset, the source of their defaults.
	GatewayBaseURL string
	GatewayAPIKey  string

	Concurrency int
	IdleTimeout time.Duration
	CacheDir    string
}

// Resolve computes the vision config from saved settings, the secrets store,
// and environment overrides. mainGateway is the gateway the launch is using
// (already resolved by the gateway Resolver); its BaseURL/APIKey are the
// defaults for the vision endpoint when those aren't separately configured.
//
// getenv defaults to os.Getenv; tests override it.
func Resolve(s *config.Settings, store secrets.Store, getenv func(string) string, mainGateway config.GatewaySetting, mainAPIKey string) Config {
	if getenv == nil {
		getenv = os.Getenv
	}

	var saved config.VisionSetting
	if s != nil {
		saved = s.Vision
	}

	cfg := Config{
		Model:          strings.TrimSpace(getenv(EnvModel)),
		BaseURL:        strings.TrimSpace(getenv(EnvBaseURL)),
		APIKey:         getenv(EnvAPIKey),
		GatewayBaseURL: mainGateway.BaseURL,
		GatewayAPIKey:  mainAPIKey,
		Concurrency:    defaultConcurrency,
		IdleTimeout:    defaultIdleTimeout,
		CacheDir:       strings.TrimSpace(getenv(EnvCacheDir)),
	}

	// Saved config is the fallback for everything except the API key, which
	// comes from the secrets store (never config.toml).
	if cfg.Model == "" {
		cfg.Model = strings.TrimSpace(saved.Model)
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = strings.TrimSpace(saved.BaseURL)
	}
	if cfg.APIKey == "" && store != nil {
		if key, ok, _ := store.Get(SecretName); ok {
			cfg.APIKey = key
		}
	}

	// Hard disable wins over everything.
	if isTruthy(getenv(EnvOff)) {
		cfg.Enabled = false
		cfg.Model = ""
		return cfg
	}
	cfg.Enabled = cfg.Model != ""

	if n, err := strconv.Atoi(strings.TrimSpace(getenv(EnvConcurrency))); err == nil && n > 0 {
		cfg.Concurrency = n
	}
	if d, ok := parseDuration(strings.TrimSpace(getenv(EnvIdleTimeout))); ok {
		cfg.IdleTimeout = d
	}

	return cfg
}

func isTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func parseDuration(s string) (time.Duration, bool) {
	if s == "" {
		return 0, false
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d, true
	}
	// A bare number is seconds, matching how SETFREE_VISION_IDLE_TIMEOUT=120
	// would naturally be written.
	if n, err := strconv.Atoi(s); err == nil {
		return time.Duration(n) * time.Second, true
	}
	return 0, false
}

// defaultCacheFilePath returns the caption cache's location inside SetFree's
// per-user config directory, so captions survive across turns, restarts, and
// sessions (a caption is deterministic per image and costs ~8s to produce).
func defaultCacheFilePath() string {
	dir, err := config.Dir()
	if err != nil {
		// Fall back to the OS temp dir rather than fail the whole request
		// over a cache location; captions just won't persist across sessions.
		return filepath.Join(os.TempDir(), "setfree-vision-captions.ndjson")
	}
	return filepath.Join(dir, "vision-captions.ndjson")
}
