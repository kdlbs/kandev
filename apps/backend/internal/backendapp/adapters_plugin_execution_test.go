package backendapp

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type exactExecutionTaskFake struct {
	task            *models.Task
	session         *models.TaskSession
	installationID  string
	instanceKey     string
	claimGeneration int64
}

func (f exactExecutionTaskFake) GetTask(context.Context, string) (*models.Task, error) {
	return f.task, nil
}

func (f exactExecutionTaskFake) GetTaskSession(context.Context, string) (*models.TaskSession, error) {
	return f.session, nil
}

func (f exactExecutionTaskFake) WithTaskManagementClaimFence(_ context.Context, taskID, installationID, instanceKey string, generation int64, effect func() error) error {
	if f.task == nil || f.task.ID != taskID {
		return status.Error(codes.NotFound, "task not found")
	}
	if f.installationID != "" && (f.installationID != installationID || f.instanceKey != instanceKey || f.claimGeneration != generation) {
		return status.Error(codes.Aborted, "task management ownership changed")
	}
	if f.installationID == "" && (instanceKey != "" || generation != 0) {
		return status.Error(codes.Aborted, "task management ownership changed")
	}
	return effect()
}

type exactExecutionOrchestratorFake struct {
	recoverCalls int
	ensureCalls  int
	stopCalls    []string
}

func (f *exactExecutionOrchestratorFake) EnsureSession(_ context.Context, taskID string, _ ...orchestrator.EnsureSessionOptions) (*orchestrator.EnsureSessionResponse, error) {
	f.ensureCalls++
	return &orchestrator.EnsureSessionResponse{Success: true, TaskID: taskID, SessionID: "session-claimed", State: "RUNNING"}, nil
}

func (f *exactExecutionOrchestratorFake) RecoverSession(context.Context, string, string, string) (*orchestrator.LaunchSessionResponse, error) {
	f.recoverCalls++
	return &orchestrator.LaunchSessionResponse{Success: true, SessionID: "session-1", State: "RUNNING", AgentExecutionID: "next-execution"}, nil
}

func (f *exactExecutionOrchestratorFake) StopExecution(_ context.Context, executionID, _ string, _ bool) error {
	f.stopCalls = append(f.stopCalls, executionID)
	return nil
}

type exactExecutionLifecycleFake struct {
	execution *lifecycle.AgentExecution
}

func (f exactExecutionLifecycleFake) GetExecutionBySessionID(string) (*lifecycle.AgentExecution, bool) {
	return f.execution, f.execution != nil
}

func (exactExecutionLifecycleFake) GetModeStateForSession(string) *lifecycle.CachedModeState {
	return nil
}

func (exactExecutionLifecycleFake) SetSessionMode(context.Context, string, string, string) error {
	return nil
}

func TestExactExecutionAdapterDoesNotStopReplacementGeneration(t *testing.T) {
	updatedAt := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	torch := &exactExecutionOrchestratorFake{}
	controller := pluginsExactExecutionController{
		tasks: exactExecutionTaskFake{
			task:    &models.Task{ID: "task-1", WorkspaceID: "workspace-1"},
			session: &models.TaskSession{ID: "session-1", TaskID: "task-1", UpdatedAt: updatedAt},
		},
		orchestrator: torch,
		lifecycle:    exactExecutionLifecycleFake{execution: &lifecycle.AgentExecution{ID: "replacement-execution"}},
	}

	_, err := controller.StopTaskRun(context.Background(), "", pluginsdk.ExactTaskExecutionCommand{
		WorkspaceID: "workspace-1", TaskID: "task-1", SessionID: "session-1",
		ExpectedSessionResourceVersion: updatedAt.Format(time.RFC3339Nano), ExpectedExecutionID: "observed-execution",
	})
	if status.Code(err) != codes.Aborted {
		t.Fatalf("StopTaskRun error = %v, want Aborted", err)
	}
	if len(torch.stopCalls) != 0 {
		t.Fatalf("stopped executions = %v, want no stop", torch.stopCalls)
	}
}

