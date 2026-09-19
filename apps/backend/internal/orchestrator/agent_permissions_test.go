package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
)

func TestListPendingAgentPermissionsUsesAuthorizedServerOwnership(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-permissions", "session-permissions", "")
	manager := &mockAgentManager{listPermissionsFunc: func(_ context.Context, sessionID string) ([]streams.PendingAgentPermission, error) {
		if sessionID != "session-permissions" {
			t.Fatalf("session = %q", sessionID)
		}
		return []streams.PendingAgentPermission{{
			TaskID:    "provider-task",
			SessionID: "provider-session",
			RequestID: "request-1",
			PendingID: "pending-1",
			CreatedAt: time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC),
			Status:    streams.PermissionStatusPending,
		}}, nil
	}}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), manager)

	permissions, err := svc.ListPendingAgentPermissions(context.Background(), "task-permissions", "session-permissions")
	if err != nil {
		t.Fatal(err)
	}
	if len(permissions) != 1 || permissions[0].TaskID != "task-permissions" || permissions[0].SessionID != "session-permissions" {
		t.Fatalf("unexpected server-owned projection: %+v", permissions)
	}
}

func TestListPendingAgentPermissionsRejectsMismatchedTaskSessionBeforeRuntime(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-a", "session-a", "")
	if err := repo.CreateTask(context.Background(), &models.Task{ID: "task-b", WorkspaceID: "ws1", WorkflowID: "wf1", Title: "Task B"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(context.Background(), &models.TaskSession{ID: "session-b", TaskID: "task-b", State: models.TaskSessionStateRunning}); err != nil {
		t.Fatal(err)
	}
	runtimeCalled := false
	manager := &mockAgentManager{listPermissionsFunc: func(context.Context, string) ([]streams.PendingAgentPermission, error) {
		runtimeCalled = true
		return nil, nil
	}}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), manager)

	_, err := svc.ListPendingAgentPermissions(context.Background(), "task-a", "session-b")
	if !errors.Is(err, ErrPermissionTaskOrSessionNotFound) {
		t.Fatalf("error = %v, want task/session not found", err)
	}
	if runtimeCalled {
		t.Fatal("runtime was queried before task/session binding was rejected")
	}
}

