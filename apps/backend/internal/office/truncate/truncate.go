// Package truncate preserves the Office import path for rune-safe byte
// truncation. The implementation lives in the dependency-neutral common
// package so task and Office code share one boundary rule.
package truncate

import commontruncate "github.com/kandev/kandev/internal/common/truncate"

// UTF8 delegates to the shared implementation.
func UTF8(s string, maxBytes int) string {
	return commontruncate.UTF8(s, maxBytes)
}
