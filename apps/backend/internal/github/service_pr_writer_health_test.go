package github

import (
	"context"
	"testing"
	"time"
)

// TestWriterHealthInvariants_AC36_AC37_AC39 is the regression for the Writer
// health verification surface: over one seeded database with a known
// activation instant, AC-36 and AC-37 must hold for rows a populating sync
// legitimately observed, AC-39 must exclude rows predating activation from
// either check entirely, and the three legitimate NULL classes named in the
// Writer health note (deleted-merger-account, merge-boundary poll-inversion,
// and the AC-12a isDraft-omission case) must be treated as exemptions, not
// invariant violations. A checker that asserts either invariant
// unconditionally would fail every exemption row below against a correct
// implementation, which is exactly what this test guards against.
func TestWriterHealthInvariants_AC36_AC37_AC39(t *testing.T) {
	svc, store, _ := setupSyncTest(t)
	ctx := context.Background()

	activation, err := time.Parse(time.RFC3339, requireOutcomeActivationOnce(t, store.db))
	if err != nil {
		t.Fatalf("parse activation instant: %v", err)
	}
	after := activation.Add(time.Hour)
	before := activation.Add(-time.Hour)

	seedOpenPR := func(taskID string, prNumber int) {
		t.Helper()
		createOutcomeSyncTestTaskPR(t, store, &TaskPR{
			TaskID: taskID, Owner: "owner", Repo: "repo", PRNumber: prNumber,
			PRURL: "https://github.com/owner/repo/pull/1", PRTitle: "Writer health fixture",
			HeadBranch: "feat", BaseBranch: "main", State: "open",
		})
	}

	// AC-36, normal case: a row merged after activation by a populating sync
	// must carry a non-NULL merged_by_login.
	seedOpenPR("t-merge-normal", 1)
	mergedAtNormal := after
	if err := svc.SyncTaskPR(ctx, "t-merge-normal", &PRStatus{
		PR: &PR{
			Number: 1, State: prStateMerged, RepoOwner: "owner", RepoName: "repo",
			MergedAt: &mergedAtNormal, MergedByLogin: "carlosflorencio",
			IsDraftObserved: true, ChangedFilesObserved: true,
		},
		OutcomeFieldsPopulated: true,
	}); err != nil {
		t.Fatalf("SyncTaskPR (normal merge): %v", err)
	}
	mergeNormal, err := store.GetTaskPR(ctx, "t-merge-normal")
	if err != nil {
		t.Fatalf("GetTaskPR (normal merge): %v", err)
	}
	if mergeNormal.MergedAt == nil || mergeNormal.MergedAt.Before(activation) {
		t.Fatalf("test setup: t-merge-normal merged_at = %v, want set and >= activation %v", mergeNormal.MergedAt, activation)
	}
	if mergeNormal.MergedByLogin == nil || *mergeNormal.MergedByLogin != "carlosflorencio" {
		t.Errorf("AC-36: merged_by_login = %v, want non-NULL for a row merged after activation by a populating sync", mergeNormal.MergedByLogin)
	}

	// AC-37, normal case: a row synced after activation by a populating sync
	// must carry a non-NULL is_draft.
	seedOpenPR("t-draft-normal", 2)
	if err := svc.SyncTaskPR(ctx, "t-draft-normal", &PRStatus{
		PR: &PR{
			Number: 2, State: "open", RepoOwner: "owner", RepoName: "repo",
			Draft: false, IsDraftObserved: true, ChangedFilesObserved: true,
		},
		OutcomeFieldsPopulated: true,
	}); err != nil {
		t.Fatalf("SyncTaskPR (normal draft sync): %v", err)
	}
	draftNormal, err := store.GetTaskPR(ctx, "t-draft-normal")
	if err != nil {
		t.Fatalf("GetTaskPR (normal draft sync): %v", err)
	}
	if draftNormal.LastSyncedAt == nil || draftNormal.LastSyncedAt.Before(activation) {
		t.Fatalf("test setup: t-draft-normal last_synced_at = %v, want set and >= activation %v", draftNormal.LastSyncedAt, activation)
	}
	if draftNormal.IsDraft == nil {
		t.Errorf("AC-37: is_draft = nil, want non-NULL for a row synced after activation by a populating sync")
	}

	// Exemption 1 (deleted-merger-account): merged after activation, but
	// upstream reports no merger. merged_by_login staying NULL here is not a
	// writer fault.
	seedOpenPR("t-deleted-merger", 3)
	mergedAtDeletedMerger := after
	if err := svc.SyncTaskPR(ctx, "t-deleted-merger", &PRStatus{
		PR: &PR{
			Number: 3, State: prStateMerged, RepoOwner: "owner", RepoName: "repo",
			MergedAt: &mergedAtDeletedMerger, MergedByLogin: "",
			IsDraftObserved: true, ChangedFilesObserved: true,
		},
		OutcomeFieldsPopulated: true,
	}); err != nil {
		t.Fatalf("SyncTaskPR (deleted merger): %v", err)
	}
	deletedMerger, err := store.GetTaskPR(ctx, "t-deleted-merger")
	if err != nil {
		t.Fatalf("GetTaskPR (deleted merger): %v", err)
	}
	if deletedMerger.MergedAt == nil || deletedMerger.MergedAt.Before(activation) {
		t.Fatalf("test setup: t-deleted-merger merged_at = %v, want set and >= activation %v", deletedMerger.MergedAt, activation)
	}
	if deletedMerger.MergedByLogin != nil {
		t.Errorf("exemption (deleted-merger-account): merged_by_login = %q, want NULL: upstream reported no merger for a real merge, not a writer fault",
			*deletedMerger.MergedByLogin)
	}

	// Exemption 2 (merge-boundary poll-inversion): two polls straddle the
	// merge; the earlier-reading poll's write lands last and clobbers
	// merged_by_login back to NULL. The row is terminal, so no later poll
	// repairs it. Simulated directly at the store layer since reproducing
	// the actual race nondeterministically would not make this assertion any
	// stronger — the accepted residual is the stored state itself, not the
	// interleaving that produced it.
	seedOpenPR("t-poll-inversion", 4)
	mergedAtInversion := after
	if err := svc.SyncTaskPR(ctx, "t-poll-inversion", &PRStatus{
		PR: &PR{
			Number: 4, State: prStateMerged, RepoOwner: "owner", RepoName: "repo",
			MergedAt: &mergedAtInversion, MergedByLogin: "alice",
			IsDraftObserved: true, ChangedFilesObserved: true,
		},
		OutcomeFieldsPopulated: true,
	}); err != nil {
		t.Fatalf("SyncTaskPR (poll-inversion, first poll): %v", err)
	}
	straddled, err := store.GetTaskPR(ctx, "t-poll-inversion")
	if err != nil {
		t.Fatalf("GetTaskPR (poll-inversion, before second poll): %v", err)
	}
	if straddled.MergedByLogin == nil {
		t.Fatalf("test setup: t-poll-inversion merged_by_login not set by first poll")
	}
	straddled.MergedByLogin = nil
	if err := store.UpdateTaskPR(ctx, straddled); err != nil {
		t.Fatalf("UpdateTaskPR (poll-inversion, second poll landing last): %v", err)
	}
	pollInversion, err := store.GetTaskPR(ctx, "t-poll-inversion")
	if err != nil {
		t.Fatalf("GetTaskPR (poll-inversion, after second poll): %v", err)
	}
	if pollInversion.MergedAt == nil || pollInversion.MergedAt.Before(activation) {
		t.Fatalf("test setup: t-poll-inversion merged_at = %v, want set and >= activation %v", pollInversion.MergedAt, activation)
	}
	if pollInversion.MergedByLogin != nil {
		t.Errorf("exemption (merge-boundary poll-inversion): merged_by_login = %q, want NULL: the earlier-straddling poll's write landed last and the row is terminal",
			*pollInversion.MergedByLogin)
	}

	// Exemption 3 (AC-12a isDraft omission): synced after activation by a
	// populating sync, but upstream omitted isDraft on that response.
	// is_draft staying NULL here is not a writer fault.
	seedOpenPR("t-missing-isdraft", 5)
	if err := svc.SyncTaskPR(ctx, "t-missing-isdraft", &PRStatus{
		PR: &PR{
			Number: 5, State: "open", RepoOwner: "owner", RepoName: "repo",
			IsDraftObserved: false, ChangedFilesObserved: true,
		},
		OutcomeFieldsPopulated: true,
	}); err != nil {
		t.Fatalf("SyncTaskPR (missing isDraft): %v", err)
	}
	missingIsDraft, err := store.GetTaskPR(ctx, "t-missing-isdraft")
	if err != nil {
		t.Fatalf("GetTaskPR (missing isDraft): %v", err)
	}
	if missingIsDraft.LastSyncedAt == nil || missingIsDraft.LastSyncedAt.Before(activation) {
		t.Fatalf("test setup: t-missing-isdraft last_synced_at = %v, want set and >= activation %v", missingIsDraft.LastSyncedAt, activation)
	}
	if missingIsDraft.IsDraft != nil {
		t.Errorf("exemption (AC-12a isDraft omission): is_draft = %v, want NULL: the populating response omitted isDraft, not a writer fault",
			*missingIsDraft.IsDraft)
	}

	// AC-39: a row whose merged_at predates activation is excluded from
	// AC-36 entirely — it is legitimately and permanently NULL, and no
	// assertion is made on merged_by_login for it below. Seeded directly
	// (not through SyncTaskPR, whose LastSyncedAt is always "now" and so
	// could never itself land before activation).
	preActivationMergedAt := before
	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID: "t-pre-activation-merge", Owner: "owner", Repo: "repo", PRNumber: 6,
		PRURL: "https://github.com/owner/repo/pull/6", PRTitle: "Merged before activation",
		HeadBranch: "feat", BaseBranch: "main", State: prStateMerged,
		MergedAt: &preActivationMergedAt,
	}); err != nil {
		t.Fatalf("CreateTaskPR (pre-activation merge): %v", err)
	}
	preActivation, err := store.GetTaskPR(ctx, "t-pre-activation-merge")
	if err != nil {
		t.Fatalf("GetTaskPR (pre-activation merge): %v", err)
	}
	if preActivation.MergedAt == nil || !preActivation.MergedAt.Before(activation) {
		t.Fatalf("test setup: t-pre-activation-merge merged_at = %v, want set and before activation %v", preActivation.MergedAt, activation)
	}
	// Deliberately no assertion on preActivation.MergedByLogin (AC-39): this
	// row predates activation, so AC-36 does not apply to it regardless of
	// what merged_by_login holds.
}
