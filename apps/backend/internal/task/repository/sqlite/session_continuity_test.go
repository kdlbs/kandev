package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
)

func TestContinuationSnapshotBoundedCanonicalHistory(t *testing.T) {
	input := lifecycle.ContinuationSnapshotInput{
		TaskObjective:     "Implement durable session recovery",
		Plan:              strings.Repeat("plan section\n", 2000),
		OriginalWorkspace: "/workspace/task",
		Messages: []*models.Message{
			{ID: "old", AuthorType: "user", Content: strings.Repeat("old message ", 12000)},
			{ID: "new", AuthorType: "agent", Content: "The latest complete result."},
		},
	}

	snapshot := lifecycle.BuildContinuationSnapshot(input)
	if snapshot.ByteCount > 64*1024 {
		t.Fatalf("snapshot byte count = %d, want <= 65536", snapshot.ByteCount)
	}
	if snapshot.ContentHash == "" || snapshot.SourceMessageID != "new" {
		t.Fatalf("snapshot identity = hash %q/source %q", snapshot.ContentHash, snapshot.SourceMessageID)
	}
	if !snapshot.Truncated || snapshot.OmittedMessages == 0 {
		t.Fatalf("snapshot truncation = %+v, want bounded omitted history", snapshot)
	}
	if strings.Contains(snapshot.Content, "<kandev-system>") || strings.Contains(snapshot.Content, "</kandev-system>") {
		t.Fatal("snapshot retained a system boundary tag")
	}
}

func TestContinuationCheckpointCrashSafety(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-continuity", Title: "Continuity"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-continuity", TaskID: "task-continuity"}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	attempt := &models.RestoreAttempt{
		ID:                 "attempt-continuity",
		SessionID:          "session-continuity",
		IncarnationID:      "incarnation-1",
		ExpectedGeneration: 0,
		Action:             "continue_from_history",
		Outcome:            models.ContinuitySnapshotPrepared,
		Authorized:         true,
		CreatedAt:          time.Now().UTC(),
	}
	if err := repo.CreateRestoreAttempt(ctx, attempt); err != nil {
		t.Fatalf("CreateRestoreAttempt: %v", err)
	}
	snapshot := &models.ContinuationSnapshot{
		AttemptID:   attempt.ID,
		SessionID:   attempt.SessionID,
		Content:     "canonical history",
		ByteCount:   len("canonical history"),
		ContentHash: "hash",
		Status:      models.ContinuitySnapshotPrepared,
	}
	if err := repo.CreateContinuationSnapshot(ctx, snapshot); err != nil {
		t.Fatalf("CreateContinuationSnapshot: %v", err)
	}
	got, err := repo.GetContinuationSnapshot(ctx, snapshot.ID)
	if err != nil {
		t.Fatalf("GetContinuationSnapshot: %v", err)
	}
	if got.Content != snapshot.Content || got.Status != models.ContinuitySnapshotPrepared {
		t.Fatalf("snapshot after checkpoint = %+v", got)
	}
	if _, err := repo.GetContinuationSnapshot(ctx, "missing"); err == nil {
		t.Fatal("missing continuation snapshot unexpectedly succeeded")
	}
}

func TestSessionRecoveryBlockLookupPreservesResolvedState(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-recovery-block", Title: "Recovery"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-recovery-block", TaskID: "task-recovery-block"}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	block := &models.SessionRecoveryBlock{
		ID:                 "block-recovery",
		SessionID:          "session-recovery-block",
		IncarnationID:      "incarnation-1",
		ExpectedGeneration: 0,
		Reason:             "native_state_missing",
		State:              models.RecoveryBlockOpen,
	}
	if err := repo.UpsertSessionRecoveryBlock(ctx, block); err != nil {
		t.Fatalf("UpsertSessionRecoveryBlock: %v", err)
	}
	if _, err := repo.ResolveSessionRecoveryBlock(ctx, block.ID, "continue_from_history", time.Now().UTC()); err != nil {
		t.Fatalf("ResolveSessionRecoveryBlock: %v", err)
	}
	got, err := repo.GetSessionRecoveryBlock(ctx, block.ID)
	if err != nil {
		t.Fatalf("GetSessionRecoveryBlock: %v", err)
	}
	if got.State != models.RecoveryBlockResolved || got.AuthorizedAction != "continue_from_history" {
		t.Fatalf("resolved block = %+v", got)
	}
}

func TestSessionRecoveryBlockUpsertRetainsCanonicalIdentity(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-recovery-block-idempotent", Title: "Recovery"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-recovery-block-idempotent", TaskID: "task-recovery-block-idempotent",
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	first := &models.SessionRecoveryBlock{
		SessionID:          "session-recovery-block-idempotent",
		IncarnationID:      "incarnation-1",
		ExpectedGeneration: 4,
		Reason:             "native_state_missing",
		State:              models.RecoveryBlockOpen,
		ConsumerReference:  "run-1",
	}
	if err := repo.UpsertSessionRecoveryBlock(ctx, first); err != nil {
		t.Fatalf("first UpsertSessionRecoveryBlock: %v", err)
	}
	if first.ID == "" {
		t.Fatal("first upsert did not assign an ID")
	}

	second := &models.SessionRecoveryBlock{
		ID:                 "retry-generated-id",
		SessionID:          first.SessionID,
		IncarnationID:      first.IncarnationID,
		ExpectedGeneration: first.ExpectedGeneration,
		Reason:             first.Reason,
		State:              models.RecoveryBlockOpen,
		ConsumerReference:  "run-2",
	}
	if err := repo.UpsertSessionRecoveryBlock(ctx, second); err != nil {
		t.Fatalf("conflicting UpsertSessionRecoveryBlock: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("conflicting upsert ID = %q, want canonical ID %q", second.ID, first.ID)
	}

	got, err := repo.GetSessionRecoveryBlock(ctx, first.ID)
	if err != nil {
		t.Fatalf("GetSessionRecoveryBlock: %v", err)
	}
	if got.ConsumerReference != "run-2" {
		t.Fatalf("updated recovery block = %+v, want the conflicting write to update the canonical row", got)
	}
	if _, err := repo.GetSessionRecoveryBlock(ctx, "retry-generated-id"); err == nil {
		t.Fatal("conflicting upsert created a second recovery block identity")
	}
}
