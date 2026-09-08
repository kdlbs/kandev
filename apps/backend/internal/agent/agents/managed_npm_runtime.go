package agents

import (
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agent/managedruntime"
)

// ManagedNPMRuntimeSpec defines a built-in npm-distributed ACP runtime.
// Package and ACPArgs must come from trusted agent metadata, never request
// input, because update jobs execute them directly.
type ManagedNPMRuntimeSpec struct {
	Package        string
	DefaultVersion string
	ACPArgs        []string
}

// DefaultVersionOrPinned returns the reviewed default version, including the
// catalogue fallback for specs that do not embed one. Custom specs without an
// explicit default remain empty rather than treating the bare package name as
// a version.
func (s ManagedNPMRuntimeSpec) DefaultVersionOrPinned() string {
	if s.DefaultVersion != "" {
		return s.DefaultVersion
	}
	packageSpec := s.PackageSpec("")
	if strings.HasPrefix(packageSpec, s.Package+"@") {
		return strings.TrimPrefix(packageSpec, s.Package+"@")
	}
	return ""
}

func newManagedNPMRuntimeSpec(packageName string, acpArgs ...string) ManagedNPMRuntimeSpec {
	return ManagedNPMRuntimeSpec{
		Package:        packageName,
		DefaultVersion: MustDefaultManagedNPMRuntimeVersion(packageName),
		ACPArgs:        acpArgs,
	}
}

// PackageSpec returns the trusted package name or exact package@version spec.
func (s ManagedNPMRuntimeSpec) PackageSpec(version string) string {
	if version == "" {
		version = s.DefaultVersion
		if version == "" {
			if reviewed, err := DefaultManagedNPMRuntimeVersion(s.Package); err == nil {
				version = reviewed
			} else if isBuiltInManagedNPMRuntimePackage(s.Package) {
				panic(fmt.Sprintf("managed runtime default is unavailable for %q: %v", s.Package, err))
			}
		}
	}
	if version == "" {
		return s.Package
	}
	return s.Package + "@" + version
}

// ExecutionCacheKey returns npm's deterministic _npx execution-tree key for
// this trusted package spec. npm derives it from the full package string using
// SHA-512 and the first 16 lowercase hexadecimal characters. The optional
// argument uses the reviewed default when omitted.
func (s ManagedNPMRuntimeSpec) ExecutionCacheKey(versions ...string) string {
	return managedruntime.NpxExecutionCacheKey(s.PackageSpec(firstVersion(versions)))
}

// ACPCommand returns the normal launch command for the exact version when one
// is supplied. An empty version uses the reviewed default.
func (s ManagedNPMRuntimeSpec) ACPCommand(version string) Command {
	return s.ACPCommandWithNpmPreference(version, false)
}

// ACPCommandWithNpmPreference builds a managed runtime launch command. The
// package spec and ACP arguments remain trusted agent metadata; recovery only
// changes npm's metadata freshness preference.
func (s ManagedNPMRuntimeSpec) ACPCommandWithNpmPreference(version string, preferOnline bool) Command {
	preference := "--prefer-offline"
	if preferOnline {
		preference = "--prefer-online"
	}
	args := []string{"npx", "--yes", preference, s.PackageSpec(version)}
	args = append(args, s.ACPArgs...)
	return NewCommand(args...)
}

// CachedACPCommand returns the default exact-version launch command.
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

func isBuiltInManagedNPMRuntimePackage(packageName string) bool {
	_, err := DefaultManagedNPMRuntimeVersion(packageName)
	return err == nil
}
