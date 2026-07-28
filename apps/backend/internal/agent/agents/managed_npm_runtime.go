package agents

import (
	"crypto/sha512"
	"encoding/hex"
)

// ManagedNPMRuntimeSpec defines a built-in npm-distributed ACP runtime.
// Package and ACPArgs must come from trusted agent metadata, never request
// input, because update jobs execute them directly.
type ManagedNPMRuntimeSpec struct {
	Package string
	ACPArgs []string
}

// ExecutionCacheKey returns npm's deterministic _npx execution-tree key for
// this trusted package spec. npm derives it from the full package string using
// SHA-512 and the first 16 lowercase hexadecimal characters.
func (s ManagedNPMRuntimeSpec) ExecutionCacheKey() string {
	digest := sha512.Sum512([]byte(s.Package))
	return hex.EncodeToString(digest[:])[:16]
}

// CachedACPCommand returns the normal launch command. The package is
// intentionally unversioned and prefer-offline lets npm reuse a suitable
// execution-cache entry while still fetching when the cache is empty.
func (s ManagedNPMRuntimeSpec) CachedACPCommand() Command {
	args := []string{"npx", "--yes", "--prefer-offline", s.Package}
	args = append(args, s.ACPArgs...)
	return NewCommand(args...)
}

// CacheUpdateCommand returns the explicit cache-refresh command. npm exec
// installs the built-in unversioned package with online freshness before
// running a no-op under the prepared execution environment.
func (s ManagedNPMRuntimeSpec) CacheUpdateCommand() Command {
	return NewCommand(
		"npm",
		"exec",
		"--yes",
		"--prefer-online",
		"--package="+s.Package,
		"--",
		"node",
		"-e",
		"",
	)
}
