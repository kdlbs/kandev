// Package npmresolution classifies bounded npm version-resolution diagnostics.
package npmresolution

import (
	"regexp"
	"strings"
)

var etargetCodePattern = regexp.MustCompile(`(?im)^\s*npm\s+(?:ERR!|error)\s+code\s+ETARGET\b`)

// MatchesExactPackage reports whether stderr contains npm ETARGET evidence for
// the exact top-level package specification supplied by a trusted caller.
func MatchesExactPackage(stderr, packageSpec string) bool {
	if packageSpec == "" || strings.TrimSpace(packageSpec) != packageSpec {
		return false
	}
	notargetPattern := regexp.MustCompile(
		`(?im)^\s*npm\s+(?:ERR!|error)\s+notarget\s+No matching version found for\s+` +
			regexp.QuoteMeta(packageSpec) + `(?:\.\s*)?$`,
	)
	return etargetCodePattern.MatchString(stderr) && notargetPattern.MatchString(stderr)
}
