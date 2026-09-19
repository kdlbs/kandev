package gitlab

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

func boolPtr(b bool) *bool       { return &b }
func stringPtr(s string) *string { return &s }
func intPtr(i int) *int          { return &i }

// mrIdentity builds the single-repo MR identity most of these tests use.
func mrIdentity(projectPath string, iid int) MRIdentity {
	return MRIdentity{ProjectPath: projectPath, MRIID: iid}
}

// setMRSwitches is the per-MR switch write these tests exercise, kept short
// so the assertions stay the focus.
func setMRSwitches(
	t *testing.T, store *Store, taskID string, id MRIdentity, patch TaskMRAutomationSwitchPatch,
) *TaskMRAutomationOptionsForMR {
	t.Helper()
	mrs, err := store.ListTaskMRsByTask(context.Background(), taskID)
	if err != nil {
		t.Fatalf("list linked MRs: %v", err)
	}
	linked := false
	for _, mr := range mrs {
		if (MRIdentity{RepositoryID: mr.RepositoryID, ProjectPath: mr.ProjectPath, MRIID: mr.MRIID}) == id {
			linked = true
			break
		}
	}
	if !linked {
		if err := store.UpsertTaskMR(context.Background(), newTestMR(taskID, id.RepositoryID, id.ProjectPath, id.MRIID)); err != nil {
			t.Fatalf("link MR for switch write: %v", err)
		}
	}
	got, err := store.UpdateTaskMRAutomationOptionsForMR(context.Background(), taskID, id, patch)
	if err != nil {
		t.Fatalf("UpdateTaskMRAutomationOptionsForMR(%s, %+v): %v", taskID, id, err)
	}
	return got
}

func TestStore_UpdateTaskMRAutomationOptionsForMR_RejectsUnlinkedMR(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")
	id := mrIdentity("group/not-linked", 99)

	_, err := store.UpdateTaskMRAutomationOptionsForMR(
		ctx, "task-1", id, TaskMRAutomationSwitchPatch{AutoMergeEnabled: boolPtr(true)},
	)
	if !errors.Is(err, ErrTaskMRNotLinked) {
		t.Fatalf("expected ErrTaskMRNotLinked, got %v", err)
	}
	options, err := store.ListTaskMRAutomationOptions(ctx, "task-1")
	if err != nil {
		t.Fatalf("list options: %v", err)
	}
	if len(options) != 0 {
		t.Fatalf("unlinked write created automation options: %+v", options)
	}
}

// TestStore_UpdateTaskMRAutomationOptionsForMR_AutoMergeRoundTrip closes a
// coverage gap: auto_fix_enabled is exercised indirectly by
// TestStore_UpdateTaskMRAutomationOptionsForMR_ReenablingAutoFixResetsRoundCap,
// but nothing wrote auto_merge_enabled and read it back.
func TestStore_UpdateTaskMRAutomationOptionsForMR_AutoMergeRoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")
	id := mrIdentity("group/a", 1)

	updated := setMRSwitches(t, store, "task-1", id, TaskMRAutomationSwitchPatch{
		AutoMergeEnabled: boolPtr(true),
	})
	if !updated.AutoMergeEnabled {
		t.Fatalf("AutoMergeEnabled = false immediately after patch, want true")
	}
	got, err := store.GetTaskMRAutomationOptionsForMR(ctx, "task-1", id)
	if err != nil {
		t.Fatalf("GetTaskMRAutomationOptionsForMR: %v", err)
	}
	if !got.AutoMergeEnabled {
		t.Fatalf("AutoMergeEnabled = false after persisted read-back, want true")
	}

	// An unrelated per-MR patch must not disturb it.
	setMRSwitches(t, store, "task-1", id, TaskMRAutomationSwitchPatch{PromptOnClosed: boolPtr(true)})
	got, err = store.GetTaskMRAutomationOptionsForMR(ctx, "task-1", id)
	if err != nil {
		t.Fatalf("GetTaskMRAutomationOptionsForMR: %v", err)
	}
	if !got.AutoMergeEnabled || !got.PromptOnClosed {
		t.Fatalf("expected both switches on after independent patches, got %+v", got)
	}
}

// TestStore_UpdateTaskMRAutomationOptionsForMR_ScopesSwitchesPerMR is the
// regression this change exists for: configuring one linked MR must leave
// every other MR on the same task untouched.
func TestStore_UpdateTaskMRAutomationOptionsForMR_ScopesSwitchesPerMR(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")
	first := mrIdentity("group/a", 1)
	second := mrIdentity("group/b", 2)

	setMRSwitches(t, store, "task-1", first, TaskMRAutomationSwitchPatch{
		AutoFixEnabled: boolPtr(true), AutoMergeEnabled: boolPtr(true),
		PromptOnMerged: boolPtr(true),
	})

	other, err := store.GetTaskMRAutomationOptionsForMR(ctx, "task-1", second)
	if err != nil {
		t.Fatalf("GetTaskMRAutomationOptionsForMR(second): %v", err)
	}
	if other.AutoFixEnabled || other.AutoMergeEnabled || other.PromptOnMerged ||
		other.PromptOnReviewRequested || other.PromptOnClosed {
		t.Fatalf("second MR inherited the first MR's switches: %+v", other)
	}

	stored, err := store.ListTaskMRAutomationOptions(ctx, "task-1")
	if err != nil {
		t.Fatalf("ListTaskMRAutomationOptions: %v", err)
	}
	if len(stored) != 1 || stored[0].Identity() != first {
		t.Fatalf("expected exactly the configured MR to be stored, got %+v", stored)
	}
}

