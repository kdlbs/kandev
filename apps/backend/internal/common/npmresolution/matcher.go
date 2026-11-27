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

const maxManagedStartupDiagnosticBytes = 16 << 10

var managedStartupCodePattern = regexp.MustCompile(`(?i)^\s*npm\s+(?:ERR!|error)\s+code\s+([A-Z][A-Z0-9_]*)\s*$`)
var managedStartupIncompleteMarkerPattern = regexp.MustCompile(`(?i)^\s*npm\s+(?:ERR!|error)\s+diagnostic incomplete\s*$`)

var transientManagedStartupCodes = map[string]struct{}{
	"ECONNRESET": {}, "ECONNREFUSED": {}, "ETIMEDOUT": {}, "EAI_AGAIN": {},
	"E502": {}, "E503": {}, "E504": {}, "EBUSY": {}, "ENOTEMPTY": {}, "EINTEGRITY": {},
}

var permanentManagedStartupCodes = map[string]struct{}{
	"EACCES": {}, "EPERM": {}, "ENOSPC": {}, "EROFS": {}, "E401": {}, "E403": {},
	"E404": {}, "EAUTH": {}, "ENEEDAUTH": {}, "EBADENGINE": {},
}

// ManagedStartupDiagnosticSummary distinguishes absent npm evidence from
// diagnostics that cannot be safely classified within the evidence bound.
type ManagedStartupDiagnosticSummary struct {
	Codes            []string
	Present          bool
	Complete         bool
	UnclassifiedCode bool
}

// ManagedStartupDiagnosticCodes extracts the closed set of npm codes that
// can inform managed-runtime startup recovery. It ignores arbitrary prose and
// rejects diagnostics beyond the collection bound.
func ManagedStartupDiagnosticCodes(stderr string) []string {
	summary := AnalyzeManagedStartupDiagnostics(stderr)
	if !summary.Complete {
		return nil
	}
	return summary.Codes
}

// AnalyzeManagedStartupDiagnostics reports whether canonical npm code
// diagnostics were present, whether the bounded diagnostic set is complete,
// and which codes belong to the closed recovery allowlist.
func AnalyzeManagedStartupDiagnostics(stderr string) ManagedStartupDiagnosticSummary {
	summary := ManagedStartupDiagnosticSummary{Complete: len(stderr) <= maxManagedStartupDiagnosticBytes}
	seen := make(map[string]struct{})
	for _, line := range strings.Split(stderr, "\n") {
		if managedStartupIncompleteMarkerPattern.MatchString(line) {
			summary.Present = true
			summary.Complete = false
			continue
		}
		match := managedStartupCodePattern.FindStringSubmatch(line)
		if len(match) != 2 {
			continue
		}
		summary.Present = true
		code := strings.ToUpper(match[1])
		if !isManagedStartupCode(code) {
			summary.UnclassifiedCode = true
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		summary.Codes = append(summary.Codes, code)
	}
	if !summary.Complete {
		summary.Codes = nil
	}
	return summary
}

// IsTransientManagedStartupCode reports whether an npm code is safe to retry
// once during managed-runtime initialization.
func IsTransientManagedStartupCode(code string) bool {
	_, ok := transientManagedStartupCodes[strings.ToUpper(code)]
	return ok
}

// IsPermanentManagedStartupCode reports whether an npm code blocks automatic
// startup recovery.
func IsPermanentManagedStartupCode(code string) bool {
	_, ok := permanentManagedStartupCodes[strings.ToUpper(code)]
	return ok
}

func isManagedStartupCode(code string) bool {
	if code == "ETARGET" {
		return true
	}
	return IsTransientManagedStartupCode(code) || IsPermanentManagedStartupCode(code)
}

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
