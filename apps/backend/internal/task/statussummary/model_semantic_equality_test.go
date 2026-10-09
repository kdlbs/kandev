package statussummary

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.2
func TestTaskStatusSummarySemanticEqualityMatchesPersistence(t *testing.T) {
	occurredAt := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	base := TaskStatusSummary{
		Revision:  7,
		UpdatedAt: occurredAt.Add(time.Minute),
		ActiveError: &ActiveErrorSummary{
			Scope:           models.ErrorScopeSession,
			SessionID:       "session-1",
			Stamp:           "error-1",
			OccurredAt:      occurredAt,
			Preview:         "agent failed",
			RecoveryActions: nil,
			Causes:          nil,
		},
		TaskError: &ActiveErrorSummary{
			Scope:           models.ErrorScopeTask,
			Stamp:           "task-error-1",
			OccurredAt:      occurredAt,
			Preview:         "task failed",
			RecoveryActions: nil,
			Causes:          nil,
		},
	}

	withEmptyCollections := base
	activeEmpty := *base.ActiveError
	activeEmpty.RecoveryActions = []string{}
	activeEmpty.Causes = []models.AgentErrorCause{}
	taskEmpty := *base.TaskError
	taskEmpty.RecoveryActions = []string{}
	taskEmpty.Causes = []models.AgentErrorCause{}
	withEmptyCollections.ActiveError = &activeEmpty
	withEmptyCollections.TaskError = &taskEmpty
	withEmptyCollections.Revision = 99
	withEmptyCollections.UpdatedAt = occurredAt.Add(2 * time.Hour)
	if !base.SemanticEqual(withEmptyCollections) {
		t.Fatalf("nil and empty error collections must compare equal: %#v vs %#v", base, withEmptyCollections)
	}

	encoded, err := base.SemanticJSON()
	if err != nil {
		t.Fatalf("encode base summary: %v", err)
	}
	var roundTrip TaskStatusSummary
	if err := unmarshalSummary(encoded, &roundTrip); err != nil {
		t.Fatalf("decode canonical summary: %v", err)
	}
	if !base.SemanticEqual(roundTrip) {
		t.Fatalf("canonical persistence round trip changed semantics: %#v", roundTrip)
	}

	changed := base
	changed.PendingAction = pendingPermission
	if base.SemanticEqual(changed) {
		t.Fatal("a real semantic field change must not compare equal")
	}
}

// @covers AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.2
func TestTaskStatusSummarySemanticEqualityRejectsInvalidValues(t *testing.T) {
	invalid := TaskStatusSummary{
		ActiveError: &ActiveErrorSummary{Preview: strings.Repeat("x", MaxActiveErrorPreviewBytes+1)},
	}
	if invalid.SemanticEqual(invalid) {
		t.Fatal("invalid summaries must not be accepted as semantic no-ops")
	}
}

func unmarshalSummary(data []byte, summary *TaskStatusSummary) error {
	return json.Unmarshal(data, summary)
}
