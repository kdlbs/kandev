package store

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

func TestMCPSelectionStateRoundTrip(t *testing.T) {
	repo := newTestRepo(t).(*sqliteRepository)
	ctx := context.Background()
	want := mcpconfig.SessionMCPSelectionState{
		DesiredRevision:     3,
		AppliedRevision:     2,
		ApplyState:          mcpconfig.SessionMCPApplyStateFailed,
		FailureCode:         "session_resume_failed",
		FailureSummary:      "resume failed",
		AttachmentAttemptID: "attempt-3",
	}
	if err := repo.SaveMCPSelectionState(ctx, "session-1", want); err != nil {
		t.Fatalf("SaveMCPSelectionState: %v", err)
	}
	got, err := repo.GetMCPSelectionState(ctx, "session-1")
	if err != nil {
		t.Fatalf("GetMCPSelectionState: %v", err)
	}
	if got != want {
		t.Fatalf("state = %#v, want %#v", got, want)
	}
}

func TestMCPSelectionStateMissingIsTyped(t *testing.T) {
	repo := newTestRepo(t).(*sqliteRepository)
	_, err := repo.GetMCPSelectionState(context.Background(), "missing")
	if !errors.Is(err, mcpconfig.ErrMCPSelectionStateNotFound) {
		t.Fatalf("GetMCPSelectionState error = %v", err)
	}
}
