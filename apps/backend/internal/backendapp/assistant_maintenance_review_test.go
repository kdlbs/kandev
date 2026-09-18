package backendapp

import (
	"context"
	"testing"
	"time"

	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/task/statussummary"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

func TestAssistantMaintenanceNativeSuccessfulRecovery(t *testing.T) {
	a, svc := newOfficeTaskAdapterHarness(t)
	a.orch = &orchestrator.Service{}
	ctx := context.Background()
	wf, err := svc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{WorkspaceID: "ws-1", Name: "Synthetic recovery"})
	require.NoError(t, err)
	require.NoError(t, a.workflow.CreateStep(ctx, &wfmodels.WorkflowStep{ID: "recovery-step", WorkflowID: wf.ID, Name: "Done"}))
	task := &models.Task{ID: "recovered", WorkspaceID: "ws-1", WorkflowID: wf.ID, WorkflowStepID: "recovery-step", Title: "Example completed workflow", State: "COMPLETED"}
	require.NoError(t, a.taskRepo.CreateTask(ctx, task))
	prepared := time.Now().UTC().Add(-time.Hour)
	finished := prepared.Add(30 * time.Minute)
	session := &models.TaskSession{ID: "native-success", TaskID: task.ID, State: models.TaskSessionStateCompleted, StartedAt: prepared.Add(time.Minute), CompletedAt: &finished}
	require.NoError(t, a.taskRepo.CreateTaskSession(ctx, &models.TaskSession{ID: "previous-failure", TaskID: task.ID, State: models.TaskSessionStateFailed, StartedAt: prepared.Add(-time.Hour)}))
	require.NoError(t, a.taskRepo.CreateTaskSession(ctx, session))
	require.NoError(t, a.taskRepo.CreateTurn(ctx, &models.Turn{ID: "native-turn", TaskID: task.ID, TaskSessionID: session.ID}))
	require.NoError(t, a.taskRepo.CreateMessage(ctx, &models.Message{ID: "native-result", TaskID: task.ID, TaskSessionID: session.ID, TurnID: "native-turn", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeMessage, Content: "The synthetic workflow completed."}))
	_, err = a.taskRepo.CompareAndUpdateTaskStatusSummary(ctx, &statussummary.StoredTaskStatusSummary{TaskID: task.ID, WorkspaceID: task.WorkspaceID, Summary: statussummary.TaskStatusSummary{Revision: 1}})
	require.NoError(t, err)
	evidence := shared.Evidence{TaskID: task.ID, SessionID: session.ID, SourceID: "native-result", SourceKind: "task_message"}
	require.NoError(t, a.ValidateMaintenanceSuccess(ctx, task.WorkspaceID, evidence, prepared), "a later verified result may recover from a historical failed attempt")
	result, err := a.FindMaintenanceSuccess(ctx, task.WorkspaceID, task.ID, "", prepared)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, evidence.SourceID, result.SourceID)
	require.Equal(t, task.Title, result.TaskTitle)
	require.Error(t, a.ValidateMaintenanceSuccess(ctx, "foreign", evidence, prepared))
	require.Error(t, a.ValidateMaintenanceSuccess(ctx, task.WorkspaceID, evidence, finished), "an old run is not subsequent evidence")
	evidence.SourceID = "missing-result"
	require.Error(t, a.ValidateMaintenanceSuccess(ctx, task.WorkspaceID, evidence, prepared))
	evidence.SourceID = "native-result"
	require.NoError(t, a.taskRepo.CreateTaskSession(ctx, &models.TaskSession{ID: "new-active-session", TaskID: task.ID, State: models.TaskSessionStateRunning, StartedAt: finished.Add(time.Minute)}))
	require.Error(t, a.ValidateMaintenanceSuccess(ctx, task.WorkspaceID, evidence, prepared), "outstanding native work prevents resolution")
}
