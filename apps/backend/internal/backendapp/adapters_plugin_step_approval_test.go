package backendapp

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type stepApprovalServiceFake struct {
	status   *taskservice.StepApprovalStatus
	err      error
	approved int
}

func (f *stepApprovalServiceFake) GetStepApprovalStatus(context.Context, string) (*taskservice.StepApprovalStatus, error) {
	return f.status, f.err
}

func (f *stepApprovalServiceFake) ApproveAwaitingStep(_ context.Context, _ string, guard func(*taskservice.StepApprovalStatus) error) (*taskservice.StepApprovalStatus, *taskservice.ApproveSessionResult, error) {
	if f.err != nil {
		return nil, nil, f.err
	}
	if err := guard(f.status); err != nil {
		return f.status, nil, err
	}
	if !f.status.Awaiting {
		return f.status, nil, taskservice.ErrStepNotAwaitingApproval
	}
	f.approved++
	return f.status, &taskservice.ApproveSessionResult{Task: &models.Task{ID: "task-1", WorkflowStepID: f.status.NextStep.ID}}, nil
}

func awaitingStepStatus() *taskservice.StepApprovalStatus {
	return &taskservice.StepApprovalStatus{
		Session:  &models.TaskSession{ID: "session-1", TaskID: "task-1", UpdatedAt: time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)},
		Task:     &models.Task{ID: "task-1", WorkspaceID: "workspace-1", WorkflowStepID: "review"},
		Step:     &wfmodels.WorkflowStep{ID: "review", Name: "Review"},
		NextStep: &wfmodels.WorkflowStep{ID: "done", Name: "Done"},
		Awaiting: true,
	}
}

func TestExactStepApprovalAdapterBindsTheObservedVersion(t *testing.T) {
	steps := &stepApprovalServiceFake{status: awaitingStepStatus()}
	controller := pluginsExactExecutionController{steps: steps}
	ctx := context.Background()

	approval, err := controller.GetStepApproval(ctx, "", pluginsdk.ExactStepApprovalQuery{WorkspaceID: "workspace-1", SessionID: "session-1"})
	if err != nil || !approval.Awaiting || approval.FromStepName != "Review" || approval.ToStepID != "done" || approval.ResourceVersion == "" {
		t.Fatalf("GetStepApproval = %+v err=%v", approval, err)
	}
	if _, err := controller.GetStepApproval(ctx, "", pluginsdk.ExactStepApprovalQuery{WorkspaceID: "workspace-other", SessionID: "session-1"}); status.Code(err) != codes.NotFound {
		t.Fatalf("cross-workspace read err = %v, want NotFound", err)
	}

	command := pluginsdk.ExactStepApprovalCommand{WorkspaceID: "workspace-1", SessionID: "session-1", ExpectedResourceVersion: "sha256:stale"}
	if _, err := controller.ApproveStep(ctx, "", command); status.Code(err) != codes.Aborted || steps.approved != 0 {
		t.Fatalf("stale approval err = %v approved=%d, want Aborted without approval", err, steps.approved)
	}
	command.ExpectedResourceVersion = approval.ResourceVersion
	result, err := controller.ApproveStep(ctx, "", command)
	if err != nil || result != (pluginsdk.StepApprovalResult{FromStepID: "review", ToStepID: "done"}) || steps.approved != 1 {
		t.Fatalf("approval = %+v err=%v approved=%d", result, err, steps.approved)
	}

	steps.status.Awaiting = false
	command.ExpectedResourceVersion = stepApprovalResourceVersion(steps.status)
	if _, err := controller.ApproveStep(ctx, "", command); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("approval with nothing awaiting err = %v, want FailedPrecondition", err)
	}
	steps.err = taskservice.ErrOfficeApprovalStage
	if _, err := controller.GetStepApproval(ctx, "", pluginsdk.ExactStepApprovalQuery{WorkspaceID: "workspace-1", SessionID: "session-1"}); status.Code(err) != codes.Unimplemented {
		t.Fatalf("office stage err = %v, want Unimplemented", err)
	}
}
