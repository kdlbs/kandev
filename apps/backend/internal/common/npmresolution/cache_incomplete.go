package npmresolution

import (
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

var (
	missingNPMCodePattern    = regexp.MustCompile(`(?i)^\s*npm\s+(?:ERR!|error)\s+code\s+([A-Z][A-Z0-9_]*)\s*$`)
	missingNPMSyscallPattern = regexp.MustCompile(`(?i)^\s*npm\s+(?:ERR!|error)\s+syscall\s+(open|read)\s*$`)
	missingNPMPathPattern    = regexp.MustCompile(`(?i)^\s*npm\s+(?:ERR!|error)\s+path\s+(.+?)\s*$`)
	npxExecutionKeyPattern   = regexp.MustCompile(`^[0-9a-f]{16}$`)
)

// MatchesMissingExecutionRootManifest verifies raw npm evidence for a missing
// manifest at the caller-derived execution root.
func MatchesMissingExecutionRootManifest(stderr, expectedManifest string) bool {
	if !validExecutionRootManifestPath(expectedManifest) || len(stderr) > maxManagedStartupDiagnosticBytes {
		return false
	}
	summary := AnalyzeManagedStartupDiagnostics(stderr)
	if !summary.Complete || !summary.Present {
		return false
	}
	codeCount := 0
	sawENOENT := false
	sawOpenOrRead := false
	pathCount := 0
	pathMatches := false
	for _, line := range strings.Split(stderr, "\n") {
		if match := missingNPMCodePattern.FindStringSubmatch(line); len(match) == 2 {
			codeCount++
			if strings.EqualFold(match[1], "ENOENT") {
				sawENOENT = true
			} else {
				return false
			}
		}
		if missingNPMSyscallPattern.MatchString(line) {
			sawOpenOrRead = true
		}
		if match := missingNPMPathPattern.FindStringSubmatch(line); len(match) == 2 {
			pathCount++
			pathMatches = equalDiagnosticPath(match[1], expectedManifest)
		}
	}
	return codeCount == 1 && sawENOENT && sawOpenOrRead && pathCount == 1 && pathMatches
}

func validExecutionRootManifestPath(path string) bool {
	if path == "" || !filepath.IsAbs(path) || filepath.Base(path) != "package.json" {
		return false
	}
	keyPath := filepath.Dir(path)
	return npxExecutionKeyPattern.MatchString(filepath.Base(keyPath)) && filepath.Base(filepath.Dir(keyPath)) == "_npx"
}

func equalDiagnosticPath(candidate, expected string) bool {
	candidate = strings.TrimSpace(candidate)
	if len(candidate) >= 2 && (candidate[0] == '\'' && candidate[len(candidate)-1] == '\'' || candidate[0] == '"' && candidate[len(candidate)-1] == '"') {
		candidate = candidate[1 : len(candidate)-1]
	}
	if candidate == "" || !filepath.IsAbs(candidate) {
		return false
	}
	cleanCandidate, cleanExpected := filepath.Clean(candidate), filepath.Clean(expected)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(cleanCandidate, cleanExpected)
	}
	return cleanCandidate == cleanExpected
}
