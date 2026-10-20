package startup

import (
	"testing"
	"time"
)

// @covers AC-PLATFORM-STARTUP-HOUSEKEEPING-003.1
func TestHousekeepingHistoryRecordsActivationAndFreezesAtReadiness(t *testing.T) {
	r := New(nil)
	r.Set(RecoveringSessions)
	r.BeginStep(StepQueueReconciliation)
	r.EndStep(StepQueueReconciliation)
	r.BeginStep(StepRuntimeInventory)
	r.Set(Ready)
	first := r.Snapshot()
	if len(first.CompletedSteps) != 2 {
		t.Fatalf("lost completed startup work: %#v", first)
	}
	if !first.CompletedSteps[0].Completed || first.CompletedSteps[1].Completed {
		t.Fatalf("forced closure claimed success: %#v", first.CompletedSteps)
	}
	if first.CompletedSteps[0].ActivationSeq >= first.CompletedSteps[1].ActivationSeq {
		t.Fatal("activation identity was reused")
	}
	r.started = r.started.Add(-time.Hour)
	if next := r.Snapshot(); next.ElapsedMS != first.ElapsedMS {
		t.Fatalf("startup duration kept growing after readiness: %d -> %d", first.ElapsedMS, next.ElapsedMS)
	}
	first.CompletedSteps[0].LabelKey = "changed snapshot"
	if r.Snapshot().CompletedSteps[0].LabelKey == "changed snapshot" {
		t.Fatal("snapshot aliases reporter history")
	}
}

// @covers AC-PLATFORM-STARTUP-HOUSEKEEPING-003.2
func TestHousekeepingHistoryIsBoundedAndBootScoped(t *testing.T) {
	r := New(nil)
	for range 40 {
		r.BeginStep(StepPromptSeqBackfill)
		r.EndStep(StepPromptSeqBackfill)
	}
	history := r.Snapshot().CompletedSteps
	if len(history) != 32 {
		t.Fatalf("history length=%d", len(history))
	}
	if len(New(nil).Snapshot().CompletedSteps) != 0 {
		t.Fatal("history escaped its boot")
	}
}

func TestHousekeepingReadySnapshotCannotBeReopenedByRuntimeWork(t *testing.T) {
	r := New(nil)
	r.BeginStep(StepPromptSeqBackfill)
	r.EndStep(StepPromptSeqBackfill)
	r.Set(Ready)
	before := r.Snapshot()
	r.BeginStep(StepStoresServices)
	if got := r.Snapshot(); got.Step != nil || len(got.CompletedSteps) != len(before.CompletedSteps) {
		t.Fatalf("runtime work reopened frozen startup: %#v", got)
	}
	r.Set(InitializingServices)
	if r.Snapshot().Phase != Ready {
		t.Fatal("runtime work regressed startup readiness")
	}
}
