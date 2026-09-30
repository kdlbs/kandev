package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/coordinator"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// With phase 3 effective but create_task still requiring approval, the propose
// result is exactly the phase 1 result: pending, with no task id and no note.
func TestHandleProposeTask_Phase3WithoutAutomaticStaysPending(t *testing.T) {
	conn, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	store, err := coordinator.NewStore(conn, conn)
	require.NoError(t, err)
	svc := coordinator.NewService(store, coordinator.NewValidator(nil, nil), nil, testLogger(t),
		coordinator.WithPhase2(true), coordinator.WithPhase3(true))
	c := &coordinator.Coordinator{WorkspaceID: "ws-propose", Name: "Ops", AgentProfileID: "agent-1", ExecutorProfileID: "executor-1"}
	require.NoError(t, store.CreateCoordinator(context.Background(), c))
	svc.SetProposalDeps(
		&fakeProposalWorkflowReader{workflows: map[string]*taskmodels.Workflow{"wf-1": {ID: "wf-1", WorkspaceID: c.WorkspaceID}}},
		nil, nil,
		&fakeProposalStepReader{stepsByWorkflow: map[string][]*wfmodels.WorkflowStep{"wf-1": {{ID: "start", IsStartStep: true}}}})
	h := &Handlers{logger: testLogger(t).WithFields(), eventBus: &recordingEventBus{}}
	h.SetCoordinatorService(svc)

	resp, err := h.handleProposeTask(coordinatorPrincipalContext(c.ID), makeWSMessage(t, coordinator.ActionProposeTask, proposeTaskPayload()))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type, "got: %s", resp.Payload)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	require.Equal(t, string(coordinator.ProposalStatusPending), payload["status"])
	require.NotContains(t, payload, "task_id")
	require.NotContains(t, payload, "note")
}