// TestStore_UpdateTaskMRAutomationOptions_PromptOverrideRoundTrip covers the
// one field that is still genuinely task-level, including the
// empty-string-normalizes-to-NULL path (normalizedMRPromptOverride).
func TestStore_UpdateTaskMRAutomationOptions_PromptOverrideRoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")

	updated, err := store.UpdateTaskMRAutomationOptions(ctx, "task-1", TaskMRAutomationPatch{
		AutoFixPromptOverride: stringPtr("custom prompt text"),
	}, nil)
	if err != nil {
		t.Fatalf("set prompt override: %v", err)
	}
	if updated.AutoFixPromptOverride == nil || *updated.AutoFixPromptOverride != "custom prompt text" {
		t.Fatalf("AutoFixPromptOverride = %v immediately after patch, want \"custom prompt text\"", updated.AutoFixPromptOverride)
	}
	firstRevision := updated.AutomationRevision
	got, err := store.GetTaskMRAutomationOptions(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTaskMRAutomationOptions: %v", err)
	}
	if got.AutoFixPromptOverride == nil || *got.AutoFixPromptOverride != "custom prompt text" {
		t.Fatalf("AutoFixPromptOverride = %v after persisted read-back, want \"custom prompt text\"", got.AutoFixPromptOverride)
	}

	// Clearing via an empty string must normalize to NULL, not persist "".
	updated, err = store.UpdateTaskMRAutomationOptions(ctx, "task-1", TaskMRAutomationPatch{
		AutoFixPromptOverride: stringPtr(""),
	}, nil)
	if err != nil {
		t.Fatalf("clear prompt override: %v", err)
	}
	if updated.AutoFixPromptOverride != nil {
		t.Fatalf("AutoFixPromptOverride = %v immediately after clearing, want nil", updated.AutoFixPromptOverride)
	}
	if updated.AutomationRevision <= firstRevision {
		t.Fatalf("automation revision = %d after update, want > %d", updated.AutomationRevision, firstRevision)
	}
	got, err = store.GetTaskMRAutomationOptions(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTaskMRAutomationOptions: %v", err)
	}
	if got.AutoFixPromptOverride != nil {
		t.Fatalf("AutoFixPromptOverride = %v after persisted read-back, want nil", got.AutoFixPromptOverride)
	}
}

func TestStore_GetTaskMRAutomationOptions_ImplicitDefault(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	opts, err := store.GetTaskMRAutomationOptions(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTaskMRAutomationOptions: %v", err)
	}
	if opts.TaskID != "task-1" || opts.PromptOnReviewRequested || opts.PromptOnMerged ||
		opts.PromptOnClosed || opts.ReviewReviewerUsername != "" {
		t.Fatalf("expected all-false implicit default, got %+v", opts)
	}
}

func TestStore_UpdateTaskMRAutomationOptionsForMR_RoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")
	id := mrIdentity("group/a", 1)

	updated := setMRSwitches(t, store, "task-1", id, TaskMRAutomationSwitchPatch{
		PromptOnMerged: boolPtr(true),
	})
	if !updated.PromptOnMerged || updated.PromptOnReviewRequested || updated.PromptOnClosed {
		t.Fatalf("unexpected options after first patch: %+v", updated)
	}

	updated = setMRSwitches(t, store, "task-1", id, TaskMRAutomationSwitchPatch{
		PromptOnReviewRequested: boolPtr(true),
	})
	if !updated.PromptOnMerged || !updated.PromptOnReviewRequested {
		t.Fatalf("expected merged switch preserved alongside the new one, got %+v", updated)
	}

	// The reviewer username stays task-level.
	username := "alice"
	if _, err := store.UpdateTaskMRAutomationOptions(ctx, "task-1", TaskMRAutomationPatch{}, &username); err != nil {
		t.Fatalf("persist reviewer: %v", err)
	}

	got, err := store.GetTaskMRAutomationOptionsForMR(ctx, "task-1", id)
	if err != nil {
		t.Fatalf("GetTaskMRAutomationOptionsForMR: %v", err)
	}
	if !got.PromptOnMerged || !got.PromptOnReviewRequested {
		t.Fatalf("persisted switches mismatch: %+v", got)
	}
	taskOpts, err := store.GetTaskMRAutomationOptions(ctx, "task-1")
	if err != nil || taskOpts.ReviewReviewerUsername != "alice" {
		t.Fatalf("persisted reviewer mismatch: %+v err=%v", taskOpts, err)
	}
}

