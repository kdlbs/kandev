package models

import (
	"encoding/json"
	"math"
	"strconv"
)

// WorkflowEntryIDFromAny accepts exact nonnegative ledger identities from typed
// metadata or JSON. Floats outside JSON's exact integer range fail closed.
func WorkflowEntryIDFromAny(value interface{}) (int64, bool) {
	switch v := value.(type) {
	case int64:
		return v, v >= 0
	case int:
		return int64(v), v >= 0
	case float64:
		if v >= 0 && v < 9007199254740992 && math.Trunc(v) == v {
			return int64(v), true
		}
	case json.Number:
		id, err := strconv.ParseInt(string(v), 10, 64)
		return id, err == nil && id >= 0
	}
	return 0, false
}

// MatchesWorkflowEntry distinguishes an explicit legacy zero entry from
// unproven provenance on a task with a known ledger entry.
func (s PendingStepCompletionSignal) MatchesWorkflowEntry(stepID string, entryID int64) bool {
	if s.StepID != stepID {
		return false
	}
	if s.WorkflowStepTransitionID == nil {
		return entryID == 0
	}
	return *s.WorkflowStepTransitionID == entryID
}
