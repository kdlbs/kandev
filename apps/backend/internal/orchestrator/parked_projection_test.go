package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

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
	t.Cleanup(func() { svc.deleteParkedState(sessionID) })
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

// TestResetParkedSampler_ClearsLastSample verifies that resetParkedSampler
// clears lastSample.
func TestResetParkedSampler_ClearsLastSample(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())

	const sessionID = "session-reset"
	ps := svc.getOrCreateParkedState(sessionID)
	ps.lastSample = probeResultLive

	svc.resetParkedSampler(sessionID)

	if ps.lastSample != "" {
		t.Fatalf("expected empty lastSample after reset, got %q", ps.lastSample)
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
