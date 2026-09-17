package backendapp

import (
	"context"
	"fmt"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/office/shared"
	orchestrationmodels "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/task/statussummary"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func TestWorkspaceTaskDetailsPreservesMultipleSessionResults(t *testing.T) {
	adapter, _ := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()
	task := &models.Task{ID: "two-sessions", WorkspaceID: "ws-1", Title: "Implementation and review", State: "REVIEW"}
	require.NoError(t, adapter.taskRepo.CreateTask(ctx, task))
	for i, id := range []string{"implementer", "reviewer"} {
		require.NoError(t, adapter.taskRepo.CreateTaskSession(ctx, &models.TaskSession{ID: id, TaskID: task.ID, AgentProfileID: id, State: models.TaskSessionStateWaitingForInput, UpdatedAt: time.Now().Add(time.Duration(i) * time.Second)}))
		require.NoError(t, adapter.taskRepo.CreateTurn(ctx, &models.Turn{ID: id + "-turn", TaskID: task.ID, TaskSessionID: id}))
		require.NoError(t, adapter.taskRepo.CreateMessage(ctx, &models.Message{ID: id + "-result", TaskID: task.ID, TaskSessionID: id, TurnID: id + "-turn", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeMessage, Content: id + " evidence"}))
	}
	result, err := adapter.WorkspaceTaskDetails(ctx, "ws-1", task.ID)
	require.NoError(t, err)
	data := result.(map[string]any)
	require.NotNil(t, data["session_results"])
	rows := data["session_results"].([]map[string]any)
	require.Len(t, rows, 2)
	require.Equal(t, "implementer", rows[0]["profile_id"])
	require.Equal(t, "reviewer", rows[1]["profile_id"])
}

func TestAssistantRoutingAdapterRejectsNonDeliveryMode(t *testing.T) {
	adapter, svc := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()
	wf, err := svc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{WorkspaceID: "ws-1", Name: "Delivery"})
	require.NoError(t, err)
	for _, mode := range []string{"inspect", "answer", "unsupported"} {
		_, err = adapter.CreateWorkspaceTask(ctx, shared.WorkspaceTaskSpec{WorkspaceID: "ws-1", WorkflowID: wf.ID, Title: "Do not create", ExecutionMode: mode})
		require.Error(t, err, "mode %s must not create a delivery task", mode)
	}
}

func TestAssistantRoutingUsesExplicitPermittedStepAndRetainsDefaults(t *testing.T) {
	adapter, svc := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()
	wf, err := svc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{WorkspaceID: "ws-1", Name: "Delivery"})
	require.NoError(t, err)
	for _, step := range []*wfmodels.WorkflowStep{
		{ID: "requirements", WorkflowID: wf.ID, Name: "Requirements", IsStartStep: true},
		{ID: "execute", WorkflowID: wf.ID, Name: "Implementación", Position: 1, AllowManualMove: true},
		{ID: "locked-review", WorkflowID: wf.ID, Name: "Review", Position: 2},
	} {
		require.NoError(t, adapter.workflow.CreateStep(ctx, step))
	}
	spec := shared.WorkspaceTaskSpec{WorkspaceID: "ws-1", WorkflowID: wf.ID, WorkflowStepID: "execute", Title: "Known change", ExecutionMode: "execute", DelegationReference: orchestrationmodels.DelegationReference{ObjectiveID: "goal", ContextRef: "packet", SourceCommentID: "source", AcceptanceRevision: 1, DispatchOperationID: "op"}}
	id, err := adapter.CreateWorkspaceTask(ctx, spec)
	require.NoError(t, err)
	task, err := svc.GetTask(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "execute", task.WorkflowStepID)
	require.Equal(t, "goal", task.Metadata["orchestration_objective_id"])
	for _, step := range []string{"locked-review", "missing"} {
		spec.WorkflowStepID = step
		_, err = adapter.CreateWorkspaceTask(ctx, spec)
		require.Error(t, err)
	}
	start, err := adapter.workflow.GetStep(ctx, "requirements")
	require.NoError(t, err)
	require.True(t, start.IsStartStep)
}

func TestAssistantCompletionDetailsExposeUnfinishedSessions(t *testing.T) {
	adapter, _ := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()
	task := &models.Task{ID: "completion-check", WorkspaceID: "ws-1", Title: "Review is not completion", State: "REVIEW"}
	require.NoError(t, adapter.taskRepo.CreateTask(ctx, task))
	require.NoError(t, adapter.taskRepo.CreateTaskSession(ctx, &models.TaskSession{ID: "unfinished", TaskID: task.ID, State: models.TaskSessionStateRunning}))
	result, err := adapter.WorkspaceTaskDetails(ctx, "ws-1", task.ID)
	require.NoError(t, err)
	data := result.(map[string]any)
	require.Equal(t, false, data["completion_ready"])
	require.Contains(t, data["completion_blocker"], "active")
}

