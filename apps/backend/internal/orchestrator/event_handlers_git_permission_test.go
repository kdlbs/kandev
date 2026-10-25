package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

func TestAutomaticPermissionCandidatePersistsBeforeProviderApproval(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "")
	steps := make([]string, 0, 4)
	manager := permissionResolvingManager(t, "session-1", &steps)
	creator := &mockMessageCreator{
		permissionMessageCreateFn: func(_ context.Context, _, _, _, _, _, _, _ string, _ []map[string]interface{}, _ string, _ map[string]interface{}, decision *models.PermissionDecision) (string, error) {
			steps = append(steps, "message")
			if decision != nil {
				t.Fatal("candidate was stored as already approved")
			}
			return "message-1", nil
		},
		permissionClaimFn: func(_ context.Context, request models.PermissionResolutionClaimRequest) (*models.PermissionResolutionClaimResult, error) {
			steps = append(steps, "claim")
			if request.Audit.Source != models.PermissionSourceAutoApprove || request.Audit.OptionID != "allow-once" || request.Audit.OptionKind != "allow_once" {
				t.Fatalf("audit = %+v", request.Audit)
			}
			return &models.PermissionResolutionClaimResult{Outcome: models.PermissionClaimed}, nil
		},
		permissionFinishFn: func(context.Context, models.PermissionResolutionFinalizeRequest) (*models.PermissionResolutionFinalizeResult, error) {
			steps = append(steps, "finalize")
			return &models.PermissionResolutionFinalizeResult{Outcome: models.PermissionFinalized}, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.messageCreator = creator
	svc.handlePermissionRequest(context.Background(), watcher.PermissionRequestData{
		TaskID: "task-1", TaskSessionID: "session-1", RequestID: "request-1", PendingID: "pending-1",
		AutoApprovedOptionID: "allow-once", AutoApprovedOptionKind: "allow_once",
		AutoApprovalSource: streams.PermissionDecisionSourceAutoApprove, AutoApprovalPending: true,
	})
	if got := strings.Join(steps, ","); got != "message,claim,deliver,finalize" {
		t.Fatalf("operation order = %s", got)
	}
}

func TestAutomaticPermissionCandidateStaysPendingWhenAuditWriteFails(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "")
	deliveries := 0
	manager := permissionResolvingManager(t, "session-1", nil)
	manager.resolvePermissionFunc = func(context.Context, string, string, string, string) (*streams.PermissionResolveResponse, error) {
		deliveries++
		return nil, nil
	}
	creator := &mockMessageCreator{permissionMessageCreateFn: func(context.Context, string, string, string, string, string, string, string, []map[string]interface{}, string, map[string]interface{}, *models.PermissionDecision) (string, error) {
		return "", errors.New("database unavailable")
	}}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.messageCreator = creator
	svc.handlePermissionRequest(context.Background(), watcher.PermissionRequestData{
		TaskID: "task-1", TaskSessionID: "session-1", RequestID: "request-1", PendingID: "pending-1",
		AutoApprovedOptionID: "allow-once", AutoApprovalPending: true,
	})
	if deliveries != 0 || creator.permissionMessageWrites != automaticPermissionMessageWriteMaxAttempts {
		t.Fatalf("deliveries=%d write attempts=%d", deliveries, creator.permissionMessageWrites)
	}
}

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
