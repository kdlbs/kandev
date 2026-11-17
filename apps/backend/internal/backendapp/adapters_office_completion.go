package backendapp

import (
	"context"
	officecompletionpolicy "github.com/kandev/kandev/internal/office/completionpolicy"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/task/repository/completion"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"

	"github.com/kandev/kandev/internal/office/dashboard"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type officeApproverNameReader interface {
	ResolvePendingApproverNames(context.Context, []string) []dashboard.PendingApproverDTO
}

// officeApproverNameResolverAdapter resolves stable policy subjects for API
// errors without participating in admission decisions.
type officeApproverNameResolverAdapter struct{ reader officeApproverNameReader }

func registerOfficeCompletionPolicy(tasks interface {
	SetCompletionPolicies(...completion.Policy)
}, office *sqlite.Repository, workflow *workflowrepo.Repository) {
	tasks.SetCompletionPolicies(officecompletionpolicy.New(office, workflow))
}

func (a officeApproverNameResolverAdapter) ResolvePendingApproverNames(ctx context.Context, ids []string) []taskservice.PendingApprover {
	pending := make([]taskservice.PendingApprover, 0, len(ids))
	if a.reader == nil {
		for _, id := range ids {
			pending = append(pending, taskservice.PendingApprover{AgentProfileID: id, Name: id})
		}
		return pending
	}
	for _, p := range a.reader.ResolvePendingApproverNames(ctx, ids) {
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
