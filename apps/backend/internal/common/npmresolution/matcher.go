// Package npmresolution classifies bounded npm version-resolution diagnostics.
package npmresolution

import (
	"regexp"
	"strings"
	"time"
)

var etargetCodePattern = regexp.MustCompile(`(?im)^\s*npm\s+(?:ERR!|error)\s+code\s+ETARGET\b`)
var releaseAgeNotargetPattern = regexp.MustCompile(
	`(?i)^\s*npm\s+(?:ERR!|error)\s+notarget\s+No matching version found for\s+(\S+)\s+with a date before\s+(\S+)\s*$`,
)
var releaseAgeLocaleNotargetPattern = regexp.MustCompile(
	`(?i)^\s*npm\s+(?:ERR!|error)\s+notarget\s+No matching version found for\s+(\S+)\s+with a date before\s+(\d{1,2}/\d{1,2}/\d{4}, \d{1,2}:\d{2}:\d{2} [AP]M)\.?\s*$`,
)
var canonicalReleaseAgeNotargetPattern = regexp.MustCompile(
	`(?i)^\s*npm\s+(?:ERR!|error)\s+notarget\s+No matching version found for\s+(\S+)\s+with a date before\s+<release-date>\.?\s*$`,
)

const ReleaseDateMarker = "<release-date>"

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

// ReleaseAgePolicyPackageSpec returns the exact package spec from a date-
// qualified npm notarget line. It accepts npm's supported date forms or the
// canonical marker used after agentctl removes the untrusted date value.
func ReleaseAgePolicyPackageSpec(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if len(line) > 2048 {
		return "", false
	}
	if match := canonicalReleaseAgeNotargetPattern.FindStringSubmatch(line); len(match) == 2 {
		return match[1], true
	}
	return RawReleaseAgePolicyPackageSpec(line)
}

// RawReleaseAgePolicyPackageSpec returns the package from npm's original
// date-qualified line after validating the date.
func RawReleaseAgePolicyPackageSpec(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if len(line) > 2048 {
		return "", false
	}
	for _, pattern := range []*regexp.Regexp{releaseAgeNotargetPattern, releaseAgeLocaleNotargetPattern} {
		match := pattern.FindStringSubmatch(line)
		if len(match) != 3 || len(match[1]) > 512 || !validReleaseDate(match[2]) {
			continue
		}
		return match[1], true
	}
	return "", false
}

func validReleaseDate(raw string) bool {
	date := strings.TrimSuffix(raw, ".")
	if _, err := time.Parse(time.RFC3339Nano, date); err == nil {
		return true
	}
	_, err := time.Parse("1/2/2006, 3:04:05 PM", strings.ToUpper(date))
	return err == nil
}

// ContainsReleaseAgePolicy reports whether the bounded diagnostic includes an
// ETARGET code and at least one valid date-qualified notarget line.
func ContainsReleaseAgePolicy(stderr string) bool {
	if !etargetCodePattern.MatchString(stderr) {
		return false
	}
	for _, line := range strings.Split(stderr, "\n") {
		if _, ok := ReleaseAgePolicyPackageSpec(line); ok {
			return true
		}
	}
	return false
}

// MatchesReleaseAgePolicy requires the date-qualified ETARGET diagnostic to
// name the trusted top-level package spec exactly.
func MatchesReleaseAgePolicy(stderr, packageSpec string) bool {
	if packageSpec == "" || strings.TrimSpace(packageSpec) != packageSpec ||
		!etargetCodePattern.MatchString(stderr) {
		return false
	}
	for _, line := range strings.Split(stderr, "\n") {
		if got, ok := ReleaseAgePolicyPackageSpec(line); ok && got == packageSpec {
			return true
		}
	}
	return false
}

// MatchesRawReleaseAgePolicy requires the original npm date-qualified
// diagnostic for the exact trusted package spec. Use it at subprocess
// boundaries before a canonical marker has been projected.
func MatchesRawReleaseAgePolicy(stderr, packageSpec string) bool {
	if packageSpec == "" || strings.TrimSpace(packageSpec) != packageSpec ||
		!etargetCodePattern.MatchString(stderr) {
		return false
	}
	for _, line := range strings.Split(stderr, "\n") {
		if got, ok := RawReleaseAgePolicyPackageSpec(line); ok && got == packageSpec {
			return true
		}
	}
	return false
}
