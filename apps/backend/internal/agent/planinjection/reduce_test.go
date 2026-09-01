package planinjection

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/sysprompt"
)

// wantNotice mirrors the fixed template from the requirement's Terminology
// section independently of renderNotice, so tests assert against the frozen
// contract text rather than against whatever the implementation produces.
func wantNotice(omitted, total int, shortened bool) string {
	clause := ""
	if shortened {
		clause = ", and the retained section was shortened"
	}
	return fmt.Sprintf("[Kandev: plan reduced to fit the injection budget; %d of %d sections omitted%s. Call get_task_plan_kandev for the full plan.]", omitted, total, clause)
}

const wantCutMarker = "[Kandev: section truncated here]"

// buildDoc returns a document with exactly n sections, each headed
// "## Section i" and carrying a body of bodyLen 'x' bytes. The first line
// begins with "## ", so the document has no preamble.
func buildDoc(n, bodyLen int) string {
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&sb, "## Section %d\n", i)
		sb.WriteString(strings.Repeat("x", bodyLen))
		sb.WriteString("\n")
	}
	return sb.String()
}

func TestReduceReturnsUnchangedUnderBudget(t *testing.T) {
	doc := buildDoc(3, 50)
	out, reduced, omitted := Reduce(doc, len(doc)+10)
	if out != doc {
		t.Fatalf("output = %q, want the document unchanged", out)
	}
	if reduced {
		t.Fatal("reduced = true for a document under budget")
	}
	if omitted != 0 {
		t.Fatalf("omitted = %d, want 0", omitted)
	}
}

func TestReduceReturnsUnchangedAtExactBudget(t *testing.T) {
	doc := buildDoc(2, 20)
	out, reduced, _ := Reduce(doc, len(doc))
	if out != doc || reduced {
		t.Fatalf("equality with the budget must be treated as within budget; out=%q reduced=%v", out, reduced)
	}
}

func TestReduceReturnsNothingForEmptyDocument(t *testing.T) {
	out, reduced, omitted := Reduce("", 100)
	if out != "" || reduced || omitted != 0 {
		t.Fatalf("Reduce(\"\", 100) = (%q, %v, %d), want (\"\", false, 0)", out, reduced, omitted)
	}
}

func TestReduceReturnsNothingForWhitespaceOnlyDocument(t *testing.T) {
	out, reduced, omitted := Reduce("   \n\t \n", 100)
	if out != "" || reduced || omitted != 0 {
		t.Fatalf("Reduce(whitespace, 100) = (%q, %v, %d), want (\"\", false, 0)", out, reduced, omitted)
	}
}

func TestReduceRetainsTailWhenFirstSectionIsOversized(t *testing.T) {
	// The 71%-of-plans trap: an oversized first section must not force the
	// intra-section fallback when a later section fits under AC-001.6.
	doc := "## Old context\n" + strings.Repeat("a", 5000) + "\n" +
		"## Working notes\n" + strings.Repeat("b", 5000) + "\n" +
		"## Current state\nPR #123 opened.\n"

	out, reduced, omitted := Reduce(doc, 200)

	if !reduced {
		t.Fatal("reduced = false, want true")
	}
	if strings.Contains(out, wantCutMarker) {
		t.Fatalf("output reached the intra-section fallback; got %q", out)
	}
	if strings.Contains(out, "Old context") || strings.Contains(out, "Working notes") {
		t.Fatalf("output retains a dropped section: %q", out)
	}
	if !strings.Contains(out, "Current state") || !strings.Contains(out, "PR #123 opened.") {
		t.Fatalf("output is missing the retained tail section: %q", out)
	}
	if want := wantNotice(omitted, 3, false); !strings.HasSuffix(out, want) {
		t.Fatalf("output does not end with the expected notice %q; got %q", want, out)
	}
	if len(out) > 200 {
		t.Fatalf("len(out) = %d, exceeds budget 200", len(out))
	}
}

