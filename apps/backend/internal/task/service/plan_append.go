package service

import (
	"errors"
	"strings"
	"unicode"
)

// ErrPlanAppendFragmentWhitespaceOnly rejects an append fragment consisting
// only of whitespace: such a fragment can only
// add separator noise to a document from which it cannot be removed.
var ErrPlanAppendFragmentWhitespaceOnly = errors.New("append fragment must contain a non-whitespace character")

// ErrPlanContentReadFailed reports that an append could not read the stored
// plan content it needed to compose against.
// Deliberately distinct from ErrTaskPlanNotFound: reporting a read failure
// as "not found" would send the caller to create_task_plan_kandev, which
// upserts and commits the fragment as the entire plan — the destruction
// this mode exists to prevent, reached through the error message. The
// message carries no storage detail.
var ErrPlanContentReadFailed = errors.New("could not read the current plan content; the append was not applied")

// validateAppendFragment requires content and rejects a whitespace-only
// fragment in an append request.
func validateAppendFragment(content string) error {
	if content == "" {
		return ErrContentRequired
	}
	if isWhitespaceOnly(content) {
		return ErrPlanAppendFragmentWhitespaceOnly
	}
	return nil
}

// isWhitespaceOnly reports whether every rune in s has Unicode's
// White_Space property, matching this capability's normative "whitespace"
// definition (docs/specs/tasks/requirements/plan-write-append-mode.md
// #terminology). An empty string is vacuously whitespace-only.
func isWhitespaceOnly(s string) bool {
	for _, r := range s {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

// composePlanAppend implements the separator normalization in
// docs/specs/tasks/system-design/plan-write-append-mode.md ("Separator
// normalization"). The caller must have already rejected a whitespace-only
// fragment via validateAppendFragment: this function does not re-check
// that, and relies on it to guarantee trimLeadingBlankLines never consumes
// fragment entirely.
func composePlanAppend(stored, fragment string) string {
	trimmedStored := strings.TrimRightFunc(stored, unicode.IsSpace)
	trimmedFragment := trimLeadingBlankLines(fragment)
	if trimmedStored == "" {
		return trimmedFragment
	}
	return trimmedStored + "\n\n" + trimmedFragment
}

// trimLeadingBlankLines removes fragment's leading empty and
// whitespace-only lines (a line's terminator is excluded from its own
// whitespace check, per the requirements' "whitespace-only line"
// definition), preserving leading spaces and tabs on the first line
// containing a non-whitespace character and every character after it.
func trimLeadingBlankLines(fragment string) string {
	rest := fragment
	for {
		line, lineLen, terminatorLen, hasMore := firstLine(rest)
		if !hasMore || !isWhitespaceOnly(line) {
			return rest
		}
		rest = rest[lineLen+terminatorLen:]
	}
}

// firstLine returns rest's first line's content (excluding its
// terminator), that content's byte length, its terminator's byte length (1
// for "\n", 2 for "\r\n"), and whether a terminator was found at all. When
// hasMore is false, rest has no terminator and line/lineLen/terminatorLen
// describe rest in its entirety.
func firstLine(rest string) (line string, lineLen, terminatorLen int, hasMore bool) {
	idx := strings.IndexByte(rest, '\n')
	if idx == -1 {
		return rest, len(rest), 0, false
	}
	lineLen = idx
	terminatorLen = 1
	if idx > 0 && rest[idx-1] == '\r' {
		lineLen = idx - 1
		terminatorLen = 2
	}
	return rest[:lineLen], lineLen, terminatorLen, true
}
