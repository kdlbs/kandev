package lifecycle

import (
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
)

func TestCorrelateRecoveryInstancesSingleInstanceReTracksRegardlessOfIDMatch(t *testing.T) {
	records := []*models.ExecutorRunning{
		{SessionID: "session-1", AgentExecutionID: "some-other-id"},
	}
	instance := &agentctl.InstanceInfo{ID: "instance-1", SessionID: "session-1"}

	result := CorrelateRecoveryInstances(records, []*agentctl.InstanceInfo{instance})

	if got := result.Winners["session-1"]; got != instance {
		t.Fatalf("Winners[session-1] = %v, want the sole live instance despite ID mismatch", got)
	}
	if len(result.ToStop) != 0 {
		t.Fatalf("ToStop = %v, want empty", result.ToStop)
	}
}

func TestCorrelateRecoveryInstancesOrphanInstanceIsStopped(t *testing.T) {
	instance := &agentctl.InstanceInfo{ID: "instance-1", SessionID: "session-no-record"}

	result := CorrelateRecoveryInstances(nil, []*agentctl.InstanceInfo{instance})

	if len(result.Winners) != 0 {
		t.Fatalf("Winners = %v, want empty", result.Winners)
	}
	if len(result.ToStop) != 1 || result.ToStop[0] != instance {
		t.Fatalf("ToStop = %v, want [instance]", result.ToStop)
	}
}

func TestCorrelateRecoveryInstancesRecordWithNoInstanceIsLeftAlone(t *testing.T) {
	records := []*models.ExecutorRunning{{SessionID: "session-1", AgentExecutionID: "instance-1"}}

	result := CorrelateRecoveryInstances(records, nil)

	if len(result.Winners) != 0 {
		t.Fatalf("Winners = %v, want empty", result.Winners)
	}
	if len(result.ToStop) != 0 {
		t.Fatalf("ToStop = %v, want empty (left to the existing stale-execution repair path)", result.ToStop)
	}
}

// TestCorrelateRecoveryInstancesDuplicateTiebreakExactMatchWins pins
// AC-EXECUTORS-SURVIVAL-002.10: with two live instances for one session, the
// one whose ID matches the record's agent execution identifier wins and the
// other is stopped.
func TestCorrelateRecoveryInstancesDuplicateTiebreakExactMatchWins(t *testing.T) {
	records := []*models.ExecutorRunning{{SessionID: "session-1", AgentExecutionID: "winner"}}
	winner := &agentctl.InstanceInfo{ID: "winner", SessionID: "session-1"}
	loser := &agentctl.InstanceInfo{ID: "loser", SessionID: "session-1"}

	result := CorrelateRecoveryInstances(records, []*agentctl.InstanceInfo{loser, winner})

	if got := result.Winners["session-1"]; got != winner {
		t.Fatalf("Winners[session-1] = %v, want winner", got)
	}
	if len(result.ToStop) != 1 || result.ToStop[0] != loser {
		t.Fatalf("ToStop = %v, want [loser]", result.ToStop)
	}
}

// TestCorrelateRecoveryInstancesDuplicateNoMatchStopsAll pins the "no match"
// branch of AC-EXECUTORS-SURVIVAL-002.10: neither instance's ID equals the
// record's agent execution identifier, so none is re-tracked and both stop.
func TestCorrelateRecoveryInstancesDuplicateNoMatchStopsAll(t *testing.T) {
	records := []*models.ExecutorRunning{{SessionID: "session-1", AgentExecutionID: "neither"}}
	instanceA := &agentctl.InstanceInfo{ID: "a", SessionID: "session-1"}
	instanceB := &agentctl.InstanceInfo{ID: "b", SessionID: "session-1"}

	result := CorrelateRecoveryInstances(records, []*agentctl.InstanceInfo{instanceA, instanceB})

	if len(result.Winners) != 0 {
		t.Fatalf("Winners = %v, want empty", result.Winners)
	}
	if len(result.ToStop) != 2 {
		t.Fatalf("ToStop = %v, want both instances stopped", result.ToStop)
	}
}

// TestCorrelateRecoveryInstancesDuplicateAmbiguousMatchStopsAll pins the
// "more than one match" branch of AC-EXECUTORS-SURVIVAL-002.10: attributing
// the transcript to the wrong instance is worse than a cold resume, so an
// ambiguous match re-tracks nothing.
func TestCorrelateRecoveryInstancesDuplicateAmbiguousMatchStopsAll(t *testing.T) {
	records := []*models.ExecutorRunning{{SessionID: "session-1", AgentExecutionID: "dup"}}
	instanceA := &agentctl.InstanceInfo{ID: "dup", SessionID: "session-1"}
	instanceB := &agentctl.InstanceInfo{ID: "dup", SessionID: "session-1"}

	result := CorrelateRecoveryInstances(records, []*agentctl.InstanceInfo{instanceA, instanceB})

	if len(result.Winners) != 0 {
		t.Fatalf("Winners = %v, want empty (ambiguous match)", result.Winners)
	}
	if len(result.ToStop) != 2 {
		t.Fatalf("ToStop = %v, want both instances stopped", result.ToStop)
	}
}

// TestCorrelateRecoveryInstancesMultipleSessionsAreIndependent pins
// AC-EXECUTORS-SURVIVAL-002.11: session-1's single-instance re-track and
// session-2's ambiguous-duplicate stop-all are decided independently, and
// neither's outcome depends on the other's.
func TestCorrelateRecoveryInstancesMultipleSessionsAreIndependent(t *testing.T) {
	records := []*models.ExecutorRunning{
		{SessionID: "session-1", AgentExecutionID: "s1-instance"},
		{SessionID: "session-2", AgentExecutionID: "neither-s2-candidate"},
	}
	s1Instance := &agentctl.InstanceInfo{ID: "s1-instance", SessionID: "session-1"}
	s2A := &agentctl.InstanceInfo{ID: "s2-a", SessionID: "session-2"}
	s2B := &agentctl.InstanceInfo{ID: "s2-b", SessionID: "session-2"}

	result := CorrelateRecoveryInstances(records, []*agentctl.InstanceInfo{s1Instance, s2A, s2B})

	if got := result.Winners["session-1"]; got != s1Instance {
		t.Fatalf("Winners[session-1] = %v, want s1Instance", got)
	}
	if _, ok := result.Winners["session-2"]; ok {
		t.Fatalf("Winners[session-2] should not exist: neither of session-2's two instances matched")
	}
	if len(result.ToStop) != 2 {
		t.Fatalf("ToStop = %v, want both session-2 instances stopped, session-1 untouched", result.ToStop)
	}
}

func TestCorrelateRecoveryInstancesEmptyInputsReturnEmptyResult(t *testing.T) {
	result := CorrelateRecoveryInstances(nil, nil)
	if len(result.Winners) != 0 || len(result.ToStop) != 0 {
		t.Fatalf("CorrelateRecoveryInstances(nil, nil) = %+v, want empty result", result)
	}
}
