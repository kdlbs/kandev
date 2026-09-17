package backendapp

import (
	"context"
	"testing"

	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/orchestrator/dispatchcontext"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/require"
)

func TestAssistantContextIdleRefreshReplacesOldPacket(t *testing.T) {
	a, svc := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()
	a.profiles = workspaceTestProfiles{"personal": {ID: "personal", Enabled: true}}
	wf, err := svc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{WorkspaceID: "ws-1", Name: "Delivery"})
	require.NoError(t, err)
	packet := &shared.ContextPacket{ID: "old", BindingID: "binding", WorkspaceID: "ws-1", ObjectiveID: "goal", ContextScope: shared.ContextScope{ProfileID: "personal"},
		Memory: []shared.ContextMemory{{ID: "preference", Content: "FORGOTTEN_RULE"}}}
	ref := shared.DelegationReference{ObjectiveID: "goal", ContextRef: "old", Packet: packet}
	id, err := a.CreateWorkspaceTask(ctx, shared.WorkspaceTaskSpec{WorkspaceID: "ws-1", WorkflowID: wf.ID, Title: "Do work", Description: "BASE_INSTRUCTION", DirectProfile: true, AssigneeID: "personal", DelegationReference: ref})
	require.NoError(t, err)
	packet.ID, packet.Memory = "fresh", nil
	ref.ContextRef = "fresh"
	require.NoError(t, a.ManageWorkspaceTask(ctx, shared.WorkspaceTaskCommand{TaskID: id, WorkspaceID: "ws-1", Action: "assign", AssigneeID: "personal", DirectProfile: true, DelegationReference: ref}))
	task, err := svc.GetTask(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "fresh", task.Metadata[dispatchcontext.MetadataKey])
	require.Contains(t, task.Description, "BASE_INSTRUCTION")
	require.NotContains(t, task.Description, "FORGOTTEN_RULE")
	require.NoError(t, a.taskRepo.CreateTaskSession(ctx, &models.TaskSession{ID: "busy", TaskID: id, AgentProfileID: "personal", State: models.TaskSessionStateRunning}))
	require.Error(t, a.ManageWorkspaceTask(ctx, shared.WorkspaceTaskCommand{TaskID: id, WorkspaceID: "ws-1", Action: "assign", AssigneeID: "personal", DirectProfile: true, DelegationReference: ref}))
}

func TestAssistantContextDisabledRuntimeDoesNotBypassGuard(t *testing.T) {
	guard := assistantDispatchGuard(nil, nil)
	task := &models.Task{ID: "task", Metadata: map[string]interface{}{dispatchcontext.MetadataKey: "packet"}}
	require.ErrorIs(t, guard(context.Background(), task, nil, "personal"), dispatchcontext.ErrStale)
	require.ErrorIs(t, guard(dispatchcontext.WithReference(context.Background(), "new"), task, nil, "personal"), dispatchcontext.ErrStale)
	require.NoError(t, guard(context.Background(), &models.Task{ID: "ordinary"}, nil, "personal"))
}
