package dashboard

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// fakeLoopHealthRepo is a hand-rolled LoopHealthRepo for exercising
// EvaluateLoopHealth's verdict precedence and failure-reason mapping
// without a database.
type fakeLoopHealthRepo struct {
	activationAt        time.Time
	activationPublished bool
	activationErr       error

	eligibleCount int
	eligibleErr   error

	triggerRows  []sqlite.TriggerHealthRow
	triggerTotal int
	triggerErr   error

	stuckRows  []sqlite.StuckRunRow
	stuckTotal int
	stuckErr   error

	silentRows  []sqlite.SilentSuccessRow
	silentTotal int
	silentErr   error

	terminalRows []sqlite.TerminalRunShapeInputRow
	terminalErr  error
}

func (f *fakeLoopHealthRepo) LoopLivenessActivationChecked() (time.Time, bool, error) {
	return f.activationAt, f.activationPublished, f.activationErr
}

func (f *fakeLoopHealthRepo) CountEligibleTriggers(context.Context, string) (int, error) {
	return f.eligibleCount, f.eligibleErr
}

func (f *fakeLoopHealthRepo) ListOverdueOrStrandedTriggers(
	context.Context, string, time.Time, time.Duration, time.Duration, int,
) ([]sqlite.TriggerHealthRow, int, error) {
	return f.triggerRows, f.triggerTotal, f.triggerErr
}

func (f *fakeLoopHealthRepo) ListStuckRuns(
	context.Context, string, time.Time, time.Duration, time.Duration, int,
) ([]sqlite.StuckRunRow, int, error) {
	return f.stuckRows, f.stuckTotal, f.stuckErr
}

func (f *fakeLoopHealthRepo) ListSilentSuccesses(
	context.Context, string, time.Time, int,
) ([]sqlite.SilentSuccessRow, int, error) {
	return f.silentRows, f.silentTotal, f.silentErr
}

func (f *fakeLoopHealthRepo) ListTerminalRunsInWindow(
	context.Context, string, time.Time,
) ([]sqlite.TerminalRunShapeInputRow, error) {
	return f.terminalRows, f.terminalErr
}

func baseFakeRepo(now time.Time) *fakeLoopHealthRepo {
	return &fakeLoopHealthRepo{
		activationAt:        now.Add(-48 * time.Hour),
		activationPublished: true,
		eligibleCount:       1,
	}
}

func TestEvaluateLoopHealth_UnpublishedActivationIsUnknown(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	repo := baseFakeRepo(now)
	repo.activationPublished = false

	resp, err := EvaluateLoopHealth(context.Background(), repo, "ws-a", now)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if resp.Verdict != VerdictUnknown {
		t.Errorf("verdict = %q, want %q", resp.Verdict, VerdictUnknown)
	}
	if len(resp.Triggers.Rows) != 0 || len(resp.StuckRuns.Rows) != 0 || len(resp.SilentSuccesses.Rows) != 0 {
		t.Errorf("expected empty evidence lists when unknown, got %+v", resp)
	}
	for shape, count := range resp.TerminalShapeCounts {
		if count != 0 {
			t.Errorf("shape %q = %d, want 0 (zero-filled)", shape, count)
		}
	}
}

func TestEvaluateLoopHealth_OverdueTriggerIsDead(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	repo := baseFakeRepo(now)
	repo.triggerRows = []sqlite.TriggerHealthRow{{TriggerID: "t1", Condition: "overdue"}}
	repo.triggerTotal = 1
	// Even with a stuck run also present, dead outranks degraded.
	repo.stuckRows = []sqlite.StuckRunRow{{RunID: "r1"}}
	repo.stuckTotal = 1

	resp, err := EvaluateLoopHealth(context.Background(), repo, "ws-a", now)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if resp.Verdict != VerdictDead {
		t.Errorf("verdict = %q, want %q", resp.Verdict, VerdictDead)
	}
}

func TestEvaluateLoopHealth_StuckRunIsDegraded(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	repo := baseFakeRepo(now)
	repo.stuckRows = []sqlite.StuckRunRow{{RunID: "r1", Status: "claimed"}}
	repo.stuckTotal = 1

	resp, err := EvaluateLoopHealth(context.Background(), repo, "ws-a", now)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if resp.Verdict != VerdictDegraded {
		t.Errorf("verdict = %q, want %q", resp.Verdict, VerdictDegraded)
	}
}

func TestEvaluateLoopHealth_SilentSuccessIsDegraded(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	repo := baseFakeRepo(now)
	repo.silentRows = []sqlite.SilentSuccessRow{{RunID: "r1", RequestedAt: now.Add(-time.Hour)}}
	repo.silentTotal = 1

	resp, err := EvaluateLoopHealth(context.Background(), repo, "ws-a", now)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if resp.Verdict != VerdictDegraded {
		t.Errorf("verdict = %q, want %q", resp.Verdict, VerdictDegraded)
	}
}

