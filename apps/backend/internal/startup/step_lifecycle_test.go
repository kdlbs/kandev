package startup

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
)

func TestBeginStepOpensRegisteredStepOpaqueOrCounting(t *testing.T) {
	r := New(nil)
	r.Set(BackingUpDatabase)
	r.BeginStep(StepDatabaseBackup)
	snap := r.Snapshot()
	if snap.Step == nil {
		t.Fatal("expected an active step")
	}
	if snap.Step.ID != StepDatabaseBackup {
		t.Fatalf("step id = %q, want %q", snap.Step.ID, StepDatabaseBackup)
	}
	if snap.Step.Measure != MeasureCounting {
		t.Fatalf("database.backup should open counting, got %q", snap.Step.Measure)
	}

	r2 := New(nil)
	r2.Set(ApplyingMigrations)
	r2.BeginStep(StepStoresRepositories)
	snap2 := r2.Snapshot()
	if snap2.Step.Measure != MeasureOpaque {
		t.Fatalf("counted-declared step should open opaque, got %q", snap2.Step.Measure)
	}
}

func TestBeginStepUnregisteredIdentifierOpensNoStep(t *testing.T) {
	r := New(nil)
	seqBefore := r.Snapshot().Seq
	r.BeginStep(StepID("not.a.real.step"))
	snap := r.Snapshot()
	if snap.Step != nil {
		t.Fatal("unregistered identifier must not open a step")
	}
	if snap.Seq != seqBefore {
		t.Fatalf("seq changed on unregistered begin: before=%d after=%d", seqBefore, snap.Seq)
	}
}

func TestBeginStepAlreadyActiveIsNoOp(t *testing.T) {
	r := New(nil)
	r.Set(ApplyingMigrations)
	r.BeginStep(StepStoresRepositories)
	r.SeedDone(StepStoresRepositories, 5)
	r.SetTotal(StepStoresRepositories, 50)
	seqAfterFirst := r.Snapshot().Seq

	r.BeginStep(StepStoresRepositories)
	snap := r.Snapshot()
	if snap.Seq != seqAfterFirst {
		t.Fatalf("re-beginning the active step changed seq: %d -> %d", seqAfterFirst, snap.Seq)
	}
	if *snap.Step.Done != 5 {
		t.Fatalf("re-beginning the active step reset counts: done=%d", *snap.Step.Done)
	}
}

func TestBeginStepEndsPriorAsOneTransition(t *testing.T) {
	r := New(nil)
	r.Set(ApplyingMigrations)
	r.BeginStep(StepStoresRepositories)
	seq0 := r.Snapshot().Seq

	r.BeginStep(StepPromptSeqBackfill)
	snap := r.Snapshot()
	if snap.Seq != seq0+1 {
		t.Fatalf("switching active steps should increment seq by exactly one, got %d -> %d", seq0, snap.Seq)
	}
	if snap.Step.ID != StepPromptSeqBackfill {
		t.Fatalf("active step = %q, want %q", snap.Step.ID, StepPromptSeqBackfill)
	}
}

func TestEndStepClosesOnlyTheActiveStep(t *testing.T) {
	r := New(nil)
	r.Set(ApplyingMigrations)
	r.BeginStep(StepStoresRepositories)
	seq0 := r.Snapshot().Seq

	r.EndStep(StepStoresServices) // not active: no-op
	if r.Snapshot().Seq != seq0 {
		t.Fatal("ending a non-active step must not change seq")
	}
	if r.Snapshot().Step == nil {
		t.Fatal("ending a non-active step must not close the active one")
	}

	r.EndStep(StepStoresRepositories)
	snap := r.Snapshot()
	if snap.Seq != seq0+1 {
		t.Fatalf("ending the active step should increment seq by one, got %d -> %d", seq0, snap.Seq)
	}
	if snap.Step != nil {
		t.Fatal("snapshot must retain no step after EndStep")
	}
}