func TestStore_TaskMRLifecycleState_CheckpointIsolation(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")

	if err := store.SetTaskMRObservedState(ctx, "task-1", "", "group/a", 1, "open"); err != nil {
		t.Fatalf("SetTaskMRObservedState a: %v", err)
	}
	if err := store.SetTaskMRObservedState(ctx, "task-1", "", "group/b", 2, "merged"); err != nil {
		t.Fatalf("SetTaskMRObservedState b: %v", err)
	}

	a, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || a == nil || a.LastObservedState != "open" {
		t.Fatalf("checkpoint a leaked or wrong: %+v err=%v", a, err)
	}
	b, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/b", 2)
	if err != nil || b == nil || b.LastObservedState != "merged" {
		t.Fatalf("checkpoint b leaked or wrong: %+v err=%v", b, err)
	}

	states, err := store.ListTaskMRLifecycleStates(ctx, "task-1")
	if err != nil || len(states) != 2 {
		t.Fatalf("expected 2 checkpoints, got %d err=%v", len(states), err)
	}
}

func TestStore_GetTaskMRLifecycleState_NilWhenAbsent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	got, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil {
		t.Fatalf("GetTaskMRLifecycleState: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil checkpoint, got %+v", got)
	}
}

func TestStore_SetTaskMRReviewRequestState(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")
	if err := store.SetTaskMRReviewRequestState(ctx, "task-1", "", "group/a", 1, true); err != nil {
		t.Fatalf("SetTaskMRReviewRequestState: %v", err)
	}
	got, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil || !got.ReviewRequestInitialized || !got.LastReviewRequested {
		t.Fatalf("unexpected checkpoint: %+v err=%v", got, err)
	}
}

func TestStore_RecordTaskMRLifecyclePrompt(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")
	err := store.RecordTaskMRLifecyclePrompt(ctx, TaskMRLifecyclePrompt{
		TaskID: "task-1", ProjectPath: "group/a", MRIID: 1,
		Event: mrLifecycleEventMerged, SessionID: "sess-1",
		PromptedAt: time.Now().UTC(), ObservedState: "merged",
	})
	if err != nil {
		t.Fatalf("RecordTaskMRLifecyclePrompt: %v", err)
	}
	got, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil {
		t.Fatalf("GetTaskMRLifecycleState: %+v err=%v", got, err)
	}
	if got.LastLifecycleEvent != mrLifecycleEventMerged || got.LastObservedState != "merged" ||
		got.LastLifecyclePromptAt == nil || got.LastLifecycleSessionID == nil || *got.LastLifecycleSessionID != "sess-1" {
		t.Fatalf("unexpected checkpoint after prompt: %+v", got)
	}
}

// TestStore_RecordTaskMRLifecyclePrompt_ClearsSessionIDWhenAbsent proves a
// prompt recorded without session context (SessionID == "") clears any
// previously stored session pointer rather than leaving it stale — the
// checkpoint must reflect the prompt actually being recorded, not a mix of
// this prompt's other fields and an earlier prompt's session.
func TestStore_RecordTaskMRLifecyclePrompt_ClearsSessionIDWhenAbsent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")
	if err := store.RecordTaskMRLifecyclePrompt(ctx, TaskMRLifecyclePrompt{
		TaskID: "task-1", ProjectPath: "group/a", MRIID: 1,
		Event: mrLifecycleEventReviewRequested, SessionID: "sess-1",
		PromptedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed prompt with session: %v", err)
	}
	if err := store.RecordTaskMRLifecyclePrompt(ctx, TaskMRLifecyclePrompt{
		TaskID: "task-1", ProjectPath: "group/a", MRIID: 1,
		Event: mrLifecycleEventMerged, PromptedAt: time.Now().UTC(), ObservedState: "merged",
	}); err != nil {
		t.Fatalf("RecordTaskMRLifecyclePrompt without session: %v", err)
	}
	got, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil {
		t.Fatalf("GetTaskMRLifecycleState: %+v err=%v", got, err)
	}
	if got.LastLifecycleSessionID != nil {
		t.Fatalf("expected session ID cleared for a session-less prompt, got %+v", got)
	}
}

func TestStore_RecordAndClearTaskMRAutomationError(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")
	if err := store.RecordTaskMRAutomationError(ctx, "task-1", "", "group/a", 1, "boom"); err != nil {
		t.Fatalf("RecordTaskMRAutomationError: %v", err)
	}
	got, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil || got.LastError == nil || *got.LastError != "boom" {
		t.Fatalf("unexpected checkpoint after error: %+v err=%v", got, err)
	}
	if err := store.ClearTaskMRAutomationError(ctx, "task-1", "", "group/a", 1); err != nil {
		t.Fatalf("ClearTaskMRAutomationError: %v", err)
	}
	got, err = store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil || got.LastError != nil {
		t.Fatalf("expected cleared error, got %+v err=%v", got, err)
	}
}

