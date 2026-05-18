package updates

import (
	"strconv"
	"strings"
)

// compareSemver returns -1, 0, or 1 when a is less than, equal to, or greater
// than b. The leading "v" is optional on either side; pre-release suffixes are
// compared lexicographically after the numeric segments to keep the comparator
// simple. Invalid versions sort below valid ones; if both are invalid the
// function falls back to plain string comparison.
//
// The intent is to support v1.X.Y kandev tags. We deliberately do not pull
// golang.org/x/mod/semver — see the package README for the dependency
// constraint. Output matches semver.Compare on well-formed v1 inputs.
func compareSemver(a, b string) int {
	an, ap, aok := parseSemver(a)
	bn, bp, bok := parseSemver(b)
	switch {
	case !aok && !bok:
		return strings.Compare(a, b)
	case !aok:
		return -1
	case !bok:
		return 1
	}
	for i := 0; i < 3; i++ {
		if an[i] < bn[i] {
			return -1
		}
		if an[i] > bn[i] {
			return 1
		}
	}
	// Numeric segments equal; treat absence of pre-release as higher than any.
	switch {
	case ap == "" && bp == "":
		return 0
	case ap == "":
		return 1
	case bp == "":
		return -1
	}
	return strings.Compare(ap, bp)
}

// parseSemver returns the three numeric segments, the pre-release suffix (or
// ""), and ok=false when the input is not a recognisable major.minor.patch
// triple. The leading "v" is optional.
func parseSemver(s string) ([3]int, string, bool) {
	var out [3]int
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	if s == "" {
		return out, "", false
	}
	// Split off pre-release / build metadata.
	pre := ""
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		pre = s[i+1:]
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return out, "", false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, "", false
		}
		out[i] = n
	}
	return out, pre, true
}

// isValidSemver returns true when s parses as a major.minor.patch triple.
func isValidSemver(s string) bool {
	_, _, ok := parseSemver(s)
	return ok
}
