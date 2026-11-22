package completionpolicy

import (
	"context"
	"database/sql"
	"errors"

	officemodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/completion"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const ApprovalsPendingCode = completion.CodeApprovalsPending

// Policy contributes the Office approver rule to task completion admission.
type Policy struct {
	office   *sqlite.Repository
	workflow *workflowrepo.Repository
}

func New(office *sqlite.Repository, workflow *workflowrepo.Repository) *Policy {
	return &Policy{office: office, workflow: workflow}
}

func (p *Policy) AdmitCompletion(
	ctx context.Context,
	q completion.Querier,
	current, candidate *models.Task,
	a completion.Admission,
) error {
	if err := p.admitFinalStepDone(ctx, q, current, candidate, a); err != nil {
		return err
	}
	if !requiresHumanApproverAdmission(current, candidate, a) {
		return nil
	}
	return p.admitHumanApprovers(ctx, q, candidate)
}

func (p *Policy) admitFinalStepDone(
	ctx context.Context,
	q completion.Querier,
	current, candidate *models.Task,
	a completion.Admission,
) error {
	if a.Kind != completion.FinalStepDone {
		return nil
	}
	if !current.IsFromOffice || candidate.State != v1.TaskStateCompleted {
		return repoerrors.ErrWorkflowChangeConflict
	}
	if current.WorkflowStepID == "" {
		return nil
	}
	var final bool
	query := p.workflow.Rebind(`SELECT ` + wfmodels.FinalStepSQL("s") + ` FROM workflow_steps s WHERE s.workflow_id=? AND s.id=?`)
	err := q.QueryRowContext(ctx, query, current.WorkflowID, current.WorkflowStepID).Scan(&final)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !final) {
		return repoerrors.ErrWorkflowChangeConflict
	}
	return err
}

func requiresHumanApproverAdmission(current, candidate *models.Task, a completion.Admission) bool {
	return a.Actor == wfmodels.StepTransitionActorHuman && current.IsFromOffice && a.Kind != completion.Fields &&
		candidate.State == v1.TaskStateCompleted && current.State != v1.TaskStateCompleted
}

func (p *Policy) admitHumanApprovers(ctx context.Context, q completion.Querier, candidate *models.Task) error {
	approvers, err := p.office.ListTaskParticipantsAtStepWithQuery(ctx, q, candidate.ID, candidate.WorkflowStepID, "approver")
	if err != nil || len(approvers) == 0 {
		return err
	}
	decisions, err := p.workflow.ListActiveTaskDecisionsWithQuery(ctx, q, candidate.ID)
	if err != nil {
		return err
	}
	pending := PendingApproverIDs(approvers, decisions)
	if len(pending) == 0 {
		return nil
	}
	return &completion.RefusedError{Code: ApprovalsPendingCode, Subjects: pending}
}

// PendingApproverIDs compares the canonical Office approver slate with active
// approvals. It is shared by completion admission and dashboard projections.
func PendingApproverIDs(approvers []sqlite.Participant, decisions []*wfmodels.WorkflowStepDecision) []string {
	approvedBy := make(map[string]struct{}, len(decisions))
	for _, decision := range decisions {
		if decision.Decision == officemodels.DecisionApproved {
			approvedBy[decision.DeciderID] = struct{}{}
		}
	}
	pending := make([]string, 0, len(approvers))
	for _, approver := range approvers {
		if _, approved := approvedBy[approver.AgentProfileID]; !approved {
			pending = append(pending, approver.AgentProfileID)
		}
	}
	return pending
}
