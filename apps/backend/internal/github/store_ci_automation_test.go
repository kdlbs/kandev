package github

import (
	"context"
	"testing"
	"time"
)

func TestStoreTaskCIOptions_DefaultAndUpdate(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	got, err := store.GetTaskCIOptions(ctx, "task-1")
	if err != nil {
		t.Fatalf("get default options: %v", err)
	}
	if got.TaskID != "task-1" {
		t.Fatalf("TaskID=%q, want task-1", got.TaskID)
	}
	if got.AutoFixEnabled || got.AutoMergeEnabled {
		t.Fatalf("default options should be disabled, got %+v", got)
	}
	if got.AutoFixPromptOverride != nil {
		t.Fatalf("default prompt override should be nil, got %q", *got.AutoFixPromptOverride)
	}

	override := "Fix only the new CI feedback."
	updated, err := store.UpdateTaskCIOptions(ctx, "task-1", TaskCIOptionsPatch{
		AutoFixEnabled:        boolPtr(true),
		AutoFixPromptOverride: &override,
	})
	if err != nil {
		t.Fatalf("update options: %v", err)
	}
	if !updated.AutoFixEnabled {
		t.Fatalf("AutoFixEnabled=false, want true")
	}
	if updated.AutoMergeEnabled {
		t.Fatalf("AutoMergeEnabled=true, want unchanged default false")
	}
	if updated.AutoFixPromptOverride == nil || *updated.AutoFixPromptOverride != override {
		t.Fatalf("override=%v, want %q", updated.AutoFixPromptOverride, override)
	}

	enableMerge := true
	clearOverride := ""
	updated, err = store.UpdateTaskCIOptions(ctx, "task-1", TaskCIOptionsPatch{
		AutoMergeEnabled:      &enableMerge,
		AutoFixPromptOverride: &clearOverride,
	})
	if err != nil {
		t.Fatalf("second update options: %v", err)
	}
	if !updated.AutoFixEnabled {
		t.Fatalf("AutoFixEnabled should remain true")
	}
	if !updated.AutoMergeEnabled {
		t.Fatalf("AutoMergeEnabled=false, want true")
	}
	if updated.AutoFixPromptOverride != nil {
		t.Fatalf("override should be cleared, got %q", *updated.AutoFixPromptOverride)
	}
}

func TestStoreTaskCIPRState_RecordAttemptsAndError(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	at := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)

	if err := store.RecordTaskCIFixAttempt(ctx, TaskCIFixAttempt{
		TaskID:         "task-1",
		RepositoryID:   "repo-1",
		PRNumber:       42,
		Signature:      "fix-sig",
		CheckpointJSON: `{"checks":["test"]}`,
		SessionID:      "session-1",
		EnqueuedAt:     at,
	}); err != nil {
		t.Fatalf("record fix attempt: %v", err)
	}
	if err := store.RecordTaskCIMergeAttempt(ctx, TaskCIMergeAttempt{
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		PRNumber:     42,
		Signature:    "merge-sig",
		AttemptedAt:  at.Add(time.Minute),
	}); err != nil {
		t.Fatalf("record merge attempt: %v", err)
	}
	if err := store.RecordTaskCIError(ctx, "task-1", "repo-1", 42, "merge failed"); err != nil {
		t.Fatalf("record error: %v", err)
	}

	state, err := store.GetTaskCIPRState(ctx, "task-1", "repo-1", 42)
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if state == nil {
		t.Fatal("expected state row")
	}
	if state.LastFixSignature != "fix-sig" || state.LastFixCheckpointJSON != `{"checks":["test"]}` {
		t.Fatalf("unexpected fix state: %+v", state)
	}
	if state.LastFixSessionID == nil || *state.LastFixSessionID != "session-1" {
		t.Fatalf("LastFixSessionID=%v, want session-1", state.LastFixSessionID)
	}
	if state.LastMergeSignature != "merge-sig" {
		t.Fatalf("LastMergeSignature=%q, want merge-sig", state.LastMergeSignature)
	}
	if state.LastError == nil || *state.LastError != "merge failed" {
		t.Fatalf("LastError=%v, want merge failed", state.LastError)
	}

	if err := store.ClearTaskCIError(ctx, "task-1", "repo-1", 42); err != nil {
		t.Fatalf("clear error: %v", err)
	}
	state, err = store.GetTaskCIPRState(ctx, "task-1", "repo-1", 42)
	if err != nil {
		t.Fatalf("get state after clear: %v", err)
	}
	if state.LastError != nil {
		t.Fatalf("LastError should be cleared, got %q", *state.LastError)
	}

	states, err := store.ListTaskCIPRStates(ctx, "task-1")
	if err != nil {
		t.Fatalf("list states: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("len(states)=%d, want 1", len(states))
	}
}

func TestStoreTaskCIPRState_RefreshCheckpointPreservesPromptDispatchMetadata(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	enqueuedAt := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)

	if err := store.RecordTaskCIFixAttempt(ctx, TaskCIFixAttempt{
		TaskID:         "task-1",
		RepositoryID:   "repo-1",
		PRNumber:       42,
		Signature:      "before",
		CheckpointJSON: `{"failed_checks":[{"name":"unit"}]}`,
		SessionID:      "session-1",
		EnqueuedAt:     enqueuedAt,
	}); err != nil {
		t.Fatalf("record fix attempt: %v", err)
	}
	if err := store.RefreshTaskCIFixCheckpoint(ctx, "task-1", "repo-1", 42, "after", `{"failed_checks":[]}`); err != nil {
		t.Fatalf("refresh checkpoint: %v", err)
	}

	state, err := store.GetTaskCIPRState(ctx, "task-1", "repo-1", 42)
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if state.LastFixSignature != "after" || state.LastFixCheckpointJSON != `{"failed_checks":[]}` {
		t.Fatalf("checkpoint was not refreshed: %+v", state)
	}
	if state.LastFixSessionID == nil || *state.LastFixSessionID != "session-1" {
		t.Fatalf("LastFixSessionID=%v, want session-1", state.LastFixSessionID)
	}
	if state.LastFixEnqueuedAt == nil || !state.LastFixEnqueuedAt.Equal(enqueuedAt) {
		t.Fatalf("LastFixEnqueuedAt=%v, want %v", state.LastFixEnqueuedAt, enqueuedAt)
	}
}

func boolPtr(v bool) *bool { return &v }
