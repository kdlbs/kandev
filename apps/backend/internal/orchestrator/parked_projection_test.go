package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

var errParkedTestProbe = errors.New("simulated probe transport error")

// fakeProbe is a test double for backgroundProbePort.
type fakeProbe struct {
	result string
	err    error
	calls  int
}

func (f *fakeProbe) ProbeBackgroundWorkloads(_ context.Context, _ string) (string, error) {
	f.calls++
	return f.result, f.err
}

// TestComputeParked_AllCombinations verifies the three-term formula for all 8 combinations.
func TestComputeParked_AllCombinations(t *testing.T) {
	tests := []struct {
		observedDetached bool
		lastSample       string
		state            models.TaskSessionState
		wantParked       bool
	}{
		{false, "", models.TaskSessionStateWaitingForInput, false},
		{true, "", models.TaskSessionStateWaitingForInput, false},
		{false, probeResultLive, models.TaskSessionStateWaitingForInput, false},
		{true, probeResultLive, models.TaskSessionStateWaitingForInput, true},
		{false, "", models.TaskSessionStateRunning, false},
		{true, "", models.TaskSessionStateRunning, false},
		{false, probeResultLive, models.TaskSessionStateRunning, false},
		{true, probeResultLive, models.TaskSessionStateRunning, false},
	}
	for _, tt := range tests {
		ps := &sessionParkedState{
			observedDetached: tt.observedDetached,
			lastSample:       tt.lastSample,
		}
		got := ps.computeParked(tt.state)
		if got != tt.wantParked {
			t.Errorf("computeParked(%v, %q, %v) = %v, want %v",
				tt.observedDetached, tt.lastSample, tt.state, got, tt.wantParked)
		}
	}
}

// TestOnSessionParkedHook_LiveProbeSetsParked verifies that onSessionParkedHook
// with a live probe result transitions to parked=true and increments revision.
func TestOnSessionParkedHook_LiveProbeSetsParked(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	// Set a non-zero interval so the sampler goroutine can start without panicking.
	// The goroutine is stopped via deleteParkedState in t.Cleanup.
	svc.config.BackgroundSampleInterval = 10 * time.Millisecond
	probe := &fakeProbe{result: probeResultLive}
	svc.SetBackgroundProbe(probe)

	const taskID, sessionID = "task-hook", "session-hook"
	// Seed observedDetached=true so computeParked can return true.
	svc.getOrCreateParkedState(sessionID).observedDetached = true

	svc.onSessionParkedHook(context.Background(), taskID, sessionID)

	if probe.calls != 1 {
		t.Fatalf("expected 1 probe call, got %d", probe.calls)
	}
	ps := svc.parkedStateFor(sessionID)
	if ps == nil {
		t.Fatal("expected parked state entry, got nil")
	}
	if !ps.parked {
		t.Fatal("expected parked=true after live probe with observedDetached=true")
	}
	if ps.revision != 1 {
		t.Fatalf("expected revision=1, got %d", ps.revision)
	}
	// Stop the sampler goroutine to avoid goroutine leak detected by goleak.
	t.Cleanup(func() { svc.evictParkedState(context.Background(), taskID, sessionID, true) })
}

// TestOnSessionParkedHook_SettledProbeNoChange verifies that a settled probe result
// does not set parked=true.
func TestOnSessionParkedHook_SettledProbeNoChange(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	probe := &fakeProbe{result: probeResultSettled}
	svc.SetBackgroundProbe(probe)

	const taskID, sessionID = "task-settled", "session-settled"
	svc.getOrCreateParkedState(sessionID).observedDetached = true

	svc.onSessionParkedHook(context.Background(), taskID, sessionID)

	ps := svc.parkedStateFor(sessionID)
	if ps != nil && ps.parked {
		t.Fatal("expected parked=false with settled probe")
	}
}

// TestClearObservedDetachedOnTurnStarted_ClearsLastSample verifies that the
// turn_started handler clears lastSample as part of the same critical
// section that clears observedDetached and bumps turnMarker.
func TestClearObservedDetachedOnTurnStarted_ClearsLastSample(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())

	const sessionID = "session-reset"
	ps := svc.getOrCreateParkedState(sessionID)
	ps.lastSample = probeResultLive
	ps.observedDetached = true

	svc.clearObservedDetachedOnTurnStarted(sessionID)

	if ps.lastSample != "" {
		t.Fatalf("expected empty lastSample after turn_started, got %q", ps.lastSample)
	}
	if ps.observedDetached {
		t.Fatal("expected observedDetached=false after turn_started")
	}
	if ps.turnMarker != 1 {
		t.Fatalf("expected turnMarker=1 after one turn_started, got %d", ps.turnMarker)
	}
}

