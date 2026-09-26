package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

func TestHandlePermissionRequestPassesAutomaticDecisionToTranscript(t *testing.T) {
	creator := &mockMessageCreator{}
	svc := &Service{logger: testLogger(), messageCreator: creator}
	svc.activeTurns.Store("session-1", "turn-1")

	svc.handlePermissionRequest(context.Background(), watcher.PermissionRequestData{
		TaskID:                 "task-1",
		TaskSessionID:          "session-1",
		RequestID:              "request-1",
		PendingID:              "pending-1",
		AutoApprovedOptionID:   "allow-once",
		AutoApprovedOptionKind: "allow_once",
		AutoApprovalSource:     streams.PermissionDecisionSourceAutoApprove,
		Options: []map[string]interface{}{
			{"option_id": "allow-once", "kind": "allow_once"},
		},
	})

	if creator.permissionMessageWrites != 1 {
		t.Fatalf("permission message writes = %d, want 1", creator.permissionMessageWrites)
	}
	want := &models.PermissionDecision{
		OptionID:   "allow-once",
		OptionKind: "allow_once",
		Source:     streams.PermissionDecisionSourceAutoApprove,
	}
	if creator.permissionMessageDecision == nil || *creator.permissionMessageDecision != *want {
		t.Fatalf("permission decision = %+v, want %+v", creator.permissionMessageDecision, want)
	}
}

func TestHandlePermissionRequestRetriesAutomaticDecisionPersistence(t *testing.T) {
	creator := &mockMessageCreator{}
	creator.permissionMessageCreateFn = func(context.Context, string, string, string, string, string, string, string, []map[string]interface{}, string, map[string]interface{}, *models.PermissionDecision) (string, error) {
		if creator.permissionMessageWrites == 1 {
			return "", errors.New("temporary database error")
		}
		return "message-1", nil
	}
	svc := &Service{logger: testLogger(), messageCreator: creator}
	svc.activeTurns.Store("session-1", "turn-1")

	svc.handlePermissionRequest(context.Background(), watcher.PermissionRequestData{
		TaskID:                 "task-1",
		TaskSessionID:          "session-1",
		RequestID:              "request-1",
		PendingID:              "pending-1",
		AutoApprovedOptionID:   "allow-once",
		AutoApprovedOptionKind: "allow_once",
		AutoApprovalSource:     streams.PermissionDecisionSourceAutoApprove,
	})

	if creator.permissionMessageWrites != 2 {
		t.Fatalf("permission message write attempts = %d, want 2 after one transient failure", creator.permissionMessageWrites)
	}
}

func TestPermissionDecisionFromEventBackfillsOlderEventFields(t *testing.T) {
	decision := permissionDecisionFromEvent(watcher.PermissionRequestData{
		AutoApprovedOptionID: "allow-always",
		Options: []map[string]interface{}{
			{"option_id": "allow-always", "kind": "allow_always"},
		},
	})

	want := &models.PermissionDecision{
		OptionID:   "allow-always",
		OptionKind: "allow_always",
		Source:     streams.PermissionDecisionSourceAutoApprove,
	}
	if decision == nil || *decision != *want {
		t.Fatalf("permission decision = %+v, want %+v", decision, want)
	}
}
