package backendapp

import (
	"context"
	"errors"
	"testing"
	"time"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
)

type fixedTaskTransferTaskReader struct {
	task *models.Task
	err  error
}

func (r fixedTaskTransferTaskReader) GetTask(context.Context, string) (*models.Task, error) {
	return r.task, r.err
}

type fixedTaskTransferAgentReader struct {
	agent *settingsmodels.AgentProfile
	err   error
}

type fixedTaskTransferSessionReader struct {
	session *models.TaskSession
	err     error
}

func (r fixedTaskTransferSessionReader) GetTaskSession(context.Context, string) (*models.TaskSession, error) {
	return r.session, r.err
}

func (r fixedTaskTransferAgentReader) GetAgentInstance(
	context.Context,
	string,
) (*settingsmodels.AgentProfile, error) {
	return r.agent, r.err
}

func TestTaskTransferCoordinatorAttestorRequiresAssignedSourceCEO(t *testing.T) {
	principal := mcpscope.Principal{
		WorkspaceID: "ws-source", CallerTaskID: "caller-task", CallerSessionID: "caller-session",
	}
	validTask := &models.Task{
		ID: "caller-task", WorkspaceID: "ws-source", IsFromOffice: true, AssigneeAgentProfileID: "ceo-1",
	}
	validSession := &models.TaskSession{
		ID: "caller-session", TaskID: "caller-task", AgentProfileID: "ceo-1", State: models.TaskSessionStateRunning,
	}
	validAgent := &settingsmodels.AgentProfile{
		ID: "ceo-1", WorkspaceID: "ws-source", Role: settingsmodels.AgentRoleCEO,
	}
	tests := []struct {
		name    string
		task    *models.Task
		session *models.TaskSession
		agent   *settingsmodels.AgentProfile
		err     error
		want    bool
	}{
		{name: "assigned CEO", task: validTask, session: validSession, agent: validAgent, want: true},
		{name: "task lookup failure", task: validTask, session: validSession, agent: validAgent, err: errors.New("unavailable")},
		{name: "missing session", task: validTask, agent: validAgent},
		{name: "completed session", task: validTask, session: &models.TaskSession{
			ID: "caller-session", TaskID: "caller-task", AgentProfileID: "ceo-1", State: models.TaskSessionStateCompleted,
		}, agent: validAgent},
		{name: "different assigned agent", task: &models.Task{
			ID: "caller-task", WorkspaceID: "ws-source", IsFromOffice: true, AssigneeAgentProfileID: "ceo-other",
		}, session: validSession, agent: validAgent},
		{name: "worker", task: validTask, agent: &settingsmodels.AgentProfile{
			ID: "ceo-1", WorkspaceID: "ws-source", Role: settingsmodels.AgentRoleWorker,
		}},
		{name: "other workspace", task: validTask, agent: &settingsmodels.AgentProfile{
			ID: "ceo-1", WorkspaceID: "ws-other", Role: settingsmodels.AgentRoleCEO,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attestor := taskTransferCoordinatorAttestor{
				tasks:    fixedTaskTransferTaskReader{task: tt.task, err: tt.err},
				sessions: fixedTaskTransferSessionReader{session: tt.session},
				agents:   fixedTaskTransferAgentReader{agent: tt.agent},
			}
			actor, ok := attestor.AttestTaskTransferCoordinator(context.Background(), principal)
			if ok != tt.want {
				t.Fatalf("attested = %v, actor=%+v", ok, actor)
			}
			if tt.want && (actor.ID != "ceo-1" || actor.SessionID != "caller-session" || actor.CallerTaskID != "caller-task" ||
				actor.Kind != models.TaskTransferActorCoordinator) {
				t.Fatalf("actor = %+v", actor)
			}
		})
	}
}

func TestTaskTransferCoordinatorAttestorRejectsDestinationBoundTask(t *testing.T) {
	principal := mcpscope.Principal{WorkspaceID: "ws-destination", CallerTaskID: "caller-task", CallerSessionID: "caller-session"}
	attestor := taskTransferCoordinatorAttestor{
		tasks: fixedTaskTransferTaskReader{task: &models.Task{ID: "caller-task", WorkspaceID: "ws-destination"}},
		sessions: fixedTaskTransferSessionReader{session: &models.TaskSession{
			ID: "caller-session", TaskID: "caller-task", AgentProfileID: "ceo-1",
		}},
		agents: fixedTaskTransferAgentReader{agent: &settingsmodels.AgentProfile{
			ID: "ceo-1", WorkspaceID: "ws-source", Role: settingsmodels.AgentRoleCEO,
		}},
	}
	if _, ok := attestor.AttestTaskTransferCoordinator(context.Background(), principal); ok {
		t.Fatal("destination-bound task was attested without a persisted replay")
	}
}