func TestAssistantCompletionSettledTaskNeedsEvidenceButIsNotBusy(t *testing.T) {
	adapter, svc := newOfficeTaskAdapterHarness(t)
	adapter.orch = &orchestrator.Service{}
	ctx := context.Background()
	wf, err := svc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{WorkspaceID: "ws-1", Name: "Delivery"})
	require.NoError(t, err)
	require.NoError(t, adapter.workflow.CreateStep(ctx, &wfmodels.WorkflowStep{ID: "review-step", WorkflowID: wf.ID, Name: "Review"}))
	task := &models.Task{ID: "settled", WorkspaceID: "ws-1", WorkflowID: wf.ID, WorkflowStepID: "review-step", Title: "Finished result", State: "REVIEW"}
	require.NoError(t, adapter.taskRepo.CreateTask(ctx, task))
	require.NoError(t, adapter.taskRepo.CreateTaskSession(ctx, &models.TaskSession{ID: "settled-session", TaskID: task.ID, State: models.TaskSessionStateCompleted}))
	_, err = adapter.taskRepo.CompareAndUpdateTaskStatusSummary(ctx, &statussummary.StoredTaskStatusSummary{TaskID: task.ID, WorkspaceID: task.WorkspaceID, Summary: statussummary.TaskStatusSummary{Revision: 1}})
	require.NoError(t, err)
	result, err := adapter.WorkspaceTaskDetails(ctx, "ws-1", task.ID)
	require.NoError(t, err)
	data := result.(map[string]any)
	require.Equal(t, true, data["completion_ready"], data["completion_blocker"])
	participant, err := adapter.workflow.UpsertTaskParticipant(ctx, "review-step", task.ID, "reviewer", "review-profile")
	require.NoError(t, err)
	require.ErrorContains(t, adapter.ValidateAssistantTaskCompletion(ctx, "ws-1", task.ID), "approval is pending")
	require.NoError(t, adapter.workflow.RecordStepDecision(ctx, &wfmodels.WorkflowStepDecision{TaskID: task.ID, StepID: "review-step", ParticipantID: participant, Decision: "approved"}))
	require.NoError(t, adapter.ValidateAssistantTaskCompletion(ctx, "ws-1", task.ID))
	require.NoError(t, adapter.taskRepo.CreateTaskReviewRun(ctx, &models.TaskReviewRun{ID: "failed-review", TaskID: task.ID, Status: models.ReviewRunFailed}))
	require.ErrorContains(t, adapter.ValidateAssistantTaskCompletion(ctx, "ws-1", task.ID), "review failed")
}

type workspaceTestProfiles map[string]*settingsmodels.AgentProfile

func (p workspaceTestProfiles) GetAgentProfile(_ context.Context, id string) (*settingsmodels.AgentProfile, error) {
	profile := p[id]
	if profile == nil {
		return nil, fmt.Errorf("missing profile")
	}
	return profile, nil
}

