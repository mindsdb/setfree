// Package envutil provides small helpers for building a child process
// environment from a base []string (as returned by os.Environ) without
// mutating the current process's environment.
package envutil

import "strings"

// Set returns env with key=value applied: any existing entry for key is
// replaced in place, otherwise key=value is appended. env is not mutated.
func Set(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	replaced := false
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			if replaced {
				// Drop duplicate entries for the same key.
				continue
			}
			out = append(out, prefix+value)
			replaced = true
			continue
		}
		out = append(out, kv)
	}
	if !replaced {
		out = append(out, prefix+value)
	}
	return out
}

// Unset returns env with any entry for key removed. env is not mutated.
func Unset(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			continue
		}
		out = append(out, kv)
	}
	return out
}
