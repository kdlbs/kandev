package backendapp

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func cancelTurnController(torch *exactExecutionOrchestratorFake) (pluginsExactExecutionController, pluginsdk.ExactTaskExecutionCommand) {
	updatedAt := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	controller := pluginsExactExecutionController{
		tasks: exactExecutionTaskFake{
			task:           &models.Task{ID: "task-1", WorkspaceID: "workspace-1"},
			session:        &models.TaskSession{ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateRunning, UpdatedAt: updatedAt},
			installationID: "installation-a", instanceKey: "delivery-lead", claimGeneration: 3,
		},
		orchestrator: torch,
		lifecycle:    exactExecutionLifecycleFake{execution: &lifecycle.AgentExecution{ID: "exec-current"}},
	}
	return controller, pluginsdk.ExactTaskExecutionCommand{
		WorkspaceID: "workspace-1", TaskID: "task-1", SessionID: "session-1",
		ExpectedSessionResourceVersion: updatedAt.Format(time.RFC3339Nano), ExpectedExecutionID: "exec-current",
		ManagementInstanceKey: "delivery-lead", ExpectedClaimGeneration: 3,
	}
}

// A turn that ended and was replaced after the observation is reported as a
// conflict; the new work is not interrupted and no success is recorded.
func TestExactExecutionAdapterCancelTurnAbortsWhenTheObservedTurnWasReplaced(t *testing.T) {
	torch := &exactExecutionOrchestratorFake{cancelErr: orchestrator.ErrCancelTargetChanged}
	controller, command := cancelTurnController(torch)

	cancelled, err := controller.CancelTurn(context.Background(), "installation-a", command)

	if status.Code(err) != codes.Aborted || cancelled {
		t.Fatalf("cancel = %v err=%v, want Aborted without a recorded change", cancelled, err)
	}
}

func TestExactExecutionAdapterCancelTurnAbortsWhenTheLiveExecutionWasReplaced(t *testing.T) {
	torch := &exactExecutionOrchestratorFake{observedExecutionID: "replacement"}
	controller, command := cancelTurnController(torch)

	cancelled, err := controller.CancelTurn(context.Background(), "installation-a", command)

	if status.Code(err) != codes.Aborted || cancelled || len(torch.cancelCalls) != 0 {
		t.Fatalf("cancel = %v err=%v calls=%v, want Aborted before any cancel", cancelled, err, torch.cancelCalls)
	}
}