// TestStore_SyncErrorAndDeliveryErrorDoNotClobberEachOther proves the fix for
// a review finding: last_error (lifecycle delivery) and last_sync_error
// (poller sync) are separate columns, so a poller sync success clearing
// last_sync_error cannot erase an unresolved delivery error recorded by the
// orchestrator's evaluation pass, and vice versa.
func TestStore_SyncErrorAndDeliveryErrorDoNotClobberEachOther(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")

	if err := store.RecordTaskMRAutomationError(ctx, "task-1", "", "group/a", 1, "delivery failed"); err != nil {
		t.Fatalf("RecordTaskMRAutomationError: %v", err)
	}
	if err := store.RecordTaskMRSyncError(ctx, "task-1", "", "group/a", 1, "sync failed"); err != nil {
		t.Fatalf("RecordTaskMRSyncError: %v", err)
	}
	got, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil ||
		got.LastError == nil || *got.LastError != "delivery failed" ||
		got.LastSyncError == nil || *got.LastSyncError != "sync failed" {
		t.Fatalf("expected both errors recorded independently, got %+v err=%v", got, err)
	}

	// A recovered sync must not erase the still-unresolved delivery error.
	if err := store.ClearTaskMRSyncError(ctx, "task-1", "", "group/a", 1); err != nil {
		t.Fatalf("ClearTaskMRSyncError: %v", err)
	}
	got, err = store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil || got.LastSyncError != nil {
		t.Fatalf("expected sync error cleared, got %+v err=%v", got, err)
	}
	if got.LastError == nil || *got.LastError != "delivery failed" {
		t.Fatalf("expected delivery error to survive a sync-error clear, got %+v", got)
	}

	// A recovered delivery must not erase a still-unresolved sync error.
	if err := store.RecordTaskMRSyncError(ctx, "task-1", "", "group/a", 1, "sync failed again"); err != nil {
		t.Fatalf("RecordTaskMRSyncError: %v", err)
	}
	if err := store.ClearTaskMRAutomationError(ctx, "task-1", "", "group/a", 1); err != nil {
		t.Fatalf("ClearTaskMRAutomationError: %v", err)
	}
	got, err = store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil || got.LastError != nil {
		t.Fatalf("expected delivery error cleared, got %+v err=%v", got, err)
	}
	if got.LastSyncError == nil || *got.LastSyncError != "sync failed again" {
		t.Fatalf("expected sync error to survive a delivery-error clear, got %+v", got)
	}
}

func TestStore_RebindTaskMRReviewer_ClearsBaselines(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")
	alice := "alice"
	if _, err := store.UpdateTaskMRAutomationOptions(ctx, "task-1", TaskMRAutomationPatch{
		PromptOnReviewRequested: boolPtr(true),
	}, &alice); err != nil {
		t.Fatalf("seed options: %v", err)
	}
	if err := store.SetTaskMRReviewRequestState(ctx, "task-1", "", "group/a", 1, true); err != nil {
		t.Fatalf("SetTaskMRReviewRequestState: %v", err)
	}

	changed, err := store.RebindTaskMRReviewer(ctx, "task-1", "alice")
	if err != nil {
		t.Fatalf("RebindTaskMRReviewer (unchanged): %v", err)
	}
	if changed {
		t.Fatalf("expected no change when username is identical")
	}
	got, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil {
		t.Fatalf("GetTaskMRLifecycleState: %+v err=%v", got, err)
	}
	if !got.LastReviewRequested {
		t.Fatalf("baseline should survive an unchanged rebind: %+v", got)
	}

	changed, err = store.RebindTaskMRReviewer(ctx, "task-1", "bob")
	if err != nil {
		t.Fatalf("RebindTaskMRReviewer (changed): %v", err)
	}
	if !changed {
		t.Fatalf("expected change when username differs")
	}
	opts, err := store.GetTaskMRAutomationOptions(ctx, "task-1")
	if err != nil || opts.ReviewReviewerUsername != "bob" {
		t.Fatalf("reviewer not updated: %+v err=%v", opts, err)
	}
	got, err = store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil {
		t.Fatalf("GetTaskMRLifecycleState: %+v err=%v", got, err)
	}
	if got.ReviewRequestInitialized || got.LastReviewRequested {
		t.Fatalf("expected review baseline reset after reviewer change: %+v", got)
	}
}

// TestStore_UpdateTaskMRAutomationOptions_ResendingSameSwitchResetsBaselineOnIdentityChange
// pins a gap in the boolean-only reviewChanged check: a PATCH that resends
// prompt_on_review_requested=true (already true) still re-resolves the
// authenticated username, which can differ after the workspace's connected
// GitLab account changes. The baseline must reset even though the boolean
// itself never flips, otherwise a request already recorded for the old
// identity can suppress the next prompt evaluated against the new one.
func TestStore_UpdateTaskMRAutomationOptions_ResendingSameSwitchResetsBaselineOnIdentityChange(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")
	alice := "alice"
	if _, err := store.UpdateTaskMRAutomationOptions(ctx, "task-1", TaskMRAutomationPatch{
		PromptOnReviewRequested: boolPtr(true),
	}, &alice); err != nil {
		t.Fatalf("seed options: %v", err)
	}
	if err := store.SetTaskMRReviewRequestState(ctx, "task-1", "", "group/a", 1, true); err != nil {
		t.Fatalf("SetTaskMRReviewRequestState: %v", err)
	}

	bob := "bob"
	if _, err := store.UpdateTaskMRAutomationOptions(ctx, "task-1", TaskMRAutomationPatch{
		PromptOnReviewRequested: boolPtr(true), // unchanged: was already true
	}, &bob); err != nil {
		t.Fatalf("resend patch with new identity: %v", err)
	}

	opts, err := store.GetTaskMRAutomationOptions(ctx, "task-1")
	if err != nil || opts.ReviewReviewerUsername != "bob" {
		t.Fatalf("reviewer not updated: %+v err=%v", opts, err)
	}
	got, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil {
		t.Fatalf("GetTaskMRLifecycleState: %+v err=%v", got, err)
	}
	if got.ReviewRequestInitialized || got.LastReviewRequested {
		t.Fatalf("expected review baseline reset after identity change even with an unchanged switch: %+v", got)
	}
}

