package vscode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mindsdb/setfree/internal/catalog"
)

// groupName is the top-level name of SetFree's provider entry in
// chatLanguageModels.json. Upserts key on it, so SetFree only ever
// manages its own entry and never touches providers the user added.
const groupName = "SetFree"

// chatModelGroup is one provider entry in VS Code's chatLanguageModels.json
// — Copilot's "Custom Endpoint" bring-your-own-key format. The file is a
// JSON array of these. The apiKey is stored as plain text because that's
// how the format works: VS Code sends the literal string as the bearer
// token, and its documented secret-storage alternative (an ${input:...}
// variable) is passed through unexpanded on current versions, yielding
// empty auth on every request.
type chatModelGroup struct {
	Name    string          `json:"name"`
	Vendor  string          `json:"vendor"`
	APIKey  string          `json:"apiKey"`
	APIType string          `json:"apiType"`
	Models  []chatModelSpec `json:"models"`
}

type chatModelSpec struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	ToolCalling bool   `json:"toolCalling"`
	Vision      bool   `json:"vision"`
	// ContextWindow and MaxOutputTokens are budgets VS Code plans against,
	// not values read from the API — the gateway's models endpoint doesn't
	// expose real windows, so these are conservative fixed values.
	ContextWindow   int            `json:"contextWindow"`
	MaxOutputTokens int            `json:"maxOutputTokens"`
	ModelOptions    map[string]any `json:"modelOptions,omitempty"`
	// RequestHeaders forces the auth header explicitly, with the literal
	// key in it. On current VS Code builds (captured on 1.134 / Copilot
	// Chat 0.62.0) the provider-level apiKey resolves to an empty bearer —
	// both implicitly and through VS Code's own ${apiKey} substitution
	// token — so every request died at the gateway with an HTML 401. A
	// raw "Bearer <key>" passes through verbatim. No new exposure: the
	// same file already holds the key in the apiKey field, which is kept
	// for extension versions where the documented path works.
	RequestHeaders map[string]string `json:"requestHeaders"`
}

const (
	defaultContextWindow   = 200000
	defaultMaxOutputTokens = 16000
)

// buildGroup turns a gateway catalog into SetFree's provider entry.
// Embedding-only and disabled models are skipped; every chat model posts
// to the gateway's chat-completions endpoint.
func buildGroup(baseURL, apiKey string, models []catalog.Model) chatModelGroup {
	endpoint := strings.TrimRight(baseURL, "/")
	if !strings.HasSuffix(strings.ToLower(endpoint), "/v1") {
		endpoint += "/v1"
	}
	endpoint += "/chat/completions"

	group := chatModelGroup{
		Name:    groupName,
		Vendor:  "customendpoint",
		APIKey:  apiKey,
		APIType: "chat-completions",
	}
	for _, m := range models {
		if !m.Usable() {
			continue
		}
		spec := chatModelSpec{
			ID:              m.ID,
			Name:            m.DisplayName(),
			URL:             endpoint,
			ToolCalling:     true, // gates agent mode; catalog chat models take tools
			ContextWindow:   defaultContextWindow,
			MaxOutputTokens: defaultMaxOutputTokens,
			RequestHeaders:  map[string]string{"Authorization": "Bearer " + apiKey},
			// VS Code sends temperature 0.1 unless overridden, and parts
			// of the catalog 400 on it: Kimi rejects 0.1 outright, and
			// reasoning-model aliases (mindshub_air, gpt) reject any
			// temperature except 1. Every catalog model accepts exactly 1,
			// so it's pinned uniformly rather than special-casing models
			// whose upstreams can change under the alias.
			ModelOptions: map[string]any{"temperature": 1},
		}
		group.Models = append(group.Models, spec)
	}
	return group
}

// chatModelsPath returns VS Code's chatLanguageModels.json location for the
// default profile of a Stable install — beside settings.json.
func chatModelsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library/Application Support/Code/User/chatLanguageModels.json"), nil
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "Code", "User", "chatLanguageModels.json"), nil
		}
		return filepath.Join(home, "AppData", "Roaming", "Code", "User", "chatLanguageModels.json"), nil
	default:
		return filepath.Join(home, ".config/Code/User/chatLanguageModels.json"), nil
	}
}

// upsertChatModels writes group into the chatLanguageModels.json at path,
// replacing SetFree's previous entry if one exists and leaving every other
// provider untouched. A file that exists but doesn't parse is left alone
// and reported — overwriting a config the user may have hand-edited (the
// file allows that; the wizard opens it in an editor) would be worse than
// asking them to fix it.
func upsertChatModels(path string, group chatModelGroup) error {
	groups := []json.RawMessage{}
	data, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		// First write: start empty.
	case err != nil:
		return err
	default:
		if err := json.Unmarshal(data, &groups); err != nil {
			return fmt.Errorf("%s exists but isn't valid JSON — fix or remove it, then relaunch: %w", path, err)
		}
	}

	kept := groups[:0]
	for _, raw := range groups {
		var probe struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(raw, &probe) == nil && probe.Name == group.Name {
			continue // SetFree's old entry: replaced below
		}
		kept = append(kept, raw)
	}

	encoded, err := json.Marshal(group)
	if err != nil {
		return err
	}
	kept = append(kept, encoded)

	out, err := json.MarshalIndent(kept, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// 0600: the file holds the API key in plain text. That's inherent to
	// the format, but nothing says other users on the machine get to read it.
	return os.WriteFile(path, append(out, '\n'), 0o600)
}

// settingsPath returns VS Code's settings.json, which lives beside
// chatLanguageModels.json.
func settingsPath() (string, error) {
	chatPath, err := chatModelsPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(chatPath), "settings.json"), nil
}

// chatSettings returns the settings.json entries a gateway-backed chat
// needs, keyed by setting name:
//
//   - chat.defaultModel: new chats start on the pinned gateway model
//     instead of a built-in Copilot one — which, signed out, fails every
//     request with GitHub's 401 even though the gateway models work fine.
//   - chat.byokUtilityModelDefault "mainAgent": VS Code's internal utility
//     tasks (chat titles, inline-chat progress) otherwise demand a Copilot
//     utility model that doesn't exist signed-out, and error with exactly
//     this setting as the printed remedy.
func chatSettings(defaultModel string) map[string]any {
	settings := map[string]any{
		"chat.byokUtilityModelDefault": "mainAgent",
	}
	if defaultModel != "" {
		settings["chat.defaultModel"] = defaultModel
	}
	return settings
}

// ensureChatSettings fills wanted into settings.json, key by key, only
// where absent: a value the user set themselves always wins over SetFree's
// idea of a good default, rather than being re-pinned on every launch.
//
// settings.json is user-owned and allows comments (JSONC), which
// encoding/json can't round-trip. A file that doesn't parse as strict
// JSON is therefore left untouched — losing someone's commented settings
// to set a default would be a terrible trade.
func ensureChatSettings(path string, wanted map[string]any) error {
	settings := map[string]json.RawMessage{}
	data, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		// First write: start empty.
	case err != nil:
		return err
	default:
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("%s isn't strict JSON (comments?), so SetFree won't edit it — set %v yourself: %w", path, keysOf(wanted), err)
		}
	}

	changed := false
	for key, value := range wanted {
		if _, exists := settings[key]; exists {
			continue // the user's choice wins
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		settings[key] = encoded
		changed = true
	}
	if !changed {
		return nil
	}

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
