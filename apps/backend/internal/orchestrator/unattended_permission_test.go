package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

type unattendedFixture struct {
	svc  *Service
	repo interface {
		GetTaskSession(context.Context, string) (*models.TaskSession, error)
	}
	audits    []models.PermissionResolutionAudit
	delivered []string
	cancelled []string
	finalized []models.PermissionStatus
}

func newUnattendedFixture(t *testing.T, options []streams.PermissionChoice) *unattendedFixture {
	t.Helper()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "")
	f := &unattendedFixture{repo: repo}
	manager := &mockAgentManager{
		listPermissionsFunc: func(context.Context, string) ([]streams.PendingAgentPermission, error) {
			return []streams.PendingAgentPermission{{RequestID: "request-live", PendingID: "pending-1", Options: options}}, nil
		},
		resolvePermissionFunc: func(_ context.Context, _, requestID, pendingID, optionID string) (*streams.PermissionResolveResponse, error) {
			f.delivered = append(f.delivered, requestID+"/"+pendingID+"/"+optionID)
			return &streams.PermissionResolveResponse{RequestID: requestID, PendingID: pendingID, OptionID: optionID, OptionKind: streams.PermissionOptionKindRejectOnce, Status: "resolved"}, nil
		},
		cancelPermissionFunc: func(_ context.Context, _, requestID, pendingID string) (*streams.PermissionCancelResponse, error) {
			f.cancelled = append(f.cancelled, requestID+"/"+pendingID)
			return &streams.PermissionCancelResponse{RequestID: requestID, PendingID: pendingID, Status: "cancelled"}, nil
		},
	}
	creator := &mockMessageCreator{
		permissionClaimFn: func(_ context.Context, request models.PermissionResolutionClaimRequest) (*models.PermissionResolutionClaimResult, error) {
			f.audits = append(f.audits, request.Audit)
			return &models.PermissionResolutionClaimResult{Outcome: models.PermissionClaimed}, nil
		},
		permissionFinishFn: func(_ context.Context, request models.PermissionResolutionFinalizeRequest) (*models.PermissionResolutionFinalizeResult, error) {
			f.finalized = append(f.finalized, request.Status)
			return &models.PermissionResolutionFinalizeResult{Outcome: models.PermissionFinalized}, nil
		},
	}
	f.svc = createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	f.svc.messageCreator = creator
	return f
}