// TestClearObservedDetachedOnTurnStarted_NoOpForUnknownSession verifies
// AC-41a's final clause: a turn_started for a session with no parkedState
// row is ignored — it creates no entry and increments no marker.
func TestClearObservedDetachedOnTurnStarted_NoOpForUnknownSession(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())

	svc.clearObservedDetachedOnTurnStarted("session-never-attested")

	if svc.parkedStateFor("session-never-attested") != nil {
		t.Fatal("expected no parkedState row created by turn_started for an unattested session")
	}
}

// TestOnSessionParkedHook_NoProbeWhenNotAttested verifies AC-40a: a turn
// settling with no attested detached launch issues no probe at all.
func TestOnSessionParkedHook_NoProbeWhenNotAttested(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	probe := &fakeProbe{result: probeResultLive}
	svc.SetBackgroundProbe(probe)

	const taskID, sessionID = "task-noattest", "session-noattest"
	// No row at all — the common case (AC-40a).
	svc.onSessionParkedHook(context.Background(), taskID, sessionID)
	if probe.calls != 0 {
		t.Fatalf("expected 0 probe calls for a session with no parkedState row, got %d", probe.calls)
	}

	// A row exists but was never attested (observedDetached=false).
	svc.getOrCreateParkedState(sessionID)
	svc.onSessionParkedHook(context.Background(), taskID, sessionID)
	if probe.calls != 0 {
		t.Fatalf("expected 0 probe calls for an unattested session, got %d", probe.calls)
	}
}

// turnBoundaryDuringProbe is a test double for backgroundProbePort that
// simulates a turn_started arriving while a probe is in flight, by clearing
// observed_detached (and bumping turnMarker) from inside the probe call
// itself before returning a result.
type turnBoundaryDuringProbe struct {
	svc       *Service
	sessionID string
	result    string
	calls     int
}

func (t *turnBoundaryDuringProbe) ProbeBackgroundWorkloads(_ context.Context, _ string) (string, error) {
	t.calls++
	t.svc.clearObservedDetachedOnTurnStarted(t.sessionID)
	return t.result, nil
}

// TestOnSessionParkedHook_DiscardsSampleWhenTurnBoundaryMovesDuringProbe
// verifies D2's revalidation rule: a sample completing after a turn boundary
// moved mid-flight is discarded — not recorded as lastSample, not published,
// no revision moves — rather than applied to the wrong turn.
func TestOnSessionParkedHook_DiscardsSampleWhenTurnBoundaryMovesDuringProbe(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	const taskID, sessionID = "task-stale", "session-stale"
	svc.getOrCreateParkedState(sessionID).observedDetached = true

	probe := &turnBoundaryDuringProbe{svc: svc, sessionID: sessionID, result: probeResultLive}
	svc.SetBackgroundProbe(probe)

	svc.onSessionParkedHook(context.Background(), taskID, sessionID)

	if probe.calls != 1 {
		t.Fatalf("expected 1 probe call, got %d", probe.calls)
	}
	ps := svc.parkedStateFor(sessionID)
	if ps.parked {
		t.Fatal("expected parked=false: the stale sample must be discarded, not applied")
	}
	if ps.revision != 0 {
		t.Fatalf("expected revision=0 (discard writes nothing), got %d", ps.revision)
	}
	if ps.lastSample != "" {
		t.Fatalf("expected lastSample unset (discard writes nothing), got %q", ps.lastSample)
	}
}

// TestSampleAndPublishParked_DiscardsStaleSample is the periodic-sampler
// counterpart of the settle-hook revalidation test above.
func TestSampleAndPublishParked_DiscardsStaleSample(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	const taskID, sessionID = "task-stale2", "session-stale2"
	ps := svc.getOrCreateParkedState(sessionID)
	ps.observedDetached = true
	ps.parked = true

	probe := &turnBoundaryDuringProbe{svc: svc, sessionID: sessionID, result: probeResultLive}
	svc.SetBackgroundProbe(probe)

	keepRunning := svc.sampleAndPublishParked(context.Background(), taskID, sessionID)
	if !keepRunning {
		t.Fatal("expected keepRunning=true: a discarded sample must not stop the sampler on its own")
	}
	ps2 := svc.parkedStateFor(sessionID)
	// parked stays whatever clearObservedDetachedOnTurnStarted's stopSampler
	// left it as (true — that function does not touch `parked`); the point is
	// lastSample was never overwritten by the stale "live" result.
	if ps2.lastSample != "" {
		t.Fatalf("expected lastSample untouched by the discarded sample, got %q", ps2.lastSample)
	}
}

