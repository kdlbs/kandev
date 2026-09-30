package scheduler

import (
	"context"
	"testing"
	"time"

	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// TestQueueRun_AssignmentRateLimit_AdmitsAgainAfterFullWindowElapses covers
// the allowance's actual recovery: every other test in this package that
// calls ageRunsRequestedAt only ages runs past CoalesceRun's 5-second
// window, never past the 10-minute AssignmentWakeAllowanceWindow itself,
// so nothing previously proved the count query's requested_at cutoff
// actually lets the allowance refill once the exhausting wakes age out.
func TestQueueRun_AssignmentRateLimit_AdmitsAgainAfterFullWindowElapses(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	ctx := context.Background()
	taskID := "rl-task-window-recovery"

	agentIDs := admitNAgentWakes(t, repo, ss, "rlw-agent", taskID)

	// Confirm exhaustion before aging, so a later admit can only be
	// explained by the window recovering, not by never having been
	// exhausted in the first place.
	createChildrenCompletedAgent(t, repo, "rlw-agent-probe")
	probeOutcome, err := ss.QueueRun(ctx, "rlw-agent-probe", RunReasonTaskAssigned, agentActorPayload(taskID), "")
	if err != nil {
		t.Fatalf("QueueRun probe: %v", err)
	}
	if probeOutcome != runsservice.QueueOutcomeRateLimited {
		t.Fatalf("probe outcome = %q, want rate_limited before aging", probeOutcome)
	}

	// Age every persisted run past the full allowance window (not just
	// CoalesceRun's 5s window), so the window-count query sees none of
	// the exhausting wakes anymore.
	ageRunsRequestedAt(t, ss, AssignmentWakeAllowanceWindow+time.Minute)

	for i, agentID := range agentIDs {
		outcome, err := ss.QueueRun(ctx, agentID, RunReasonTaskAssigned, agentActorPayload(taskID), "")
		if err != nil {
			t.Fatalf("QueueRun after window recovery [%d]: %v", i, err)
		}
		if outcome != runsservice.QueueOutcomeQueued {
			t.Fatalf("QueueRun after window recovery [%d] outcome = %q, want queued (the aged-out wakes must no longer count)", i, outcome)
		}
	}

	// The N freshly-admitted wakes above re-exhaust the allowance.
	createChildrenCompletedAgent(t, repo, "rlw-agent-probe-2")
	finalOutcome, err := ss.QueueRun(ctx, "rlw-agent-probe-2", RunReasonTaskAssigned, agentActorPayload(taskID), "")
	if err != nil {
		t.Fatalf("QueueRun final probe: %v", err)
	}
	if finalOutcome != runsservice.QueueOutcomeRateLimited {
		t.Fatalf("final probe outcome = %q, want rate_limited (re-exhausted after recovery)", finalOutcome)
	}
}
