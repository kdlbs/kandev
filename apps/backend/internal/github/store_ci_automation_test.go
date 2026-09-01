package github

import (
	"context"
	"errors"
	"strings"
	"sync"
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
			"pr_scope_migrated_at",
		},
		"github_task_ci_pr_state": {
			"merge_retry_pending",
			"review_request_initialized",
			"last_review_requested",
			"last_observed_pr_state",
			"last_lifecycle_event",
			"last_lifecycle_prompt_at",
			"last_lifecycle_session_id",
		},
		"github_task_pr_automation_options": {
			"task_id",
			"repository_id",
			"pr_number",
			"auto_fix_enabled",
			"auto_merge_enabled",
			"prompt_on_review_requested",
			"prompt_on_merged",
			"prompt_on_closed",
			"created_at",
			"updated_at",
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

func TestStoreRetryMergeAuthorizesOneSameSignatureAttempt(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	attempt := TaskCIMergeAttempt{
		TaskID: "task-1", RepositoryID: "repo-1", PRNumber: 42,
		Signature: "ready-v1", AttemptedHeadSHA: "head-v1", AttemptedAt: time.Now().UTC(),
	}
	if err := store.RecordTaskCIMergeAttempt(ctx, attempt); err != nil {
		t.Fatalf("reserve initial attempt: %v", err)
	}
	if err := store.RecordTaskCIMergeAttemptResult(
		ctx, attempt.TaskID, attempt.RepositoryID, attempt.PRNumber,
		attempt.Signature, TaskCIMergeResultFailed, "merge PR: provider unavailable",
	); err != nil {
		t.Fatalf("record failed attempt: %v", err)
	}

	if err := store.AuthorizeTaskCIMergeRetry(
		ctx, attempt.TaskID, attempt.RepositoryID, attempt.PRNumber, time.Now().UTC(),
	); err != nil {
		t.Fatalf("authorize retry: %v", err)
	}
	state, err := store.GetTaskCIPRState(ctx, attempt.TaskID, attempt.RepositoryID, attempt.PRNumber)
	if err != nil {
		t.Fatalf("get authorized state: %v", err)
	}
	if state == nil || !state.MergeRetryPending {
		t.Fatalf("retry authorization not persisted: %+v", state)
	}

	if err := store.RecordTaskCIMergeAttempt(ctx, attempt); err != nil {
		t.Fatalf("consume retry authorization: %v", err)
	}
	state, err = store.GetTaskCIPRState(ctx, attempt.TaskID, attempt.RepositoryID, attempt.PRNumber)
	if err != nil {
		t.Fatalf("get consumed state: %v", err)
	}
	if state == nil || state.MergeRetryPending {
		t.Fatalf("retry authorization was not consumed: %+v", state)
	}
	if err := store.RecordTaskCIMergeAttempt(ctx, attempt); !errors.Is(err, ErrTaskCIMergeAttemptAlreadyReserved) {
		t.Fatalf("second same-signature retry error = %v, want %v", err, ErrTaskCIMergeAttemptAlreadyReserved)
	}
}

func TestStoreRetryMergeRejectsAcceptedAndDuplicateRequests(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	attempt := TaskCIMergeAttempt{
		TaskID: "task-1", RepositoryID: "repo-1", PRNumber: 42,
		Signature: "ready-v1", AttemptedHeadSHA: "head-v1", AttemptedAt: time.Now().UTC(),
	}
	if err := store.RecordTaskCIMergeAttempt(ctx, attempt); err != nil {
		t.Fatalf("reserve attempt: %v", err)
	}
	if err := store.RecordTaskCIMergeAttemptResult(
		ctx, attempt.TaskID, attempt.RepositoryID, attempt.PRNumber,
		attempt.Signature, TaskCIMergeResultFailed, "merge PR: provider unavailable",
	); err != nil {
		t.Fatalf("record failure: %v", err)
	}
	if err := store.AuthorizeTaskCIMergeRetry(
		ctx, attempt.TaskID, attempt.RepositoryID, attempt.PRNumber, time.Now().UTC(),
	); err != nil {
		t.Fatalf("authorize first retry: %v", err)
	}
	if err := store.AuthorizeTaskCIMergeRetry(
		ctx, attempt.TaskID, attempt.RepositoryID, attempt.PRNumber, time.Now().UTC(),
	); !errors.Is(err, ErrTaskCIMergeRetryNotAllowed) {
		t.Fatalf("duplicate authorization error = %v, want %v", err, ErrTaskCIMergeRetryNotAllowed)
	}

	if err := store.RecordTaskCIMergeAttempt(ctx, TaskCIMergeAttempt{
		TaskID: attempt.TaskID, RepositoryID: attempt.RepositoryID, PRNumber: 43,
		Signature: "ready-v2", AttemptedHeadSHA: "head-v2", AttemptedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("reserve accepted attempt: %v", err)
	}
	if err := store.RecordTaskCIMergeAttemptResult(
		ctx, attempt.TaskID, attempt.RepositoryID, 43,
		"ready-v2", TaskCIMergeResultAccepted, "",
	); err != nil {
		t.Fatalf("record accepted attempt: %v", err)
	}
	if err := store.AuthorizeTaskCIMergeRetry(
		ctx, attempt.TaskID, attempt.RepositoryID, 43, time.Now().UTC(),
	); !errors.Is(err, ErrTaskCIMergeRetryNotAllowed) {
		t.Fatalf("accepted authorization error = %v, want %v", err, ErrTaskCIMergeRetryNotAllowed)
	}
}

func TestStoreRetryMergeAllowsExpiredInFlightAttempt(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	attempt := TaskCIMergeAttempt{
		TaskID: "task-1", RepositoryID: "repo-1", PRNumber: 42,
		Signature: "ready-v1", AttemptedHeadSHA: "head-v1", AttemptedAt: time.Now().UTC().Add(-3 * time.Minute),
	}
	if err := store.RecordTaskCIMergeAttempt(ctx, attempt); err != nil {
		t.Fatalf("reserve stale attempt: %v", err)
	}
	if err := store.AuthorizeTaskCIMergeRetry(
		ctx, attempt.TaskID, attempt.RepositoryID, attempt.PRNumber, time.Now().UTC(),
	); err != nil {
		t.Fatalf("authorize expired retry: %v", err)
	}
}

// TestStoreTaskPRAutomationOptionsSchemaReplay confirms initSchema can run
// twice against the same database without error — the fresh-DB CREATE TABLE
// and the idempotent ADD COLUMN/fan-out migration must both tolerate replay.
func TestStoreTaskPRAutomationOptionsSchemaReplay(t *testing.T) {
	store := newTestStore(t)
	if err := store.initSchema(false); err != nil {
		t.Fatalf("replay schema migration: %v", err)
	}
	got, err := store.tableColumns("github_task_pr_automation_options")
	if err != nil {
		t.Fatalf("tableColumns: %v", err)
	}
	if _, ok := got["auto_fix_enabled"]; !ok {
		t.Fatal("github_task_pr_automation_options.auto_fix_enabled is missing after replay")
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

// TestStoreTaskPRAutomationOptionsReenableTerminalPromptResetsOnlyMatchingCheckpoint
// covers AC11/AC12-adjacent PR-scoping: re-enabling the merged-prompt switch
// on repo-merged/1 must reset only that PR's checkpoint, not repo-closed/2's
// — both because the reset predicate is keyed by observed state ("merged"
// vs "closed") and because it is now scoped to the specific PR.
func TestStoreTaskPRAutomationOptionsReenableTerminalPromptResetsOnlyMatchingCheckpoint(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	enabled := true
	disabled := false
	if _, err := store.UpdateTaskPRAutomationOptions(ctx, "task-1", "repo-merged", 1,
		TaskPRAutomationOptionsPatch{PromptOnMerged: &enabled}, false); err != nil {
		t.Fatalf("enable merged prompt for repo-merged/1: %v", err)
	}
	if _, err := store.UpdateTaskPRAutomationOptions(ctx, "task-1", "repo-closed", 2,
		TaskPRAutomationOptionsPatch{PromptOnClosed: &enabled}, false); err != nil {
		t.Fatalf("enable closed prompt for repo-closed/2: %v", err)
	}
	for _, prompt := range []TaskPRLifecyclePrompt{
		{TaskID: "task-1", RepositoryID: "repo-merged", PRNumber: 1, Event: "merged", ObservedState: "merged"},
		{TaskID: "task-1", RepositoryID: "repo-closed", PRNumber: 2, Event: "closed", ObservedState: "closed"},
	} {
		if err := store.RecordTaskPRLifecyclePrompt(ctx, prompt); err != nil {
			t.Fatalf("seed terminal checkpoint: %v", err)
		}
	}
	if _, err := store.UpdateTaskPRAutomationOptions(ctx, "task-1", "repo-merged", 1,
		TaskPRAutomationOptionsPatch{PromptOnMerged: &disabled}, false); err != nil {
		t.Fatalf("disable merged prompt: %v", err)
	}
	if _, err := store.UpdateTaskPRAutomationOptions(ctx, "task-1", "repo-merged", 1,
		TaskPRAutomationOptionsPatch{PromptOnMerged: &enabled}, false); err != nil {
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
		t.Fatalf("closed checkpoint changed while re-enabling merged on a different PR: %+v", closed)
	}
}

// TestStoreTaskCIOptions_DefaultAndUpdate covers the genuinely task-level
// fields still owned by github_task_ci_options: AutoFixPromptOverride. The
// five automation switches moved to per-PR scope — see
// TestStoreTaskPRAutomationOptions_DefaultAndUpdate.
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
	if got.AutoFixPromptOverride != nil {
		t.Fatalf("default prompt override should be nil, got %q", *got.AutoFixPromptOverride)
	}

	override := "Fix only the new CI feedback."
	updated, err := store.UpdateTaskCIOptions(ctx, "task-1", TaskCIOptionsPatch{
		AutoFixPromptOverride: &override,
	})
	if err != nil {
		t.Fatalf("update options: %v", err)
	}
	if updated.AutoFixPromptOverride == nil || *updated.AutoFixPromptOverride != override {
		t.Fatalf("override=%v, want %q", updated.AutoFixPromptOverride, override)
	}

	clearOverride := ""
	updated, err = store.UpdateTaskCIOptions(ctx, "task-1", TaskCIOptionsPatch{
		AutoFixPromptOverride: &clearOverride,
	})
	if err != nil {
		t.Fatalf("second update options: %v", err)
	}
	if updated.AutoFixPromptOverride != nil {
		t.Fatalf("override should be cleared, got %q", *updated.AutoFixPromptOverride)
	}
}

// TestStoreTaskPRAutomationOptions_DefaultAndUpdate covers the per-PR
// automation switches, keyed by (task_id, repository_id, pr_number).
func TestStoreTaskPRAutomationOptions_DefaultAndUpdate(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	got, err := store.GetTaskPRAutomationOptions(ctx, "task-1", "repo-1", 1)
	if err != nil {
		t.Fatalf("get default options: %v", err)
	}
	if got.TaskID != "task-1" || got.RepositoryID != "repo-1" || got.PRNumber != 1 {
		t.Fatalf("identity = %+v, want task-1/repo-1/1", got)
	}
	if got.AutoFixEnabled || got.AutoMergeEnabled {
		t.Fatalf("default options should be disabled, got %+v", got)
	}

	updated, err := store.UpdateTaskPRAutomationOptions(ctx, "task-1", "repo-1", 1,
		TaskPRAutomationOptionsPatch{AutoFixEnabled: boolPtr(true)}, false)
	if err != nil {
		t.Fatalf("update options: %v", err)
	}
	if !updated.AutoFixEnabled {
		t.Fatalf("AutoFixEnabled=false, want true")
	}
	if updated.AutoMergeEnabled {
		t.Fatalf("AutoMergeEnabled=true, want unchanged default false")
	}

	enableMerge := true
	updated, err = store.UpdateTaskPRAutomationOptions(ctx, "task-1", "repo-1", 1,
		TaskPRAutomationOptionsPatch{AutoMergeEnabled: &enableMerge}, false)
	if err != nil {
		t.Fatalf("second update options: %v", err)
	}
	if !updated.AutoFixEnabled {
		t.Fatalf("AutoFixEnabled should remain true")
	}
	if !updated.AutoMergeEnabled {
		t.Fatalf("AutoMergeEnabled=false, want true")
	}

	// A different PR on the same task starts independently disabled (AC1/AC2).
	other, err := store.GetTaskPRAutomationOptions(ctx, "task-1", "repo-1", 2)
	if err != nil {
		t.Fatalf("get other PR options: %v", err)
	}
	if other.AutoFixEnabled || other.AutoMergeEnabled {
		t.Fatalf("other PR should be independently disabled, got %+v", other)
	}

	// Same PR number in a different repository is independent (AC4).
	otherRepo, err := store.GetTaskPRAutomationOptions(ctx, "task-1", "repo-2", 1)
	if err != nil {
		t.Fatalf("get other repo options: %v", err)
	}
	if otherRepo.AutoFixEnabled || otherRepo.AutoMergeEnabled {
		t.Fatalf("same PR number in a different repo should be independent, got %+v", otherRepo)
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

func TestStoreTaskCIMergeQueueRecoveryState(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if err := store.RecordTaskCIMergeQueueObservation(ctx, TaskCIMergeQueueObservation{
		TaskID: "task-1", RepositoryID: "repo-1", PRNumber: 42,
		ActiveQueueHeadSHA: "head-a", MergeSignature: "merge-a",
	}); err != nil {
		t.Fatalf("record active queue observation: %v", err)
	}
	observed, err := store.GetTaskCIPRState(ctx, "task-1", "repo-1", 42)
	if err != nil {
		t.Fatalf("get observed queue state: %v", err)
	}
	if observed == nil || observed.LastMergeAttemptAt != nil {
		t.Fatalf("passive queue observation claimed a merge attempt: %+v", observed)
	}
	if err := store.RecordTaskCIFixAttempt(ctx, TaskCIFixAttempt{
		TaskID: "task-1", RepositoryID: "repo-1", PRNumber: 42,
		QueueRemovalEventID: "removal-a", QueueRemovalCause: "checks_failed",
		IncrementRound: true,
	}); err != nil {
		t.Fatalf("record queue recovery fix: %v", err)
	}
	if err := store.RecordTaskCIMergeAttempt(ctx, TaskCIMergeAttempt{
		TaskID: "task-1", RepositoryID: "repo-1", PRNumber: 42,
		Signature: "merge-b", AttemptedHeadSHA: "head-b",
	}); err != nil {
		t.Fatalf("record queue merge attempt: %v", err)
	}

	state, err := store.GetTaskCIPRState(ctx, "task-1", "repo-1", 42)
	if err != nil {
		t.Fatalf("get queue automation state: %v", err)
	}
	if state == nil {
		t.Fatal("expected queue automation state")
	}
	if state.LastQueueAttemptHeadSHA != "head-b" || state.LastMergeSignature != "merge-b" {
		t.Fatalf("queue attempt state = %+v, want head-b and merge-b", state)
	}
	if state.LastQueueFixEventID != "removal-a" || state.LastQueueRemovalCause != "checks_failed" || state.AutoFixRoundCount != 1 {
		t.Fatalf("queue repair state = %+v, want removal checkpoint and one round", state)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.2
// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.8
func TestStoreTaskCIMergeAttemptJournalSchema(t *testing.T) {
	store := newTestStore(t)
	columns, err := store.tableColumns("github_task_ci_pr_state")
	if err != nil {
		t.Fatalf("read journal columns: %v", err)
	}
	for _, name := range []string{"last_merge_result", "last_error_kind"} {
		if _, ok := columns[name]; !ok {
			t.Errorf("missing journal column %q", name)
		}
	}
}

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.2
func TestStoreTaskCIMergeAttemptReservationBlocksUnchangedAttempt(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	first := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	attempt := TaskCIMergeAttempt{
		TaskID: "task-1", RepositoryID: "repo-1", PRNumber: 42,
		Signature: "ready-head-a", AttemptedHeadSHA: "head-a", AttemptedAt: first,
	}
	if err := store.RecordTaskCIMergeAttempt(ctx, attempt); err != nil {
		t.Fatalf("reserve first merge attempt: %v", err)
	}
	attempt.AttemptedAt = first.Add(time.Minute)
	if err := store.RecordTaskCIMergeAttempt(ctx, attempt); err == nil ||
		!strings.Contains(err.Error(), "already reserved") {
		t.Fatalf("reserve unchanged attempt error = %v, want already reserved", err)
	}

	state, err := store.GetTaskCIPRState(ctx, "task-1", "repo-1", 42)
	if err != nil {
		t.Fatalf("get merge attempt: %v", err)
	}
	if state.LastMergeResult != TaskCIMergeResultInFlight {
		t.Fatalf("merge result = %q, want in_flight", state.LastMergeResult)
	}
	if state.LastMergeAttemptAt == nil || !state.LastMergeAttemptAt.Equal(first) {
		t.Fatalf("attempt time = %v, want first reservation %v", state.LastMergeAttemptAt, first)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.5
func TestStoreTaskCIMergeQueueObservationReconcilesOnlyAutoMergeError(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := store.db.Exec(`
		INSERT INTO github_task_ci_pr_state (
			task_id, repository_id, pr_number, last_merge_signature, last_merge_result,
			last_error, last_error_kind, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"task-1", "repo-1", 42, "merge-a", TaskCIMergeResultFailed,
		"merge PR: provider status was lost", TaskCIErrorKindAutoMerge, now, now); err != nil {
		t.Fatalf("seed failed merge attempt: %v", err)
	}
	if err := store.RecordTaskCIMergeQueueObservation(ctx, TaskCIMergeQueueObservation{
		TaskID: "task-1", RepositoryID: "repo-1", PRNumber: 42,
		ActiveQueueHeadSHA: "head-a", MergeSignature: "merge-a",
	}); err != nil {
		t.Fatalf("record queue observation: %v", err)
	}
	state, err := store.GetTaskCIPRState(ctx, "task-1", "repo-1", 42)
	if err != nil {
		t.Fatalf("get reconciled attempt: %v", err)
	}
	if state.LastMergeResult != TaskCIMergeResultAccepted || state.LastError != nil || state.LastErrorKind != "" {
		t.Fatalf("reconciled state = %+v, want accepted with cleared merge error", state)
	}

	if _, err := store.db.Exec(`
		UPDATE github_task_ci_pr_state
		SET last_error = ?, last_error_kind = ?
		WHERE task_id = ? AND repository_id = ? AND pr_number = ?`,
		"auto-fix failed", "auto_fix", "task-1", "repo-1", 42); err != nil {
		t.Fatalf("seed unrelated error: %v", err)
	}
	if err := store.RecordTaskCIMergeQueueObservation(ctx, TaskCIMergeQueueObservation{
		TaskID: "task-1", RepositoryID: "repo-1", PRNumber: 42,
		ActiveQueueHeadSHA: "head-a", MergeSignature: "merge-a",
	}); err != nil {
		t.Fatalf("record repeated queue observation: %v", err)
	}
	state, err = store.GetTaskCIPRState(ctx, "task-1", "repo-1", 42)
	if err != nil {
		t.Fatalf("get unrelated error state: %v", err)
	}
	if state.LastError == nil || *state.LastError != "auto-fix failed" || state.LastErrorKind != "auto_fix" {
		t.Fatalf("unrelated error was cleared: %+v", state)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.2
func TestStoreTaskCIMergeAttemptResultRecordsFailureAtomically(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if err := store.RecordTaskCIMergeAttempt(ctx, TaskCIMergeAttempt{
		TaskID: "task-1", RepositoryID: "repo-1", PRNumber: 42,
		Signature: "ready-head-a", AttemptedHeadSHA: "head-a",
	}); err != nil {
		t.Fatalf("reserve merge attempt: %v", err)
	}
	message := "merge PR: GitHub response was lost"
	if err := store.RecordTaskCIMergeAttemptResult(
		ctx, "task-1", "repo-1", 42, "ready-head-a", TaskCIMergeResultFailed, message,
	); err != nil {
		t.Fatalf("record failed merge attempt: %v", err)
	}
	state, err := store.GetTaskCIPRState(ctx, "task-1", "repo-1", 42)
	if err != nil {
		t.Fatalf("get failed merge attempt: %v", err)
	}
	if state.LastMergeResult != TaskCIMergeResultFailed || state.LastErrorKind != TaskCIErrorKindAutoMerge {
		t.Fatalf("failed merge state = %+v", state)
	}
	if state.LastError == nil || *state.LastError != message {
		t.Fatalf("last error = %v, want %q", state.LastError, message)
	}
	if err := store.MarkTaskCIAutoFixExhausted(
		ctx, "task-1", "repo-1", 42, "CI auto-fix paused after the round limit",
	); err != nil {
		t.Fatalf("record auto-fix exhaustion: %v", err)
	}
	state, err = store.GetTaskCIPRState(ctx, "task-1", "repo-1", 42)
	if err != nil {
		t.Fatalf("get auto-fix exhaustion state: %v", err)
	}
	if state.LastError == nil || *state.LastError != "CI auto-fix paused after the round limit" ||
		state.LastErrorKind != TaskCIErrorKindAutoFix {
		t.Fatalf("auto-fix error did not replace merge error atomically: %+v", state)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.3
func TestStoreTaskCIMergeAttemptResultIsSignatureBound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	for _, signature := range []string{"ready-head-a", "ready-head-b"} {
		if err := store.RecordTaskCIMergeAttempt(ctx, TaskCIMergeAttempt{
			TaskID: "task-1", RepositoryID: "repo-1", PRNumber: 42,
			Signature: signature, AttemptedHeadSHA: strings.TrimPrefix(signature, "ready-"),
		}); err != nil {
			t.Fatalf("reserve %s: %v", signature, err)
		}
	}
	err := store.RecordTaskCIMergeAttemptResult(
		ctx, "task-1", "repo-1", 42, "ready-head-a", TaskCIMergeResultAccepted, "",
	)
	if !errors.Is(err, ErrTaskCIMergeAttemptNotFound) {
		t.Fatalf("stale result error = %v, want attempt not found", err)
	}
	state, err := store.GetTaskCIPRState(ctx, "task-1", "repo-1", 42)
	if err != nil {
		t.Fatalf("get current attempt: %v", err)
	}
	if state.LastMergeSignature != "ready-head-b" || state.LastMergeResult != TaskCIMergeResultInFlight {
		t.Fatalf("current attempt was overwritten: %+v", state)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.2
func TestStoreTaskCIMergeAttemptReservationIsConcurrentSafe(t *testing.T) {
	store := newTestStore(t)
	start := make(chan struct{})
	results := make(chan error, 2)
	attempt := TaskCIMergeAttempt{
		TaskID: "task-1", RepositoryID: "repo-1", PRNumber: 42,
		Signature: "ready-head-a", AttemptedHeadSHA: "head-a",
	}
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- store.RecordTaskCIMergeAttempt(context.Background(), attempt)
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	reserved, blocked := 0, 0
	for err := range results {
		switch {
		case err == nil:
			reserved++
		case errors.Is(err, ErrTaskCIMergeAttemptAlreadyReserved):
			blocked++
		default:
			t.Fatalf("reserve merge attempt: %v", err)
		}
	}
	if reserved != 1 || blocked != 1 {
		t.Fatalf("reserved=%d blocked=%d, want one each", reserved, blocked)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.8
func TestStoreTaskCIAutoMergeErrorKindBackfillIsNarrow(t *testing.T) {
	store := newTestStore(t)
	now := time.Now().UTC()
	for _, row := range []struct {
		repositoryID string
		message      string
	}{
		{repositoryID: "known", message: "merge PR: GitHub unavailable"},
		{repositoryID: "unknown", message: "review prompt failed"},
	} {
		if _, err := store.db.Exec(`
			INSERT INTO github_task_ci_pr_state (
				task_id, repository_id, pr_number, last_error, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?)`,
			"task-1", row.repositoryID, 42, row.message, now, now); err != nil {
			t.Fatalf("seed %s error: %v", row.repositoryID, err)
		}
	}
	if err := store.backfillTaskCIAutoMergeErrorKinds(); err != nil {
		t.Fatalf("backfill error kinds: %v", err)
	}
	known, err := store.GetTaskCIPRState(context.Background(), "task-1", "known", 42)
	if err != nil {
		t.Fatalf("get known error: %v", err)
	}
	unknown, err := store.GetTaskCIPRState(context.Background(), "task-1", "unknown", 42)
	if err != nil {
		t.Fatalf("get unknown error: %v", err)
	}
	if known.LastErrorKind != TaskCIErrorKindAutoMerge {
		t.Fatalf("known error kind = %q, want auto_merge", known.LastErrorKind)
	}
	if unknown.LastErrorKind != "" {
		t.Fatalf("unknown error kind = %q, want empty", unknown.LastErrorKind)
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
	if _, err := store.UpdateTaskPRAutomationOptions(ctx, "task-1", "repo-1", 42,
		TaskPRAutomationOptionsPatch{AutoFixEnabled: &disabled}, false); err != nil {
		t.Fatalf("disable auto-fix: %v", err)
	}
	enabled := true
	if _, err := store.UpdateTaskPRAutomationOptions(ctx, "task-1", "repo-1", 42,
		TaskPRAutomationOptionsPatch{AutoFixEnabled: &enabled}, false); err != nil {
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

// seedLegacyTaskCIOptions inserts a pre-migration github_task_ci_options row
// directly, bypassing UpdateTaskCIOptions (which no longer writes the five
// legacy boolean columns), to simulate a database from before per-PR scoping.
func seedLegacyTaskCIOptions(t *testing.T, store *Store, taskID string, autoFix, autoMerge bool) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := store.db.Exec(`
		INSERT INTO github_task_ci_options (task_id, auto_fix_enabled, auto_merge_enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`,
		taskID, autoFix, autoMerge, now, now); err != nil {
		t.Fatalf("seed legacy task CI options: %v", err)
	}
}

// insertTestTask registers taskID in the tasks table so the task-contribution
// orphan sweep does not delete rows the test seeds against it.
func insertTestTask(t *testing.T, store *Store, taskID string) {
	t.Helper()
	if _, err := store.db.Exec(`INSERT INTO tasks (id, workspace_id) VALUES (?, ?)`, taskID, "ws-1"); err != nil {
		t.Fatalf("insert task %s: %v", taskID, err)
	}
}

// TestStoreMigrateTaskCIOptionsToPRScope_FansOutToLinkedPRs covers AC14: a
// pre-upgrade task row with two linked PRs yields, after one boot, two
// per-PR rows each matching the legacy booleans.
func TestStoreMigrateTaskCIOptionsToPRScope_FansOutToLinkedPRs(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	insertTestTask(t, store, "task-1")
	seedLegacyTaskCIOptions(t, store, "task-1", true, false)
	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID: "task-1", RepositoryID: "repo-1", Owner: "o", Repo: "r", PRNumber: 1, CreatedAt: now,
	}); err != nil {
		t.Fatalf("seed PR 1: %v", err)
	}
	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID: "task-1", RepositoryID: "repo-1", Owner: "o", Repo: "r", PRNumber: 2, CreatedAt: now,
	}); err != nil {
		t.Fatalf("seed PR 2: %v", err)
	}

	if err := store.initSchema(false); err != nil {
		t.Fatalf("replay schema migration: %v", err)
	}

	for _, prNumber := range []int{1, 2} {
		opts, err := store.GetTaskPRAutomationOptions(ctx, "task-1", "repo-1", prNumber)
		if err != nil {
			t.Fatalf("get PR %d options: %v", prNumber, err)
		}
		if !opts.AutoFixEnabled || opts.AutoMergeEnabled {
			t.Fatalf("PR %d options = %+v, want auto_fix_enabled=true auto_merge_enabled=false", prNumber, opts)
		}
	}
	migrated, err := store.GetTaskCIOptions(ctx, "task-1")
	if err != nil {
		t.Fatalf("get task options: %v", err)
	}
	if migrated.PRScopeMigratedAt == nil {
		t.Fatal("pr_scope_migrated_at was not stamped")
	}
}

func TestStoreMigrateTaskCIOptionsToPRScope_SkipsDetachedPRs(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	insertTestTask(t, store, "task-1")
	seedLegacyTaskCIOptions(t, store, "task-1", true, true)
	active := &TaskPR{
		TaskID: "task-1", RepositoryID: "repo-1", Owner: "o", Repo: "r", PRNumber: 1, CreatedAt: now,
	}
	detached := &TaskPR{
		TaskID: "task-1", RepositoryID: "repo-1", Owner: "o", Repo: "r", PRNumber: 2, CreatedAt: now.Add(time.Second),
	}
	for _, pr := range []*TaskPR{active, detached} {
		if err := store.CreateTaskPR(ctx, pr); err != nil {
			t.Fatalf("seed PR #%d: %v", pr.PRNumber, err)
		}
	}
	if _, transitioned, err := store.DetachTaskPR(ctx, detached.ID); err != nil || !transitioned {
		t.Fatalf("detach PR #%d: transitioned=%v err=%v", detached.PRNumber, transitioned, err)
	}

	if err := store.initSchema(false); err != nil {
		t.Fatalf("replay schema migration: %v", err)
	}
	activeOptions, err := store.GetTaskPRAutomationOptions(ctx, "task-1", "repo-1", active.PRNumber)
	if err != nil {
		t.Fatalf("get active PR options: %v", err)
	}
	if !activeOptions.AutoFixEnabled || !activeOptions.AutoMergeEnabled {
		t.Fatalf("active PR options = %+v, want legacy switches enabled", activeOptions)
	}
	detachedOptions, err := store.GetTaskPRAutomationOptions(ctx, "task-1", "repo-1", detached.PRNumber)
	if err != nil {
		t.Fatalf("get detached PR options: %v", err)
	}
	if detachedOptions.AutoFixEnabled || detachedOptions.AutoMergeEnabled {
		t.Fatalf("detached PR inherited legacy switches: %+v", detachedOptions)
	}
}

// TestStoreMigrateTaskCIOptionsToPRScope_Idempotent covers AC15: replaying
// the migration on an already-migrated database changes no per-PR row.
func TestStoreMigrateTaskCIOptionsToPRScope_Idempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	insertTestTask(t, store, "task-1")
	seedLegacyTaskCIOptions(t, store, "task-1", true, false)
	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID: "task-1", RepositoryID: "repo-1", Owner: "o", Repo: "r", PRNumber: 1, CreatedAt: now,
	}); err != nil {
		t.Fatalf("seed PR: %v", err)
	}
	if err := store.initSchema(false); err != nil {
		t.Fatalf("first replay: %v", err)
	}
	before, err := store.GetTaskPRAutomationOptions(ctx, "task-1", "repo-1", 1)
	if err != nil {
		t.Fatalf("get options after first replay: %v", err)
	}

	if err := store.initSchema(false); err != nil {
		t.Fatalf("second replay: %v", err)
	}
	after, err := store.GetTaskPRAutomationOptions(ctx, "task-1", "repo-1", 1)
	if err != nil {
		t.Fatalf("get options after second replay: %v", err)
	}
	if !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("updated_at changed on replay: before=%v after=%v", before.UpdatedAt, after.UpdatedAt)
	}
}

// TestStoreMigrateTaskCIOptionsToPRScope_DoesNotReenableUserDisabled covers
// AC16: once migrated, a user's deliberate per-PR disable must survive a
// later boot even though the stale legacy row still says "enabled".
func TestStoreMigrateTaskCIOptionsToPRScope_DoesNotReenableUserDisabled(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	insertTestTask(t, store, "task-1")
	seedLegacyTaskCIOptions(t, store, "task-1", true, false)
	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID: "task-1", RepositoryID: "repo-1", Owner: "o", Repo: "r", PRNumber: 1, CreatedAt: now,
	}); err != nil {
		t.Fatalf("seed PR: %v", err)
	}
	if err := store.initSchema(false); err != nil {
		t.Fatalf("first boot migration: %v", err)
	}

	disabled := false
	if _, err := store.UpdateTaskPRAutomationOptions(ctx, "task-1", "repo-1", 1,
		TaskPRAutomationOptionsPatch{AutoFixEnabled: &disabled}, false); err != nil {
		t.Fatalf("user disables auto-fix: %v", err)
	}

	if err := store.initSchema(false); err != nil {
		t.Fatalf("second boot: %v", err)
	}

	opts, err := store.GetTaskPRAutomationOptions(ctx, "task-1", "repo-1", 1)
	if err != nil {
		t.Fatalf("get options: %v", err)
	}
	if opts.AutoFixEnabled {
		t.Fatal("auto-fix was re-enabled by a replayed migration after the user turned it off")
	}
}

// TestStoreMigrateTaskCIOptionsToPRScope_NewlyLinkedPRStartsAllOff covers
// AC17: a PR linked to the task after migration has already run starts with
// all five switches off rather than inheriting the stale legacy value.
func TestStoreMigrateTaskCIOptionsToPRScope_NewlyLinkedPRStartsAllOff(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	insertTestTask(t, store, "task-1")
	seedLegacyTaskCIOptions(t, store, "task-1", true, true)
	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID: "task-1", RepositoryID: "repo-1", Owner: "o", Repo: "r", PRNumber: 1, CreatedAt: now,
	}); err != nil {
		t.Fatalf("seed PR 1: %v", err)
	}
	if err := store.initSchema(false); err != nil {
		t.Fatalf("boot migration: %v", err)
	}

	// PR 2 links to the task only after the fan-out migration already ran.
	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID: "task-1", RepositoryID: "repo-1", Owner: "o", Repo: "r", PRNumber: 2, CreatedAt: now,
	}); err != nil {
		t.Fatalf("seed PR 2: %v", err)
	}

	opts, err := store.GetTaskPRAutomationOptions(ctx, "task-1", "repo-1", 2)
	if err != nil {
		t.Fatalf("get options: %v", err)
	}
	if opts.AutoFixEnabled || opts.AutoMergeEnabled {
		t.Fatalf("newly linked PR should start all-off, got %+v", opts)
	}
}

func boolPtr(v bool) *bool { return &v }

func intPtr(v int) *int { return &v }