func TestReduceFallsBackToFirstSectionWhenNothingWholeFits(t *testing.T) {
	doc := "## Old context\n" + strings.Repeat("a", 5000) + "\n" +
		"## Working notes\n" + strings.Repeat("b", 5000) + "\n" +
		"## Current state\n" + strings.Repeat("c", 5000) + "\n"

	out, reduced, omitted := Reduce(doc, 300)

	if !reduced {
		t.Fatal("reduced = false, want true")
	}
	if !strings.Contains(out, wantCutMarker) {
		t.Fatalf("output did not reach the intra-section fallback; got %q", out)
	}
	if strings.Contains(out, "Working notes") || strings.Contains(out, "Current state") {
		t.Fatalf("output retains a section beyond the shortened first one: %q", out)
	}
	if omitted != 2 {
		t.Fatalf("omitted = %d, want {total}-1 = 2", omitted)
	}
	if want := wantNotice(2, 3, true); !strings.HasSuffix(out, want) {
		t.Fatalf("output does not end with the expected notice %q; got %q", want, out)
	}
	if len(out) > 300 {
		t.Fatalf("len(out) = %d, exceeds budget 300", len(out))
	}
}

func TestReduceOmittedCountOnIntraSectionPathIsNotZero(t *testing.T) {
	doc := buildDoc(4, 5000)
	out, reduced, omitted := Reduce(doc, 300)
	if !reduced {
		t.Fatal("reduced = false, want true")
	}
	if omitted != 3 {
		t.Fatalf("omitted = %d, want {total}-1 = 3 (not 0)", omitted)
	}
	if !strings.Contains(out, wantCutMarker) {
		t.Fatal("expected the intra-section fallback to have run")
	}
}

func TestReduceOmittedCountOnSingleSectionFallbackIsZero(t *testing.T) {
	doc := "## Only section\n" + strings.Repeat("x", 5000) + "\n"
	out, reduced, omitted := Reduce(doc, 300)
	if !reduced {
		t.Fatal("reduced = false, want true")
	}
	if omitted != 0 {
		t.Fatalf("omitted = %d, want 0 for a single-section document", omitted)
	}
	if want := wantNotice(0, 1, true); !strings.HasSuffix(out, want) {
		t.Fatalf("output does not end with the expected notice %q; got %q", want, out)
	}
}

func TestReduceNeverExceedsBudgetAtNarrowSectionCounts(t *testing.T) {
	// N=10 and N=100 hide the missing-separator defect on the intra-section
	// path; N=2, 3, 11 do not, because the reserved and rendered notice
	// widths coincide only at powers of ten.
	for _, n := range []int{2, 3, 11} {
		n := n
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			doc := buildDoc(n, 5000)
			out, reduced, omitted := Reduce(doc, 300)
			if !reduced {
				t.Fatal("reduced = false, want true")
			}
			if len(out) > 300 {
				t.Fatalf("len(out) = %d, exceeds budget 300 at n=%d", len(out), n)
			}
			if omitted != n-1 {
				t.Fatalf("omitted = %d, want %d", omitted, n-1)
			}
		})
	}
}

func TestReduceNeverExceedsBudgetAtWideSectionCount(t *testing.T) {
	// A three-digit section count exposes a fixed-width notice reservation.
	doc := buildDoc(150, 30)
	out, reduced, _ := Reduce(doc, 400)
	if !reduced {
		t.Fatal("reduced = false, want true")
	}
	if len(out) > 400 {
		t.Fatalf("len(out) = %d, exceeds budget 400", len(out))
	}
}

func TestReduceIsDeterministic(t *testing.T) {
	doc := buildDoc(20, 300)
	out1, reduced1, omitted1 := Reduce(doc, 1000)
	out2, reduced2, omitted2 := Reduce(doc, 1000)
	if out1 != out2 || reduced1 != reduced2 || omitted1 != omitted2 {
		t.Fatalf("Reduce is not deterministic: (%q,%v,%d) vs (%q,%v,%d)", out1, reduced1, omitted1, out2, reduced2, omitted2)
	}
}

func TestReduceIsIdempotentOnReducedOutput(t *testing.T) {
	doc := buildDoc(20, 300)
	out, reduced, _ := Reduce(doc, 1000)
	if !reduced {
		t.Fatal("fixture did not actually reduce; strengthen it")
	}
	out2, reduced2, omitted2 := Reduce(out, 1000)
	if out2 != out {
		t.Fatalf("re-reducing the output changed it: %q -> %q", out, out2)
	}
	if reduced2 {
		t.Fatal("re-reducing an already-bounded output reported reduced=true")
	}
	if omitted2 != 0 {
		t.Fatalf("re-reducing an already-bounded output reported omitted=%d, want 0", omitted2)
	}
}