func TestResolveAgentPermissionClaimsBeforeDeliveryAndAuditsPATActor(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-resolve", "session-resolve", "")
	steps := make([]string, 0, 3)
	manager := permissionResolvingManager(t, "session-resolve", &steps)
	messages := &mockMessageCreator{
		permissionClaimFn: func(_ context.Context, request models.PermissionResolutionClaimRequest) (*models.PermissionResolutionClaimResult, error) {
			steps = append(steps, "claim")
			encoded, err := json.Marshal(request.Audit)
			if err != nil {
				t.Fatal(err)
			}
			if request.Audit.ActorKind != models.PermissionActorPersonalAccessToken || request.Audit.ActorUserID != "user-1" {
				t.Fatalf("unexpected actor: %+v", request.Audit)
			}
			if strings.Contains(string(encoded), "token-record-secret") {
				t.Fatalf("audit leaked token ID: %s", encoded)
			}
			return &models.PermissionResolutionClaimResult{Outcome: models.PermissionClaimed}, nil
		},
		permissionFinishFn: func(_ context.Context, request models.PermissionResolutionFinalizeRequest) (*models.PermissionResolutionFinalizeResult, error) {
			steps = append(steps, "finalize")
			if request.Result != models.PermissionResolutionAccepted || request.Status != models.PermissionStatusApproved {
				t.Fatalf("unexpected finalization: %+v", request)
			}
			return &models.PermissionResolutionFinalizeResult{Outcome: models.PermissionFinalized}, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.messageCreator = messages
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-1", TokenID: "token-record-secret"})

	result, err := svc.ResolveAgentPermission(ctx, ResolveAgentPermissionRequest{
		TaskID:    "task-resolve",
		SessionID: "session-resolve",
		RequestID: "request-1",
		PendingID: "pending-1",
		OptionID:  "allow-once",
		Source:    models.PermissionSourceExternalMCP,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "resolved" || result.OptionKind != streams.PermissionOptionKindAllowOnce {
		t.Fatalf("unexpected result: %+v", result)
	}
	if strings.Join(steps, ",") != "claim,deliver,finalize" {
		t.Fatalf("operation order = %v, want claim, deliver, finalize", steps)
	}
}

func TestResolveAgentPermissionRejectsInvalidOrReplayedRequestsWithoutDelivery(t *testing.T) {
	tests := []struct {
		name        string
		permissions []streams.PendingAgentPermission
		audit       *models.PermissionResolutionAudit
		requestID   string
		optionID    string
		wantErr     error
	}{
		{
			name:      "no pending request",
			requestID: "missing",
			optionID:  "allow-once",
			wantErr:   ErrPermissionNotFound,
		},
		{
			name:        "replaced request",
			permissions: []streams.PendingAgentPermission{{RequestID: "request-new", PendingID: "pending-1"}},
			requestID:   "request-old",
			optionID:    "allow-once",
			wantErr:     ErrPermissionStale,
		},
		{
			name: "unknown option",
			permissions: []streams.PendingAgentPermission{{
				RequestID: "request-1", PendingID: "pending-1",
				Options: []streams.PermissionChoice{{OptionID: "allow-once", Kind: streams.PermissionOptionKindAllowOnce}},
			}},
			requestID: "request-1",
			optionID:  "invented",
			wantErr:   ErrPermissionOptionNotOffered,
		},
		{
			name:      "already resolved",
			audit:     &models.PermissionResolutionAudit{ClaimID: "claim-old", Result: models.PermissionResolutionAccepted},
			requestID: "request-1",
			optionID:  "allow-once",
			wantErr:   ErrPermissionAlreadyResolved,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := setupTestRepo(t)
			seedSession(t, repo, "task-invalid", "session-invalid", "")
			deliveries := 0
			manager := &mockAgentManager{
				listPermissionsFunc: func(context.Context, string) ([]streams.PendingAgentPermission, error) {
					return test.permissions, nil
				},
				resolvePermissionFunc: func(context.Context, string, string, string, string) (*streams.PermissionResolveResponse, error) {
					deliveries++
					return nil, nil
				},
			}
			messages := &mockMessageCreator{permissionAuditFn: func(context.Context, string, string, string, string) (*models.PermissionResolutionAudit, error) {
				return test.audit, nil
			}}
			svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), manager)
			svc.messageCreator = messages
			_, err := svc.ResolveAgentPermission(context.Background(), ResolveAgentPermissionRequest{
				TaskID: "task-invalid", SessionID: "session-invalid", RequestID: test.requestID,
				PendingID: "pending-1", OptionID: test.optionID, Source: models.PermissionSourceExternalMCP,
			})
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}
			if deliveries != 0 {
				t.Fatalf("deliveries = %d, want 0", deliveries)
			}
		})
	}
}

