package routingerr

import (
	"regexp"
	"strings"
)

// ManagedRuntimeNpmResolutionMatchesPackage reports whether stderr contains a
// bounded npm ETARGET diagnostic for the exact managed package specification.
// Callers must derive the expected specification from the trusted launch
// arguments before using this guard.
func ManagedRuntimeNpmResolutionMatchesPackage(stderr, packageSpec string) bool {
	if packageSpec == "" || strings.TrimSpace(packageSpec) != packageSpec {
		return false
	}
	codePattern := regexp.MustCompile(`(?im)^\s*npm\s+(?:ERR!|error)\s+code\s+ETARGET\b`)
	notargetPattern := regexp.MustCompile(
		`(?im)^\s*npm\s+(?:ERR!|error)\s+notarget\s+No matching version found for\s+` +
			regexp.QuoteMeta(packageSpec) + `(?:\.\s*)?$`,
	)
	return codePattern.MatchString(stderr) && notargetPattern.MatchString(stderr)
}