func TestReduceTreatsNoHeadingLineDocumentAsOneSection(t *testing.T) {
	doc := strings.Repeat("no headings here at all\n", 400)
	out, reduced, omitted := Reduce(doc, 300)
	if !reduced {
		t.Fatal("reduced = false, want true")
	}
	if omitted != 0 {
		t.Fatalf("omitted = %d, want 0 for a single-section document", omitted)
	}
	if !strings.Contains(out, wantCutMarker) {
		t.Fatal("expected the intra-section fallback for a headingless document")
	}
}

func TestReduceRetainsPreambleAsFirstSection(t *testing.T) {
	doc := strings.Repeat("intro ", 20) + "\n## Heading\n" + strings.Repeat("body ", 20) + "\n"
	sections := splitSections(doc)
	if len(sections) != 2 {
		t.Fatalf("splitSections returned %d sections, want 2 (preamble + heading)", len(sections))
	}
	if strings.HasPrefix(sections[0], "## ") {
		t.Fatalf("section[0] should be the preamble, got %q", sections[0])
	}
}

func TestSplitSectionsGivesNoZeroLengthPreambleWhenDocStartsWithHeading(t *testing.T) {
	doc := "## Heading\nbody\n"
	sections := splitSections(doc)
	if len(sections) != 1 {
		t.Fatalf("splitSections returned %d sections, want 1 (no preamble)", len(sections))
	}
	if sections[0] != doc {
		t.Fatalf("sections[0] = %q, want the whole document", sections[0])
	}
}

func TestReduceOutputIsValidUTF8ForMultibyteInput(t *testing.T) {
	rune3byte := "中" // a 3-byte UTF-8 rune
	doc := "## Section\n" + strings.Repeat(rune3byte, 3000) + "\n"
	out, reduced, _ := Reduce(doc, 200)
	if !reduced {
		t.Fatal("reduced = false, want true")
	}
	if !utf8.ValidString(out) {
		t.Fatalf("output is not valid UTF-8: %q", out)
	}
}

func TestReduceReturnsNothingForNonPositiveBudget(t *testing.T) {
	doc := buildDoc(3, 100)
	for _, budget := range []int{0, -5} {
		out, reduced, omitted := Reduce(doc, budget)
		if out != "" {
			t.Fatalf("budget=%d: out = %q, want \"\"", budget, out)
		}
		if !reduced {
			t.Fatalf("budget=%d: reduced = false, want true (content was dropped)", budget)
		}
		if omitted != 3 {
			t.Fatalf("budget=%d: omitted = %d, want 3 (total, nothing represented)", budget, omitted)
		}
	}
}

func TestReduceReturnsNothingWhenBudgetBelowNoticeReservation(t *testing.T) {
	doc := buildDoc(3, 100)
	reservation := len(renderNotice(3, 3, true)) + 1
	out, reduced, omitted := Reduce(doc, reservation-1)
	if out != "" || !reduced || omitted != 3 {
		t.Fatalf("Reduce below the notice reservation = (%q, %v, %d), want (\"\", true, 3)", out, reduced, omitted)
	}
}

func TestReduceReturnsNothingWhenBudgetHoldsNoticeButNotMarker(t *testing.T) {
	doc := buildDoc(3, 100)
	noticeReservation := len(renderNotice(3, 3, true)) + 1
	markerReservation := len(cutMarker) + 1
	budget := noticeReservation + markerReservation - 1
	out, reduced, omitted := Reduce(doc, budget)
	if out != "" || !reduced || omitted != 3 {
		t.Fatalf("Reduce holding notice but not notice+marker = (%q, %v, %d), want (\"\", true, 3)", out, reduced, omitted)
	}
}

func TestReduceReturnsNothingWhenBudgetHoldsBothReservationsButNotOneLine(t *testing.T) {
	// A single line far larger than what's left after both reservations.
	doc := "## Section\n" + strings.Repeat("x", 5000) + "\n"
	noticeReservation := len(renderNotice(1, 1, true)) + 1
	markerReservation := len(cutMarker) + 1
	budget := noticeReservation + markerReservation + 5
	out, reduced, omitted := Reduce(doc, budget)
	if out != "" || !reduced || omitted != 1 {
		t.Fatalf("Reduce with no complete line fitting = (%q, %v, %d), want (\"\", true, 1)", out, reduced, omitted)
	}
}

func TestReduceOutputNeverEndsWithTrailingNewline(t *testing.T) {
	doc := buildDoc(20, 300)
	out, reduced, _ := Reduce(doc, 1000)
	if !reduced {
		t.Fatal("fixture did not actually reduce; strengthen it")
	}
	if strings.HasSuffix(out, "\n") {
		t.Fatalf("output ends with a trailing newline: %q", out)
	}
}

