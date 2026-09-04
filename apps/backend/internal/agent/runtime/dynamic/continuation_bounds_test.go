package dynamic

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestBoundedTruncatesOnRuneBoundary proves bounded() never splits a
// multi-byte rune. Vietnamese (3-byte) and CJK (3-byte) runes are repeated at
// every byte alignment relative to continuationFieldLimit by padding the
// prefix with 0..3 ASCII bytes, so the limit boundary falls inside a rune at
// some alignment if truncation is not rune-safe.
func TestBoundedTruncatesOnRuneBoundary(t *testing.T) {
	vietnamese := "Xin chào các bạn, đây là một đoạn văn bản tiếng Việt có dấu để kiểm tra việc cắt chuỗi theo byte thay vì theo ký tự Unicode. "
	cjk := "这是一段中文文本用来测试按字节截断而不是按字符截断可能导致的无效UTF八编码问题。"

	for _, sample := range []struct {
		name string
		text string
	}{
		{"vietnamese", vietnamese},
		{"cjk", cjk},
	} {
		repeated := strings.Repeat(sample.text, 200)
		for alignment := 0; alignment < 4; alignment++ {
			padded := strings.Repeat("x", alignment) + repeated
			got := bounded(padded)
			if !utf8.ValidString(got) {
				t.Fatalf("%s alignment=%d: bounded() produced invalid UTF-8: %q", sample.name, alignment, got)
			}
			tail := boundedTailN(padded, continuationFieldLimit)
			if !utf8.ValidString(tail) {
				t.Fatalf("%s alignment=%d: boundedTailN() produced invalid UTF-8: %q", sample.name, alignment, tail)
			}
		}
	}
}

// TestBoundedConversationRetainsNewestContent is the defect-C regression:
// Conversation must keep the tail (most recent turns), not the head. Filler
// is space-separated tokens rather than one long run of a single character:
// a bare run of 32+ alnum characters matches routingerr's generic
// credential-shaped rule and would redact to a few bytes regardless of
// position, which would make this test assert nothing about truncation.
func TestBoundedConversationRetainsNewestContent(t *testing.T) {
	oldest := "OLDEST-MARKER " + strings.Repeat("aa ", continuationFieldLimit/3)
	newest := strings.Repeat("bb ", continuationFieldLimit/3) + " NEWEST-MARKER"

	continuation := BuildBoundedContinuation(ContinuationInput{
		Conversation: oldest + "\n" + newest,
	})

	if strings.Contains(continuation.Conversation, "OLDEST-MARKER") {
		t.Fatalf("Conversation kept the oldest content: %q", continuation.Conversation)
	}
	if !strings.Contains(continuation.Conversation, "NEWEST-MARKER") {
		t.Fatalf("Conversation dropped the newest content: %q", continuation.Conversation)
	}
}

// TestBoundedConversationRetainsUserMessagesWhenConversationOverflows is the
// R1-2 regression: once the agent conversation alone reaches the field
// limit, user messages must still survive on their own budget rather than
// being crowded out by a single tail cut over the concatenated string. The
// newest-agent-turn marker proves the independent tail budget, not just a
// head-truncating cut that happens to keep both user messages: a plain
// head-cut over the concatenated string would keep the user messages (they
// sort first) but drop the newest agent content, which this also checks.
func TestBoundedConversationRetainsUserMessagesWhenConversationOverflows(t *testing.T) {
	overflowingConversation := strings.Repeat("agent: working on it. ", 500) + "NEWEST-AGENT-MARKER" // well over continuationFieldLimit

	continuation := BuildBoundedContinuation(ContinuationInput{
		UserMessages: []string{"USERMSG-1: please fix the bug", "USERMSG-2: also add a test"},
		Conversation: overflowingConversation,
	})

	if !strings.Contains(continuation.Conversation, "USERMSG-1") {
		t.Fatalf("Conversation dropped USERMSG-1: %q", continuation.Conversation)
	}
	if !strings.Contains(continuation.Conversation, "USERMSG-2") {
		t.Fatalf("Conversation dropped USERMSG-2: %q", continuation.Conversation)
	}
	if !strings.Contains(continuation.Conversation, "NEWEST-AGENT-MARKER") {
		t.Fatalf("Conversation dropped the newest agent turn: %q", continuation.Conversation)
	}
	if len(continuation.Conversation) > continuationFieldLimit {
		t.Fatalf("Conversation exceeded continuationFieldLimit: %d bytes", len(continuation.Conversation))
	}
}

