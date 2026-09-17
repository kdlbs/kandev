package routines

import (
	"expvar"
	"strings"
)

// coordinatorInstallConditionsTotal counts every reportable condition
// CreateDefaultCoordinatorRoutine observes, keyed by workspace, assignee,
// and condition (AC-OFFICE-COORDINATOR-INSTALL-001.10). These count calls
// in which a condition was *observed*, not calls that completed
// successfully — a call that is ultimately rejected still counts the
// conditions it had already detected before rejecting.
var coordinatorInstallConditionsTotal = expvar.NewMap("office_coordinator_install_conditions_total")

// Condition labels for coordinatorInstallConditionsTotal. Each names a
// distinct reportable outcome the acceptance criteria require to be told
// apart from every other one.
const (
	coordinatorInstallConditionEmptyIdentity        = "empty_identity"
	coordinatorInstallConditionLookupFailed         = "lookup_failed"
	coordinatorInstallConditionTriggerReadFailed    = "trigger_read_failed"
	coordinatorInstallConditionDuplicateMatches     = "duplicate_matches"
	coordinatorInstallConditionScheduleState        = "schedule_state"
	coordinatorInstallConditionTriggerCompleted     = "trigger_completed"
	coordinatorInstallConditionRoutineCreated       = "routine_created"
	coordinatorInstallConditionNextOccurrenceFailed = "next_occurrence_failed"
	coordinatorInstallConditionTriggerCreateFailed  = "trigger_create_failed"
	coordinatorInstallConditionRoutineCreateFailed  = "routine_create_failed"
	coordinatorInstallConditionContention           = "contention"
	coordinatorInstallConditionCancelled            = "cancelled"
	coordinatorInstallConditionCommitFailed         = "commit_failed"
)

// coordinatorInstallLabel builds the "k1=v1;k2=v2" label string the office
// stall metrics (internal/orchestrator/office_stall_metrics.go) and the
// routine-arming-scan metrics (arming_metrics.go) already use.
func coordinatorInstallLabel(pairs ...string) string {
	if len(pairs)%2 != 0 {
		return ""
	}
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		parts = append(parts, pairs[i]+"="+pairs[i+1])
	}
	return strings.Join(parts, ";")
}

// coordinatorInstallObserved increments the counter for one reportable
// condition on one install call, naming the workspace and assignee agent
// per AC-OFFICE-COORDINATOR-INSTALL-001.10.
func coordinatorInstallObserved(condition, workspaceID, agentID string) {
	coordinatorInstallConditionsTotal.Add(
		coordinatorInstallLabel("condition", condition, "workspace", workspaceID, "assignee", agentID), 1)
}