// TestRunProbe_ErrorResolvesToUnknown verifies AC-46: a non-nil error
// resolves to "unknown" regardless of the value returned beside it.
func TestRunProbe_ErrorResolvesToUnknown(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	svc.SetBackgroundProbe(&fakeProbe{result: probeResultLive, err: errParkedTestProbe})

	got := svc.runProbe(context.Background(), "session-err")
	if got != "unknown" {
		t.Fatalf("expected unknown on probe error, got %q", got)
	}
}

// TestRunProbe_UnexpectedValueResolvesToUnknown verifies AC-46: a result
// outside the three literals is "unknown" even with a nil error.
func TestRunProbe_UnexpectedValueResolvesToUnknown(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	svc.SetBackgroundProbe(&fakeProbe{result: "bogus"})

	got := svc.runProbe(context.Background(), "session-bogus")
	if got != "unknown" {
		t.Fatalf("expected unknown for an out-of-domain result, got %q", got)
	}
}

// panicProbe is a test double whose ProbeBackgroundWorkloads panics.
type panicProbe struct{}

func (panicProbe) ProbeBackgroundWorkloads(context.Context, string) (string, error) {
	panic("simulated probe implementation panic")
}

// TestRunProbe_PanicResolvesToUnknown verifies AC-46's panic clause: a
// panicking port implementation resolves to "unknown" rather than escaping
// the settle path.
func TestRunProbe_PanicResolvesToUnknown(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	svc.SetBackgroundProbe(panicProbe{})

	got := svc.runProbe(context.Background(), "session-panic")
	if got != "unknown" {
		t.Fatalf("expected unknown when the probe panics, got %q", got)
	}
}

// TestUnparkOnStateLeave_PublishesWhenParked verifies AC-53/AC-68: leaving
// WAITING_FOR_INPUT un-parks immediately without taking a further sample.
func TestUnparkOnStateLeave_PublishesWhenParked(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	const taskID, sessionID = "task-leave", "session-leave"
	ps := svc.getOrCreateParkedState(sessionID)
	ps.observedDetached = true
	ps.parked = true
	ps.lastSample = probeResultLive
	ps.revision = 1

	svc.unparkOnStateLeave(context.Background(), taskID, sessionID)

	ps2 := svc.parkedStateFor(sessionID)
	if ps2.parked {
		t.Fatal("expected parked=false after the session left WAITING_FOR_INPUT")
	}
	if ps2.revision != 2 {
		t.Fatalf("expected revision=2, got %d", ps2.revision)
	}
}

// TestUnparkOnStateLeave_NoOpWhenNotParked verifies that leaving
// WAITING_FOR_INPUT while not parked publishes nothing and moves no revision.
func TestUnparkOnStateLeave_NoOpWhenNotParked(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	const taskID, sessionID = "task-leave2", "session-leave2"
	ps := svc.getOrCreateParkedState(sessionID)
	ps.revision = 3

	svc.unparkOnStateLeave(context.Background(), taskID, sessionID)

	ps2 := svc.parkedStateFor(sessionID)
	if ps2.revision != 3 {
		t.Fatalf("expected revision unchanged at 3, got %d", ps2.revision)
	}
}

// TestEvictParkedState_UnparksThenReduces verifies AC-85: an eviction of a
// parked row (remove=false — execution end / context reset) un-parks first
// with a strictly higher revision, then reduces the row in place rather than
// deleting it, retaining the revision.
func TestEvictParkedState_UnparksThenReduces(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	const taskID, sessionID = "task-evict1", "session-evict1"
	ps := svc.getOrCreateParkedState(sessionID)
	ps.observedDetached = true
	ps.parked = true
	ps.lastSample = probeResultLive
	ps.turnMarker = 7
	ps.revision = 3

	svc.evictParkedState(context.Background(), taskID, sessionID, false)

	ps2 := svc.parkedStateFor(sessionID)
	if ps2 == nil {
		t.Fatal("expected the row to be reduced, not removed, when remove=false")
	}
	if ps2.parked {
		t.Fatal("expected parked=false after eviction")
	}
	if ps2.revision != 4 {
		t.Fatalf("expected revision retained and bumped to 4, got %d", ps2.revision)
	}
	if ps2.observedDetached || ps2.turnMarker != 0 || ps2.lastSample != "" {
		t.Fatalf("expected a reduced row (observedDetached=false, turnMarker=0, lastSample=\"\"), got %+v", ps2)
	}
}

