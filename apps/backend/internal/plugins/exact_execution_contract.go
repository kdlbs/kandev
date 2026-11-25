package plugins

import (
	"context"

	"github.com/kdlbs/kandev/pluginsdk"
)

// exactExecutionController is satisfied by backendapp adapters over the native
// task, orchestrator, queue, and lifecycle services.
type ExactExecutionController interface {
	EnsureTaskRun(context.Context, string, pluginsdk.ExactTaskRunCommand) (pluginsdk.ExactTaskRunResult, error)
	StopTaskRun(context.Context, string, pluginsdk.ExactTaskExecutionCommand) (bool, error)
	CancelTurn(context.Context, string, pluginsdk.ExactTaskExecutionCommand) (bool, error)
	RecoverSession(context.Context, string, pluginsdk.ExactSessionRecoveryCommand) (pluginsdk.ExactTaskRunResult, error)
	CancelPendingTaskTransition(context.Context, string, pluginsdk.ExactPendingTransitionCommand) (bool, error)
	GetSessionModeContext(context.Context, string, pluginsdk.ExactTaskExecutionCommand) (pluginsdk.SessionModeContext, error)
	SetSessionMode(context.Context, string, pluginsdk.ExactSessionModeCommand) (string, error)
	GetStepApproval(context.Context, string, pluginsdk.ExactStepApprovalQuery) (pluginsdk.StepApproval, error)
	// ApproveStep runs with the context of the Kandev user whose receipt
	// authorized it and approves through the native Approve path.
	ApproveStep(context.Context, string, pluginsdk.ExactStepApprovalCommand) (pluginsdk.StepApprovalResult, error)
}

type exactExecutionController = ExactExecutionController
