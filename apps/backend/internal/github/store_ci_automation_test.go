package github

import (
	"context"
	"testing"
	"time"
)

func TestStoreTaskPRAgentAutomationSchema(t *testing.T) {
	store := newTestStore(t)

	for table, columns := range map[string][]string{
		"github_task_ci_options": {
			"prompt_on_review_requested",
			"prompt_on_merged",
			"prompt_on_closed",
			"review_reviewer_login",
			"review_prompt_override",
			"merged_prompt_override",
			"closed_prompt_override",
		},
		"github_task_ci_pr_state": {
			"review_request_initialized",
			"last_review_requested",
			"last_observed_pr_state",
			"last_lifecycle_event",
			"last_lifecycle_prompt_at",
			"last_lifecycle_session_id",
		},
	} {
		got, err := store.tableColumns(table)
		if err != nil {
			t.Fatalf("tableColumns(%s): %v", table, err)
		}
		for _, column := range columns {
			if _, ok := got[column]; !ok {
				t.Errorf("%s.%s is missing", table, column)
			}
		}
	}
}

func TestStoreTaskPRAgentAutomationMigrationClearsLifecyclePromptOverrides(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO github_task_ci_options (
			task_id, review_prompt_override, merged_prompt_override, closed_prompt_override, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?)`,
		"task-1", "review override", "merged override", "closed override", time.Now().UTC(), time.Now().UTC()); err != nil {
		t.Fatalf("seed lifecycle prompt overrides: %v", err)
	}

	if err := store.initSchema(false); err != nil {
		t.Fatalf("replay schema migration: %v", err)
	}
	options, err := store.GetTaskCIOptions(ctx, "task-1")
	if err != nil {
		t.Fatalf("get options: %v", err)
	}
	if options.ReviewPromptOverride != nil || options.MergedPromptOverride != nil || options.ClosedPromptOverride != nil {
		t.Fatalf("lifecycle prompt overrides were not cleared: %+v", options)
	}
}

func TestStoreTaskPRAgentAutomationCheckpoints(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	recorder, ok := any(store).(interface {
		SetTaskPRReviewRequestState(context.Context, string, string, int, bool) error
		SetTaskPRObservedState(context.Context, string, string, int, string) error
		RecordTaskPRLifecyclePrompt(context.Context, TaskPRLifecyclePrompt) error
	})
	if !ok {
		t.Fatal("Store does not implement PR agent automation checkpoint operations")
	}

	if err := recorder.SetTaskPRReviewRequestState(ctx, "task-1", "repo-1", 42, false); err != nil {
		t.Fatalf("baseline review request: %v", err)
	}
	if err := recorder.SetTaskPRObservedState(ctx, "task-1", "repo-1", 42, "open"); err != nil {
		t.Fatalf("observe open: %v", err)
	}
	at := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	if err := recorder.RecordTaskPRLifecyclePrompt(ctx, TaskPRLifecyclePrompt{
		TaskID: "task-1", RepositoryID: "repo-1", PRNumber: 42,
		Event: "review_requested", SessionID: "session-1", PromptedAt: at,
		ReviewRequested: true,
	}); err != nil {
		t.Fatalf("record lifecycle prompt: %v", err)
	}
	state, err := store.GetTaskCIPRState(ctx, "task-1", "repo-1", 42)
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if state == nil || !state.ReviewRequestInitialized || !state.LastReviewRequested {
		t.Fatalf("review request checkpoint = %+v", state)
	}
	if state.LastObservedPRState != "open" || state.LastLifecycleEvent != "review_requested" {
		t.Fatalf("lifecycle checkpoint = %+v", state)
	}
	if state.LastLifecyclePromptAt == nil || !state.LastLifecyclePromptAt.Equal(at) {
		t.Fatalf("prompted_at = %v, want %v", state.LastLifecyclePromptAt, at)
	}
	if state.LastLifecycleSessionID == nil || *state.LastLifecycleSessionID != "session-1" {
		t.Fatalf("session = %v, want session-1", state.LastLifecycleSessionID)
	}
	if err := recorder.RecordTaskPRLifecyclePrompt(ctx, TaskPRLifecyclePrompt{
		TaskID: "task-1", RepositoryID: "repo-1", PRNumber: 42,
		Event: "merged", SessionID: "session-1", PromptedAt: at,
		ObservedState: "merged",
	}); err != nil {
		t.Fatalf("record terminal prompt: %v", err)
	}
	if err := recorder.SetTaskPRObservedState(ctx, "task-1", "repo-1", 42, "open"); err != nil {
		t.Fatalf("rearm terminal state: %v", err)
	}
	state, err = store.GetTaskCIPRState(ctx, "task-1", "repo-1", 42)
	if err != nil {
		t.Fatalf("get rearmed state: %v", err)
	}
	if state.LastLifecycleEvent != "" {
		t.Fatalf("last lifecycle event = %q, want cleared after reopen", state.LastLifecycleEvent)
	}
}

func TestStoreRebindTaskPRReviewerQuietlyResetsReviewBaselines(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	oldLogin := "reviewer-a"
	enabled := true
	if _, err := store.UpdateTaskCIOptions(ctx, "task-1", TaskCIOptionsPatch{
		PromptOnReviewRequested: &enabled,
		ReviewReviewerLogin:     &oldLogin,
	}); err != nil {
		t.Fatalf("seed options: %v", err)
	}
	if err := store.RecordTaskCIFixAttempt(ctx, TaskCIFixAttempt{
		TaskID: "task-1", RepositoryID: "repo-1", PRNumber: 1,
		Signature: "ci-checkpoint", CheckpointJSON: `{"failed_checks":[]}`,
	}); err != nil {
		t.Fatalf("seed CI checkpoint: %v", err)
	}
	if err := store.SetTaskPRReviewRequestState(ctx, "task-1", "repo-1", 1, true); err != nil {
		t.Fatalf("seed review baseline: %v", err)
	}
	if err := store.RecordTaskPRLifecyclePrompt(ctx, TaskPRLifecyclePrompt{
		TaskID: "task-1", RepositoryID: "repo-1", PRNumber: 1,
		Event: "merged", ObservedState: "merged",
	}); err != nil {
		t.Fatalf("seed terminal checkpoint: %v", err)
	}
	if err := store.SetTaskPRReviewRequestState(ctx, "task-1", "repo-2", 2, true); err != nil {
		t.Fatalf("seed second review baseline: %v", err)
	}

	rebinder, ok := any(store).(interface {
		RebindTaskPRReviewer(context.Context, string, string) (bool, error)
	})
	if !ok {
		t.Fatal("Store does not implement atomic task PR reviewer rebinding")
	}
	changed, err := rebinder.RebindTaskPRReviewer(ctx, "task-1", "reviewer-b")
	if err != nil {
		t.Fatalf("rebind reviewer: %v", err)
	}
	if !changed {
		t.Fatal("rebind changed=false, want true")
	}

	options, err := store.GetTaskCIOptions(ctx, "task-1")
	if err != nil {
		t.Fatalf("get options: %v", err)
	}
	if options.ReviewReviewerLogin != "reviewer-b" {
		t.Fatalf("reviewer login = %q, want reviewer-b", options.ReviewReviewerLogin)
	}
	for _, key := range []struct {
		repositoryID string
		prNumber     int
	}{{"repo-1", 1}, {"repo-2", 2}} {
		state, err := store.GetTaskCIPRState(ctx, "task-1", key.repositoryID, key.prNumber)
		if err != nil {
			t.Fatalf("get state %s#%d: %v", key.repositoryID, key.prNumber, err)
		}
		if state.ReviewRequestInitialized || state.LastReviewRequested {
			t.Fatalf("review baseline for %s#%d was not reset: %+v", key.repositoryID, key.prNumber, state)
		}
	}
	state, err := store.GetTaskCIPRState(ctx, "task-1", "repo-1", 1)
	if err != nil {
		t.Fatalf("get first state: %v", err)
	}
	if state.LastFixSignature != "ci-checkpoint" || state.LastObservedPRState != "merged" || state.LastLifecycleEvent != "merged" {
		t.Fatalf("rebind changed non-review checkpoints: %+v", state)
	}
}

func TestStoreTaskCIOptionsReenableTerminalPromptResetsOnlyMatchingCheckpoint(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	enabled := true
	disabled := false
	if _, err := store.UpdateTaskCIOptions(ctx, "task-1", TaskCIOptionsPatch{
		PromptOnMerged: &enabled, PromptOnClosed: &enabled,
	}); err != nil {
		t.Fatalf("enable terminal prompts: %v", err)
	}
	for _, prompt := range []TaskPRLifecyclePrompt{
		{TaskID: "task-1", RepositoryID: "repo-merged", PRNumber: 1, Event: "merged", ObservedState: "merged"},
		{TaskID: "task-1", RepositoryID: "repo-closed", PRNumber: 2, Event: "closed", ObservedState: "closed"},
	} {
		if err := store.RecordTaskPRLifecyclePrompt(ctx, prompt); err != nil {
			t.Fatalf("seed terminal checkpoint: %v", err)
		}
	}
	if _, err := store.UpdateTaskCIOptions(ctx, "task-1", TaskCIOptionsPatch{PromptOnMerged: &disabled}); err != nil {
		t.Fatalf("disable merged prompt: %v", err)
	}
	if _, err := store.UpdateTaskCIOptions(ctx, "task-1", TaskCIOptionsPatch{PromptOnMerged: &enabled}); err != nil {
		t.Fatalf("re-enable merged prompt: %v", err)
	}

	merged, err := store.GetTaskCIPRState(ctx, "task-1", "repo-merged", 1)
	if err != nil {
		t.Fatalf("get merged state: %v", err)
	}
	if merged.LastObservedPRState != "" || merged.LastLifecycleEvent != "" {
		t.Fatalf("merged checkpoint was not reset: %+v", merged)
	}
	closed, err := store.GetTaskCIPRState(ctx, "task-1", "repo-closed", 2)
	if err != nil {
		t.Fatalf("get closed state: %v", err)
	}
	if closed.LastObservedPRState != "closed" || closed.LastLifecycleEvent != "closed" {
		t.Fatalf("closed checkpoint changed while re-enabling merged: %+v", closed)
	}
}

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
		IncrementRound: true,
	}); err != nil {
		t.Fatalf("record fix attempt: %v", err)
	}
	if err := store.RecordTaskCIFixAttempt(ctx, TaskCIFixAttempt{
		TaskID:         "task-1",
		RepositoryID:   "repo-1",
		PRNumber:       42,
		Signature:      "fix-sig-2",
		CheckpointJSON: `{"checks":["test","lint"]}`,
		SessionID:      "session-1",
		EnqueuedAt:     at.Add(30 * time.Second),
		IncrementRound: false,
	}); err != nil {
		t.Fatalf("record replacement fix attempt: %v", err)
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
	if state.LastFixSignature != "fix-sig-2" || state.LastFixCheckpointJSON != `{"checks":["test","lint"]}` {
		t.Fatalf("unexpected fix state: %+v", state)
	}
	if state.AutoFixRoundCount != 1 {
		t.Fatalf("AutoFixRoundCount=%d, want 1", state.AutoFixRoundCount)
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

func TestStoreTaskCIPRState_MarkExhaustedAndResetOnReenable(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	at := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)

	if err := store.RecordTaskCIFixAttempt(ctx, TaskCIFixAttempt{
		TaskID:         "task-1",
		RepositoryID:   "repo-1",
		PRNumber:       42,
		Signature:      "fix-sig",
		CheckpointJSON: `{}`,
		SessionID:      "session-1",
		EnqueuedAt:     at,
		IncrementRound: true,
	}); err != nil {
		t.Fatalf("record fix attempt: %v", err)
	}
	if err := store.MarkTaskCIAutoFixExhausted(ctx, "task-1", "repo-1", 42, "CI auto-fix paused after 10 rounds for this PR"); err != nil {
		t.Fatalf("mark exhausted: %v", err)
	}
	state, err := store.GetTaskCIPRState(ctx, "task-1", "repo-1", 42)
	if err != nil {
		t.Fatalf("get exhausted state: %v", err)
	}
	if state.AutoFixExhaustedAt == nil || state.LastError == nil {
		t.Fatalf("expected exhausted timestamp and error, got %+v", state)
	}

	disabled := false
	if _, err := store.UpdateTaskCIOptions(ctx, "task-1", TaskCIOptionsPatch{AutoFixEnabled: &disabled}); err != nil {
		t.Fatalf("disable auto-fix: %v", err)
	}
	enabled := true
	if _, err := store.UpdateTaskCIOptions(ctx, "task-1", TaskCIOptionsPatch{AutoFixEnabled: &enabled}); err != nil {
		t.Fatalf("re-enable auto-fix: %v", err)
	}
	state, err = store.GetTaskCIPRState(ctx, "task-1", "repo-1", 42)
	if err != nil {
		t.Fatalf("get reset state: %v", err)
	}
	if state.AutoFixRoundCount != 0 || state.AutoFixExhaustedAt != nil || state.LastError != nil {
		t.Fatalf("expected auto-fix round state reset, got %+v", state)
	}
	if state.LastFixSignature != "" || state.LastFixCheckpointJSON != "" || state.LastFixEnqueuedAt != nil || state.LastFixSessionID != nil {
		t.Fatalf("expected auto-fix checkpoint state reset, got %+v", state)
	}
}

func TestStoreTaskCIPRState_RefreshCheckpointClearsPromptDispatchMetadata(t *testing.T) {
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
	if state.LastFixSessionID != nil {
		t.Fatalf("LastFixSessionID=%v, want nil", state.LastFixSessionID)
	}
	if state.LastFixEnqueuedAt != nil {
		t.Fatalf("LastFixEnqueuedAt=%v, want nil", state.LastFixEnqueuedAt)
	}
}

func boolPtr(v bool) *bool { return &v }