func (f *unattendedFixture) sessionState(t *testing.T) models.TaskSessionState {
	t.Helper()
	session, err := f.repo.GetTaskSession(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	return session.State
}

var rejectAndAllow = []streams.PermissionChoice{
	{OptionID: "allow-once", Kind: streams.PermissionOptionKindAllowOnce},
	{OptionID: "reject-once", Kind: streams.PermissionOptionKindRejectOnce},
}

func TestResolveUnattendedPermissionRejectsWithLiveRequestIDAndAudit(t *testing.T) {
	f := newUnattendedFixture(t, rejectAndAllow)
	f.svc.setSessionWaitingForInput(context.Background(), "task-1", "session-1")
	if f.sessionState(t) != models.TaskSessionStateWaitingForInput {
		t.Fatal("precondition: session must be waiting")
	}

	if err := f.svc.ResolveUnattendedPermission(context.Background(), "task-1", "session-1", "pending-1", "turn-row-1"); err != nil {
		t.Fatal(err)
	}
	if len(f.delivered) != 1 || f.delivered[0] != "request-live/pending-1/reject-once" {
		t.Fatalf("delivered = %v, want the live request id and the reject option", f.delivered)
	}
	audit := f.audits[0]
	if audit.ActorKind != models.PermissionActorCoordinatorUnattended || audit.Source != models.PermissionSourceCoordinatorWake || audit.UnattendedTurnID != "turn-row-1" {
		t.Fatalf("audit = %+v", audit)
	}
	if f.sessionState(t) != models.TaskSessionStateRunning {
		t.Fatalf("session state = %s, want RUNNING", f.sessionState(t))
	}
}

func TestResolveUnattendedPermissionWithoutRejectOptionCancelsAndRestoresRunning(t *testing.T) {
	f := newUnattendedFixture(t, []streams.PermissionChoice{{OptionID: "allow-once", Kind: streams.PermissionOptionKindAllowOnce}})
	f.svc.setSessionWaitingForInput(context.Background(), "task-1", "session-1")
	if f.sessionState(t) != models.TaskSessionStateWaitingForInput {
		t.Fatal("precondition: session must be waiting")
	}

	if err := f.svc.ResolveUnattendedPermission(context.Background(), "task-1", "session-1", "pending-1", "turn-row-1"); err != nil {
		t.Fatal(err)
	}
	if len(f.cancelled) != 1 || f.cancelled[0] != "request-live/pending-1" || len(f.delivered) != 0 {
		t.Fatalf("cancelled=%v delivered=%v, want cancel only", f.cancelled, f.delivered)
	}
	if a := f.audits[0]; a.ActorKind != models.PermissionActorCoordinatorUnattended || a.UnattendedTurnID != "turn-row-1" {
		t.Fatalf("audit = %+v", a)
	}
	if f.sessionState(t) != models.TaskSessionStateRunning {
		t.Fatalf("session state = %s, want RUNNING after cancel", f.sessionState(t))
	}
}

func TestResolveUnattendedPermissionMissingEntryIsSuccess(t *testing.T) {
	f := newUnattendedFixture(t, rejectAndAllow)
	if err := f.svc.ResolveUnattendedPermission(context.Background(), "task-1", "session-1", "pending-gone", "turn-row-1"); err != nil {
		t.Fatalf("missing entry: %v", err)
	}
	if len(f.audits) != 0 || len(f.delivered) != 0 {
		t.Fatal("nothing may be claimed or delivered for a vanished request")
	}
}

func TestResolveUnattendedPermissionAlreadyResolvedIsSuccessOtherFailuresAreNot(t *testing.T) {
	f := newUnattendedFixture(t, rejectAndAllow)
	f.svc.messageCreator.(*mockMessageCreator).permissionClaimFn = func(context.Context, models.PermissionResolutionClaimRequest) (*models.PermissionResolutionClaimResult, error) {
		return &models.PermissionResolutionClaimResult{Outcome: models.PermissionClaimAlreadyFinal}, nil
	}
	if err := f.svc.ResolveUnattendedPermission(context.Background(), "task-1", "session-1", "pending-1", "t"); err != nil {
		t.Fatalf("already resolved: %v", err)
	}
	f.svc.messageCreator.(*mockMessageCreator).permissionClaimFn = func(context.Context, models.PermissionResolutionClaimRequest) (*models.PermissionResolutionClaimResult, error) {
		return nil, errors.New("db down")
	}
	if err := f.svc.ResolveUnattendedPermission(context.Background(), "task-1", "session-1", "pending-1", "t"); err == nil {
		t.Fatal("an audit failure must be reported so the denial is retried")
	}
}

func TestPermissionResolutionAuditOmitsUnattendedTurnIDWhenUnset(t *testing.T) {
	plain, err := json.Marshal(models.PermissionResolutionAudit{ClaimID: "c"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain), "unattended_turn_id") {
		t.Fatalf("ordinary audit gained a key: %s", plain)
	}
	withTurn, _ := json.Marshal(models.PermissionResolutionAudit{ClaimID: "c", UnattendedTurnID: "t1"})
	if !strings.Contains(string(withTurn), `"unattended_turn_id":"t1"`) {
		t.Fatalf("unattended audit lost its turn id: %s", withTurn)
	}
}

func TestHandlePermissionRequestCallsUnattendedHandlerOnlyAfterMessageStored(t *testing.T) {
	type call struct{ task, session, pending, turn string }
	run := func(t *testing.T, creator *mockMessageCreator, data watcher.PermissionRequestData) []call {
		t.Helper()
		repo := setupTestRepo(t)
		seedSession(t, repo, "task-1", "session-1", "")
		manager := permissionResolvingManager(t, "session-1", nil)
		svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), manager)
		svc.messageCreator = creator
		svc.activeTurns.Store("session-1", "turn-9")
		var calls []call
		svc.SetUnattendedPermissionHandler(func(_ context.Context, task, session, pending, turn string) {
			calls = append(calls, call{task, session, pending, turn})
		})
		svc.handlePermissionRequest(context.Background(), data)
		return calls
	}
	base := watcher.PermissionRequestData{TaskID: "task-1", TaskSessionID: "session-1", RequestID: "request-1", PendingID: "pending-1"}

	if got := run(t, &mockMessageCreator{}, base); len(got) != 1 || got[0] != (call{"task-1", "session-1", "pending-1", "turn-9"}) {
		t.Fatalf("stored request: calls = %+v", got)
	}
	failing := &mockMessageCreator{permissionMessageCreateFn: func(context.Context, string, string, string, string, string, string, string, []map[string]interface{}, string, map[string]interface{}, *models.PermissionDecision) (string, error) {
		return "", errors.New("db down")
	}}
	if got := run(t, failing, base); len(got) != 0 {
		t.Fatalf("a failed message write must leave the request to a person: %+v", got)
	}
	auto := base
	auto.AutoApprovedOptionID = "allow-once"
	if got := run(t, &mockMessageCreator{}, auto); len(got) != 0 {
		t.Fatalf("an auto-approved request is untouched: %+v", got)
	}
}