func TestSeedDoneAssignsUntilTotalOrAdvance(t *testing.T) {
	r := New(nil)
	r.Set(ApplyingMigrations)
	r.BeginStep(StepStoresRepositories)

	r.SeedDone(StepStoresRepositories, -1) // rejected; still opaque, done unexposed
	if got := r.Snapshot().Step; got.Measure != MeasureOpaque {
		t.Fatalf("negative seed must be rejected without promoting, measure = %q", got.Measure)
	}

	r.SeedDone(StepStoresRepositories, 10)
	r.SetTotal(StepStoresRepositories, 100) // promotes: done should reflect the seed
	if *r.Snapshot().Step.Done != 10 {
		t.Fatalf("done = %d, want 10", *r.Snapshot().Step.Done)
	}

	r.SeedDone(StepStoresRepositories, 20) // rejected: total already set
	if *r.Snapshot().Step.Done != 10 {
		t.Fatalf("seed after total must be rejected, done = %d", *r.Snapshot().Step.Done)
	}
}

func TestSeedDoneRejectedAfterAdvance(t *testing.T) {
	r := New(nil)
	r.Set(ApplyingMigrations)
	r.BeginStep(StepStoresRepositories)
	r.SetTotal(StepStoresRepositories, 100)
	r.Advance(StepStoresRepositories, 5)

	r.SeedDone(StepStoresRepositories, 50)
	if *r.Snapshot().Step.Done != 5 {
		t.Fatalf("seed after advance must be rejected, done = %d", *r.Snapshot().Step.Done)
	}
}

func TestSetTotalPromotesAndClampsSeededDone(t *testing.T) {
	r := New(nil)
	r.Set(ApplyingMigrations)
	r.BeginStep(StepStoresRepositories)
	r.SeedDone(StepStoresRepositories, 90)

	r.SetTotal(StepStoresRepositories, -1)
	if r.Snapshot().Step.Measure != MeasureOpaque {
		t.Fatal("negative total must be rejected without promoting")
	}

	r.SetTotal(StepStoresRepositories, 50) // below the seeded done: clamp
	snap := r.Snapshot().Step
	if snap.Measure != MeasureCounted {
		t.Fatalf("measure = %q, want counted", snap.Measure)
	}
	if *snap.Done != 50 {
		t.Fatalf("seeded done should clamp to total, got %d", *snap.Done)
	}
	if *snap.Total != 50 {
		t.Fatalf("total = %d, want 50", *snap.Total)
	}
}

func TestSetTotalZeroSealsOpaqueForActivation(t *testing.T) {
	r := New(nil)
	r.Set(ApplyingMigrations)
	r.BeginStep(StepStoresRepositories)
	r.SetTotal(StepStoresRepositories, 0)
	if snap := r.Snapshot().Step; snap.Measure != MeasureOpaque {
		t.Fatalf("total of zero must seal the step opaque, got %q", snap.Measure)
	}
	r.SetTotal(StepStoresRepositories, 10) // later different total must not revive it
	if snap := r.Snapshot().Step; snap.Measure != MeasureOpaque {
		t.Fatalf("a later total must not revive a zero-total step, got %q", snap.Measure)
	}
}

func TestSetTotalRejectsADifferentTotalOnceAccepted(t *testing.T) {
	r := New(nil)
	r.Set(ApplyingMigrations)
	r.BeginStep(StepStoresRepositories)
	r.SetTotal(StepStoresRepositories, 100)
	r.SetTotal(StepStoresRepositories, 100) // equal: no-op, no warning
	r.SetTotal(StepStoresRepositories, 200) // different: rejected
	if *r.Snapshot().Step.Total != 100 {
		t.Fatalf("total = %d, want 100 (unchanged)", *r.Snapshot().Step.Total)
	}
}