func TestEvaluateLoopHealth_NoEligibleTriggerIsNotArmed(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	repo := baseFakeRepo(now)
	repo.eligibleCount = 0

	resp, err := EvaluateLoopHealth(context.Background(), repo, "ws-empty", now)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if resp.Verdict != VerdictNotArmed {
		t.Errorf("verdict = %q, want %q", resp.Verdict, VerdictNotArmed)
	}
	if len(resp.Triggers.Rows) != 0 || len(resp.StuckRuns.Rows) != 0 || len(resp.SilentSuccesses.Rows) != 0 {
		t.Errorf("expected empty evidence for a workspace with nothing wrong, got %+v", resp)
	}
}

func TestEvaluateLoopHealth_NothingWrongIsHealthy(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	repo := baseFakeRepo(now)

	resp, err := EvaluateLoopHealth(context.Background(), repo, "ws-a", now)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if resp.Verdict != VerdictHealthy {
		t.Errorf("verdict = %q, want %q", resp.Verdict, VerdictHealthy)
	}
}

func TestEvaluateLoopHealth_EchoesThresholds(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	repo := baseFakeRepo(now)

	resp, err := EvaluateLoopHealth(context.Background(), repo, "ws-a", now)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	want := LoopHealthThresholdsDTO{
		TriggerOverdueGraceSeconds:  180,
		TriggerStrandedGraceSeconds: 300,
		RunQueuedGraceSeconds:       120,
		RunClaimedGraceSeconds:      600,
		EvaluationWindowSeconds:     86400,
		EvidenceCap:                 50,
	}
	if resp.Thresholds != want {
		t.Errorf("thresholds = %+v, want %+v", resp.Thresholds, want)
	}
	wantWindowStart := now.Add(-24 * time.Hour)
	if !resp.EvaluationWindowStart.Equal(wantWindowStart) {
		t.Errorf("window start = %v, want %v", resp.EvaluationWindowStart, wantWindowStart)
	}
}

func TestEvaluateLoopHealth_ClassifiesTerminalRunsIntoShapeCounts(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	repo := baseFakeRepo(now)
	processed := "processed"
	repo.terminalRows = []sqlite.TerminalRunShapeInputRow{
		{Status: "finished", Outcome: &processed, SessionID: "s1", RequestedAt: now.Add(-time.Hour)},
		{Status: "finished", Outcome: &processed, SessionID: "", RequestedAt: now.Add(-time.Hour)},
	}

	resp, err := EvaluateLoopHealth(context.Background(), repo, "ws-a", now)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if resp.TerminalShapeCounts["launched_completed"] != 1 {
		t.Errorf("launched_completed = %d, want 1", resp.TerminalShapeCounts["launched_completed"])
	}
	if resp.TerminalShapeCounts["silent_success"] != 1 {
		t.Errorf("silent_success = %d, want 1", resp.TerminalShapeCounts["silent_success"])
	}
}

func TestEvaluateLoopHealth_ActivationReadFailureDegrades(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	repo := baseFakeRepo(now)
	repo.activationErr = errors.New("boom")

	_, err := EvaluateLoopHealth(context.Background(), repo, "ws-a", now)
	assertDegradedReason(t, err, ReasonActivationReadFailed)
}

func TestEvaluateLoopHealth_TriggerReadFailureDegrades(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	repo := baseFakeRepo(now)
	repo.triggerErr = errors.New("boom")

	_, err := EvaluateLoopHealth(context.Background(), repo, "ws-a", now)
	assertDegradedReason(t, err, ReasonTriggerReadFailed)
}

func TestEvaluateLoopHealth_EligibleTriggerCountReadFailureDegrades(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	repo := baseFakeRepo(now)
	repo.eligibleErr = errors.New("boom")

	_, err := EvaluateLoopHealth(context.Background(), repo, "ws-a", now)
	assertDegradedReason(t, err, ReasonTriggerReadFailed)
}

func TestEvaluateLoopHealth_StuckRunReadFailureDegrades(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	repo := baseFakeRepo(now)
	repo.stuckErr = errors.New("boom")

	_, err := EvaluateLoopHealth(context.Background(), repo, "ws-a", now)
	assertDegradedReason(t, err, ReasonRunReadFailed)
}

// OPERATOR DECISION F31: both the silent-success read and the
// terminal-shape read map to ReasonTerminalReadFailed.
func TestEvaluateLoopHealth_SilentSuccessReadFailureDegradesWithTerminalReason(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	repo := baseFakeRepo(now)
	repo.silentErr = errors.New("boom")

	_, err := EvaluateLoopHealth(context.Background(), repo, "ws-a", now)
	assertDegradedReason(t, err, ReasonTerminalReadFailed)
}

func TestEvaluateLoopHealth_TerminalShapeReadFailureDegradesWithTerminalReason(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	repo := baseFakeRepo(now)
	repo.terminalErr = errors.New("boom")

	_, err := EvaluateLoopHealth(context.Background(), repo, "ws-a", now)
	assertDegradedReason(t, err, ReasonTerminalReadFailed)
}

func assertDegradedReason(t *testing.T, err error, wantReason string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	var degraded *LoopHealthDegradedError
	if !errors.As(err, &degraded) {
		t.Fatalf("error = %v, want *LoopHealthDegradedError", err)
	}
	if degraded.Reason != wantReason {
		t.Errorf("reason = %q, want %q", degraded.Reason, wantReason)
	}
}
