package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/coordinator"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type resumeTargets struct{}

func (resumeTargets) GetTarget(_ context.Context, id string) (*coordinator.TargetTask, error) {
	return &coordinator.TargetTask{ID: id, WorkspaceID: "ws-resume", WorkflowID: "wf-1",
		Primary: &coordinator.TargetSession{ID: "s1", State: "IDLE", HasExecutorRecord: true}}, nil
}
func (resumeTargets) HasLiveExecution(context.Context, string) bool { return false }

type countingResumer struct{ calls int }

func (r *countingResumer) ResumeTaskSession(context.Context, string, string) (bool, error) {
	r.calls++
	return true, nil
}

// With phase 3 effective but resume still requiring approval, the resume
// propose result is the phase 1 result, and a deduplicated repeat stays so.
func TestHandleProposeResume_Phase3WithoutAutomaticStaysPending(t *testing.T) {
	conn, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	store, err := coordinator.NewStore(conn, conn)
	require.NoError(t, err)
	_, err = conn.Exec(`CREATE TABLE IF NOT EXISTS workflows (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, name TEXT)`)
	require.NoError(t, err)
	_, err = conn.Exec(`INSERT INTO workflows (id, workspace_id, name) VALUES ('wf-1', 'ws-resume', 'wf-1')`)
	require.NoError(t, err)
	svc := coordinator.NewService(store, coordinator.NewValidator(nil, nil), nil, testLogger(t),
		coordinator.WithPhase2(true), coordinator.WithPhase3(true))
	c := &coordinator.Coordinator{WorkspaceID: "ws-resume", Name: "Ops", AgentProfileID: "agent-1", ExecutorProfileID: "executor-1"}
	require.NoError(t, store.CreateCoordinator(context.Background(), c))
	resumer := &countingResumer{}
	svc.SetKindDeps(coordinator.KindDeps{Tasks: resumeTargets{}, Resumer: resumer})
	h := &Handlers{logger: testLogger(t).WithFields(), eventBus: &recordingEventBus{}}
	h.SetCoordinatorService(svc)

	propose := func() map[string]interface{} {
		msg := makeWSMessage(t, coordinator.ActionProposeResume, map[string]interface{}{"task_id": "task-9", "rationale": "stuck"})
		resp, err := h.proposeKindHandler(coordinator.ProposalKindResume)(coordinatorPrincipalContext(c.ID), msg)
		require.NoError(t, err)
		require.Equal(t, ws.MessageTypeResponse, resp.Type, "got: %s", resp.Payload)
		var payload map[string]interface{}
		require.NoError(t, json.Unmarshal(resp.Payload, &payload))
		return payload
	}
	first := propose()
	require.Equal(t, string(coordinator.ProposalStatusPending), first["status"])
	require.Equal(t, false, first["deduplicated"])
	require.NotContains(t, first, "note")
	second := propose()
	require.Equal(t, true, second["deduplicated"])
	require.Equal(t, string(coordinator.ProposalStatusPending), second["status"])
	require.Zero(t, resumer.calls)
}

// A raised resume class makes the first propose reach the automatic path (it
// carries a held note here, as no containment is wired); the deduplicated
// repeat of the same open target never reaches it again.
func TestHandleProposeResume_DeduplicatedRepeatSkipsTheAutomaticPath(t *testing.T) {
	conn, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	store, err := coordinator.NewStore(conn, conn)
	require.NoError(t, err)
	_, err = conn.Exec(`CREATE TABLE IF NOT EXISTS workflows (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, name TEXT)`)
	require.NoError(t, err)
	_, err = conn.Exec(`INSERT INTO workflows (id, workspace_id, name) VALUES ('wf-1', 'ws-resume', 'wf-1')`)
	require.NoError(t, err)
	svc := coordinator.NewService(store, coordinator.NewValidator(nil, nil), nil, testLogger(t),
		coordinator.WithPhase2(true), coordinator.WithPhase3(true))
	c := &coordinator.Coordinator{WorkspaceID: "ws-resume", Name: "Ops", AgentProfileID: "agent-1", ExecutorProfileID: "executor-1"}
	require.NoError(t, store.CreateCoordinator(context.Background(), c))
	_, err = conn.Exec(`UPDATE coordinators SET autonomy_enabled = ?, policy_json = ? WHERE id = ?`, true,
		`{"version":1,"actions":{"create_task":"requires_approval","start_agent":"denied","message":"denied","move":"denied","resume":"automatic","stop":"denied"}}`, c.ID)
	require.NoError(t, err)
	require.NoError(t, store.InsertClassChangeTx(context.Background(), conn, coordinator.ClassChange{
		CoordinatorID: c.ID, Class: coordinator.ActionResume, FromValue: coordinator.SettingRequiresApproval,
		ToValue: coordinator.SettingAutomatic, ChangedBy: "mgr-1"}))
	resumer := &countingResumer{}
	svc.SetKindDeps(coordinator.KindDeps{Tasks: resumeTargets{}, Resumer: resumer})
	svc.SetDecisionDeps(decisionTasksStub{}, nil, nil)
	h := &Handlers{logger: testLogger(t).WithFields(), eventBus: &recordingEventBus{}}
	h.SetCoordinatorService(svc)

	propose := func() map[string]interface{} {
		msg := makeWSMessage(t, coordinator.ActionProposeResume, map[string]interface{}{"task_id": "task-9", "rationale": "stuck"})
		resp, err := h.proposeKindHandler(coordinator.ProposalKindResume)(coordinatorPrincipalContext(c.ID), msg)
		require.NoError(t, err)
		require.Equal(t, ws.MessageTypeResponse, resp.Type, "got: %s", resp.Payload)
		var payload map[string]interface{}
		require.NoError(t, json.Unmarshal(resp.Payload, &payload))
		return payload
	}
	first := propose()
	require.Equal(t, false, first["deduplicated"])
	require.Contains(t, first, "note", "the raised class must reach the automatic path")
	require.Equal(t, string(coordinator.ProposalStatusPending), first["status"])
	second := propose()
	require.Equal(t, true, second["deduplicated"])
	require.NotContains(t, second, "note")
	require.Equal(t, string(coordinator.ProposalStatusPending), second["status"])
	require.Zero(t, resumer.calls)
}

// decisionTasksStub satisfies coordinator.DecisionTaskService for tests that
// only need the automatic path to be reachable; none of its methods run.
type decisionTasksStub struct {
	coordinator.DecisionTaskService
}