func TestExactExecutionAdapterRejectsCompetingInstallationForClaimedTask(t *testing.T) {
	updatedAt := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	torch := &exactExecutionOrchestratorFake{}
	controller := pluginsExactExecutionController{
		tasks: exactExecutionTaskFake{
			task:           &models.Task{ID: "task-claimed", WorkspaceID: "workspace-1", UpdatedAt: updatedAt},
			session:        &models.TaskSession{ID: "session-claimed", TaskID: "task-claimed", UpdatedAt: updatedAt},
			installationID: "installation-a", instanceKey: "delivery-lead", claimGeneration: 7,
		},
		orchestrator: torch,
		lifecycle:    exactExecutionLifecycleFake{execution: &lifecycle.AgentExecution{ID: "exec-current"}},
	}
	command := pluginsdk.ExactTaskExecutionCommand{
		WorkspaceID: "workspace-1", TaskID: "task-claimed", SessionID: "session-claimed",
		ExpectedSessionResourceVersion: updatedAt.Format(time.RFC3339Nano), ExpectedExecutionID: "exec-current",
		ManagementInstanceKey: "delivery-lead", ExpectedClaimGeneration: 7,
	}

	run := pluginsdk.ExactTaskRunCommand{
		WorkspaceID: "workspace-1", TaskID: "task-claimed",
		ExpectedTaskResourceVersion: updatedAt.Format(time.RFC3339Nano),
		ManagementInstanceKey:       "delivery-lead", ExpectedClaimGeneration: 7,
	}
	if _, err := controller.EnsureTaskRun(context.Background(), "installation-b", run); status.Code(err) != codes.Aborted {
		t.Fatalf("competing installation ensure error = %v, want Aborted", err)
	}
	if _, err := controller.StopTaskRun(context.Background(), "installation-b", command); status.Code(err) != codes.Aborted {
		t.Fatalf("competing installation stop error = %v, want Aborted", err)
	}
	recovery := pluginsdk.ExactSessionRecoveryCommand{ExactTaskExecutionCommand: command, Action: "resume"}
	if _, err := controller.RecoverSession(context.Background(), "installation-b", recovery); status.Code(err) != codes.Aborted {
		t.Fatalf("competing installation recovery error = %v, want Aborted", err)
	}
	cancel := pluginsdk.ExactPendingTransitionCommand{
		WorkspaceID: "workspace-1", TaskID: "task-claimed", TransitionID: "transition-1",
		ExpectedResourceVersion: "transition-version", ManagementInstanceKey: "delivery-lead", ExpectedClaimGeneration: 7,
	}
	if _, err := controller.CancelPendingTaskTransition(context.Background(), "installation-b", cancel); status.Code(err) != codes.Aborted {
		t.Fatalf("competing installation pending-transition cancellation error = %v, want Aborted", err)
	}
	modeRead := command
	modeRead.ExpectedExecutionID = ""
	if _, err := controller.GetSessionModeContext(context.Background(), "installation-b", modeRead); status.Code(err) != codes.Aborted {
		t.Fatalf("competing installation mode read error = %v, want Aborted", err)
	}
	modeSet := pluginsdk.ExactSessionModeCommand{ExactTaskExecutionCommand: command, ModeID: "plan"}
	if _, err := controller.SetSessionMode(context.Background(), "installation-b", modeSet); status.Code(err) != codes.Aborted {
		t.Fatalf("competing installation mode change error = %v, want Aborted", err)
	}
	if torch.ensureCalls != 0 || torch.recoverCalls != 0 || len(torch.stopCalls) != 0 {
		t.Fatalf("competing installation reached an execution effect: ensures=%d recoveries=%d stops=%v", torch.ensureCalls, torch.recoverCalls, torch.stopCalls)
	}
	if len(torch.stopCalls) != 0 {
		t.Fatalf("competing installation stopped executions = %v, want none", torch.stopCalls)
	}

	if runResult, err := controller.EnsureTaskRun(context.Background(), "installation-a", run); err != nil || runResult.SessionID != "session-claimed" {
		t.Fatalf("current owner ensure = %+v err=%v, want successful run", runResult, err)
	}
	if stopped, err := controller.StopTaskRun(context.Background(), "installation-a", command); err != nil || !stopped {
		t.Fatalf("current owner stop = stopped:%v err:%v, want successful stop", stopped, err)
	}
	if torch.ensureCalls != 1 || len(torch.stopCalls) != 1 || torch.stopCalls[0] != "exec-current" {
		t.Fatalf("current owner calls = ensure:%d stop:%v, want one ensure and only exec-current stop", torch.ensureCalls, torch.stopCalls)
	}
}

func TestExactExecutionAdapterDoesNotRecoverManuallyCancelledSession(t *testing.T) {
	updatedAt := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	torch := &exactExecutionOrchestratorFake{}
	controller := pluginsExactExecutionController{
		tasks: exactExecutionTaskFake{
			task: &models.Task{ID: "task-1", WorkspaceID: "workspace-1"},
			session: &models.TaskSession{
				ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateCancelled,
				AgentExecutionID: "observed-execution", UpdatedAt: updatedAt,
			},
		},
		orchestrator: torch,
		lifecycle:    exactExecutionLifecycleFake{},
	}

	_, err := controller.RecoverSession(context.Background(), "", pluginsdk.ExactSessionRecoveryCommand{
		ExactTaskExecutionCommand: pluginsdk.ExactTaskExecutionCommand{
			WorkspaceID: "workspace-1", TaskID: "task-1", SessionID: "session-1",
			ExpectedSessionResourceVersion: updatedAt.Format(time.RFC3339Nano), ExpectedExecutionID: "observed-execution",
		},
		Action: "resume",
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("RecoverSession error = %v, want FailedPrecondition", err)
	}
	if torch.recoverCalls != 0 {
		t.Fatalf("orchestrator recover calls = %d, want zero", torch.recoverCalls)
	}
}