// TestEvictParkedState_RemovesRowWhenRemoveTrue verifies AC-85's "dropped for
// good when the session is deleted" clause.
func TestEvictParkedState_RemovesRowWhenRemoveTrue(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	const taskID, sessionID = "task-evict2", "session-evict2"
	ps := svc.getOrCreateParkedState(sessionID)
	ps.parked = true

	svc.evictParkedState(context.Background(), taskID, sessionID, true)

	if svc.parkedStateFor(sessionID) != nil {
		t.Fatal("expected the row to be removed outright when remove=true")
	}
}

// TestEvictParkedState_NoOpWhenAbsent verifies eviction of an unknown session
// does not panic and creates no row.
func TestEvictParkedState_NoOpWhenAbsent(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	svc.evictParkedState(context.Background(), "task-x", "session-never-existed", false)
	if svc.parkedStateFor("session-never-existed") != nil {
		t.Fatal("expected no row to be created by evicting an unknown session")
	}
}

// TestParkedProjectionSnapshot_ReturnsCorrectValues verifies the
// ParkedProjectionSnapshot method.
func TestParkedProjectionSnapshot_ReturnsCorrectValues(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())

	const sessionID = "session-snap"
	ps := svc.getOrCreateParkedState(sessionID)
	ps.parked = true
	ps.revision = 5

	parked, epoch, revision := svc.ParkedProjectionSnapshot(sessionID)
	if !parked {
		t.Fatal("expected parked=true")
	}
	if epoch != svc.parkedEpoch {
		t.Fatalf("expected epoch=%d, got %d", svc.parkedEpoch, epoch)
	}
	if revision != 5 {
		t.Fatalf("expected revision=5, got %d", revision)
	}
}

// TestParkedProjectionSnapshot_UnknownSession verifies that an unknown session
// returns false with revision=0 and the process epoch.
func TestParkedProjectionSnapshot_UnknownSession(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())

	parked, epoch, revision := svc.ParkedProjectionSnapshot("unknown-session")
	if parked {
		t.Fatal("expected parked=false for unknown session")
	}
	if epoch != svc.parkedEpoch {
		t.Fatalf("expected epoch=%d, got %d", svc.parkedEpoch, epoch)
	}
	if revision != 0 {
		t.Fatalf("expected revision=0, got %d", revision)
	}
}

// TestTaskParkedProjectionSnapshot_MultiSessionOR verifies that the task-level
// parked state is the OR of all session parked states.
func TestTaskParkedProjectionSnapshot_MultiSessionOR(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())

	const taskID = "task-or"

	// session A is not parked, session B is parked.
	svc.updateTaskParkedState(context.Background(), taskID, "sess-a", false)
	svc.updateTaskParkedState(context.Background(), taskID, "sess-b", true)

	parked, revision := svc.TaskParkedProjectionSnapshot(taskID)
	if !parked {
		t.Fatal("expected task parked=true when any session is parked")
	}
	if revision == 0 {
		t.Fatal("expected non-zero revision after OR flip")
	}

	// Now session B clears — both sessions not parked → OR = false.
	svc.updateTaskParkedState(context.Background(), taskID, "sess-b", false)

	parked, revision2 := svc.TaskParkedProjectionSnapshot(taskID)
	if parked {
		t.Fatal("expected task parked=false when all sessions not parked")
	}
	if revision2 <= revision {
		t.Fatal("expected revision to increment after second OR flip")
	}
}

// TestSampleAndPublishParked_StopsWhenNotParked verifies that sampleAndPublishParked
// returns false (stop sampling) when the probe returns settled.
func TestSampleAndPublishParked_StopsWhenNotParked(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	probe := &fakeProbe{result: probeResultSettled}
	svc.SetBackgroundProbe(probe)

	const taskID, sessionID = "task-sample", "session-sample"
	ps := svc.getOrCreateParkedState(sessionID)
	ps.observedDetached = true
	ps.parked = true // pretend was parked

	keepRunning := svc.sampleAndPublishParked(context.Background(), taskID, sessionID)
	if keepRunning {
		t.Fatal("expected keepRunning=false when probe returns settled")
	}
}

// TestRunParkingSampler_ContextCancellation verifies that the sampler exits
// promptly when its context is cancelled.
func TestRunParkingSampler_ContextCancellation(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	svc.config.BackgroundSampleInterval = 100 * time.Millisecond
	probe := &fakeProbe{result: probeResultLive}
	svc.SetBackgroundProbe(probe)

	const taskID, sessionID = "task-ctx", "session-ctx"
	ps := svc.getOrCreateParkedState(sessionID)
	ps.observedDetached = true
	ps.parked = true
	ps.lastSample = probeResultLive

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.runParkingSampler(ctx, taskID, sessionID)
	}()

	cancel()
	select {
	case <-done:
		// sampler exited cleanly
	case <-time.After(2 * time.Second):
		t.Fatal("sampler goroutine did not exit after context cancellation")
	}
}