// TestBoundedConversationValidUTF8WhenSanitizeGrowsPastRawLimit is the R2-1
// regression: routingerr.Sanitize can grow its input past MaxRawExcerptBytes
// (redacting "/Users/henry/" to the longer "/Users/<redacted>/") and then
// truncates with a bare byte slice, which can split a multi-byte rune at the
// tail. boundedTailN only repairs the leading cut, so that broken tail must
// otherwise survive into Continuation.Conversation. Swept across byte
// alignments the way TestBoundedTruncatesOnRuneBoundary sweeps bounded(): the
// redaction-dense unit is repeated well past continuationFieldLimit so the
// budget's internal tail-cut still lands in a redaction-growing, multi-byte
// region regardless of the alignment prefix.
func TestBoundedConversationValidUTF8WhenSanitizeGrowsPastRawLimit(t *testing.T) {
	unit := "/Users/henry/p à"

	for alignment := 0; alignment < 12; alignment++ {
		pad := strings.Repeat("x", alignment)
		conversation := pad + strings.Repeat(unit, 300)

		continuation := BuildBoundedContinuation(ContinuationInput{Conversation: conversation})
		if !utf8.ValidString(continuation.Conversation) {
			t.Errorf("alignment=%d: Conversation is invalid UTF-8: %q", alignment, continuation.Conversation)
		}
	}
}

// TestSanitizedTailRetainsNewestContentWhenRedactionsGrowPastRawLimit is the
// R2-A regression: routingerr.Sanitize can grow its input past
// routingerr.MaxRawExcerptBytes (each "/Users/henry/" redaction to the longer
// "/Users/<redacted>/" adds bytes) and then head-truncates its own result,
// which chopped the newest turns before sanitizedTail's own tail cut ever
// ran. sanitizedTail must retry with a smaller window until Sanitize stops
// truncating, so the newest content always survives regardless of how many
// growing redactions the window contains.
func TestSanitizedTailRetainsNewestContentWhenRedactionsGrowPastRawLimit(t *testing.T) {
	for _, homePaths := range []int{0, 1, 5, 20, 100} {
		conversation := strings.Repeat("agent: thinking about the change. ", 200) +
			strings.Repeat("agent: edited /Users/henry/proj/file.go ok. ", homePaths) +
			strings.Repeat("agent: nearly done. ", 20) + "NEWEST-MARKER-END"

		continuation := BuildBoundedContinuation(ContinuationInput{Conversation: conversation})

		if !strings.Contains(continuation.Conversation, "NEWEST-MARKER-END") {
			t.Fatalf("homePaths=%d: Conversation dropped the newest content: %q", homePaths, continuation.Conversation)
		}
	}
}

// TestBoundedConversationSurvivesLargeLeadingWhitespace is the F2 regression:
// sanitizedTail computed truncatedWindow from len(raw) while boundedTailN
// trims the value first, so whitespace padding alone (never itself
// truncated) could make truncatedWindow spuriously true. windowGuard was
// then prepended to content that was never actually cut by the window, the
// generic redaction rule swallowed guard+content into a single match, and
// stripping the guard prefix from that match discarded the user's only
// message entirely.
func TestBoundedConversationSurvivesLargeLeadingWhitespace(t *testing.T) {
	message := strings.Repeat(" ", 2513) + "hello"

	continuation := BuildBoundedContinuation(ContinuationInput{
		UserMessages: []string{message},
	})

	if !strings.Contains(continuation.Conversation, "hello") {
		t.Fatalf("Conversation dropped the only user message: %q", continuation.Conversation)
	}
}

