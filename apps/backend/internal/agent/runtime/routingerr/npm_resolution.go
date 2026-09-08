package routingerr

import "github.com/kandev/kandev/internal/common/npmresolution"

// ManagedRuntimeNpmResolutionMatchesPackage reports whether stderr contains a
// bounded npm ETARGET diagnostic for the exact managed package specification.
// Callers must derive the expected specification from the trusted launch
// arguments before using this guard.
func ManagedRuntimeNpmResolutionMatchesPackage(stderr, packageSpec string) bool {
	return npmresolution.MatchesExactPackage(stderr, packageSpec)
}
