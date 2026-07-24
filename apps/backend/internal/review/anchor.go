package review

import (
	"regexp"
	"strconv"
	"strings"
)

// hunkHeaderPattern matches a unified-diff hunk header and captures the new-side
// start line, e.g. `@@ -12,7 +34,9 @@ func Foo()` -> 34.
var hunkHeaderPattern = regexp.MustCompile(`^@@+ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// ExtractAnchorText returns the new-side content of lines startLine..endLine as
// they appear in the given unified diff, without the leading +/space marker.
//
// This snippet is what makes a stale finding recoverable: when the diff later
// moves, the client searches for this text instead of trusting the stored line
// number, so a finding follows the code it was written about rather than pointing
// at whatever now occupies that line. An empty result simply means the finding
// can be marked stale but not relocated.
func ExtractAnchorText(diff string, startLine, endLine int) string {
	if diff == "" || startLine <= 0 {
		return ""
	}
	if endLine < startLine {
		endLine = startLine
	}

	var collected []string
	newLine := 0
	inHunk := false

	for _, raw := range diffBodyLines(diff) {
		line := strings.TrimSuffix(raw, "\r")
		if start, ok := parseHunkStart(line); ok {
			newLine = start
			inHunk = true
			continue
		}
		if !inHunk {
			continue
		}
		switch {
		case strings.HasPrefix(line, "-"):
			// Deletion: present only on the old side, so the new-side counter
			// does not advance.
			continue
		case strings.HasPrefix(line, "\\"):
			// "\ No newline at end of file" is metadata, not content.
			continue
		case strings.HasPrefix(line, "+"), strings.HasPrefix(line, " "), line == "":
			if newLine >= startLine && newLine <= endLine {
				collected = append(collected, stripDiffMarker(line))
			}
			newLine++
			if newLine > endLine {
				return strings.Join(collected, "\n")
			}
		default:
			// Any other line ("diff --git", "index", "+++", "---") is file
			// metadata between hunks.
			inHunk = false
		}
	}
	return strings.Join(collected, "\n")
}

// stripDiffMarker removes exactly the one-byte '+' or ' ' marker, preserving
// content indentation. Chained TrimPrefix calls would eat a second character
// from a line whose own content starts with a space (`+ indented`).
func stripDiffMarker(line string) string {
	if line == "" {
		return ""
	}
	return line[1:]
}

// diffBodyLines splits a diff into lines, dropping the empty element a trailing
// newline leaves behind. That artifact carries no diff marker, so counting it as
// a context line would push the new-side numbering one past the hunk's end and
// let an anchor pick up a phantom trailing line.
func diffBodyLines(diff string) []string {
	lines := strings.Split(diff, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		return lines[:len(lines)-1]
	}
	return lines
}

func parseHunkStart(line string) (int, bool) {
	m := hunkHeaderPattern.FindStringSubmatch(line)
	if m == nil {
		return 0, false
	}
	start, err := strconv.Atoi(m[1])
	if err != nil || start <= 0 {
		return 0, false
	}
	return start, true
}