// TestBoundedTaskDescriptionKeepsHead pins the unchanged head-keep contract
// for every field other than Conversation (PlanSummary is separately pinned
// by TestBoundedIsNoOpOnReducedPlanOutput).
func TestBoundedTaskDescriptionKeepsHead(t *testing.T) {
	continuation := BuildBoundedContinuation(ContinuationInput{
		TaskDescription: "HEAD-MARKER " + strings.Repeat("a", continuationFieldLimit) + " TAIL-MARKER",
	})
	if !strings.Contains(continuation.TaskDescription, "HEAD-MARKER") {
		t.Fatalf("TaskDescription dropped the head: %q", continuation.TaskDescription)
	}
	if strings.Contains(continuation.TaskDescription, "TAIL-MARKER") {
		t.Fatalf("TaskDescription unexpectedly kept the tail: %q", continuation.TaskDescription)
	}
}

// TestBoundedConversationRetainsBudgetVolumeRegardlessOfNewlinePlacement is
// the Review-round-1 regression: dropLeadingPartialLine used to discard a
// truncated window's front up to its first newline with no bound on how
// much, so a conversation whose only newline sits near the very start of the
// kept window threw away nearly the whole budget. Content is built from
// space-separated tokens (never a run of 32+ non-space characters) so the
// generic credential-shaped redaction rule never fires here — this test is
// about retained volume under truncation, not about redaction, and a
// collision with that rule would make the byte-count assertion meaningless.
// Assertions check retained BYTE COUNT, not marker presence, so a fix that
// keeps only a small fragment cannot pass vacuously.
func TestBoundedConversationRetainsBudgetVolumeRegardlessOfNewlinePlacement(t *testing.T) {
	const minKept = continuationFieldLimit - 8

	t.Run("single_newline_near_front_of_long_tail", func(t *testing.T) {
		// 10003 bytes, budget 4000; the only "\n" sits right after 10000
		// bytes of leading content, so it is nowhere near the kept window.
		conv := strings.Repeat("word ", 2000) + "\nok"
		continuation := BuildBoundedContinuation(ContinuationInput{Conversation: conv})
		if got := len(continuation.Conversation); got < minKept {
			t.Fatalf("kept only %d of a %d-byte budget: %q", got, continuationFieldLimit, continuation.Conversation)
		}
	})

	t.Run("many_long_lines", func(t *testing.T) {
		// 20 lines of 1500 bytes each (30020 bytes total, budget 4000); the
		// kept window straddles several line boundaries.
		line := strings.Repeat("word ", 300)
		conv := strings.Repeat(line+"\n", 20)
		continuation := BuildBoundedContinuation(ContinuationInput{Conversation: conv})
		if got := len(continuation.Conversation); got < minKept {
			t.Fatalf("kept only %d of a %d-byte budget: %q", got, continuationFieldLimit, continuation.Conversation)
		}
	})

	// Controls: shapes that must stay healthy after the fix.
	t.Run("control_normal_multiline", func(t *testing.T) {
		conv := strings.Repeat("agent turn discussing the change in detail.\n", 400) + "NEWEST-TAIL-LINE"
		continuation := BuildBoundedContinuation(ContinuationInput{Conversation: conv})
		if got := len(continuation.Conversation); got != continuationFieldLimit {
			t.Fatalf("control regressed: kept %d bytes, want %d", got, continuationFieldLimit)
		}
		if !strings.Contains(continuation.Conversation, "NEWEST-TAIL-LINE") {
			t.Fatalf("control regressed: dropped the newest line: %q", continuation.Conversation)
		}
	})

	t.Run("control_single_long_line_no_newline", func(t *testing.T) {
		conv := strings.Repeat("b ", 6000)
		continuation := BuildBoundedContinuation(ContinuationInput{Conversation: conv})
		if got := len(continuation.Conversation); got != continuationFieldLimit {
			t.Fatalf("control regressed: kept %d bytes, want %d", got, continuationFieldLimit)
		}
	})
}