func TestAdvanceClampsToTotalAndSaturates(t *testing.T) {
	r := New(nil)
	r.Set(ApplyingMigrations)
	r.BeginStep(StepStoresRepositories)
	r.SetTotal(StepStoresRepositories, 10)

	r.Advance(StepStoresRepositories, 0) // no-op
	if *r.Snapshot().Step.Done != 0 {
		t.Fatal("advance of zero must be a no-op")
	}
	r.Advance(StepStoresRepositories, -5) // no-op
	if *r.Snapshot().Step.Done != 0 {
		t.Fatal("negative advance must be a no-op")
	}
	r.Advance(StepStoresRepositories, 999)
	if *r.Snapshot().Step.Done != 10 {
		t.Fatalf("advance exceeding total must clamp, done = %d", *r.Snapshot().Step.Done)
	}
}

func TestAdvanceOnNonActiveIdentifierIsNoOp(t *testing.T) {
	r := New(nil)
	r.Set(ApplyingMigrations)
	r.BeginStep(StepStoresRepositories)
	r.SetTotal(StepStoresRepositories, 10)
	r.Advance(StepStoresServices, 5) // not the active step
	if *r.Snapshot().Step.Done != 0 {
		t.Fatal("advance naming a non-active identifier must change nothing")
	}
}

// TestMismatchOnNonActiveStepWarnsOncePerOperation covers
// SeedDone/SetTotal/Advance/Degrade calls naming a step that isn't active:
// without a warning these are silent no-ops, leaving an operator no way to
// learn a caller has drifted from the reporter's real active step. Each
// mismatched call must log exactly one warning per (step, operation) pair,
// not once per call.
func TestMismatchOnNonActiveStepWarnsOncePerOperation(t *testing.T) {
	core, observed := observer.New(zap.InfoLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("observer logger: %v", err)
	}
	r := New(log)
	r.Set(ApplyingMigrations)
	r.BeginStep(StepStoresRepositories)

	r.SeedDone(StepStoresServices, 5)
	r.SeedDone(StepStoresServices, 5) // repeat: must not log again
	r.SetTotal(StepStoresServices, 10)
	r.Advance(StepStoresServices, 1)
	r.Degrade(StepStoresServices)

	wantOps := map[string]bool{"seed_done": false, "set_total": false, "advance": false, "degrade": false}
	var mismatchCount int
	for _, entry := range observed.All() {
		if entry.Message != "Startup step call for a step that is not active" {
			continue
		}
		mismatchCount++
		fields := entry.ContextMap()
		if fields["step"] != string(StepStoresServices) {
			t.Fatalf("warning step = %v, want %q", fields["step"], StepStoresServices)
		}
		if fields["active_step"] != string(StepStoresRepositories) {
			t.Fatalf("warning active_step = %v, want %q", fields["active_step"], StepStoresRepositories)
		}
		op, _ := fields["op"].(string)
		if _, known := wantOps[op]; !known {
			t.Fatalf("unexpected op %q in mismatch warning", op)
		}
		wantOps[op] = true
	}
	if mismatchCount != 4 {
		t.Fatalf("mismatch warning count = %d, want 4 (one per operation, repeat suppressed)", mismatchCount)
	}
	for op, seen := range wantOps {
		if !seen {
			t.Fatalf("no mismatch warning logged for op %q", op)
		}
	}
}

func TestDegradeMovesDownOneRungAndNeverBack(t *testing.T) {
	r := New(nil)
	r.Set(ApplyingMigrations)
	r.BeginStep(StepStoresRepositories)
	if r.Snapshot().Step.Measure != MeasureOpaque {
		t.Fatal("precondition: step should open opaque")
	}
	r.SetTotal(StepStoresRepositories, 10) // promotes to counted
	r.Degrade(StepStoresRepositories)
	if got := r.Snapshot().Step.Measure; got != MeasureCounting {
		t.Fatalf("measure after one degrade = %q, want counting", got)
	}
	r.Degrade(StepStoresRepositories)
	if got := r.Snapshot().Step.Measure; got != MeasureOpaque {
		t.Fatalf("measure after two degrades = %q, want opaque", got)
	}
	r.SetTotal(StepStoresRepositories, 20) // must never re-promote this activation
	if got := r.Snapshot().Step.Measure; got != MeasureOpaque {
		t.Fatalf("measure after SetTotal post-degrade = %q, want opaque (no re-promotion)", got)
	}
}

