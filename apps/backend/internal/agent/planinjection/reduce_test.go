package planinjection

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
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

// buildTightFirstSectionDoc returns a document with n sections: the first
// built from many 1-byte lines so the greedy fill in reduceFirstSection can
// exhaust the available budget almost exactly, and n-1 trailing sections
// each too large to be retained whole, forcing AC-001.7's fallback. The
// near-zero slack this leaves is what lets a reservation-arithmetic defect
// be observed at all; buildDoc's two-line sections leave tens of bytes of
// unspent budget, which a one-byte or few-byte reservation error hides in.
func buildTightFirstSectionDoc(n int) string {
	var sb strings.Builder
	sb.WriteString("## First\n")
	sb.WriteString(strings.Repeat("\n", 5000))
	for i := 2; i <= n; i++ {
		fmt.Fprintf(&sb, "## Section %d\n", i)
		sb.WriteString(strings.Repeat("x", 5000))
		sb.WriteString("\n")
	}
	return sb.String()
}

func TestReduceNeverExceedsBudgetAtNarrowSectionCounts(t *testing.T) {
	// N=10 and N=100 hide the missing-separator defect on the intra-section
	// path; N=2, 3, 11, 101 do not, because the reserved and rendered notice
	// widths coincide only at powers of ten.
	for _, n := range []int{2, 3, 11, 101} {
		n := n
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			doc := buildTightFirstSectionDoc(n)
			out, reduced, omitted := Reduce(doc, 300)
			if !reduced {
				t.Fatal("reduced = false, want true")
			}
			if !strings.Contains(out, wantCutMarker) {
				t.Fatal("expected the intra-section fallback to have run")
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
	// A three-digit section count exposes a fixed-width notice reservation;
	// the tight first section leaves no slack for the wider rendered count
	// to hide in.
	doc := buildTightFirstSectionDoc(150)
	out, reduced, omitted := Reduce(doc, 400)
	if !reduced {
		t.Fatal("reduced = false, want true")
	}
	if !strings.Contains(out, wantCutMarker) {
		t.Fatal("expected the intra-section fallback to have run")
	}
	if len(out) > 400 {
		t.Fatalf("len(out) = %d, exceeds budget 400", len(out))
	}
	if omitted != 149 {
		t.Fatalf("omitted = %d, want 149", omitted)
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

// TestReduceOutputIsValidUTF8ForMultibyteInput exercises reduceFirstSection's
// line-boundary cut, not just the empty-output shadow path: the multibyte
// content is spread across many short lines, so at this budget the greedy
// fill retains several whole lines and stops mid-body, forcing the output to
// actually contain multibyte runes. A single 9000-byte line (the previous
// fixture) never fits even one line after reservations, so Reduce returns
// "" under AC-001.13 and utf8.ValidString("") trivially passes regardless of
// whether the cut logic is rune-safe.
func TestReduceOutputIsValidUTF8ForMultibyteInput(t *testing.T) {
	rune3byte := "中" // a 3-byte UTF-8 rune
	var body strings.Builder
	for i := 0; i < 2000; i++ {
		body.WriteString(strings.Repeat(rune3byte, 10))
		body.WriteString("\n")
	}
	doc := "## Section\n" + body.String() +
		"## Oversized\n" + strings.Repeat("x", 90000) + "\n"

	out, reduced, _ := Reduce(doc, 400)
	if !reduced {
		t.Fatal("reduced = false, want true")
	}
	if out == "" {
		t.Fatal("fixture did not exercise the line-boundary cut; strengthen it")
	}
	if !strings.Contains(out, rune3byte) {
		t.Fatal("fixture did not retain any multibyte content; strengthen it")
	}
	if !utf8.ValidString(out) {
		t.Fatalf("output is not valid UTF-8: %q", out)
	}
}

func TestReduceReturnsNothingForNonPositiveBudget(t *testing.T) {
	doc := buildDoc(3, 100)
	for _, budget := range []int{0, -5, math.MinInt, math.MinInt + 50} {
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

// TestReduceSelectionPrefersTailWhenOnlyOneRunCanFit is the AC-001.6
// tail-first discriminator: a document shaped so exactly one of the head or
// tail section can be retained, never both. A head-first implementation
// would retain the opposite section here while still passing every other
// committed assertion (staying under budget, emitting a well-formed notice,
// being deterministic and idempotent) — verified by hand: flipping this
// package's tail-first tie-break to head-first left the rest of this file
// green.
func TestReduceSelectionPrefersTailWhenOnlyOneRunCanFit(t *testing.T) {
	head := "## Framing\n" + strings.Repeat("h", 10) + "\n"
	mid := "## Working notes\n" + strings.Repeat("m", 4000) + "\n"
	tail := "## Current state\n" + strings.Repeat("t", 40) + "\n"
	doc := head + mid + tail

	total := 3
	noticeReservation := len(renderNotice(total, total, true)) + 1
	// Sized to hold exactly one of the two small sections plus the notice,
	// never both: the tail-first tie-break is what decides which.
	budget := noticeReservation + len(tail)

	out, reduced, omitted := Reduce(doc, budget)
	if !reduced {
		t.Fatal("reduced = false, want true")
	}
	if !strings.HasPrefix(out, tail) {
		t.Fatalf("output does not retain the tail section first, contrary to AC-001.6's tail-first rule: %q", out)
	}
	if strings.Contains(out, "Framing") {
		t.Fatalf("output retains the head section instead of the tail: %q", out)
	}
	if omitted != 2 {
		t.Fatalf("omitted = %d, want 2 (head and the oversized middle)", omitted)
	}
	if len(out) > budget {
		t.Fatalf("len(out) = %d, exceeds budget %d", len(out), budget)
	}
}

// TestReduceAssemblyConcatenatesMultiSectionHeadAndTailInDocumentOrder covers
// what TestReduceAssemblyInsertsNoSeparatorBetweenRetainedSections's name
// promises but its single-retained-section fixture cannot show: two
// sections in the head run and two in the tail run, both non-empty, with an
// oversized section dropped from the middle. AC-001.6 requires the retained
// sections to be concatenated in original document order with no inserted
// separator.
func TestReduceAssemblyConcatenatesMultiSectionHeadAndTailInDocumentOrder(t *testing.T) {
	s0 := "## H1\nh1\n"
	s1 := "## H2\nh2\n"
	mid := "## Mid\n" + strings.Repeat("m", 5000) + "\n"
	s3 := "## T1\nt1\n"
	s4 := "## T2\nt2\n"
	doc := s0 + s1 + mid + s3 + s4

	total := 5
	noticeReservation := len(renderNotice(total, total, true)) + 1
	// Exactly enough for the four small sections plus the notice; the
	// oversized middle cannot fit alongside them.
	budget := noticeReservation + len(s0) + len(s1) + len(s3) + len(s4)

	out, reduced, omitted := Reduce(doc, budget)
	if !reduced {
		t.Fatal("reduced = false, want true")
	}
	if omitted != 1 {
		t.Fatalf("omitted = %d, want 1 (only the oversized middle section)", omitted)
	}
	want := s0 + s1 + s3 + s4
	if !strings.HasPrefix(out, want) {
		t.Fatalf("retained sections are not concatenated in document order with no separator: got %q, want prefix %q", out, want)
	}
	if strings.Contains(out, "Mid") {
		t.Fatalf("output retains the oversized middle section: %q", out)
	}
	if len(out) > budget {
		t.Fatalf("len(out) = %d, exceeds budget %d", len(out), budget)
	}
}

// TestReduceSelectionClosesRunInsteadOfSkippingNonFittingSection is the
// AC-001.5/AC-001.6 contiguity discriminator: a document shaped [small,
// oversized, small, oversized] where a fitting section sits *behind* a
// non-fitting one within the same run. AC-001.6 requires that run to close
// permanently the moment a candidate does not fit, never skip past it to try
// a smaller one behind it — skipping would retain the second small section
// too, producing a "patchwork" whose hole a reader cannot locate.
// TestReduceAssemblyConcatenatesMultiSectionHeadAndTailInDocumentOrder can't
// catch a skip-instead-of-close regression: its dropped section sits at the
// head/tail boundary, where skipping and closing produce identical output.
func TestReduceSelectionClosesRunInsteadOfSkippingNonFittingSection(t *testing.T) {
	s0 := "## Small1\nfits\n"
	s1 := "## Big1\n" + strings.Repeat("a", 5000) + "\n"
	s2 := "## Small2\nalso\n"
	s3 := "## Big2\n" + strings.Repeat("b", 5000) + "\n"
	doc := s0 + s1 + s2 + s3

	out, reduced, omitted := Reduce(doc, 250)
	if !reduced {
		t.Fatal("reduced = false, want true")
	}
	if !strings.HasPrefix(out, s0) {
		t.Fatalf("output does not retain the head section: %q", out)
	}
	if strings.Contains(out, "Small2") {
		t.Fatalf("output retains a section from behind a non-fitting one in the same run: %q", out)
	}
	if omitted != 3 {
		t.Fatalf("omitted = %d, want 3 (only the head section retained)", omitted)
	}
	if len(out) > 250 {
		t.Fatalf("len(out) = %d, exceeds budget 250", len(out))
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
					out, reduced, omitted := Reduce(doc, budget)

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
					if out2 != out || reduced2 != reduced || omitted2 != omitted {
						t.Fatal("Reduce is not deterministic across repeat calls")
					}

					if reduced && out != "" {
						out3, reduced3, _ := Reduce(out, budget)
						if reduced3 || out3 != out {
							t.Fatalf("Reduce is not idempotent on its own output: %q -> %q", out, out3)
						}
					}
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

// TestContainTagsHandlesLargeAdversarialNestingInLinearTime is a regression
// test for a quadratic-time defect: an implementation that rescans the
// whole text after every removal is O(n^2) on this construction, because
// stripping one layer of nesting exposes the next occurrence only after a
// full rescan. Measured on the rescan-based implementation: a 480,000-byte
// input of this shape took ~5s and the trend was still climbing. A
// linear-time implementation collapses this in well under a second.
func TestContainTagsHandlesLargeAdversarialNestingInLinearTime(t *testing.T) {
	const k = 20000
	input := strings.Repeat("<kandev", k) + strings.Repeat("-system>", k)

	start := time.Now()
	out := ContainTags(input)
	elapsed := time.Since(start)

	if out != "" {
		t.Fatalf("ContainTags did not fully collapse the nested construction; %d bytes remained: %q", len(out), out)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("ContainTags took %v on a %d-byte adversarial input; want well under a second from a linear-time implementation", elapsed, len(input))
	}
}
