// Package unidiff reads unified-diff text without mistaking content for
// structure. Diff markers are prepended to the original line, so a textual test
// like `strings.HasPrefix(line, "---")` cannot tell the header `--- a/seed.sql`
// apart from a removed SQL comment (`-- seed data` -> `--- seed data`) or an
// added C increment (`++counter;` -> `+++counter;`). The only reliable test is
// positional: a line is content when it sits inside a hunk.
package unidiff

import (
	"regexp"
	"strings"
)

// hunkHeaderPattern matches a unified-diff hunk header, e.g. `@@ -12,7 +34,9 @@ func Foo()`.
// It mirrors the pattern used by internal/review's anchor walker, which walks the
// same format for a different purpose.
var hunkHeaderPattern = regexp.MustCompile(`^@@+ -\d+(?:,\d+)? \+\d+(?:,\d+)? @@`)

// CountLines returns the number of added and removed content lines in a unified
// diff. It tracks in-hunk state exactly the way review.ExtractAnchorText does:
// counting only starts after a `@@` hunk header, and any line that is neither
// content nor hunk metadata (`diff --git`, `index`, `+++`, `---`) ends the hunk.
// A diff with no hunk header at all therefore counts nothing, which is correct —
// binary and mode-only changes carry no line counts.
func CountLines(diff string) (additions, deletions int) {
	inHunk := false

	for _, raw := range strings.Split(diff, "\n") {
		line := strings.TrimSuffix(raw, "\r")
		if hunkHeaderPattern.MatchString(line) {
			inHunk = true
			continue
		}
		if !inHunk {
			continue
		}
		switch {
		case strings.HasPrefix(line, "+"):
			additions++
		case strings.HasPrefix(line, "-"):
			deletions++
		case strings.HasPrefix(line, " "), strings.HasPrefix(line, "\\"), line == "":
			// Context line, the `\ No newline at end of file` marker, or the
			// empty element a trailing newline leaves behind. None is content.
		default:
			// File metadata between hunks ends the current hunk.
			inHunk = false
		}
	}

	return additions, deletions
}