func TestDegradeBeforeSetTotalBlocksLaterPromotion(t *testing.T) {
	r := New(nil)
	r.Set(ApplyingMigrations)
	r.BeginStep(StepStoresRepositories)
	r.Degrade(StepStoresRepositories) // fails counter query before total is known
	r.SetTotal(StepStoresRepositories, 100)
	if got := r.Snapshot().Step.Measure; got != MeasureOpaque {
		t.Fatalf("measure = %q, want opaque: a pre-total degrade must block promotion", got)
	}
}

func TestSnapshotHasNoStepAfterEndStep(t *testing.T) {
	r := New(nil)
	r.Set(ApplyingMigrations)
	r.BeginStep(StepStoresRepositories)
	r.EndStep(StepStoresRepositories)
	if r.Snapshot().Step != nil {
		t.Fatal("snapshot must not retain an earlier step's identifier")
	}
}

// TestRateSampleTrimFoldsDeltaIntoAnchorDone covers the rate-sample ring:
// once it exceeds maxRateSamples, appendSample drops the oldest entries. If
// that drop left anchorDone stale, rateWindowStart's cumulative-sum walk
// would understate "done at the window's older edge" by exactly the dropped
// samples' total delta, overstating the rate reported for a fast sweep that
// advances more than maxRateSamples times within one 30s window (e.g.
// storage.go's required-store sweeps).
func TestRateSampleTrimFoldsDeltaIntoAnchorDone(t *testing.T) {
	r := New(nil)
	r.Set(ApplyingMigrations)
	r.BeginStep(StepStoresRepositories)
	r.SetTotal(StepStoresRepositories, 1000)

	const advances = maxRateSamples + 36
	for i := 0; i < advances; i++ {
		r.Advance(StepStoresRepositories, 1)
	}

	if got := len(r.active.samples); got != maxRateSamples {
		t.Fatalf("retained samples = %d, want %d (ring capped)", got, maxRateSamples)
	}
	wantDropped := int64(advances - maxRateSamples)
	if r.active.anchorDone != wantDropped {
		t.Fatalf("anchorDone = %d, want %d (the %d trimmed samples' folded delta)", r.active.anchorDone, wantDropped, wantDropped)
	}

	_, windowDone := r.active.rateWindowStart(time.Now())
	if windowDone != wantDropped {
		t.Fatalf("rateWindowStart done = %d, want %d: a stale anchorDone understates the window baseline and overstates the rate", windowDone, wantDropped)
	}
}

func TestPackageLevelHelpersAreNoOpsWithoutReporter(t *testing.T) {
	ctx := context.Background() // no reporter attached
	BeginStep(ctx, StepStoresRepositories)
	SeedDone(ctx, StepStoresRepositories, 1)
	SetTotal(ctx, StepStoresRepositories, 1)
	Advance(ctx, StepStoresRepositories, 1)
	Degrade(ctx, StepStoresRepositories)
	EndStep(ctx, StepStoresRepositories) // must not panic
}

func TestPackageLevelHelpersDelegateToReporter(t *testing.T) {
	r := New(nil)
	r.Set(ApplyingMigrations)
	ctx := WithReporter(context.Background(), r)
	BeginStep(ctx, StepStoresRepositories)
	SetTotal(ctx, StepStoresRepositories, 10)
	Advance(ctx, StepStoresRepositories, 3)
	if *r.Snapshot().Step.Done != 3 {
		t.Fatalf("done = %d, want 3", *r.Snapshot().Step.Done)
	}
	EndStep(ctx, StepStoresRepositories)
	if r.Snapshot().Step != nil {
		t.Fatal("expected no active step after EndStep")
	}
}
