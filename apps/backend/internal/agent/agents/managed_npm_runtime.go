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

// PackageSpec returns the trusted package name or exact package@version spec.
func (s ManagedNPMRuntimeSpec) PackageSpec(version string) string {
	if version == "" {
		return s.Package
	}
	return s.Package + "@" + version
}

// ExecutionCacheKey returns npm's deterministic _npx execution-tree key for
// this trusted package spec. npm derives it from the full package string using
// SHA-512 and the first 16 lowercase hexadecimal characters. The optional
// argument preserves the unversioned legacy key when omitted.
func (s ManagedNPMRuntimeSpec) ExecutionCacheKey(versions ...string) string {
	digest := sha512.Sum512([]byte(s.PackageSpec(firstVersion(versions))))
	return hex.EncodeToString(digest[:])[:16]
}

// ACPCommand returns the normal launch command for the exact version when one
// is supplied. An empty version keeps the legacy unversioned behavior.
func (s ManagedNPMRuntimeSpec) ACPCommand(version string) Command {
	args := []string{"npx", "--yes", "--prefer-offline", s.PackageSpec(version)}
	args = append(args, s.ACPArgs...)
	return NewCommand(args...)
}

// CachedACPCommand returns the legacy unversioned launch command.
func (s ManagedNPMRuntimeSpec) CachedACPCommand() Command {
	return s.ACPCommand("")
}

// CacheUpdateCommand returns the explicit cache preparation command. The
// optional version makes npm prepare one deterministic package@version tree.
func (s ManagedNPMRuntimeSpec) CacheUpdateCommand(versions ...string) Command {
	packageSpec := s.PackageSpec(firstVersion(versions))
	return NewCommand(
		"npm",
		"exec",
		"--yes",
		"--prefer-online",
		"--package="+packageSpec,
		"--",
		"node",
		"-e",
		"",
	)
}

func firstVersion(versions []string) string {
	if len(versions) == 0 {
		return ""
	}
	return versions[0]
}
