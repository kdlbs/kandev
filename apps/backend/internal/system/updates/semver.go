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
	an, ap, aAhead, aok := parseSemver(a)
	bn, bp, bAhead, bok := parseSemver(b)
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
	// Numeric segments equal. A git-describe build ("<tag>-<N>-g<hash>") is N
	// commits AHEAD of its base tag, so it ranks at or above the plain tag and
	// above any real pre-release. Order such builds by commit count first.
	if aAhead > 0 || bAhead > 0 {
		switch {
		case aAhead > bAhead:
			return 1
		case aAhead < bAhead:
			return -1
		default:
			return 0
		}
	}
	// Neither is a git-describe build; absence of pre-release ranks higher.
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
// ""), the git-describe "commits ahead" count (0 when not a git-describe
// build), and ok=false when the input is not a recognisable major.minor.patch
// triple. The leading "v" is optional. When the suffix is a git-describe
// "<N>-g<hash>[-dirty]" form, the pre-release is reported as "" and the commit
// count is returned instead, so the build sorts above its base tag.
func parseSemver(s string) ([3]int, string, int, bool) {
	var out [3]int
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	if s == "" {
		return out, "", 0, false
	}
	// Strip build metadata (semver: build metadata does not affect
	// precedence). The remaining string may still carry a `-pre` suffix.
	if i := strings.Index(s, "+"); i >= 0 {
		s = s[:i]
	}
	// Split off the pre-release suffix.
	pre := ""
	if i := strings.Index(s, "-"); i >= 0 {
		pre = s[i+1:]
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return out, "", 0, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, "", 0, false
		}
		out[i] = n
	}
	// A git-describe suffix marks a build ahead of the base tag rather than a
	// pre-release of it.
	if ahead, ok := gitDescribeCommits(pre); ok {
		return out, "", ahead, true
	}
	return out, pre, 0, true
}

// gitDescribeCommits recognises the "<N>-g<hash>[-dirty]" suffix that
// `git describe --tags` appends when the build is N commits past the nearest
// tag. It returns the commit count and ok=true for that shape; otherwise
// ok=false so the suffix is treated as an ordinary pre-release.
func gitDescribeCommits(pre string) (int, bool) {
	if pre == "" {
		return 0, false
	}
	parts := strings.SplitN(pre, "-", 3)
	if len(parts) < 2 {
		return 0, false
	}
	n, err := strconv.Atoi(parts[0])
	if err != nil || n < 0 {
		return 0, false
	}
	hash := parts[1]
	if len(hash) < 2 || hash[0] != 'g' || !isHexString(hash[1:]) {
		return 0, false
	}
	return n, true
}

// isHexString reports whether s is a non-empty run of hexadecimal digits.
func isHexString(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

// isValidSemver returns true when s parses as a major.minor.patch triple.
func isValidSemver(s string) bool {
	_, _, _, ok := parseSemver(s)
	return ok
}
