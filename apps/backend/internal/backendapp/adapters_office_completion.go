package backendapp

import (
	"context"
	"database/sql"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"

	"github.com/kandev/kandev/internal/office/dashboard"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type officePendingApproverReader interface {
	PendingApproversForCompletion(ctx context.Context, taskID, stepID string) ([]dashboard.PendingApproverDTO, error)
}

// officeApprovalGuardAdapter lets the task service consult Office approvers
// before a human move completes an Office task.
type officeApprovalGuardAdapter struct {
	reader officePendingApproverReader
}

func (a officeApprovalGuardAdapter) PendingApproversForCompletion(
	ctx context.Context, taskID, stepID string,
) ([]taskservice.PendingApprover, error) {
	pending, err := a.reader.PendingApproversForCompletion(ctx, taskID, stepID)
	if err != nil {
		return nil, err
	}
	out := make([]taskservice.PendingApprover, 0, len(pending))
	for _, p := range pending {
		out = append(out, taskservice.PendingApprover{AgentProfileID: p.AgentProfileID, Name: p.Name})
	}
	return out, nil
}

func configureOfficeHumanCompletionGuard(tasks *tasksqlite.Repository, office *sqlite.Repository, workflow *workflowrepo.Repository) {
	tasks.SetOfficeHumanCompletionPendingReader(officeHumanCompletionPendingReader(office, workflow))
}

func officeHumanCompletionPendingReader(office *sqlite.Repository, workflow *workflowrepo.Repository) func(context.Context, *sql.Tx, string, string) ([]string, error) {
	return func(ctx context.Context, tx *sql.Tx, taskID, stepID string) ([]string, error) {
		approvers, err := office.ListTaskParticipantsAtStepTx(ctx, tx, taskID, stepID, "approver")
		if err != nil || len(approvers) == 0 {
			return nil, err
		}
		decisions, err := workflow.ListActiveTaskDecisionsTx(ctx, tx, taskID)
		if err != nil {
			return nil, err
		}
		return dashboard.PendingApproverIDs(approvers, decisions), nil
	}
}

func (a officeApprovalGuardAdapter) ResolvePendingApproverNames(ctx context.Context, ids []string) []taskservice.PendingApprover {
	pending := make([]taskservice.PendingApprover, 0, len(ids))
	resolver, ok := a.reader.(interface {
		ResolvePendingApproverNames(context.Context, []string) []dashboard.PendingApproverDTO
	})
	if !ok {
		for _, id := range ids {
			pending = append(pending, taskservice.PendingApprover{AgentProfileID: id, Name: id})
		}
		return pending
	}
	for _, p := range resolver.ResolvePendingApproverNames(ctx, ids) {
		pending = append(pending, taskservice.PendingApprover{AgentProfileID: p.AgentProfileID, Name: p.Name})
	}
	return pending
}

type officeHumanDoneCompleterAdapter struct{ tasks *taskservice.Service }

func (a officeHumanDoneCompleterAdapter) CompleteOfficeTaskAsHuman(ctx context.Context, id, stepID string) (*dashboard.FinalStepMoveResult, error) {
	task, err := a.tasks.CompleteOfficeTaskAsHuman(ctx, id, stepID)
	if err != nil {
		return nil, classifyFinalStepMoveError(err)
	}
	return &dashboard.FinalStepMoveResult{State: string(task.State), StepID: task.WorkflowStepID}, nil
}
