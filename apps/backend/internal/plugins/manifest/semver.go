package manifest

import (
	"strconv"
	"strings"
)

// CompareVersions performs a best-effort, dependency-free comparison of two
// dotted version strings (e.g. plugin manifest "version" values), for
// callers that need to pick the "greatest" of a small set of versions
// (sideload directory scanning, min_kandev_version enforcement) without a
// full semver parser. Each "."-separated segment is compared numerically
// when both sides parse as a non-negative integer; the moment a segment
// pair does not (a pre-release suffix like "1.0.0-beta", a malformed
// version, etc.), comparison falls back to a plain byte-wise string compare
// of the two original, full version strings so the function always returns
// a total order rather than panicking or guessing.
//
// Returns -1 if a<b, 0 if equal, 1 if a>b.
func CompareVersions(a, b string) int {
	if a == b {
		return 0
	}
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		av, bv := segmentAt(as, i), segmentAt(bs, i)
		ai, aErr := strconv.Atoi(av)
		bi, bErr := strconv.Atoi(bv)
		if aErr != nil || bErr != nil {
			return strings.Compare(a, b)
		}
		if ai != bi {
			if ai < bi {
				return -1
			}
			return 1
		}
	}
	return 0
}

// segmentAt returns segments[i], or "0" if i is out of range (a version
// with fewer dotted segments than its comparison partner) so "1.0" compares
// equal to "1.0.0" rather than being treated as unparseable.
func segmentAt(segments []string, i int) string {
	if i < len(segments) {
		return segments[i]
	}
	return "0"
}
