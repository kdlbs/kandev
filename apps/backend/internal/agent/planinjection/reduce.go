// Package planinjection bounds a task plan document before it is injected
// into a launching agent session's prompt. It is the single place budgeting,
// section selection, and truncation are implemented; callers must not carry
// their own copy of any of the three.
package planinjection

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/sysprompt"
)

const (
	// HandoverBudget bounds the plan text injected at session handover.
	HandoverBudget = 12000
	// DynamicBudget bounds the plan text injected into a dynamic
	// continuation. It must stay at or below the dynamic runtime's
	// continuationFieldLimit; internal/agent/runtime/dynamic asserts that
	// relationship, since the limit is unexported there.
	DynamicBudget = 4000
)

const (
	omissionNoticeTemplate = "[Kandev: plan reduced to fit the injection budget; %d of %d sections omitted%s. Call get_task_plan_kandev for the full plan.]"
	shortenedClause        = ", and the retained section was shortened"
	cutMarker              = "[Kandev: section truncated here]"
)

var (
	tagStartBytes = []byte(sysprompt.TagStart)
	tagEndBytes   = []byte(sysprompt.TagEnd)
)

// ContainTags removes every occurrence of the <kandev-system> and
// </kandev-system> literals from text. A single pass, or two independent
// per-literal loops, can each be evaded by an input where removing one
// literal constructs the other, so every byte is appended to an output
// buffer one at a time and, after each append, the buffer's tail is checked
// against both literals and popped immediately on a match — equivalent to
// rescanning for both literals after every removal until neither remains,
// but in one linear pass instead of a multi-pass rescan, which is
// quadratic on adversarially nested input.
func ContainTags(text string) string {
	buf := make([]byte, 0, len(text))
	for i := 0; i < len(text); i++ {
		buf = append(buf, text[i])
		switch {
		case bytes.HasSuffix(buf, tagEndBytes):
			buf = buf[:len(buf)-len(tagEndBytes)]
		case bytes.HasSuffix(buf, tagStartBytes):
			buf = buf[:len(buf)-len(tagStartBytes)]
		}
	}
	return string(buf)
}

// Reduce bounds document to at most budget bytes. Under budget it returns
// document unchanged. Over budget it drops whole sections from the middle,
// keeping a contiguous run from each end (tail first), falling back to
// shortening the document's first section only when no whole section can be
// retained. It returns the reduced text, whether anything was dropped, and
// the number of whole sections not represented in the output at all.
func Reduce(document string, budget int) (output string, reduced bool, omittedSections int) {
	if strings.TrimSpace(document) == "" {
		return "", false, 0
	}
	if budget <= 0 {
		// Handled before any reservation is subtracted from budget: for a
		// budget near the minimum int, that subtraction underflows and
		// wraps positive.
		return "", true, len(splitSections(document))
	}
	if len(document) <= budget {
		return document, false, 0
	}

	sections := splitSections(document)
	total := len(sections)
	noticeReservation := len(renderNotice(total, total, true)) + 1
	markerReservation := len(cutMarker) + 1

	headRetain, tailRetain := selectSections(sections, budget-noticeReservation)
	if len(headRetain) > 0 || len(tailRetain) > 0 {
		return assembleWholeSections(sections, headRetain, tailRetain, total)
	}

	return reduceFirstSection(sections[0], total, budget-noticeReservation-markerReservation)
}

// selectSections grows a head run from the start and a tail run from the
// end, considering candidates alternately with the tail run first. A
// candidate is retained when it fits avail minus the sections already
// retained; when it does not, that run closes permanently. A closed run
// forfeits all its remaining turns to the other. Selection stops when both
// runs are closed or no unconsidered section remains between them. A
// non-positive avail retains nothing; the caller never reaches this today
// (Reduce guards budget <= 0 before the reservation arithmetic that could
// drive avail negative), but the invariant is made explicit so the function
// stays safe if reused.
func selectSections(sections []string, avail int) (headRetain, tailRetain []int) {
	if avail <= 0 {
		return nil, nil
	}
	return selectSectionsRun(sections, avail)
}

