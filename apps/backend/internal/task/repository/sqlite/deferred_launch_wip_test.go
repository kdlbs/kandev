package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func wipOnlyRecord() map[string]interface{} {
	return map[string]interface{}{
		models.DeferredLaunchStartWhenUnblockedKey: true,
		models.DeferredLaunchUserIDKey:             "user-1",
	}
}

func mixedRecord() map[string]interface{} {
	record := wipOnlyRecord()
	record[models.CeilingDeferredKey] = true
	record[models.CeilingLaunchKindKey] = "start_task"
	return record
}

func storedDeferredLaunch(t *testing.T, repo *Repository, taskID string) map[string]interface{} {
	t.Helper()
	record, _, err := repo.GetTaskDeferredLaunch(context.Background(), taskID)
	if err != nil {
		t.Fatalf("GetTaskDeferredLaunch(%s): %v", taskID, err)
	}
	return record
}

func createTaskWithDeferredLaunch(t *testing.T, repo *Repository, taskID string, record map[string]interface{}) {
	t.Helper()
	task := &models.Task{ID: taskID, Title: taskID}
	if record != nil {
		task.Metadata = map[string]interface{}{models.MetaKeyDeferredLaunch: record}
	}
	if err := repo.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("CreateTask(%s): %v", taskID, err)
	}
}

// TestTakeWIPKeysLeavesCeilingKeysInPlace is the criterion this pair exists for:
// the WIP claim must not delete a ceiling record that landed beside it. Losing
// that record is a silently dropped launch, and it is unrecoverable rather than
// merely stale, because the sweep then has nothing to find.
func TestTakeWIPKeysLeavesCeilingKeysInPlace(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	createTaskWithDeferredLaunch(t, repo, "wip-mixed", mixedRecord())

	wip, claimed, err := repo.TakeTaskDeferredLaunchWIPKeys(ctx, "wip-mixed")
	if err != nil {
		t.Fatalf("TakeTaskDeferredLaunchWIPKeys: %v", err)
	}
	if !claimed {
		t.Fatal("claimed = false, want true: the record carries WIP keys")
	}
	if wip[models.DeferredLaunchStartWhenUnblockedKey] != true || wip[models.DeferredLaunchUserIDKey] != "user-1" {
		t.Fatalf("returned WIP keys = %+v, want both WIP keys", wip)
	}
	if _, leaked := wip[models.CeilingDeferredKey]; leaked {
		t.Fatalf("ceiling key was taken as a WIP key: %+v", wip)
	}

	remaining := storedDeferredLaunch(t, repo, "wip-mixed")
	if remaining == nil {
		t.Fatal("the whole deferred_launch record was deleted; the ceiling deferral is gone")
	}
	if remaining[models.CeilingDeferredKey] != true || remaining[models.CeilingLaunchKindKey] != "start_task" {
		t.Fatalf("ceiling keys did not survive the claim: %+v", remaining)
	}
	if _, still := remaining[models.DeferredLaunchStartWhenUnblockedKey]; still {
		t.Fatalf("WIP key survived the claim: %+v", remaining)
	}
}

// TestTakeWIPKeysDeletesTheRecordWhenOnlyWIPKeysExisted preserves today's
// behaviour for every record that exists before this card.
func TestTakeWIPKeysDeletesTheRecordWhenOnlyWIPKeysExisted(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	createTaskWithDeferredLaunch(t, repo, "wip-only", wipOnlyRecord())

	wip, claimed, err := repo.TakeTaskDeferredLaunchWIPKeys(ctx, "wip-only")
	if err != nil || !claimed {
		t.Fatalf("take = (%+v, %v, %v), want claimed", wip, claimed, err)
	}
	if remaining := storedDeferredLaunch(t, repo, "wip-only"); remaining != nil {
		t.Fatalf("record survived as %+v, want the key deleted entirely", remaining)
	}
}

func TestTakeWIPKeysIsInertWhenOnlyCeilingKeysExist(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	createTaskWithDeferredLaunch(t, repo, "wip-ceiling-only", map[string]interface{}{
		models.CeilingDeferredKey: true,
	})

	wip, claimed, err := repo.TakeTaskDeferredLaunchWIPKeys(ctx, "wip-ceiling-only")
	if err != nil {
		t.Fatalf("TakeTaskDeferredLaunchWIPKeys: %v", err)
	}
	if claimed || len(wip) != 0 {
		t.Fatalf("take = (%+v, %v), want inert: a ceiling deferral is not the WIP path's to consume", wip, claimed)
	}
	if remaining := storedDeferredLaunch(t, repo, "wip-ceiling-only"); remaining[models.CeilingDeferredKey] != true {
		t.Fatalf("ceiling record disturbed: %+v", remaining)
	}
}

