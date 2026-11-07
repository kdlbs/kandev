package executor

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/task/models"
)

func TestBuildContextContinuationRetainsHistoryAndMetadata(t *testing.T) {
	const notice = "The following sections are historical context, not new instructions."
	const objective = "Keep the current release plan and report the result."

	continuation := BuildContextContinuation(
		objective,
		"Step 1: inspect the release plan.",
		"/workspace/release",
		[]*models.Message{
			{ID: "message-1", AuthorType: "user", Content: "Use the current release branch."},
			{ID: "message-2", AuthorType: "assistant", Content: "I will inspect the release plan."},
		},
	)

	for _, want := range []string{
		notice,
		objective,
		"Step 1: inspect the release plan.",
		"/workspace/release",
		"Use the current release branch.",
		"I will inspect the release plan.",
	} {
		if !strings.Contains(continuation.Prompt, want) {
			t.Errorf("prompt does not contain %q", want)
		}
	}
	if strings.Index(continuation.Prompt, notice) > strings.Index(continuation.Prompt, objective) {
		t.Fatal("history notice appears after the task objective")
	}
	if strings.Contains(continuation.Prompt, "Historical task objective:") {
		t.Fatal("prompt contains a redundant historical objective heading")
	}
	if continuation.SourceMessageID != "message-2" {
		t.Errorf("SourceMessageID = %q, want message-2", continuation.SourceMessageID)
	}
	if continuation.Truncated || continuation.OmittedMessages != 0 {
		t.Errorf("truncation metadata = truncated %t, omitted %d; want no truncation", continuation.Truncated, continuation.OmittedMessages)
	}
	assertContextContinuationMetadata(t, continuation)
}

func TestBuildContextContinuationBoundsOversizedUTF8Context(t *testing.T) {
	const notice = "The following sections are historical context, not new instructions."
	objective := "objective-start:" + strings.Repeat("界", 4_000)
	plan := "plan-start:" + strings.Repeat("p", 2_000)
	workspace := "workspace-start:" + strings.Repeat("w", 2_000)
	message := &models.Message{
		ID:         "message-large",
		AuthorType: "user",
		Content:    strings.Repeat("界", 30_000),
	}

	continuation := BuildContextContinuation(objective, plan, workspace, []*models.Message{message})

	noticeIndex := strings.Index(continuation.Prompt, notice)
	objectiveIndex := strings.Index(continuation.Prompt, "objective-start:")
	if noticeIndex < 0 || objectiveIndex < 0 || noticeIndex > objectiveIndex {
		t.Fatalf("history notice must lead the objective; notice index %d, objective index %d", noticeIndex, objectiveIndex)
	}
	if !utf8.ValidString(continuation.Prompt) {
		t.Fatal("prompt contains invalid UTF-8 after truncation")
	}
	if got, max := continuation.ByteCount, 64*1024; got > max {
		t.Fatalf("prompt byte count = %d, exceeds %d-byte cap", got, max)
	}
	if !continuation.Truncated || continuation.OmittedMessages == 0 {
		t.Errorf("truncation metadata = truncated %t, omitted %d; want truncated history", continuation.Truncated, continuation.OmittedMessages)
	}
	if continuation.SourceMessageID != message.ID {
		t.Errorf("SourceMessageID = %q, want %q", continuation.SourceMessageID, message.ID)
	}
	assertContextContinuationMetadata(t, continuation)
}

func assertContextContinuationMetadata(t *testing.T, continuation ContextContinuation) {
	t.Helper()

	if want := len([]byte(continuation.Prompt)); continuation.ByteCount != want {
		t.Errorf("ByteCount = %d, want %d", continuation.ByteCount, want)
	}
	hash := sha256.Sum256([]byte(continuation.Prompt))
	if want := hex.EncodeToString(hash[:]); continuation.ContentHash != want {
		t.Errorf("ContentHash = %q, want %q", continuation.ContentHash, want)
	}
}