func TestOfficeCEOTransfersAnotherTaskAndDestinationReplay(t *testing.T) {
	ctx := context.Background()
	taskSvc, taskRepo, officeRepo := newRunSubscriptionCheckHarness(t)
	for _, workspace := range []*models.Workspace{
		{ID: "ceo-transfer-source", Name: "Source", OwnerID: "owner-1", OfficeWorkflowID: "ceo-wf-source"},
		{ID: "ceo-transfer-destination", Name: "Destination", OwnerID: "owner-1"},
	} {
		if err := taskRepo.CreateWorkspace(ctx, workspace); err != nil {
			t.Fatalf("CreateWorkspace: %v", err)
		}
	}
	for _, workflow := range []*models.Workflow{
		{ID: "ceo-wf-source", WorkspaceID: "ceo-transfer-source", Name: "Source"},
		{ID: "ceo-wf-destination", WorkspaceID: "ceo-transfer-destination", Name: "Destination"},
	} {
		if err := taskRepo.CreateWorkflow(ctx, workflow); err != nil {
			t.Fatalf("CreateWorkflow: %v", err)
		}
	}
	for _, step := range []struct{ id, workflow string }{
		{"ceo-step-source", "ceo-wf-source"}, {"ceo-step-destination", "ceo-wf-destination"},
	} {
		if _, err := officeRepo.ExecRaw(ctx,
			`INSERT INTO workflow_steps (id, workflow_id, name, position) VALUES (?, ?, 'Work', 0)`,
			step.id, step.workflow); err != nil {
			t.Fatalf("insert workflow step: %v", err)
		}
	}
	now := time.Now().UTC()
	for _, profile := range []struct{ id, workspace string }{
		{"ceo-profile-source", "ceo-transfer-source"},
		{"ceo-profile-destination", "ceo-transfer-destination"},
	} {
		if _, err := officeRepo.ExecRaw(ctx,
			`INSERT INTO agents (id, name, workspace_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			"agent-"+profile.id, profile.id, profile.workspace, now, now); err != nil {
			t.Fatalf("insert agent: %v", err)
		}
		if _, err := officeRepo.ExecRaw(ctx, `INSERT INTO agent_profiles
			(id, agent_id, name, agent_display_name, workspace_id, role, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, 'ceo', ?, ?)`, profile.id, "agent-"+profile.id,
			profile.id, profile.id, profile.workspace, now, now); err != nil {
			t.Fatalf("insert agent profile: %v", err)
		}
	}
	callerTask := &models.Task{
		ID: "ceo-caller-task", WorkspaceID: "ceo-transfer-source", WorkflowID: "ceo-wf-source",
		WorkflowStepID: "ceo-step-source", Title: "Synthetic Office coordinator",
		AssigneeAgentProfileID: "ceo-profile-source",
	}
	if err := taskRepo.CreateTask(ctx, callerTask); err != nil {
		t.Fatalf("CreateTask caller: %v", err)
	}
	targetTask := &models.Task{
		ID: "ceo-transfer-target", WorkspaceID: "ceo-transfer-source", WorkflowID: "ceo-wf-source",
		WorkflowStepID: "ceo-step-source", Title: "Synthetic transfer target",
	}
	if err := taskRepo.CreateTask(ctx, targetTask); err != nil {
		t.Fatalf("CreateTask target: %v", err)
	}
	if err := taskRepo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "ceo-transfer-session", TaskID: callerTask.ID, AgentProfileID: "ceo-profile-source",
		State: models.TaskSessionStateRunning, IsPrimary: true,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	callerTask, err := taskRepo.GetTask(ctx, callerTask.ID)
	if err != nil || !callerTask.IsOfficeOwnedAndAssigned() {
		t.Fatalf("Office coordinator task = %+v, err=%v", callerTask, err)
	}
	targetTask, err = taskRepo.GetTask(ctx, targetTask.ID)
	if err != nil {
		t.Fatalf("GetTask target: %v", err)
	}
	principal := mcpscope.Principal{
		WorkspaceID: "ceo-transfer-source", CallerTaskID: callerTask.ID, CallerSessionID: "ceo-transfer-session",
	}
	attestor := taskTransferCoordinatorAttestor{tasks: taskRepo, sessions: taskRepo, agents: officeRepo}
	actor, ok := attestor.AttestTaskTransferCoordinator(ctx, principal)
	if !ok {
		t.Fatal("source CEO was not attested")
	}
	command := models.TaskTransferCommand{
		TaskID: targetTask.ID, ExpectedSourceWorkspaceID: targetTask.WorkspaceID,
		ExpectedSourceWorkflowID: targetTask.WorkflowID, ExpectedSourceStepID: targetTask.WorkflowStepID,
		ExpectedTaskUpdatedAt: targetTask.UpdatedAt, DestinationWorkspaceID: "ceo-transfer-destination",
		DestinationWorkflowID: "ceo-wf-destination", DestinationStepID: "ceo-step-destination",
		IdempotencyKey: "ceo-delegated-transfer", PreservationPolicy: models.TaskTransferPreservationPolicyV1,
		Actor: actor,
	}
	receipt, err := taskSvc.TransferTask(ctx, command)
	if err != nil {
		t.Fatalf("TransferTask: %v", err)
	}
	replay, err := taskSvc.TransferTask(ctx, command)
	if err != nil || replay.OperationID != receipt.OperationID || !replay.IdempotentReplay {
		t.Fatalf("destination replay = %+v, err=%v", replay, err)
	}
	transferred, err := taskRepo.GetTask(ctx, targetTask.ID)
	if err != nil || transferred.WorkspaceID != "ceo-transfer-destination" {
		t.Fatalf("transferred task = %+v, err=%v", transferred, err)
	}
	callerTask, err = taskRepo.GetTask(ctx, callerTask.ID)
	if err != nil || callerTask.WorkspaceID != "ceo-transfer-source" ||
		callerTask.AssigneeAgentProfileID != "ceo-profile-source" {
		t.Fatalf("coordinator task changed = %+v, err=%v", callerTask, err)
	}
	session, err := taskRepo.GetTaskSession(ctx, "ceo-transfer-session")
	if err != nil || session.AgentProfileID != "ceo-profile-source" || session.State != models.TaskSessionStateRunning {
		t.Fatalf("preserved session = %+v, err=%v", session, err)
	}
	selfCommand := models.TaskTransferCommand{
		TaskID: callerTask.ID, ExpectedSourceWorkspaceID: callerTask.WorkspaceID,
		ExpectedSourceWorkflowID: callerTask.WorkflowID, ExpectedSourceStepID: callerTask.WorkflowStepID,
		ExpectedTaskUpdatedAt: callerTask.UpdatedAt, DestinationWorkspaceID: "ceo-transfer-destination",
		DestinationWorkflowID: "ceo-wf-destination", DestinationStepID: "ceo-step-destination",
		IdempotencyKey: "ceo-self-transfer", PreservationPolicy: models.TaskTransferPreservationPolicyV1,
		Actor: actor,
	}
	selfReceipt, err := taskSvc.TransferTask(ctx, selfCommand)
	if err != nil {
		t.Fatalf("TransferTask self: %v", err)
	}
	destinationPrincipal := mcpscope.Principal{
		WorkspaceID: "ceo-transfer-destination", CallerTaskID: callerTask.ID, CallerSessionID: "ceo-transfer-session",
	}
	replayActor, ok := attestor.AttestTaskTransferCoordinatorReplay(ctx, destinationPrincipal, selfCommand, actor)
	if !ok {
		t.Fatal("committed self-transfer into a non-Office workflow was not replay-attested")
	}
	selfCommand.Actor = replayActor
	selfReplay, err := taskSvc.TransferTask(ctx, selfCommand)
	if err != nil || selfReplay.OperationID != selfReceipt.OperationID || !selfReplay.IdempotentReplay {
		t.Fatalf("self-transfer replay = %+v, err=%v", selfReplay, err)
	}
	if _, err := officeRepo.ExecRaw(ctx, `UPDATE task_sessions SET state = ? WHERE id = ?`,
		models.TaskSessionStateCompleted, "ceo-transfer-session"); err != nil {
		t.Fatalf("revoke coordinator session: %v", err)
	}
	if _, ok := attestor.AttestTaskTransferCoordinatorReplay(ctx, destinationPrincipal, selfCommand, actor); ok {
		t.Fatal("completed coordinator session re-attested a self-transfer replay")
	}
}
