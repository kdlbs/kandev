package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
	runsmodels "github.com/kandev/kandev/internal/runs/models"
)

type fakeRunPermissionResolver struct {
	resolved []string
	canceled []string
	err      error
}

func (f *fakeRunPermissionResolver) ResolveRunPermission(_ context.Context, executionID, requestID, pendingID, optionID string) error {
	f.resolved = append(f.resolved, executionID+"|"+requestID+"|"+pendingID+"|"+optionID)
	return f.err
}

func (f *fakeRunPermissionResolver) CancelRunPermission(_ context.Context, executionID, requestID, pendingID string) error {
	f.canceled = append(f.canceled, executionID+"|"+requestID+"|"+pendingID)
	return f.err
}

type tasklessPermissionFixture struct {
	svc      *service.Service
	eb       bus.EventBus
	resolver *fakeRunPermissionResolver
	stopper  *fakeRunExecutionStopper
	agent    *models.AgentInstance
	run      *runsmodels.Run
}

// newTasklessPermissionFixture launches one taskless run whose attempt 1 is
// running as execution "execution-run-session-1".
func newTasklessPermissionFixture(t *testing.T) *tasklessPermissionFixture {
	t.Helper()
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	f := &tasklessPermissionFixture{
		svc: svc, eb: eb, resolver: &fakeRunPermissionResolver{}, stopper: &fakeRunExecutionStopper{},
	}
	svc.SetRunPermissionResolver(f.resolver)
	svc.SetRunExecutionStopper(f.stopper)
	svc.SetRunSessionLauncher(&tasklessTestLauncher{svc: svc})
	f.agent = &models.AgentInstance{
		ID: "taskless-agent", WorkspaceID: "ws-1", Name: "taskless-agent",
		Role: models.AgentRoleCEO, Status: models.AgentStatusIdle,
		ExecutorPreference: `{"type":"local_pc"}`,
	}
	if err := svc.CreateAgentInstance(ctx, f.agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := svc.QueueRun(ctx, f.agent.ID, service.RunReasonRoutineTrigger, `{}`, "taskless-permission"); err != nil {
		t.Fatalf("queue: %v", err)
	}
	service.RunSchedulerTick(svc, ctx)
	runs, err := svc.ListRuns(ctx, f.agent.WorkspaceID)
	if err != nil || len(runs) != 1 || runs[0].Status != "claimed" {
		t.Fatalf("launched run = %#v, err=%v", runs, err)
	}
	f.run = runs[0]
	return f
}

func (f *tasklessPermissionFixture) request(mutate func(*lifecycle.PermissionRequestEventPayload)) lifecycle.PermissionRequestEventPayload {
	payload := lifecycle.PermissionRequestEventPayload{
		Type: "permission_request", AgentExecutionID: "execution-run-session-1",
		OwnerKind: lifecycle.ExecutionOwnerRun, WorkspaceID: f.agent.WorkspaceID,
		RunID: f.run.ID, RunSessionID: "run-session-1", RunAttempt: 1, AgentProfileID: f.agent.ID,
		RequestID: "request-1", PendingID: "pending-1", Title: "Run ls",
		Options: []lifecycle.PermissionOption{
			{OptionID: "allow-once", Name: "Allow", Kind: "allow_once"},
			{OptionID: "reject-once", Name: "Reject", Kind: "reject_once"},
		},
	}
	if mutate != nil {
		mutate(&payload)
	}
	return payload
}

func (f *tasklessPermissionFixture) publish(t *testing.T, payload lifecycle.PermissionRequestEventPayload) {
	t.Helper()
	subject := events.BuildPermissionRequestSubject(payload.RunSessionID)
	if err := f.eb.Publish(context.Background(), subject,
		bus.NewEvent(events.PermissionRequestReceived, "test", payload)); err != nil {
		t.Fatalf("publish permission request: %v", err)
	}
}

func (f *tasklessPermissionFixture) decisions(t *testing.T) []map[string]any {
	t.Helper()
	runEvents, err := f.svc.ListRunEventsForTest(context.Background(), f.run.ID)
	if err != nil {
		t.Fatalf("list run events: %v", err)
	}
	var out []map[string]any
	for _, event := range runEvents {
		if event.EventType != "permission.decision" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(event.Payload), &payload); err != nil {
			t.Fatalf("decode decision payload: %v", err)
		}
		out = append(out, payload)
	}
	return out
}