// TestStore_UpdateTaskMRAutomationOptionsForMR_ReenablingMergedSwitchResetsCheckpoint
// is the P1 finding: an MR that reached the merged state while the switch was
// off must not stay permanently suppressed once the switch is re-enabled.
// The sibling MR proves the reset is scoped to the MR being reconfigured.
func TestStore_UpdateTaskMRAutomationOptionsForMR_ReenablingMergedSwitchResetsCheckpoint(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")
	id := mrIdentity("group/a", 1)

	setMRSwitches(t, store, "task-1", id, TaskMRAutomationSwitchPatch{PromptOnMerged: boolPtr(true)})
	for _, mr := range []struct {
		project string
		iid     int
	}{{"group/a", 1}, {"group/b", 2}} {
		if err := store.RecordTaskMRLifecyclePrompt(ctx, TaskMRLifecyclePrompt{
			TaskID: "task-1", ProjectPath: mr.project, MRIID: mr.iid,
			Event: gitlabStateMerged, PromptedAt: time.Now().UTC(), ObservedState: gitlabStateMerged,
		}); err != nil {
			t.Fatalf("record merged prompt for %s: %v", mr.project, err)
		}
	}

	// Disable, then re-enable — the checkpoint from the still-merged MR must
	// not survive, or the re-enabled switch would never fire for it again.
	setMRSwitches(t, store, "task-1", id, TaskMRAutomationSwitchPatch{PromptOnMerged: boolPtr(false)})
	setMRSwitches(t, store, "task-1", id, TaskMRAutomationSwitchPatch{PromptOnMerged: boolPtr(true)})

	got, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil {
		t.Fatalf("GetTaskMRLifecycleState: %+v err=%v", got, err)
	}
	if got.LastObservedState != "" || got.LastLifecycleEvent != "" {
		t.Fatalf("expected terminal checkpoint reset after re-enabling the switch, got %+v", got)
	}
	sibling, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/b", 2)
	if err != nil || sibling == nil {
		t.Fatalf("GetTaskMRLifecycleState(sibling): %+v err=%v", sibling, err)
	}
	if sibling.LastLifecycleEvent != gitlabStateMerged {
		t.Fatalf("sibling MR's checkpoint was cleared by another MR's switch flip: %+v", sibling)
	}
}

// TestStore_UpdateTaskMRAutomationOptionsForMR_ReenablingAutoFixResetsRoundCap
// covers AC11: toggling auto-fix off and on again must clear
// auto_fix_exhausted_at and auto_fix_round_count so a subsequent failing
// pipeline can dispatch again.
func TestStore_UpdateTaskMRAutomationOptionsForMR_ReenablingAutoFixResetsRoundCap(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")
	id := mrIdentity("group/a", 1)

	setMRSwitches(t, store, "task-1", id, TaskMRAutomationSwitchPatch{AutoFixEnabled: boolPtr(true)})
	if err := store.RecordTaskMRFixAttempt(ctx, TaskMRFixAttempt{
		TaskID: "task-1", ProjectPath: "group/a", MRIID: 1,
		Signature: "sig-1", CheckpointJSON: "{}", SessionID: "sess-1",
		EnqueuedAt: time.Now().UTC(), IncrementRound: true,
	}); err != nil {
		t.Fatalf("record fix attempt: %v", err)
	}
	if err := store.MarkTaskMRAutoFixExhausted(ctx, "task-1", "", "group/a", 1, "paused"); err != nil {
		t.Fatalf("mark exhausted: %v", err)
	}

	got, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil {
		t.Fatalf("GetTaskMRLifecycleState: %+v err=%v", got, err)
	}
	if got.AutoFixRoundCount != 1 || got.AutoFixExhaustedAt == nil {
		t.Fatalf("expected round count 1 and exhausted set before reset, got %+v", got)
	}

	// Disable, then re-enable — the round cap and exhaustion must clear.
	setMRSwitches(t, store, "task-1", id, TaskMRAutomationSwitchPatch{AutoFixEnabled: boolPtr(false)})
	setMRSwitches(t, store, "task-1", id, TaskMRAutomationSwitchPatch{AutoFixEnabled: boolPtr(true)})

	got, err = store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil {
		t.Fatalf("GetTaskMRLifecycleState after reset: %+v err=%v", got, err)
	}
	if got.AutoFixRoundCount != 0 || got.AutoFixExhaustedAt != nil {
		t.Fatalf("expected round count and exhaustion cleared after re-enabling, got %+v", got)
	}
}

