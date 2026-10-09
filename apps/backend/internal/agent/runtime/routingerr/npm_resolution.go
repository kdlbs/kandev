package routingerr

import "github.com/kandev/kandev/internal/common/npmresolution"

// ManagedRuntimeNpmResolutionMatchesPackage reports whether stderr contains a
// bounded npm ETARGET diagnostic for the exact managed package specification.
// Callers must derive the expected specification from the trusted launch
// arguments before using this guard.
func ManagedRuntimeNpmResolutionMatchesPackage(stderr, packageSpec string) bool {
	return npmresolution.MatchesExactPackage(stderr, packageSpec)
}

// ManagedRuntimeNpmReleaseAgePolicyMatchesPackage reports whether bounded
// ETARGET evidence names the exact selected package and carries a valid npm
// date-qualified notarget diagnostic.
func ManagedRuntimeNpmReleaseAgePolicyMatchesPackage(stderr, packageSpec string) bool {
	return npmresolution.MatchesReleaseAgePolicy(stderr, packageSpec)
}

func classifyManagedRuntimeNpmPolicy(in Input, rawText string) *Error {
	if in.ManagedRuntimePackageSpec == "" ||
		!npmresolution.MatchesReleaseAgePolicy(rawText, in.ManagedRuntimePackageSpec) {
		return nil
	}
	return &Error{
		Code:           CodeManagedRuntimeNpmPolicy,
		Confidence:     ConfHigh,
		ClassifierRule: "npm.etarget.release_age_policy.v1",
		RawExcerpt:     "npm error code ETARGET\nnpm error notarget The selected managed runtime package was blocked by a date policy: <release-date>",
	}
}
