package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// This file pins AC-59/AC-59a for the second writer of the shared
// deferred_launch record: claimDeferredLaunch/restoreDeferredLaunch, used by
// the gate that fires on WIP promotion or dependency resolution. It mirrors
// deferred_launch_consume_ceiling_test.go, which pins the same contract for
// the direct-start path (claimDeferredLaunchForStart/releaseIfHeld) — the
// two consumers of the record must never clobber each other's half of it.

// TestClaimDeferredLaunchLeavesACeilingRecordInPlace pins the claim half: the
// gate takes only the launch-intent keys it owns and must not delete a
// ceiling deferral sharing the record. Deleting it is a silently dropped
// launch that the AC-17a sweep can no longer find.
func TestClaimDeferredLaunchLeavesACeilingRecordInPlace(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "gate-claim-mixed", Title: "Mixed",
		Metadata: map[string]interface{}{models.MetaKeyDeferredLaunch: map[string]interface{}{
			models.DeferredLaunchStartWhenUnblockedKey: true,
			models.CeilingDeferredKey:                  true,
			models.CeilingLaunchKindKey:                "start_task",
		}},
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	wip, claimed := svc.claimDeferredLaunch(ctx, "gate-claim-mixed", "test.event")
	if !claimed {
		t.Fatal("claim did not succeed; the WIP intent should have been claimable")
	}
	if _, ok := wip[models.DeferredLaunchStartWhenUnblockedKey]; !ok {
		t.Fatalf("claim did not return its own key: %+v", wip)
	}

	after := deferredLaunchOf(t, svc, "gate-claim-mixed")
	if after == nil {
		t.Fatal("the whole deferred_launch record was deleted by the claim")
	}
	if after[models.CeilingDeferredKey] != true || after[models.CeilingLaunchKindKey] != "start_task" {
		t.Fatalf("ceiling keys did not survive the claim: %+v", after)
	}
	if _, still := after[models.DeferredLaunchStartWhenUnblockedKey]; still {
		t.Fatalf("the claim did not take its own key out of the durable record: %+v", after)
	}
}

// TestRestoreDeferredLaunchMergesRatherThanOverwriting pins the restore half:
// a ceiling record written while the launch was in flight must survive a
// failed launch's restore. Restoring a pre-claim snapshot wholesale would
// erase it — merging back only the claimed keys is what AC-59a requires.
func TestRestoreDeferredLaunchMergesRatherThanOverwriting(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "gate-claim-release", Title: "Release",
		Metadata: map[string]interface{}{models.MetaKeyDeferredLaunch: map[string]interface{}{
			models.DeferredLaunchStartWhenUnblockedKey: true,
			models.DeferredLaunchUserIDKey:             "user-7",
		}},
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	wip, claimed := svc.claimDeferredLaunch(ctx, "gate-claim-release", "test.event")
	if !claimed {
		t.Fatal("claim did not succeed")
	}
	if record := deferredLaunchOf(t, svc, "gate-claim-release"); record != nil {
		t.Fatalf("a WIP-only record should have been removed entirely, got %+v", record)
	}

	// A ceiling refusal lands while the launch is in flight.
	if err := repo.SetTaskMetadataKey(ctx, "gate-claim-release", models.MetaKeyDeferredLaunch, map[string]interface{}{
		models.CeilingDeferredKey:   true,
		models.CeilingLaunchKindKey: "start_task",
	}); err != nil {
		t.Fatalf("SetTaskMetadataKey: %v", err)
	}

	svc.restoreDeferredLaunch(ctx, "gate-claim-release", wip, "test.event")

	after := deferredLaunchOf(t, svc, "gate-claim-release")
	if after[models.CeilingDeferredKey] != true || after[models.CeilingLaunchKindKey] != "start_task" {
		t.Fatalf("restore overwrote the ceiling record: %+v", after)
	}
	if after[models.DeferredLaunchStartWhenUnblockedKey] != true || after[models.DeferredLaunchUserIDKey] != "user-7" {
		t.Fatalf("restore did not put the claimed keys back: %+v", after)
	}
}

// TestClaimDeferredLaunchDoesNotClaimACeilingOnlyRecord keeps the two
// meanings separate in the other direction: a ceiling deferral is replayed by
// its own sweep, so the gate has nothing to claim and must leave it
// untouched.
func TestClaimDeferredLaunchDoesNotClaimACeilingOnlyRecord(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "gate-claim-ceiling-only", Title: "CeilingOnly",
		Metadata: map[string]interface{}{models.MetaKeyDeferredLaunch: map[string]interface{}{
			models.CeilingDeferredKey: true,
		}},
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	wip, claimed := svc.claimDeferredLaunch(ctx, "gate-claim-ceiling-only", "test.event")
	if claimed {
		t.Fatalf("a ceiling-only record must not be claimed by the gate path, got wip=%+v", wip)
	}
	if after := deferredLaunchOf(t, svc, "gate-claim-ceiling-only"); after[models.CeilingDeferredKey] != true {
		t.Fatalf("ceiling record disturbed: %+v", after)
	}
}