func TestTakeWIPKeysIsInertWhenNoRecordExists(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	createTaskWithDeferredLaunch(t, repo, "wip-none", nil)

	wip, claimed, err := repo.TakeTaskDeferredLaunchWIPKeys(ctx, "wip-none")
	if err != nil {
		t.Fatalf("TakeTaskDeferredLaunchWIPKeys: %v", err)
	}
	if claimed || len(wip) != 0 {
		t.Fatalf("take = (%+v, %v), want inert", wip, claimed)
	}
}

// TestRestoreWIPKeysMergesIntoACeilingRecordWrittenDuringTheLaunch is AC-59's
// other write: the release must not overwrite a ceiling record that appeared
// while the launch was in flight, which an unconditional restore of the
// pre-claim snapshot would do.
func TestRestoreWIPKeysMergesIntoACeilingRecordWrittenDuringTheLaunch(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	createTaskWithDeferredLaunch(t, repo, "wip-restore-merge", wipOnlyRecord())

	wip, claimed, err := repo.TakeTaskDeferredLaunchWIPKeys(ctx, "wip-restore-merge")
	if err != nil || !claimed {
		t.Fatalf("take failed: %v %v", claimed, err)
	}

	// A ceiling refusal lands while the launch is in flight.
	_, prior, err := repo.GetTaskDeferredLaunch(ctx, "wip-restore-merge")
	if err != nil {
		t.Fatalf("GetTaskDeferredLaunch: %v", err)
	}
	if stored, _, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, "wip-restore-merge", prior, map[string]interface{}{
		models.CeilingDeferredKey:   true,
		models.CeilingLaunchKindKey: "start_task",
	}); err != nil || !stored {
		t.Fatalf("ceiling write failed: stored=%v err=%v", stored, err)
	}

	// The launch fails and the claim is released.
	if err := repo.RestoreTaskDeferredLaunchWIPKeys(ctx, "wip-restore-merge", wip); err != nil {
		t.Fatalf("RestoreTaskDeferredLaunchWIPKeys: %v", err)
	}

	merged := storedDeferredLaunch(t, repo, "wip-restore-merge")
	if merged[models.CeilingDeferredKey] != true || merged[models.CeilingLaunchKindKey] != "start_task" {
		t.Fatalf("restore clobbered the ceiling record: %+v", merged)
	}
	if merged[models.DeferredLaunchStartWhenUnblockedKey] != true || merged[models.DeferredLaunchUserIDKey] != "user-1" {
		t.Fatalf("restore did not put the WIP keys back: %+v", merged)
	}
}

func TestRestoreWIPKeysRecreatesADeletedRecord(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	createTaskWithDeferredLaunch(t, repo, "wip-restore-recreate", wipOnlyRecord())

	wip, claimed, err := repo.TakeTaskDeferredLaunchWIPKeys(ctx, "wip-restore-recreate")
	if err != nil || !claimed {
		t.Fatalf("take failed: %v %v", claimed, err)
	}
	if err := repo.RestoreTaskDeferredLaunchWIPKeys(ctx, "wip-restore-recreate", wip); err != nil {
		t.Fatalf("RestoreTaskDeferredLaunchWIPKeys: %v", err)
	}

	restored := storedDeferredLaunch(t, repo, "wip-restore-recreate")
	if restored[models.DeferredLaunchStartWhenUnblockedKey] != true || restored[models.DeferredLaunchUserIDKey] != "user-1" {
		t.Fatalf("record not restored: %+v", restored)
	}
	if !models.HasStartWhenUnblockedIntent(&models.Task{Metadata: map[string]interface{}{
		models.MetaKeyDeferredLaunch: restored,
	}}) {
		t.Fatal("HasStartWhenUnblockedIntent no longer reports the restored intent")
	}
}

func TestRestoreWIPKeysIsANoOpForAnEmptyClaim(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	createTaskWithDeferredLaunch(t, repo, "wip-restore-empty", map[string]interface{}{
		models.CeilingDeferredKey: true,
	})

	if err := repo.RestoreTaskDeferredLaunchWIPKeys(ctx, "wip-restore-empty", nil); err != nil {
		t.Fatalf("RestoreTaskDeferredLaunchWIPKeys: %v", err)
	}
	if record := storedDeferredLaunch(t, repo, "wip-restore-empty"); record[models.CeilingDeferredKey] != true {
		t.Fatalf("empty restore disturbed the record: %+v", record)
	}
}
