package service_test

import (
	"context"
	"expvar"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// mockTaskStarterWithSession wraps mockTaskStarter and additionally
// implements service.TaskStarterWithSession, so direct-launch tests can
// drive the session-returning seam (AC-OFFICE-LOOP-LIVENESS-002.7)
// without touching the production orchestrator adapter.
type mockTaskStarterWithSession struct {
	*mockTaskStarter
	sessionID string
}

func (m *mockTaskStarterWithSession) StartTaskWithEnvReturningSession(
	ctx context.Context, taskID, agentProfileID, executorID,
	executorProfileID string, priority string, prompt, workflowStepID string,
	planMode bool, attachments []v1.MessageAttachment, env map[string]string,
) (string, error) {
	if err := m.StartTaskWithEnv(ctx, taskID, agentProfileID, executorID,
		executorProfileID, priority, prompt, workflowStepID, planMode, attachments, env); err != nil {
		return "", err
	}
	return m.sessionID, nil
}

func directHopExpvarInt(t *testing.T, mapName, key string) int64 {
	t.Helper()
	v := expvar.Get(mapName)
	if v == nil {
		t.Fatalf("expvar map %q not registered", mapName)
	}
	m, ok := v.(*expvar.Map)
	if !ok {
		t.Fatalf("expvar %q is not a Map", mapName)
	}
	sub := m.Get(key)
	if sub == nil {
		return 0
	}
	iv, ok := sub.(*expvar.Int)
	if !ok {
		t.Fatalf("expvar %q[%q] is not an Int", mapName, key)
	}
	return iv.Value()
}

// AC-OFFICE-LOOP-LIVENESS-002.7: on a direct (non-routed) launch, the
// session id the starter returns is persisted onto the run row.
func TestSchedulerTick_DirectLaunchPersistsSessionID(t *testing.T) {
	mock := &mockTaskStarterWithSession{mockTaskStarter: &mockTaskStarter{}, sessionID: "sess-direct-1"}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:                 "direct-session-agent",
		WorkspaceID:        "ws-1",
		Name:               "direct-session-worker",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, description, created_at, updated_at)
		VALUES ('task-direct-session-1', 'ws-1', 'Build API', 'Implement endpoint', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"task-direct-session-1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	before := directHopExpvarInt(t, "office_loop_launch_total", "workspace=ws-1")

	service.RunSchedulerTick(svc, ctx)

	if mock.callCount() != 1 {
		t.Fatalf("expected 1 launch call, got %d", mock.callCount())
	}
	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].SessionID != "sess-direct-1" {
		t.Fatalf("session_id = %q, want sess-direct-1", runs[0].SessionID)
	}
	after := directHopExpvarInt(t, "office_loop_launch_total", "workspace=ws-1")
	if after != before+1 {
		t.Fatalf("office_loop_launch_total delta = %d, want 1", after-before)
	}
}

// AC-002.8: a starter that only implements TaskStarterWithEnv (no
// session-returning seam) leaves the run's session id empty and counts
// under launch_without_session — the existing production fallback.
func TestSchedulerTick_DirectLaunchWithoutSessionSeamCountsWithoutSession(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:                 "direct-nosession-agent",
		WorkspaceID:        "ws-1",
		Name:               "direct-nosession-worker",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, description, created_at, updated_at)
		VALUES ('task-direct-nosession-1', 'ws-1', 'Build API', 'Implement endpoint', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"task-direct-nosession-1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	before := directHopExpvarInt(t, "office_loop_launch_without_session_total", "workspace=ws-1")

	service.RunSchedulerTick(svc, ctx)

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].SessionID != "" {
		t.Fatalf("session_id = %q, want empty", runs[0].SessionID)
	}
	after := directHopExpvarInt(t, "office_loop_launch_without_session_total", "workspace=ws-1")
	if after != before+1 {
		t.Fatalf("office_loop_launch_without_session_total delta = %d, want 1", after-before)
	}
}
