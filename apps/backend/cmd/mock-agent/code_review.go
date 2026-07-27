package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// codeReviewSentinel is the marker the native code-review prompt carries (see
// internal/review.PromptSentinel). Matching on it lets dev and E2E runs get a
// deterministic findings payload without a real provider.
const codeReviewSentinel = "KANDEV_CODE_REVIEW_REQUEST"

// isCodeReviewRequest reports whether the prompt is a native code-review pass.
func isCodeReviewRequest(prompt string) bool {
	return strings.Contains(prompt, codeReviewSentinel)
}

// reviewDiffHeaderRe matches the per-file headers the review prompt emits for
// each batch: `### path (status)` or `### repo=name file=path (status)`.
var reviewDiffHeaderRe = regexp.MustCompile(`(?m)^### (?:repo=(\S+) file=(\S+)|(\S+)) \(`)

// reviewTargetFile is a file the prompt actually showed the reviewer, along with
// a line number that exists in its diff.
type reviewTargetFile struct {
	repo string
	path string
	line int
}

// codeReviewResponse builds the fenced-JSON reply for a review prompt.
//
// Anchors are derived from the prompt's own diff rather than hardcoded, so the
// findings land on files and lines that really exist in whatever the test
// changed. That keeps the payload deterministic without pinning the fixture.
func codeReviewResponse(prompt string) string {
	targets := reviewTargets(prompt)
	if len(targets) == 0 {
		return "```json\n{\"summary\":\"No reviewable files were provided.\",\"findings\":[]}\n```"
	}

	primary := targets[0]
	findings := []string{reviewFindingJSON(primary, "blocker", "correctness",
		"Unchecked value can be nil",
		"This value is used without a nil check. A request that omits it panics at runtime.\n\nConsider guarding before the dereference.",
		"if value == nil {\n\treturn ErrMissingValue\n}")}

	// A second file, when present, proves per-file grouping in the panel.
	secondary := primary
	if len(targets) > 1 {
		secondary = targets[1]
	}
	findings = append(findings, reviewFindingJSON(secondary, "nit", "naming",
		"Name does not match the surrounding style",
		"Neighbouring identifiers use camelCase; this one does not. Purely cosmetic.", ""))

	// A deliberately malformed entry exercises the rejected-entry path: the run
	// must still complete and report the discard rather than failing.
	malformed := `{"line":3,"severity":"major","category":"correctness",` +
		`"title":"Entry with no file","body":"This entry is missing its file and must be discarded."}`

	return "```json\n{\n  \"summary\": \"Reviewed the working changes and found one blocking issue.\",\n  \"findings\": [\n    " +
		strings.Join(append(findings, malformed), ",\n    ") + "\n  ]\n}\n```"
}

func reviewFindingJSON(target reviewTargetFile, severity, category, title, body, suggestion string) string {
	fields := []string{
		fmt.Sprintf("%q: %q", "file", target.path),
		fmt.Sprintf("%q: %d", "line", target.line),
		fmt.Sprintf("%q: %q", "severity", severity),
		fmt.Sprintf("%q: %q", "category", category),
		fmt.Sprintf("%q: %q", "title", title),
		fmt.Sprintf("%q: %q", "body", body),
	}
	if target.repo != "" {
		fields = append(fields, fmt.Sprintf("%q: %q", "repo", target.repo))
	}
	if suggestion != "" {
		fields = append(fields, fmt.Sprintf("%q: %q", "suggestion", suggestion))
	}
	return "{" + strings.Join(fields, ", ") + "}"
}

// reviewTargets extracts each file the prompt included together with the first
// new-side line number inside its diff.
func reviewTargets(prompt string) []reviewTargetFile {
	headers := reviewDiffHeaderRe.FindAllStringSubmatchIndex(prompt, -1)
	targets := make([]reviewTargetFile, 0, len(headers))
	for i, match := range headers {
		groups := reviewDiffHeaderRe.FindStringSubmatch(prompt[match[0]:match[1]])
		target := reviewTargetFile{repo: groups[1], path: groups[2]}
		if target.path == "" {
			target.path = groups[3]
		}
		if target.path == "" {
			continue
		}
		// The section runs to the next header, or to the end of the prompt.
		end := len(prompt)
		if i+1 < len(headers) {
			end = headers[i+1][0]
		}
		target.line = firstAddedLine(prompt[match[1]:end])
		if target.line <= 0 {
			continue
		}
		targets = append(targets, target)
	}
	return targets
}

var reviewHunkHeaderRe = regexp.MustCompile(`^@@+ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// diffLines splits a diff body into lines, dropping the empty element a trailing
// newline leaves behind. That artifact is not a diff line, and counting it would
// push every new-side line number one past the end of the hunk.
func diffLines(body string) []string {
	lines := strings.Split(body, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		return lines[:len(lines)-1]
	}
	return lines
}

// firstAddedLine returns the new-side line number of the section's first added
// line, so the finding anchors to a line the diff really contains.
func firstAddedLine(section string) int {
	lineNumber := 0
	firstHunkStart := 0
	inHunk := false
	for _, raw := range diffLines(section) {
		line := strings.TrimSuffix(raw, "\r")
		if m := reviewHunkHeaderRe.FindStringSubmatch(line); m != nil {
			if start, err := strconv.Atoi(m[1]); err == nil {
				lineNumber = start
				if firstHunkStart == 0 {
					firstHunkStart = start
				}
				inHunk = true
			}
			continue
		}
		if !inHunk {
			continue
		}
		switch {
		case strings.HasPrefix(line, "+"):
			return lineNumber
		case strings.HasPrefix(line, "-"), strings.HasPrefix(line, "\\"):
			continue
		case line == "" || strings.HasPrefix(line, " "):
			lineNumber++
		default:
			inHunk = false
		}
	}
	// A file with no added line still anchors to its first hunk's start, which is
	// a line that exists. Returning the running counter instead would point one
	// line past the end of the hunk.
	return firstHunkStart
}