func selectSectionsRun(sections []string, avail int) (headRetain, tailRetain []int) {
	lo, hi := 0, len(sections)-1
	headOpen, tailOpen := true, true
	nextIsTail := true
	used := 0

	for (headOpen || tailOpen) && lo <= hi {
		actTail := tailOpen && (nextIsTail || !headOpen)
		if headOpen && tailOpen {
			nextIsTail = !nextIsTail
		}

		idx := hi
		if !actTail {
			idx = lo
		}
		size := len(sections[idx])
		fits := used+size <= avail
		switch {
		case fits && actTail:
			used += size
			tailRetain = append(tailRetain, idx)
		case fits:
			used += size
			headRetain = append(headRetain, idx)
		case actTail:
			tailOpen = false
		default:
			headOpen = false
		}

		if actTail {
			hi--
		} else {
			lo++
		}
	}

	return headRetain, tailRetain
}

// assembleWholeSections concatenates the retained sections in original
// document order with no inserted separator, then appends the omission
// notice, preceded by a single newline only when the assembled text does
// not already end with one.
func assembleWholeSections(sections []string, headRetain, tailRetain []int, total int) (string, bool, int) {
	retained := append(append([]int{}, headRetain...), reverseInts(tailRetain)...)
	var sb strings.Builder
	for _, idx := range retained {
		sb.WriteString(sections[idx])
	}
	result := sb.String()
	omitted := total - len(retained)
	result = appendWithSeparator(result, renderNotice(omitted, total, false))
	return result, true, omitted
}

// reduceFirstSection keeps leading whole lines of the document's first
// section while they fit avail, cuts at the resulting line boundary, and
// marks the cut. When not even one complete line fits, it returns no plan
// text at all rather than a bare marker or notice.
func reduceFirstSection(firstSection string, total, avail int) (string, bool, int) {
	lines := splitLines(firstSection)
	var sb strings.Builder
	used := 0
	kept := 0
	for _, line := range lines {
		size := len(line)
		if used+size > avail {
			break
		}
		sb.WriteString(line)
		used += size
		kept++
	}
	if kept == 0 {
		return "", true, total
	}

	result := sb.String()
	result = appendWithSeparator(result, cutMarker)
	result = appendWithSeparator(result, renderNotice(total-1, total, true))
	return result, true, total - 1
}

// appendWithSeparator appends next to base, preceded by a single newline
// only when base does not already end with one.
func appendWithSeparator(base, next string) string {
	if base != "" && !strings.HasSuffix(base, "\n") {
		base += "\n"
	}
	return base + next
}

func renderNotice(omitted, total int, shortened bool) string {
	clause := ""
	if shortened {
		clause = shortenedClause
	}
	return fmt.Sprintf(omissionNoticeTemplate, omitted, total, clause)
}

// splitSections splits document at lines starting with "## ". Content
// before the first such line, when non-empty, is the preamble section. A
// document whose first line already begins with "## " has no preamble. A
// document with no such line is one section.
func splitSections(document string) []string {
	var headings []int
	for i := range document {
		if i != 0 && document[i-1] != '\n' {
			continue
		}
		if strings.HasPrefix(document[i:], "## ") {
			headings = append(headings, i)
		}
	}
	if len(headings) == 0 {
		return []string{document}
	}

	var sections []string
	if headings[0] > 0 {
		sections = append(sections, document[:headings[0]])
	}
	for i, start := range headings {
		end := len(document)
		if i+1 < len(headings) {
			end = headings[i+1]
		}
		sections = append(sections, document[start:end])
	}
	return sections
}

// splitLines splits s into complete lines, each running up to and including
// its trailing "\n", except a final line with no terminator, which runs to
// s's end.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i+1])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func reverseInts(s []int) []int {
	out := make([]int, len(s))
	for i, v := range s {
		out[len(s)-1-i] = v
	}
	return out
}