func TestResolveAgentPermissionExpiresPersistedCardWhenLiveRequestIsMissing(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-stale-card", "session-stale-card", "")
	manager := &mockAgentManager{listPermissionsFunc: func(context.Context, string) ([]streams.PendingAgentPermission, error) {
		return nil, nil
	}}
	expired := false
	messages := &mockMessageCreator{
		permissionAuditFn: func(context.Context, string, string, string, string) (*models.PermissionResolutionAudit, error) {
			return nil, nil
		},
		permissionUpdateFn: func(_ context.Context, taskID, sessionID, requestID, pendingID string, status models.PermissionStatus) error {
			expired = taskID == "task-stale-card" && sessionID == "session-stale-card" &&
				requestID == "request-stale-card" && pendingID == "pending-stale-card" &&
				status == models.PermissionStatusExpired
			return nil
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.messageCreator = messages

	_, err := svc.ResolveAgentPermission(context.Background(), ResolveAgentPermissionRequest{
		TaskID: "task-stale-card", SessionID: "session-stale-card", RequestID: "request-stale-card",
		PendingID: "pending-stale-card", OptionID: "allow-once", Source: models.PermissionSourceWeb,
	})
	if !errors.Is(err, ErrPermissionNotFound) || !expired {
		t.Fatalf("error=%v expired=%v, want not found with persisted card expired", err, expired)
	}
}

func TestCancelAgentPermissionUsesSyntheticCancelledAuditChoice(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-cancel-audit", "session-cancel-audit", "")
	manager := permissionResolvingManager(t, "session-cancel-audit", nil)
	manager.cancelPermissionFunc = func(_ context.Context, _, requestID, pendingID string) (*streams.PermissionCancelResponse, error) {
		return &streams.PermissionCancelResponse{RequestID: requestID, PendingID: pendingID, Status: "cancelled"}, nil
	}
	messages := &mockMessageCreator{permissionClaimFn: func(_ context.Context, request models.PermissionResolutionClaimRequest) (*models.PermissionResolutionClaimResult, error) {
		if request.Audit.OptionID != "cancelled" || request.Audit.OptionKind != "cancelled" {
			t.Fatalf("cancel audit recorded provider option: %+v", request.Audit)
		}
		return &models.PermissionResolutionClaimResult{Outcome: models.PermissionClaimed}, nil
	}}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.messageCreator = messages

	err := svc.RespondToPermission(context.Background(), "task-cancel-audit", "session-cancel-audit",
		"request-1", "pending-1", "ignored-provider-option", true, true)
	if err != nil {
		t.Fatalf("cancel permission: %v", err)
	}
}

func TestResolveAgentPermissionAuditFailurePreventsDelivery(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-audit-fail", "session-audit-fail", "")
	deliveries := 0
	manager := permissionResolvingManager(t, "session-audit-fail", nil)
	manager.resolvePermissionFunc = func(context.Context, string, string, string, string) (*streams.PermissionResolveResponse, error) {
		deliveries++
		return nil, nil
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.messageCreator = &mockMessageCreator{permissionClaimFn: func(context.Context, models.PermissionResolutionClaimRequest) (*models.PermissionResolutionClaimResult, error) {
		return nil, errors.New("database unavailable")
	}}

	_, err := svc.ResolveAgentPermission(context.Background(), ResolveAgentPermissionRequest{
		TaskID: "task-audit-fail", SessionID: "session-audit-fail", RequestID: "request-1",
		PendingID: "pending-1", OptionID: "allow-once", Source: models.PermissionSourceExternalMCP,
	})
	if !errors.Is(err, ErrPermissionAuditFailed) || deliveries != 0 {
		t.Fatalf("error=%v deliveries=%d", err, deliveries)
	}
}

func TestResolveAgentPermissionNilClaimFailsClosed(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-nil-claim", "session-nil-claim", "")
	deliveries := 0
	manager := permissionResolvingManager(t, "session-nil-claim", nil)
	manager.resolvePermissionFunc = func(context.Context, string, string, string, string) (*streams.PermissionResolveResponse, error) {
		deliveries++
		return nil, nil
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.messageCreator = &mockMessageCreator{permissionClaimFn: func(context.Context, models.PermissionResolutionClaimRequest) (*models.PermissionResolutionClaimResult, error) {
		return nil, nil
	}}

	_, err := svc.ResolveAgentPermission(context.Background(), ResolveAgentPermissionRequest{
		TaskID: "task-nil-claim", SessionID: "session-nil-claim", RequestID: "request-1",
		PendingID: "pending-1", OptionID: "allow-once", Source: models.PermissionSourceExternalMCP,
	})
	if !errors.Is(err, ErrPermissionAuditFailed) || deliveries != 0 {
		t.Fatalf("error=%v deliveries=%d, want audit failure before delivery", err, deliveries)
	}
}

func TestCancelAgentPermissionNilClaimFailsClosed(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-nil-cancel-claim", "session-nil-cancel-claim", "")
	cancellations := 0
	manager := permissionResolvingManager(t, "session-nil-cancel-claim", nil)
	manager.cancelPermissionFunc = func(context.Context, string, string, string) (*streams.PermissionCancelResponse, error) {
		cancellations++
		return nil, nil
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.messageCreator = &mockMessageCreator{permissionClaimFn: func(context.Context, models.PermissionResolutionClaimRequest) (*models.PermissionResolutionClaimResult, error) {
		return nil, nil
	}}

	err := svc.RespondToPermission(
		context.Background(),
		"task-nil-cancel-claim",
		"session-nil-cancel-claim",
		"request-1",
		"pending-1",
		"",
		true,
		true,
	)
	if !errors.Is(err, ErrPermissionAuditFailed) || cancellations != 0 {
		t.Fatalf("error=%v cancellations=%d, want audit failure before cancellation", err, cancellations)
	}
}

func TestResolveAgentPermissionFinalizesStaleDelivery(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-stale", "session-stale", "")
	manager := permissionResolvingManager(t, "session-stale", nil)
	manager.resolvePermissionFunc = func(context.Context, string, string, string, string) (*streams.PermissionResolveResponse, error) {
		return nil, &agentctlclient.PermissionOperationError{Code: streams.PermissionErrorStale, Message: streams.PermissionErrorStale}
	}
	finalized := false
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.messageCreator = &mockMessageCreator{permissionFinishFn: func(_ context.Context, request models.PermissionResolutionFinalizeRequest) (*models.PermissionResolutionFinalizeResult, error) {
		finalized = request.Result == models.PermissionResolutionStale && request.Status == models.PermissionStatusExpired
		return &models.PermissionResolutionFinalizeResult{Outcome: models.PermissionFinalized}, nil
	}}

	_, err := svc.ResolveAgentPermission(context.Background(), ResolveAgentPermissionRequest{
		TaskID: "task-stale", SessionID: "session-stale", RequestID: "request-1",
		PendingID: "pending-1", OptionID: "allow-once", Source: models.PermissionSourceExternalMCP,
	})
	if !errors.Is(err, ErrPermissionStale) || !finalized {
		t.Fatalf("error=%v finalized=%v", err, finalized)
	}
}

func TestRespondToPermissionPreservesGenerationSafeCancelFallback(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-cancel", "session-cancel", "")
	cancelled := false
	manager := permissionResolvingManager(t, "session-cancel", nil)
	manager.cancelPermissionFunc = func(_ context.Context, sessionID, requestID, pendingID string) (*streams.PermissionCancelResponse, error) {
		cancelled = sessionID == "session-cancel" && requestID == "request-1" && pendingID == "pending-1"
		return &streams.PermissionCancelResponse{RequestID: requestID, PendingID: pendingID, Status: "cancelled"}, nil
	}
	finalized := false
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.messageCreator = &mockMessageCreator{permissionFinishFn: func(_ context.Context, request models.PermissionResolutionFinalizeRequest) (*models.PermissionResolutionFinalizeResult, error) {
		finalized = request.Status == models.PermissionStatusRejected && request.Result == models.PermissionResolutionAccepted
		return &models.PermissionResolutionFinalizeResult{Outcome: models.PermissionFinalized}, nil
	}}

	err := svc.RespondToPermission(context.Background(), "task-cancel", "session-cancel", "request-1", "pending-1", "", true, true)
	if err != nil || !cancelled || !finalized {
		t.Fatalf("error=%v cancelled=%v finalized=%v", err, cancelled, finalized)
	}
}

func permissionResolvingManager(t *testing.T, sessionID string, steps *[]string) *mockAgentManager {
	t.Helper()
	return &mockAgentManager{
		listPermissionsFunc: func(_ context.Context, gotSessionID string) ([]streams.PendingAgentPermission, error) {
			if gotSessionID != sessionID {
				t.Fatalf("session = %q, want %q", gotSessionID, sessionID)
			}
			return []streams.PendingAgentPermission{{
				RequestID: "request-1",
				PendingID: "pending-1",
				Options: []streams.PermissionChoice{{
					OptionID: "allow-once",
					Name:     "Allow once",
					Kind:     streams.PermissionOptionKindAllowOnce,
				}},
			}}, nil
		},
		resolvePermissionFunc: func(_ context.Context, gotSessionID, requestID, pendingID, optionID string) (*streams.PermissionResolveResponse, error) {
			if steps != nil {
				*steps = append(*steps, "deliver")
			}
			if gotSessionID != sessionID || requestID != "request-1" || pendingID != "pending-1" || optionID != "allow-once" {
				t.Fatalf("unexpected delivery tuple: %q %q %q %q", gotSessionID, requestID, pendingID, optionID)
			}
			return &streams.PermissionResolveResponse{
				RequestID: requestID, PendingID: pendingID, OptionID: optionID,
				OptionKind: streams.PermissionOptionKindAllowOnce, Status: "resolved",
			}, nil
		},
	}
}