// TestStore_RecordTaskMRFixAttempt_IncrementsRoundOnlyWhenRequested covers
// the round-cap accounting AC10 depends on: IncrementRound=false (a queued
// replace, not a new round) must not advance the counter.
func TestStore_RecordTaskMRFixAttempt_IncrementsRoundOnlyWhenRequested(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")

	for i := 0; i < 3; i++ {
		if err := store.RecordTaskMRFixAttempt(ctx, TaskMRFixAttempt{
			TaskID: "task-1", ProjectPath: "group/a", MRIID: 1,
			Signature: "sig", CheckpointJSON: "{}", IncrementRound: true,
		}); err != nil {
			t.Fatalf("record fix attempt %d: %v", i, err)
		}
	}
	if err := store.RecordTaskMRFixAttempt(ctx, TaskMRFixAttempt{
		TaskID: "task-1", ProjectPath: "group/a", MRIID: 1,
		Signature: "sig2", CheckpointJSON: "{}", IncrementRound: false,
	}); err != nil {
		t.Fatalf("record non-incrementing attempt: %v", err)
	}

	got, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil {
		t.Fatalf("GetTaskMRLifecycleState: %+v err=%v", got, err)
	}
	if got.AutoFixRoundCount != 3 {
		t.Errorf("AutoFixRoundCount = %d, want 3 (the non-incrementing attempt must not count)", got.AutoFixRoundCount)
	}
	if got.LastFixSignature != "sig2" {
		t.Errorf("LastFixSignature = %q, want the latest attempt's signature", got.LastFixSignature)
	}
}

// TestStore_ObservedStateAndErrorMutatorsDoNotClobberEachOther pins the
// data-integrity fix directly: SetTaskMRObservedState and
// RecordTaskMRAutomationError each write only their own column now, so
// calling one after the other must not erase the first one's write (the
// previous full-row-upsert implementation would have lost this on a
// concurrent — or here, simply interleaved — pair of calls).
func TestStore_ObservedStateAndErrorMutatorsDoNotClobberEachOther(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")

	if err := store.SetTaskMRObservedState(ctx, "task-1", "", "group/a", 1, "opened"); err != nil {
		t.Fatalf("SetTaskMRObservedState: %v", err)
	}
	if err := store.RecordTaskMRAutomationError(ctx, "task-1", "", "group/a", 1, "transient failure"); err != nil {
		t.Fatalf("RecordTaskMRAutomationError: %v", err)
	}

	got, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil {
		t.Fatalf("GetTaskMRLifecycleState: %+v err=%v", got, err)
	}
	if got.LastObservedState != "opened" {
		t.Fatalf("RecordTaskMRAutomationError clobbered last_observed_state: %+v", got)
	}
	if got.LastError == nil || *got.LastError != "transient failure" {
		t.Fatalf("expected last_error set: %+v", got)
	}
}

// TestStore_TaskDeleteCascadesMRAutomationRows is the FK-cascade finding: a
// hard-deleted task must not leave its MR automation options or lifecycle
// checkpoints behind — both tables declare ON DELETE CASCADE against tasks.
func TestStore_TaskDeleteCascadesMRAutomationRows(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")

	if _, err := store.UpdateTaskMRAutomationOptions(ctx, "task-1", TaskMRAutomationPatch{
		AutoFixPromptOverride: stringPtr("custom"),
	}, nil); err != nil {
		t.Fatalf("seed options: %v", err)
	}
	setMRSwitches(t, store, "task-1", mrIdentity("group/a", 1), TaskMRAutomationSwitchPatch{
		PromptOnMerged: boolPtr(true),
	})
	if err := store.SetTaskMRObservedState(ctx, "task-1", "", "group/a", 1, "opened"); err != nil {
		t.Fatalf("seed checkpoint: %v", err)
	}

	if _, err := store.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, "task-1"); err != nil {
		t.Fatalf("delete task: %v", err)
	}

	opts, err := store.GetTaskMRAutomationOptions(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTaskMRAutomationOptions: %v", err)
	}
	if opts.AutoFixPromptOverride != nil {
		t.Fatalf("expected options row cascaded away, got %+v", opts)
	}
	switches, err := store.ListTaskMRAutomationOptions(ctx, "task-1")
	if err != nil {
		t.Fatalf("ListTaskMRAutomationOptions: %v", err)
	}
	if len(switches) != 0 {
		t.Fatalf("expected per-MR switch rows cascaded away, got %+v", switches)
	}
	state, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil {
		t.Fatalf("GetTaskMRLifecycleState: %v", err)
	}
	if state != nil {
		t.Fatalf("expected lifecycle state row cascaded away, got %+v", state)
	}
}

func TestStore_ListAutomationSubscribedTaskMRs(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedWorkspace(t, store, "ws-1")
	seedTask(t, store, "task-1", "ws-1")
	seedTask(t, store, "task-2", "ws-1")
	seedTask(t, store, "task-3", "ws-1")

	subscribed := newTestMR("task-1", "", "group/subscribed", 1)
	if err := store.UpsertTaskMR(ctx, subscribed); err != nil {
		t.Fatalf("upsert subscribed MR: %v", err)
	}
	unsubscribed := newTestMR("task-2", "", "group/unsubscribed", 2)
	if err := store.UpsertTaskMR(ctx, unsubscribed); err != nil {
		t.Fatalf("upsert unsubscribed MR: %v", err)
	}
	autoFixOnly := newTestMR("task-3", "", "group/autofix", 3)
	if err := store.UpsertTaskMR(ctx, autoFixOnly); err != nil {
		t.Fatalf("upsert auto-fix-only MR: %v", err)
	}
	setMRSwitches(t, store, "task-1", mrIdentity("group/subscribed", 1), TaskMRAutomationSwitchPatch{
		PromptOnMerged: boolPtr(true),
	})
	// task-3 has no lifecycle switch on, only auto-fix — must still be
	// returned since the query was widened to OR in auto_fix_enabled /
	// auto_merge_enabled (this task's change).
	setMRSwitches(t, store, "task-3", mrIdentity("group/autofix", 3), TaskMRAutomationSwitchPatch{
		AutoFixEnabled: boolPtr(true),
	})

	rows, err := store.ListAutomationSubscribedTaskMRs(ctx)
	if err != nil {
		t.Fatalf("ListAutomationSubscribedTaskMRs: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected task-1's lifecycle-subscribed MR and task-3's auto-fix-subscribed MR, got %+v", rows)
	}
	gotTasks := map[string]bool{rows[0].TaskID: true, rows[1].TaskID: true}
	if !gotTasks["task-1"] || !gotTasks["task-3"] {
		t.Fatalf("expected task-1 and task-3, got %+v", rows)
	}
}

