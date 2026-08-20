// Package secrets isolates API key storage from SetFree's regular settings.
//
// All secret handling goes through the Store interface so a future OS
// keychain-backed implementation (macOS Keychain, Windows Credential
// Manager, libsecret) can be swapped in without touching any caller. The
// only implementation today, FileStore, keeps keys in a dedicated file with
// restrictive permissions, separate from the human-editable config.toml.
package secrets

// Store persists API keys, keyed by gateway name.
type Store interface {
	// Get returns the stored key for a gateway, and whether one exists.
	Get(gateway string) (key string, ok bool, err error)
	// Set stores (or replaces) the key for a gateway.
	Set(gateway, key string) error
	// Delete removes the key for a gateway, if any.
	Delete(gateway string) error
	// Reset removes all stored keys.
	Reset() error
}

// Redact returns a value safe to print in place of a real secret. SetFree
// never displays partial keys, fingerprints, or lengths — only whether one
// is configured — since even a truncated key can narrow a brute-force
// search or leak into a shared terminal/screenshot.
func Redact(key string) string {
	if key == "" {
		return "not configured"
	}
	return "configured"
}