func TestReduceAssemblyInsertsNoSeparatorBetweenRetainedSections(t *testing.T) {
	// A small first section and oversized later ones force the tail run
	// closed immediately, so every remaining turn goes to the head run and
	// the first section is what gets retained.
	doc := "## Small\nfits easily\n" +
		"## Big one\n" + strings.Repeat("a", 5000) + "\n" +
		"## Big two\n" + strings.Repeat("b", 5000) + "\n"
	sections := splitSections(doc)
	out, reduced, _ := Reduce(doc, 300)
	if !reduced {
		t.Fatal("fixture did not actually reduce; strengthen it")
	}
	if !strings.HasPrefix(out, sections[0]) {
		t.Fatalf("output does not start with the retained head section verbatim: %q", out)
	}
}

func TestReduceInvariantsAcrossShapesAndBudgets(t *testing.T) {
	budgets := []int{-5, 0, 50, 100, 161, 194, 200, 500, 4000, 12000}
	sectionCounts := []int{1, 2, 3, 10, 11, 100, 101, 150}
	bodyLens := []int{5, 50, 500}

	for _, n := range sectionCounts {
		for _, bodyLen := range bodyLens {
			doc := buildDoc(n, bodyLen)
			for _, budget := range budgets {
				name := fmt.Sprintf("n=%d/body=%d/budget=%d", n, bodyLen, budget)
				t.Run(name, func(t *testing.T) {
					out, reduced, _ := Reduce(doc, budget)

					if budget >= 0 && len(out) > budget {
						t.Fatalf("len(out)=%d exceeds budget=%d", len(out), budget)
					}
					if !utf8.ValidString(out) {
						t.Fatalf("output is not valid UTF-8: %q", out)
					}
					if reduced && strings.HasSuffix(out, "\n") {
						t.Fatalf("reduced output ends with a trailing newline: %q", out)
					}
					if reduced != strings.Contains(out, "sections omitted") && out != "" {
						t.Fatalf("notice presence does not match reduced=%v: %q", reduced, out)
					}
					if !reduced && strings.Contains(out, "sections omitted") {
						t.Fatal("notice present without a reduction")
					}

					out2, reduced2, omitted2 := Reduce(doc, budget)
					if out2 != out || reduced2 != reduced {
						t.Fatal("Reduce is not deterministic across repeat calls")
					}

					if reduced && out != "" {
						out3, reduced3, _ := Reduce(out, budget)
						if reduced3 || out3 != out {
							t.Fatalf("Reduce is not idempotent on its own output: %q -> %q", out, out3)
						}
					}
					_ = omitted2
				})
			}
		}
	}
}

func TestContainTagsRemovesCrossNestedSingleLiteral(t *testing.T) {
	in := "<kandev<kandev-system>-system>"
	if got := ContainTags(in); got != "" {
		t.Fatalf("ContainTags(%q) = %q, want empty string", in, got)
	}
}

func TestContainTagsRemovesStartThenEndConstructedPair(t *testing.T) {
	in := "<kandev" + "</kandev-system>" + "-system>"
	if got := ContainTags(in); got != "" {
		t.Fatalf("ContainTags(%q) = %q, want empty string", in, got)
	}
}

func TestContainTagsRemovesEndThenStartConstructedPair(t *testing.T) {
	in := "</kandev" + "<kandev-system>" + "-system>"
	if got := ContainTags(in); got != "" {
		t.Fatalf("ContainTags(%q) = %q, want empty string", in, got)
	}
}

func TestContainTagsLeavesOrdinaryTextUnchanged(t *testing.T) {
	in := "Nothing suspicious here, just plan prose about the kandev-system tag."
	if got := ContainTags(in); got != in {
		t.Fatalf("ContainTags changed ordinary text: %q -> %q", in, got)
	}
}

func TestContainTagsIsCaseSensitiveExactLiteral(t *testing.T) {
	in := "<KANDEV-SYSTEM>not the real tag</KANDEV-SYSTEM>"
	if got := ContainTags(in); got != in {
		t.Fatalf("ContainTags changed a case-variant literal it should have left alone: %q -> %q", in, got)
	}
}

func TestContainTagsRemovesRealTagLiterals(t *testing.T) {
	in := sysprompt.TagStart + "content" + sysprompt.TagEnd
	if got := ContainTags(in); got != "content" {
		t.Fatalf("ContainTags(%q) = %q, want %q", in, got, "content")
	}
}
