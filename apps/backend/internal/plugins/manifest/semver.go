package manifest

import (
	"regexp"
	"strconv"
	"strings"
)

var safeComparableVersion = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+-]*$`)

// NormalizeReleaseVersion returns a dotted numeric release version suitable
// for compatibility comparisons. Kandev release builds are tagged `vX.Y.Z`,
// while plugin manifests use `X.Y.Z`; development and git-describe strings
// are deliberately not release versions and must not fall through to the
// byte-wise fallback in CompareVersions.
func NormalizeReleaseVersion(raw string) (string, bool) {
	version := strings.TrimSpace(raw)
	if strings.HasPrefix(version, "v") || strings.HasPrefix(version, "V") {
		version = version[1:]
	}
	parts := strings.Split(version, ".")
	if version == "" || len(parts) > 3 {
		return "", false
	}
	for _, part := range parts {
		if part == "" {
			return "", false
		}
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return "", false
		}
	}
	return version, true
}

// CompareVersions performs a dependency-free comparison of the safe release
// versions the marketplace accepts. Numeric core and prerelease identifiers
// sort without integer overflow; a prerelease sorts below its matching stable
// version and build metadata does not affect precedence. Other manifest
// versions retain the legacy byte-wise fallback.
//
// Returns -1 if a<b, 0 if equal, 1 if a>b.
func CompareVersions(a, b string) int {
	if a == b {
		return 0
	}
	left, leftOK := parseComparableVersion(a)
	right, rightOK := parseComparableVersion(b)
	if !leftOK || !rightOK {
		return strings.Compare(a, b)
	}
	for i := 0; i < max(len(left.core), len(right.core)); i++ {
		if comparison := compareVersionIdentifier(versionPart(left.core, i, "0"), versionPart(right.core, i, "0")); comparison != 0 {
			return comparison
		}
	}
	if len(left.prerelease) == 0 || len(right.prerelease) == 0 {
		if len(left.prerelease) == len(right.prerelease) {
			return 0
		}
		if len(left.prerelease) == 0 {
			return 1
		}
		return -1
	}
	for i := 0; i < max(len(left.prerelease), len(right.prerelease)); i++ {
		if i >= len(left.prerelease) {
			return -1
		}
		if i >= len(right.prerelease) {
			return 1
		}
		if comparison := compareVersionIdentifier(left.prerelease[i], right.prerelease[i]); comparison != 0 {
			return comparison
		}
	}
	return 0
}

type comparableVersion struct {
	core       []string
	prerelease []string
}

func parseComparableVersion(version string) (comparableVersion, bool) {
	if !safeComparableVersion.MatchString(version) {
		return comparableVersion{}, false
	}
	withoutBuild, _, _ := strings.Cut(version, "+")
	core, prerelease, hasPrerelease := strings.Cut(withoutBuild, "-")
	parsed := comparableVersion{core: strings.Split(core, ".")}
	if hasPrerelease {
		parsed.prerelease = strings.Split(prerelease, ".")
	}
	return parsed, true
}

func versionPart(parts []string, index int, fallback string) string {
	if index < len(parts) {
		return parts[index]
	}
	return fallback
}

func compareVersionIdentifier(left, right string) int {
	leftNumeric := isNumericIdentifier(left)
	rightNumeric := isNumericIdentifier(right)
	if comparison, comparable := compareNumericIdentifiers(left, right, leftNumeric, rightNumeric); comparable {
		return comparison
	}
	if leftNumeric != rightNumeric {
		if leftNumeric {
			return -1
		}
		return 1
	}
	return strings.Compare(left, right)
}

func compareNumericIdentifiers(left, right string, leftNumeric, rightNumeric bool) (int, bool) {
	if !leftNumeric || !rightNumeric {
		return 0, false
	}
	leftTrimmed := trimLeadingZeroes(left)
	rightTrimmed := trimLeadingZeroes(right)
	if len(leftTrimmed) != len(rightTrimmed) {
		if len(leftTrimmed) < len(rightTrimmed) {
			return -1, true
		}
		return 1, true
	}
	return strings.Compare(leftTrimmed, rightTrimmed), true
}

func trimLeadingZeroes(value string) string {
	trimmed := strings.TrimLeft(value, "0")
	if trimmed == "" {
		return "0"
	}
	return trimmed
}

func isNumericIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}
