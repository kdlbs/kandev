package scheduler

import (
	"context"
	"encoding/json"
	"time"

	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// assignmentWakePredicate is the result of classifying an incoming wake
// against the three-step predicate in the system design's "## The
// predicate": reason == task_assigned, actor_type == "agent", and the
// wake names a task. Evaluated in that order because a wake failing step
// 2 is out of scope and must never reach step 3's counted unattributed
// case (AC-OFFICE-ASSIGN-RATE-002.3).
type assignmentWakePredicate struct {
	// inScope is true once steps 1 and 2 both pass: this is an
	// agent-initiated assignment wake, per AC-OFFICE-ASSIGN-RATE-001.1.
	inScope bool
	// taskAttributed is true once step 3 also passes: the wake names a
	// task the allowance can be scoped to.
	taskAttributed bool
	taskID         string
	// actorID is the acting agent identifier, carried through for the
	// Warn log fields REQ-OFFICE-ASSIGN-RATE-003 names. Empty when the
	// payload carries none.
	actorID string
}

// classifyAssignmentWake evaluates the predicate against the incoming
// (not-yet-persisted) wake. It reads actor_type and task_id out of the
// encoded payload in Go, with one json.Unmarshal — the incoming wake is
// not a row yet, so dialect.JSONExtract (a SQL fragment over a column)
// cannot apply to it; that only applies to the stored rows the window
// count scans. An unparseable payload carries no readable actor_type
// either, so it fails step 2 and is classified out of scope, exactly
// like a payload whose actor_type is merely absent or not "agent" —
// never counted as unattributed, which AC-OFFICE-ASSIGN-RATE-002.3
// reserves for a wake already shown to be in scope.
func classifyAssignmentWake(reason, payload string) assignmentWakePredicate {
	if reason != RunReasonTaskAssigned {
		return assignmentWakePredicate{}
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return assignmentWakePredicate{}
	}
	actorType, _ := raw["actor_type"].(string)
	if actorType != "agent" {
		return assignmentWakePredicate{}
	}
	actorID, _ := raw["actor_id"].(string)
	pred := assignmentWakePredicate{inScope: true, actorID: actorID}

	// A task cannot be determined when the identifier is absent, null,
	// empty, or present but not a string; those shapes are treated
	// identically (AC-OFFICE-ASSIGN-RATE-002.3).
	v, present := raw["task_id"]
	if !present || v == nil {
		return pred
	}
	taskID, ok := v.(string)
	if !ok || taskID == "" {
		return pred
	}
	pred.taskID = taskID
	pred.taskAttributed = true
	return pred
}

// checkAssignmentWakeAllowance enforces REQ-OFFICE-ASSIGN-RATE-001..003.
// Returns true only when the wake must be refused — the caller must then
// insert no row and report QueueOutcomeRateLimited. Every other path
// (out of scope, unattributed, a failed count read, or room left in the
// allowance) returns false: admit the wake as queueRun otherwise would.
func (ss *SchedulerService) checkAssignmentWakeAllowance(
	ctx context.Context, agentInstanceID, reason, payload string,
) bool {
	pred := classifyAssignmentWake(reason, payload)
	if !pred.inScope {
		return false
	}
	if !pred.taskAttributed {
		runsservice.ReportAssignmentRateLimitUnattributed(pred.actorID, AssignmentWakeAllowanceN)
		return false
	}

	windowStart := time.Now().UTC().Add(-AssignmentWakeAllowanceWindow)
	count, err := ss.repo.CountAgentInitiatedAssignmentWakes(ctx, pred.taskID, reason, windowStart)
	if err != nil {
		runsservice.ReportAssignmentRateLimitCountReadFailed(
			pred.taskID, agentInstanceID, pred.actorID, AssignmentWakeAllowanceN, err)
		return false
	}
	if count < AssignmentWakeAllowanceN {
		return false
	}
	runsservice.ReportAssignmentRateLimitRefused(
		pred.taskID, agentInstanceID, pred.actorID, count, AssignmentWakeAllowanceN)
	return true
}
