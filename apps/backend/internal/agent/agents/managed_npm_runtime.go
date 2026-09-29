package agents

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/kandev/kandev/internal/agent/managedruntime"
)

// ManagedNPMRuntimeSpec defines a built-in npm-distributed agent runtime.
// Package and Args must come from trusted agent metadata, never request
// input, because update jobs execute them directly.
//
// NativeBinary optionally names a standalone CLI that ships the same ACP
// interface as the npm package. When that binary is resolvable on PATH,
// host-side launch, probe, refresh, and update commands use it consistently.
// The managed npm runtime remains the fallback for containers and remotes.
type ManagedNPMRuntimeSpec struct {
	Package        string
	DefaultVersion string
	Args           []string
	// ACPArgs remains as a compatibility alias for existing ACP agents.
	ACPArgs      []string
	NativeBinary string
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

func newManagedNPMRuntimeSpec(packageName string, args ...string) ManagedNPMRuntimeSpec {
	return ManagedNPMRuntimeSpec{
		Package:        packageName,
		DefaultVersion: MustDefaultManagedNPMRuntimeVersion(packageName),
		Args:           args,
		ACPArgs:        args,
	}
}

func (s ManagedNPMRuntimeSpec) runtimeArgs() []string {
	if s.Args != nil {
		return s.Args
	}
	return s.ACPArgs
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
// this trusted package spec. The optional version uses the reviewed default.
func (s ManagedNPMRuntimeSpec) ExecutionCacheKey(versions ...string) string {
	return managedruntime.NpxExecutionCacheKey(s.PackageSpec(firstVersion(versions)))
}

// ACPCommand returns the normal launch command for the exact version when one
// is supplied. An empty version uses the reviewed default.
func (s ManagedNPMRuntimeSpec) ACPCommand(version string) Command {
	return s.RuntimeCommand(version)
}

// ACPCommandWithNpmPreference builds a managed runtime launch command. The
// package spec and ACP arguments remain trusted agent metadata; recovery only
// changes npm's metadata freshness preference.
func (s ManagedNPMRuntimeSpec) ACPCommandWithNpmPreference(version string, preferOnline bool) Command {
	return s.runtimeCommandWithNpmPreference(version, preferOnline)
}

// RuntimeCommand returns the normal launch command for the exact managed
// runtime version when one is supplied.
func (s ManagedNPMRuntimeSpec) RuntimeCommand(version string) Command {
	return s.runtimeCommandWithNpmPreference(version, false)
}

func (s ManagedNPMRuntimeSpec) runtimeCommandWithNpmPreference(version string, preferOnline bool) Command {
	preference := "--prefer-offline"
	if preferOnline {
		preference = "--prefer-online"
	}
	args := []string{"npx", "--yes", preference}
	args = append(args, managedruntime.NPMProjectPrefixArgs()...)
	args = append(args, s.PackageSpec(version))
	args = append(args, s.runtimeArgs()...)
	return NewCommand(args...)
}

// CachedACPCommand returns the default exact-version launch command.
func (s ManagedNPMRuntimeSpec) CachedACPCommand() Command {
	return s.CachedCommand()
}

// CachedCommand returns the default exact-version launch command.
func (s ManagedNPMRuntimeSpec) CachedCommand() Command { return s.RuntimeCommand("") }

// CacheUpdateCommand returns the explicit cache preparation command. The
// optional version makes npm prepare one deterministic package@version tree.
func (s ManagedNPMRuntimeSpec) CacheUpdateCommand(versions ...string) Command {
	packageSpec := s.PackageSpec(firstVersion(versions))
	return NewCommand(
		"npm",
		"--prefix",
		managedruntime.NPMProjectPrefix,
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

// NativeCommand returns the direct-binary launch command. Callers must gate it
// on NativeBinaryOnPath so remotes and containers use the managed runtime.
func (s ManagedNPMRuntimeSpec) NativeCommand() Command {
	args := []string{s.NativeBinary}
	args = append(args, s.runtimeArgs()...)
	return NewCommand(args...)
}

// NativeBinaryOnPath reports whether the optional native binary is resolvable
// from the host PATH.
func (s ManagedNPMRuntimeSpec) NativeBinaryOnPath() bool {
	if s.NativeBinary == "" {
		return false
	}
	_, err := exec.LookPath(s.NativeBinary)
	return err == nil
}

// NativeUpdateCommand updates the npm package that owns the native binary.
// The optional version keeps exact-version updates on the same runtime.
func (s ManagedNPMRuntimeSpec) NativeUpdateCommand(versions ...string) Command {
	return NewCommand("npm", "install", "-g", s.PackageSpec(firstVersion(versions)))
}

// UpdateCommand selects the update recipe for the runtime that launches on
// this host. Native binaries use the global npm install recipe; other hosts
// keep the managed execution-cache update.
func (s ManagedNPMRuntimeSpec) UpdateCommand(versions ...string) Command {
	if s.NativeBinaryOnPath() {
		return s.NativeUpdateCommand(versions...)
	}
	return s.CacheUpdateCommand(versions...)
}

// RefreshCommand selects the command used to probe the runtime after an
// update. Native hosts probe the same binary that the update installs.
func (s ManagedNPMRuntimeSpec) RefreshCommand(versions ...string) Command {
	if s.NativeBinaryOnPath() {
		return s.NativeCommand()
	}
	if len(versions) > 0 && versions[0] != "" {
		return s.ACPCommand(versions[0])
	}
	return s.CachedACPCommand()
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