func TestTasklessPermission_RejectsAndRunFinishesOnReady(t *testing.T) {
	f := newTasklessPermissionFixture(t)

	f.publish(t, f.request(nil))

	if len(f.resolver.resolved) != 1 || f.resolver.resolved[0] != "execution-run-session-1|request-1|pending-1|reject-once" {
		t.Fatalf("resolved = %v, want the reject option", f.resolver.resolved)
	}
	decisions := f.decisions(t)
	if len(decisions) != 1 || decisions[0]["decision"] != "rejected" || decisions[0]["title"] != "Run ls" {
		t.Fatalf("decision events = %v", decisions)
	}

	// The answered turn ends through the normal AgentReady path.
	if err := f.eb.Publish(context.Background(), events.AgentReady, bus.NewEvent(events.AgentReady, "test", lifecycle.AgentEventPayload{
		AgentExecutionID: "execution-run-session-1", AgentProfileID: f.agent.ID,
		RunID: f.run.ID, RunSessionID: "run-session-1", RunAttempt: 1,
		OwnerKind: lifecycle.ExecutionOwnerRun, WorkspaceID: f.agent.WorkspaceID, Status: "READY",
	})); err != nil {
		t.Fatalf("publish ready: %v", err)
	}
	runs, err := f.svc.ListRuns(context.Background(), f.agent.WorkspaceID)
	if err != nil || len(runs) != 1 || runs[0].Status != service.RunStatusFinished {
		t.Fatalf("runs after ready = %#v, err=%v", runs, err)
	}
}

func TestTasklessPermission_ApprovesAutoApprovalCandidate(t *testing.T) {
	f := newTasklessPermissionFixture(t)

	f.publish(t, f.request(func(p *lifecycle.PermissionRequestEventPayload) {
		p.AutoApprovedOptionID = "allow-once"
		p.AutoApprovedOptionKind = "allow_once"
		p.AutoApprovalPending = true
	}))

	if len(f.resolver.resolved) != 1 || f.resolver.resolved[0] != "execution-run-session-1|request-1|pending-1|allow-once" {
		t.Fatalf("resolved = %v, want the auto-approval option", f.resolver.resolved)
	}
	if decisions := f.decisions(t); len(decisions) != 1 || decisions[0]["decision"] != "approved" {
		t.Fatalf("decision events = %v", decisions)
	}
}

func TestTasklessPermission_CancelsWithoutRejectOption(t *testing.T) {
	f := newTasklessPermissionFixture(t)

	f.publish(t, f.request(func(p *lifecycle.PermissionRequestEventPayload) {
		p.Options = []lifecycle.PermissionOption{{OptionID: "allow-once", Kind: "allow_once"}}
	}))

	if len(f.resolver.resolved) != 0 || len(f.resolver.canceled) != 1 ||
		f.resolver.canceled[0] != "execution-run-session-1|request-1|pending-1" {
		t.Fatalf("resolved = %v canceled = %v, want one cancel", f.resolver.resolved, f.resolver.canceled)
	}
	if decisions := f.decisions(t); len(decisions) != 1 || decisions[0]["decision"] != "cancelled" {
		t.Fatalf("decision events = %v", decisions)
	}
}

func TestTasklessPermission_IgnoresStaleAndTaskOwnedRequests(t *testing.T) {
	f := newTasklessPermissionFixture(t)

	f.publish(t, f.request(func(p *lifecycle.PermissionRequestEventPayload) { p.RunAttempt = 2 }))
	f.publish(t, f.request(func(p *lifecycle.PermissionRequestEventPayload) { p.AgentExecutionID = "execution-older" }))
	f.publish(t, f.request(func(p *lifecycle.PermissionRequestEventPayload) { p.RunID = "run-unknown" }))
	f.publish(t, f.request(func(p *lifecycle.PermissionRequestEventPayload) {
		p.OwnerKind = lifecycle.ExecutionOwnerTask
		p.TaskID, p.SessionID, p.RunSessionID = "task-1", "session-1", ""
	}))

	if len(f.resolver.resolved) != 0 || len(f.resolver.canceled) != 0 {
		t.Fatalf("stale or task-owned request answered: resolved=%v canceled=%v", f.resolver.resolved, f.resolver.canceled)
	}
	if decisions := f.decisions(t); len(decisions) != 0 {
		t.Fatalf("decision events = %v, want none", decisions)
	}
}

func TestTasklessPermission_RecordsDecisionWhenDeliveryFails(t *testing.T) {
	f := newTasklessPermissionFixture(t)
	f.resolver.err = errors.New("agentctl unavailable")

	f.publish(t, f.request(nil))

	decisions := f.decisions(t)
	if len(decisions) != 1 || decisions[0]["decision"] != "rejected" {
		t.Fatalf("decision events = %v, want the decision recorded before delivery", decisions)
	}
}
