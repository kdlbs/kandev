package mcpconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"slices"
)

// BuiltinSelectionHash identifies an effective set independently of manifest order.
func BuiltinSelectionHash(packages []p.PackageID) string {
	canonical := slices.Clone(packages)
	slices.Sort(canonical)
	// Package identifiers are strings, so their JSON encoding cannot fail.
	payload, _ := json.Marshal(canonical)
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}
