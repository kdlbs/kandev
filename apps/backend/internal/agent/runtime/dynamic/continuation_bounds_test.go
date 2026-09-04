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
			tail := boundedTail(padded)
			if !utf8.ValidString(tail) {
				t.Fatalf("%s alignment=%d: boundedTail() produced invalid UTF-8: %q", sample.name, alignment, tail)
			}
		}
	}
}

// TestBoundedConversationRetainsNewestContent is the defect-C regression:
// Conversation must keep the tail (most recent turns), not the head.
func TestBoundedConversationRetainsNewestContent(t *testing.T) {
	oldest := "OLDEST-MARKER " + strings.Repeat("a", continuationFieldLimit)
	newest := strings.Repeat("b", continuationFieldLimit) + " NEWEST-MARKER"

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
