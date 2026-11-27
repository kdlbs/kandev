package lifecycle

import (
	"context"
	"maps"
	"testing"
	"time"

	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
)

// A collection deadline drops pending sessions immediately while the real
// control-server stop outcomes continue through joint-failure handling.
func TestStandaloneExecutorRecoverInstancesDeadlineDropsPendingSessionAndFinishesInBackground(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.listInstances = []*agentctlclient.InstanceInfo{
		{ID: "winner", Port: 5001, SessionID: "session-1"},
		{ID: "loser", Port: 5002, SessionID: "session-1"},
	}
	control.deleteFailures["loser"] = 999  // never succeeds within its retry budget
	control.deleteFailures["winner"] = 999 // never succeeds either, once tried
	control.deleteDelay["loser"] = 150 * time.Millisecond
	exec := control.executor(t)
	exec.SetRecoveryRetryConfig(20*time.Millisecond, 0)
	recorder := &fakeUnstoppableRecorder{}
	exec.SetUnstoppableSessionRecorder(recorder)

	records := []*models.ExecutorRunning{{SessionID: "session-1", AgentExecutionID: "winner"}}

	instances, err := exec.listInstancesWithRetry(context.Background())
	if err != nil {
		t.Fatalf("listInstancesWithRetry: %v", err)
	}
	correlation := CorrelateRecoveryInstances(records, instances)
	pending := pendingLoserCounts(correlation.ToStop, correlation.Winners)
	tracker := &jointFailureTracker{exec: exec, winners: maps.Clone(correlation.Winners)}
	stops := exec.dispatchRecoveryStops(correlation.ToStop)
	// Hold stop outcomes until the collection deadline so setup latency cannot
	// consume the interval intended to exercise the background drain.
	results := make(chan recoveryStopOutcome, len(correlation.ToStop))
	defer close(results)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	start := time.Now()
	exec.collectRecoveryStops(ctx, results, correlation.Winners, pending, tracker)
	if len(correlation.Winners) != 0 {
		t.Fatalf("pending session survived the collection deadline: %v", correlation.Winners)
	}
	recovered := exec.buildRecoveredInstances(ctx, correlation.Winners, indexRecordsBySession(records))
	elapsed := time.Since(start)
	if len(recovered) != 0 {
		t.Fatalf("recovered = %+v, want none: the session's loser stop was still unresolved at the deadline", recovered)
	}
	if elapsed > 100*time.Millisecond {
		t.Fatalf("RecoverInstances took %s, want it to return promptly at the deadline instead of waiting for the in-flight stop", elapsed)
	}

	for outcome := range stops {
		results <- outcome
	}

	pollDeadline := time.Now().Add(2 * time.Second)
	for {
		recorder.mu.Lock()
		retained := len(recorder.retained)
		recorder.mu.Unlock()
		if retained == 1 {
			break
		}
		if time.Now().After(pollDeadline) {
			t.Fatal("timed out waiting for the background drain to finish joint-failure handling")
		}
		time.Sleep(5 * time.Millisecond)
	}

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.retained[0] != "session-1" {
		t.Fatalf("retained = %v, want session-1", recorder.retained)
	}
}