func TestWorkspaceDelegationValidatesResourcesAndRetainsAccount(t *testing.T) {
	adapter, svc := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()
	ws, err := svc.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{Name: "Board"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := svc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{WorkspaceID: ws.ID, Name: "First"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{WorkspaceID: ws.ID, Name: "Second"})
	if err != nil {
		t.Fatal(err)
	}
	adapter.profiles = workspaceTestProfiles{
		"worker":          {ID: "worker", WorkspaceID: ws.ID, Role: "worker", Settings: `{"routing":{"execution_profile_id":"work"}}`},
		"work":            {ID: "work", Enabled: true, Model: "default"},
		"personal-worker": {ID: "personal-worker", WorkspaceID: ws.ID, Role: "worker", Settings: `{"routing":{"execution_profile_id":"personal"}}`},
		"personal":        {ID: "personal", Enabled: true, Model: "default"},
	}
	spec := shared.WorkspaceTaskSpec{WorkspaceID: ws.ID, ChiefID: "chief", AssigneeID: "worker", Title: "Review", ExternalID: "review-1"}
	if _, err := adapter.CreateWorkspaceTask(ctx, spec); err == nil {
		t.Fatal("ambiguous workflow accepted")
	}
	spec.WorkflowID = "foreign"
	if _, err := adapter.CreateWorkspaceTask(ctx, spec); err == nil {
		t.Fatal("foreign workflow accepted")
	}
	spec.WorkflowID = first.ID
	id, err := adapter.CreateWorkspaceTask(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	again, err := adapter.CreateWorkspaceTask(ctx, spec)
	if err != nil || again != id {
		t.Fatalf("retry duplicated task: %s %v", again, err)
	}
	task, err := svc.GetTask(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if task.IsFromOffice || task.Metadata[models.MetaKeyAgentProfileID] != "work" || task.Metadata["orchestration_chief_id"] != "chief" {
		t.Fatalf("incorrect execution ownership: %+v", task)
	}
	command := shared.WorkspaceTaskCommand{WorkspaceID: "foreign", ChiefID: "chief", TaskID: id, Action: "adopt"}
	if err := adapter.ManageWorkspaceTask(ctx, command); err == nil {
		t.Fatal("foreign task adoption accepted")
	}
	command.WorkspaceID = ws.ID
	command.Action = "assign"
	command.AssigneeID = "personal-worker"
	if err := adapter.ManageWorkspaceTask(ctx, command); err == nil {
		t.Fatal("reassignment changed account")
	}
}

func TestChiefCanObserveExistingRunningTaskWithoutStoppingIt(t *testing.T) {
	adapter, svc := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()
	task := &models.Task{ID: "running-board-task", WorkspaceID: "ws-1", Title: "Existing work", State: "IN_PROGRESS", Metadata: map[string]interface{}{"keep": "value"}}
	if err := adapter.taskRepo.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	session := &models.TaskSession{ID: "running-session", TaskID: task.ID, AgentProfileID: "work", State: models.TaskSessionStateRunning}
	if err := adapter.taskRepo.CreateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	if err := adapter.ManageWorkspaceTask(ctx, shared.WorkspaceTaskCommand{WorkspaceID: "ws-1", ChiefID: "chief", TaskID: task.ID, Action: "adopt"}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Metadata["keep"] != "value" || got.Metadata["orchestration_chief_id"] != "chief" || got.State != "IN_PROGRESS" {
		t.Fatalf("adoption changed running task: %+v", got)
	}
}

func TestWorkspaceTaskDetailsReadsBoundedWorkerMessages(t *testing.T) {
	adapter, _ := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()
	task := &models.Task{ID: "results-task", WorkspaceID: "ws-1", Title: "Review", State: "REVIEW"}
	if err := adapter.taskRepo.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	session := &models.TaskSession{ID: "results-session", TaskID: task.ID, State: models.TaskSessionStateWaitingForInput}
	if err := adapter.taskRepo.CreateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	if err := adapter.taskRepo.CreateTurn(ctx, &models.Turn{ID: "result-turn", TaskID: task.ID, TaskSessionID: session.ID}); err != nil {
		t.Fatal(err)
	}
	message := &models.Message{TurnID: "result-turn", ID: "result", TaskID: task.ID, TaskSessionID: session.ID, AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeMessage, Content: strings.Repeat("x", 4500)}
	if err := adapter.taskRepo.CreateMessage(ctx, message); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.WorkspaceTaskDetails(ctx, "foreign", task.ID); err == nil {
		t.Fatal("foreign worker transcript exposed")
	}
	result, err := adapter.WorkspaceTaskDetails(ctx, "ws-1", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	rows := result.(map[string]any)["messages"].([]map[string]any)
	if len(rows) != 1 || len(rows[0]["content"].(string)) != 4000 || rows[0]["truncated"] != true {
		t.Fatalf("missing worker result: %+v", rows)
	}
}

func TestWorkspaceMessageRejectsForeignSessionAndAccount(t *testing.T) {
	adapter, _ := newOfficeTaskAdapterHarness(t)
	adapter.orch = &orchestrator.Service{}
	ctx := context.Background()
	task := &models.Task{ID: "message-task", WorkspaceID: "ws-1", Title: "Review", Metadata: map[string]interface{}{"orchestration_execution_profile_id": "work"}}
	if err := adapter.taskRepo.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	session := &models.TaskSession{ID: "message-session", TaskID: task.ID, AgentProfileID: "personal", State: models.TaskSessionStateWaitingForInput}
	if err := adapter.taskRepo.CreateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"missing-session", session.ID} {
		err := adapter.messageWorkspaceTask(ctx, task, shared.WorkspaceTaskCommand{SessionID: id, Prompt: "Continue"})
		if err == nil {
			t.Fatalf("unsafe session accepted: %s", id)
		}
	}
}

func TestOrchestratorUsesTaskProfilesWithoutProviderPin(t *testing.T) {
	adapter, svc := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()
	ws, err := svc.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{Name: "Profiles"})
	if err != nil {
		t.Fatal(err)
	}
	wf, err := svc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{WorkspaceID: ws.ID, Name: "Delivery"})
	if err != nil {
		t.Fatal(err)
	}
	adapter.profiles = workspaceTestProfiles{"claude": {ID: "claude", Enabled: true}, "codex": {ID: "codex", Enabled: true}, "foreign": {ID: "foreign", Enabled: true, WorkspaceID: "other"}}
	spec := shared.WorkspaceTaskSpec{WorkspaceID: ws.ID, WorkflowID: wf.ID, ChiefID: "coordinator", DirectProfile: true, AssigneeID: "claude", Title: "Delegated task"}
	id, err := adapter.CreateWorkspaceTask(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	task, err := svc.GetTask(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if task.Metadata["orchestration_execution_profile_id"] != nil {
		t.Fatal("orchestration froze execution profile")
	}
	err = adapter.ManageWorkspaceTask(ctx, shared.WorkspaceTaskCommand{WorkspaceID: ws.ID, TaskID: id, ChiefID: "coordinator", DirectProfile: true, Action: "assign", AssigneeID: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	task, err = svc.GetTask(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if task.Metadata[models.MetaKeyAgentProfileID] != "codex" {
		t.Fatalf("explicit assignment ignored: %+v", task.Metadata)
	}
	spec.AssigneeID = "foreign"
	if _, err = adapter.CreateWorkspaceTask(ctx, spec); err == nil {
		t.Fatal("foreign profile accepted")
	}
}
