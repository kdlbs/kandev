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
		permissionMessageCreateFn: func(_ context.Context, _, _, _, _, _, _, _ string, _ []map[string]interface{}, _ string, _ map[string]interface{}, options models.PermissionRequestMessageOptions) (string, error) {
			steps = append(steps, "message")
			if options.Decision != nil {
				t.Fatal("candidate was stored as already approved")
			}
			if options.Presentation != models.PermissionPresentationAutomatic {
				t.Fatalf("presentation = %q, want automatic", options.Presentation)
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
	creator := &mockMessageCreator{permissionMessageCreateFn: func(context.Context, string, string, string, string, string, string, string, []map[string]interface{}, string, map[string]interface{}, models.PermissionRequestMessageOptions) (string, error) {
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
	creator.permissionMessageCreateFn = func(context.Context, string, string, string, string, string, string, string, []map[string]interface{}, string, map[string]interface{}, models.PermissionRequestMessageOptions) (string, error) {
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

func TestAutomaticPermissionFallbackRequiresLiveUnclaimedRequest(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "")
	manager := permissionResolvingManager(t, "session-1", nil)
	var got models.PermissionPresentationUpdate
	creator := &mockMessageCreator{
		permissionClaimFn: func(context.Context, models.PermissionResolutionClaimRequest) (*models.PermissionResolutionClaimResult, error) {
			return nil, errors.New("claim storage unavailable")
		},
		permissionPresentationFn: func(_ context.Context, request models.PermissionPresentationUpdate) (*models.Message, bool, error) {
			got = request
			return &models.Message{ID: "message-1"}, true, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.messageCreator = creator
	svc.handlePermissionRequest(context.Background(), watcher.PermissionRequestData{
		TaskID: "task-1", TaskSessionID: "session-1", RequestID: "request-1", PendingID: "pending-1",
		AutoApprovedOptionID: "allow-once", AutoApprovedOptionKind: "allow_once", AutoApprovalPending: true,
	})
	if got.Presentation != models.PermissionPresentationInteractive || !got.RequireUnclaimed ||
		got.RequirePresentation != models.PermissionPresentationAutomatic {
		t.Fatalf("presentation update = %+v, want guarded interactive promotion", got)
	}
}

func TestAutomaticPermissionFailureDoesNotReopenDispatch(t *testing.T) {
	for _, tc := range []struct {
		name   string
		fail   func(*mockAgentManager)
		finish func(*mockMessageCreator)
	}{
		{
			name: "delivery failure after claim",
			fail: func(manager *mockAgentManager) {
				manager.resolvePermissionFunc = func(context.Context, string, string, string, string) (*streams.PermissionResolveResponse, error) {
					return nil, errors.New("provider delivery failed")
				}
			},
		},
		{
			name: "finalization failure after delivery",
			finish: func(creator *mockMessageCreator) {
				creator.permissionFinishFn = func(context.Context, models.PermissionResolutionFinalizeRequest) (*models.PermissionResolutionFinalizeResult, error) {
					return nil, errors.New("audit storage unavailable")
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := setupTestRepo(t)
			seedSession(t, repo, "task-1", "session-1", "")
			manager := permissionResolvingManager(t, "session-1", nil)
			if tc.fail != nil {
				tc.fail(manager)
			}
			var got models.PermissionPresentationUpdate
			creator := &mockMessageCreator{
				permissionAuditFn: func(context.Context, string, string, string, string) (*models.PermissionResolutionAudit, error) {
					return &models.PermissionResolutionAudit{
						ClaimID: "claim-auto", Source: models.PermissionSourceAutoApprove,
						Result: models.PermissionResolutionDispatching,
					}, nil
				},
				permissionPresentationFn: func(_ context.Context, request models.PermissionPresentationUpdate) (*models.Message, bool, error) {
					got = request
					return &models.Message{ID: "message-1"}, true, nil
				},
			}
			if tc.finish != nil {
				tc.finish(creator)
			}
			svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), manager)
			svc.messageCreator = creator
			svc.handlePermissionRequest(context.Background(), watcher.PermissionRequestData{
				TaskID: "task-1", TaskSessionID: "session-1", RequestID: "request-1", PendingID: "pending-1",
				AutoApprovedOptionID: "allow-once", AutoApprovedOptionKind: "allow_once", AutoApprovalPending: true,
			})
			if got.Presentation != models.PermissionPresentationUnavailable || got.RequireUnclaimed ||
				got.RequirePresentation != models.PermissionPresentationAutomatic {
				t.Fatalf("presentation update = %+v, want unavailable without reopening the claim", got)
			}
		})
	}
}

func TestAutomaticPermissionFailureShowsWithdrawnRequestAsUnavailable(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "")
	manager := permissionResolvingManager(t, "session-1", nil)
	manager.listPermissionsFunc = func(context.Context, string) ([]streams.PendingAgentPermission, error) {
		return nil, nil
	}
	var got models.PermissionPresentationUpdate
	creator := &mockMessageCreator{
		permissionPresentationFn: func(_ context.Context, request models.PermissionPresentationUpdate) (*models.Message, bool, error) {
			got = request
			return &models.Message{ID: "message-1"}, true, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.messageCreator = creator
	svc.handlePermissionRequest(context.Background(), watcher.PermissionRequestData{
		TaskID: "task-1", TaskSessionID: "session-1", RequestID: "request-1", PendingID: "pending-1",
		AutoApprovedOptionID: "allow-once", AutoApprovedOptionKind: "allow_once", AutoApprovalPending: true,
	})

	if got.Presentation != models.PermissionPresentationUnavailable || !got.RequireUnclaimed {
		t.Fatalf("presentation update = %+v, want withdrawn unavailable feedback", got)
	}
}

func TestAutomaticPermissionFailureSettlesClaimReadFailureAsUnavailable(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "")
	manager := permissionResolvingManager(t, "session-1", nil)
	deliveries := 0
	manager.resolvePermissionFunc = func(context.Context, string, string, string, string) (*streams.PermissionResolveResponse, error) {
		deliveries++
		return nil, nil
	}
	var presentationCalls []models.PermissionPresentationUpdate
	creator := &mockMessageCreator{
		permissionClaimFn: func(context.Context, models.PermissionResolutionClaimRequest) (*models.PermissionResolutionClaimResult, error) {
			// The SQL claim is installed, then its read-back fails.
			return nil, errors.New("claim read failed")
		},
		permissionAuditFn: func(context.Context, string, string, string, string) (*models.PermissionResolutionAudit, error) {
			return &models.PermissionResolutionAudit{
				ClaimID: "claim-auto", Source: models.PermissionSourceAutoApprove,
				Result: models.PermissionResolutionDispatching,
			}, nil
		},
		permissionPresentationFn: func(_ context.Context, request models.PermissionPresentationUpdate) (*models.Message, bool, error) {
			presentationCalls = append(presentationCalls, request)
			if request.RequireUnclaimed {
				return nil, false, nil
			}
			return &models.Message{ID: "message-1"}, true, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.messageCreator = creator
	svc.handlePermissionRequest(context.Background(), watcher.PermissionRequestData{
		TaskID: "task-1", TaskSessionID: "session-1", RequestID: "request-1", PendingID: "pending-1",
		AutoApprovedOptionID: "allow-once", AutoApprovedOptionKind: "allow_once", AutoApprovalPending: true,
	})

	if deliveries != 0 {
		t.Fatalf("provider deliveries = %d, want 0", deliveries)
	}
	if len(presentationCalls) != 2 {
		t.Fatalf("presentation updates = %+v, want a guarded attempt then an automatic-claim settlement", presentationCalls)
	}
	last := presentationCalls[len(presentationCalls)-1]
	if last.Presentation != models.PermissionPresentationUnavailable || last.RequireUnclaimed ||
		last.RequirePresentation != models.PermissionPresentationAutomatic {
		t.Fatalf("final presentation update = %+v, want unavailable for this automatic claim", last)
	}
}

func TestAutomaticPermissionFailureSettlesEnumerationFailureAsUnavailable(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "")
	manager := permissionResolvingManager(t, "session-1", nil)
	manager.listPermissionsFunc = func(context.Context, string) ([]streams.PendingAgentPermission, error) {
		return nil, errors.New("runtime list unavailable")
	}
	var got models.PermissionPresentationUpdate
	creator := &mockMessageCreator{
		permissionPresentationFn: func(_ context.Context, request models.PermissionPresentationUpdate) (*models.Message, bool, error) {
			got = request
			return &models.Message{ID: "message-1"}, true, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.messageCreator = creator
	svc.handlePermissionRequest(context.Background(), watcher.PermissionRequestData{
		TaskID: "task-1", TaskSessionID: "session-1", RequestID: "request-1", PendingID: "pending-1",
		AutoApprovedOptionID: "allow-once", AutoApprovedOptionKind: "allow_once", AutoApprovalPending: true,
	})

	if got.Presentation != models.PermissionPresentationUnavailable || !got.RequireUnclaimed ||
		got.RequirePresentation != models.PermissionPresentationAutomatic {
		t.Fatalf("presentation update = %+v, want guarded unavailable feedback", got)
	}
}

func TestAutomaticPermissionFailureKeepsCompetingResolverOwnership(t *testing.T) {
	creator := &mockMessageCreator{
		permissionAuditFn: func(context.Context, string, string, string, string) (*models.PermissionResolutionAudit, error) {
			return &models.PermissionResolutionAudit{
				ClaimID: "claim-browser", Source: models.PermissionSourceWeb,
				Result: models.PermissionResolutionDispatching,
			}, nil
		},
		permissionPresentationFn: func(context.Context, models.PermissionPresentationUpdate) (*models.Message, bool, error) {
			t.Fatal("a competing resolver must keep presentation ownership")
			return nil, false, nil
		},
	}
	svc := &Service{logger: testLogger(), messageCreator: creator}
	svc.settleAutomaticPermissionUnavailable(context.Background(), watcher.PermissionRequestData{
		TaskID: "task-1", TaskSessionID: "session-1", RequestID: "request-1", PendingID: "pending-1",
	})
}

func TestAutomaticCandidateReplayDoesNotClaimPromotedFallback(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "")
	manager := permissionResolvingManager(t, "session-1", nil)
	deliveries := 0
	manager.resolvePermissionFunc = func(context.Context, string, string, string, string) (*streams.PermissionResolveResponse, error) {
		deliveries++
		return nil, nil
	}
	presentation := models.PermissionPresentationAutomatic
	var claimRequests []models.PermissionResolutionClaimRequest
	creator := &mockMessageCreator{
		permissionClaimFn: func(_ context.Context, request models.PermissionResolutionClaimRequest) (*models.PermissionResolutionClaimResult, error) {
			claimRequests = append(claimRequests, request)
			if len(claimRequests) == 1 {
				return nil, errors.New("claim read failed")
			}
			if request.AutomaticOnly && presentation == models.PermissionPresentationInteractive {
				return &models.PermissionResolutionClaimResult{Outcome: models.PermissionClaimAlreadyFinal}, nil
			}
			return &models.PermissionResolutionClaimResult{Outcome: models.PermissionClaimed}, nil
		},
		permissionPresentationFn: func(_ context.Context, request models.PermissionPresentationUpdate) (*models.Message, bool, error) {
			presentation = request.Presentation
			return &models.Message{ID: "message-1"}, true, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.messageCreator = creator
	data := watcher.PermissionRequestData{
		TaskID: "task-1", TaskSessionID: "session-1", RequestID: "request-1", PendingID: "pending-1",
		AutoApprovedOptionID: "allow-once", AutoApprovedOptionKind: "allow_once", AutoApprovalPending: true,
	}
	svc.handlePermissionRequest(context.Background(), data)
	svc.handlePermissionRequest(context.Background(), data)

	if deliveries != 0 {
		t.Fatalf("provider deliveries = %d, want 0 after a manual fallback", deliveries)
	}
	if len(claimRequests) != 2 || !claimRequests[1].AutomaticOnly {
		t.Fatalf("claim requests = %+v, want a replay guarded by automatic presentation", claimRequests)
	}
	if presentation != models.PermissionPresentationInteractive {
		t.Fatalf("presentation = %q, want interactive", presentation)
	}
}

func TestAutomaticPermissionFailureKeepsAcceptedFinalizationReadFailure(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "")
	manager := permissionResolvingManager(t, "session-1", nil)
	creator := &mockMessageCreator{
		permissionFinishFn: func(context.Context, models.PermissionResolutionFinalizeRequest) (*models.PermissionResolutionFinalizeResult, error) {
			return &models.PermissionResolutionFinalizeResult{Outcome: models.PermissionFinalized}, errors.New("finalize read-back failed")
		},
		permissionAuditFn: func(context.Context, string, string, string, string) (*models.PermissionResolutionAudit, error) {
			return &models.PermissionResolutionAudit{
				ClaimID: "claim-auto", Source: models.PermissionSourceAutoApprove,
				Result: models.PermissionResolutionAccepted,
			}, nil
		},
		permissionPresentationFn: func(context.Context, models.PermissionPresentationUpdate) (*models.Message, bool, error) {
			t.Fatal("an accepted finalization must not be presented as unavailable")
			return nil, false, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.messageCreator = creator
	svc.handlePermissionRequest(context.Background(), watcher.PermissionRequestData{
		TaskID: "task-1", TaskSessionID: "session-1", RequestID: "request-1", PendingID: "pending-1",
		AutoApprovedOptionID: "allow-once", AutoApprovedOptionKind: "allow_once", AutoApprovalPending: true,
	})
}

func TestAutomaticPermissionFailureSettlesAfterCanceledRequestContext(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "")
	manager := permissionResolvingManager(t, "session-1", nil)
	manager.listPermissionsFunc = func(context.Context, string) ([]streams.PendingAgentPermission, error) {
		return nil, nil
	}
	var updates int
	creator := &mockMessageCreator{
		permissionPresentationFn: func(_ context.Context, request models.PermissionPresentationUpdate) (*models.Message, bool, error) {
			updates++
			return &models.Message{ID: "message-1"}, true, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.messageCreator = creator
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc.handlePermissionRequest(ctx, watcher.PermissionRequestData{
		TaskID: "task-1", TaskSessionID: "session-1", RequestID: "request-1", PendingID: "pending-1",
		AutoApprovedOptionID: "allow-once", AutoApprovedOptionKind: "allow_once", AutoApprovalPending: true,
	})

	if updates == 0 {
		t.Fatal("failure settlement must use a cleanup context after request cancellation")
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