// TestStore_MRAutomationTables_FreshDBAndReplay covers ADR 0027: schema
// creation must succeed both against a brand new database file and when run
// a second time against an existing one (idempotent CREATE TABLE IF NOT
// EXISTS — no ALTER TABLE migration is involved since both tables are new).
func TestStore_MRAutomationTables_FreshDBAndReplay(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "gitlab-replay.db")

	dbConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.Close() })
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	if _, err := sqlxDB.Exec(`CREATE TABLE workspaces (id TEXT PRIMARY KEY);
		CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '', archived_at DATETIME)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	store, err := NewStore(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("fresh-DB NewStore: %v", err)
	}
	assertMRAutomationTablesExist(t, sqlxDB)

	// Same-DB replay: open a second Store against the same file/handle and
	// confirm createTables (and specifically createMRAutomationTables) is a
	// no-op rather than an error on an existing database.
	if _, err := NewStore(sqlxDB, sqlxDB); err != nil {
		t.Fatalf("same-DB replay NewStore: %v", err)
	}
	assertMRAutomationTablesExist(t, sqlxDB)

	// A nil error from CREATE TABLE IF NOT EXISTS doesn't prove the columns
	// this package's mutators depend on actually exist — exercise a
	// representative write/read round trip through every column added this
	// session (including the last_sync_error split) as a functional check.
	ctx := context.Background()
	if _, err := sqlxDB.Exec(`INSERT INTO tasks (id, workspace_id) VALUES ('task-1', '')`); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if err := store.RecordTaskMRAutomationError(ctx, "task-1", "", "group/a", 1, "delivery"); err != nil {
		t.Fatalf("RecordTaskMRAutomationError: %v", err)
	}
	if err := store.RecordTaskMRSyncError(ctx, "task-1", "", "group/a", 1, "sync"); err != nil {
		t.Fatalf("RecordTaskMRSyncError: %v", err)
	}
	got, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil ||
		got.LastError == nil || *got.LastError != "delivery" ||
		got.LastSyncError == nil || *got.LastSyncError != "sync" {
		t.Fatalf("expected both error columns to round-trip, got %+v err=%v", got, err)
	}
}

// assertMRAutomationTablesExist queries sqlite_master directly so a nil error
// from CREATE TABLE IF NOT EXISTS can't mask a table that was never actually
// created (e.g. a typo in the DDL that SQLite silently no-ops on replay).
func assertMRAutomationTablesExist(t *testing.T, sqlxDB *sqlx.DB) {
	t.Helper()
	for _, table := range []string{"gitlab_task_mr_options", "gitlab_task_mr_state"} {
		var name string
		if err := sqlxDB.Get(&name, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table); err != nil {
			t.Fatalf("expected table %q to exist: %v", table, err)
		}
	}
}

// TestStore_DeleteTaskMRForWorkspace_DropsPerMRAutomationOptions covers the
// unlink half of the per-MR switch lifecycle. Leaving the switch row behind
// meant re-linking the same MR — by hand, or through push-detection auto-link
// — silently re-armed whatever was configured before the unlink, including
// auto-merge, with no surface showing it (taskMRAutomationOptionsList hides
// rows whose MR is not linked) but the evaluator still reading it.
func TestStore_DeleteTaskMRForWorkspace_DropsPerMRAutomationOptions(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedWorkspace(t, store, "ws-1")
	seedTask(t, store, "task-1", "ws-1")

	mr := newTestMR("task-1", "", "group/a", 1)
	if err := store.UpsertTaskMR(ctx, mr); err != nil {
		t.Fatalf("upsert MR: %v", err)
	}
	id := MRIdentity{RepositoryID: "", ProjectPath: "group/a", MRIID: 1}
	if _, err := store.UpdateTaskMRAutomationOptionsForMR(
		ctx, "task-1", id, TaskMRAutomationSwitchPatch{AutoMergeEnabled: boolPtr(true)},
	); err != nil {
		t.Fatalf("enable auto-merge: %v", err)
	}
	if err := store.SetTaskMRObservedState(ctx, "task-1", "", "group/a", 1, "merged"); err != nil {
		t.Fatalf("seed lifecycle state: %v", err)
	}

	if err := store.DeleteTaskMRForWorkspace(ctx, "ws-1", mr.ID); err != nil {
		t.Fatalf("DeleteTaskMRForWorkspace: %v", err)
	}

	stored, err := store.ListTaskMRAutomationOptions(ctx, "task-1")
	if err != nil {
		t.Fatalf("ListTaskMRAutomationOptions: %v", err)
	}
	if len(stored) != 0 {
		t.Fatalf("unlink left automation rows behind: %+v", stored)
	}
	state, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil {
		t.Fatalf("get lifecycle state: %v", err)
	}
	if state != nil {
		t.Fatalf("unlink left lifecycle state behind: %+v", state)
	}

	// Re-linking the same MR must start from all-off, not resurrect the
	// pre-unlink configuration.
	relinked := newTestMR("task-1", "", "group/a", 1)
	if err := store.UpsertTaskMR(ctx, relinked); err != nil {
		t.Fatalf("re-link MR: %v", err)
	}
	opts, err := store.GetTaskMRAutomationOptionsForMR(ctx, "task-1", id)
	if err != nil {
		t.Fatalf("GetTaskMRAutomationOptionsForMR: %v", err)
	}
	if opts.AutoMergeEnabled {
		t.Errorf("re-linked MR silently re-armed auto-merge: %+v", opts)
	}
}

// TestStore_DeleteTaskMR_DropsPerMRAutomationOptions pins the same cleanup on
// the association-ID delete path, which cascades to gitlab_task_mr_state
// independently of DeleteTaskMRForWorkspace. The two must stay in step: a
// caller routed through this one would otherwise leave an enabled auto-merge
// switch behind for the next link of the same MR to inherit.
func TestStore_DeleteTaskMR_DropsPerMRAutomationOptions(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedWorkspace(t, store, "ws-1")
	seedTask(t, store, "task-1", "ws-1")

	mr := newTestMR("task-1", "", "group/a", 1)
	if err := store.UpsertTaskMR(ctx, mr); err != nil {
		t.Fatalf("upsert MR: %v", err)
	}
	id := MRIdentity{RepositoryID: "", ProjectPath: "group/a", MRIID: 1}
	if _, err := store.UpdateTaskMRAutomationOptionsForMR(
		ctx, "task-1", id, TaskMRAutomationSwitchPatch{AutoMergeEnabled: boolPtr(true)},
	); err != nil {
		t.Fatalf("enable auto-merge: %v", err)
	}

	if err := store.DeleteTaskMR(ctx, mr.ID); err != nil {
		t.Fatalf("DeleteTaskMR: %v", err)
	}

	stored, err := store.ListTaskMRAutomationOptions(ctx, "task-1")
	if err != nil {
		t.Fatalf("ListTaskMRAutomationOptions: %v", err)
	}
	if len(stored) != 0 {
		t.Fatalf("delete left automation rows behind: %+v", stored)
	}
}

// TestStore_UpdateTaskMRAutomationOptionsForMRs_IsAllOrNothing pins the
// fan-out atomicity: a failure partway through must not leave the switch
// armed on the MRs processed before it while the caller is told the operation
// failed. A trigger supplies the deterministic mid-batch failure — the second
// identity's UPDATE aborts, and the first identity's already-applied write
// must roll back with it.
func TestStore_UpdateTaskMRAutomationOptionsForMRs_IsAllOrNothing(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedWorkspace(t, store, "ws-1")
	seedTask(t, store, "task-1", "ws-1")

	first := MRIdentity{RepositoryID: "", ProjectPath: "group/a", MRIID: 1}
	second := MRIdentity{RepositoryID: "", ProjectPath: "group/b", MRIID: 2}
	for _, id := range []MRIdentity{first, second} {
		if err := store.UpsertTaskMR(ctx, newTestMR("task-1", id.RepositoryID, id.ProjectPath, id.MRIID)); err != nil {
			t.Fatalf("link MR %s: %v", id.ProjectPath, err)
		}
	}
	if _, err := store.db.Exec(`
		CREATE TRIGGER fail_second_mr BEFORE UPDATE ON gitlab_task_mr_automation_options
		WHEN NEW.project_path = 'group/b'
		BEGIN SELECT RAISE(ABORT, 'injected failure'); END`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	err := store.UpdateTaskMRAutomationOptionsForMRs(
		ctx, "task-1", []MRIdentity{first, second},
		TaskMRAutomationSwitchPatch{AutoMergeEnabled: boolPtr(true)},
	)
	if err == nil {
		t.Fatal("expected the batch to fail on the second identity")
	}

	got, err := store.GetTaskMRAutomationOptionsForMR(ctx, "task-1", first)
	if err != nil {
		t.Fatalf("GetTaskMRAutomationOptionsForMR: %v", err)
	}
	if got.AutoMergeEnabled {
		t.Error("first MR kept auto-merge after the batch failed — fan-out was not atomic")
	}
}

// TestStore_UpdateTaskMRAutomationOptionsForMRs_EmptyIsNoop guards the
// zero-target call so it cannot open an empty transaction per request.
func TestStore_UpdateTaskMRAutomationOptionsForMRs_EmptyIsNoop(t *testing.T) {
	store := newTestStore(t)
	seedWorkspace(t, store, "ws-1")
	seedTask(t, store, "task-1", "ws-1")
	if err := store.UpdateTaskMRAutomationOptionsForMRs(
		context.Background(), "task-1", nil,
		TaskMRAutomationSwitchPatch{AutoMergeEnabled: boolPtr(true)},
	); err != nil {
		t.Fatalf("empty batch should be a no-op, got %v", err)
	}
}
