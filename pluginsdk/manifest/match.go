package manifest

import "strings"

// MatchSubject reports whether subject matches pattern, where pattern may
// use "*" as a single-segment wildcard. Segments are dot-separated, mirroring
// event subject naming (e.g. "task.created", "agent.completed").
//
// A pattern segment of "*" matches exactly one subject segment. All other
// pattern segments must match the corresponding subject segment exactly.
// The pattern and subject must have the same number of segments to match.
func MatchSubject(pattern, subject string) bool {
	if pattern == subject {
		return true
	}

	patternSegments := strings.Split(pattern, ".")
	subjectSegments := strings.Split(subject, ".")
	if len(patternSegments) != len(subjectSegments) {
		return false
	}

	for i, seg := range patternSegments {
		if seg == "*" {
			continue
		}
		if seg != subjectSegments[i] {
			return false
		}
	}
	return true
}
