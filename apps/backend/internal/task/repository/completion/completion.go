package completion

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

type AdmissionKind string

const CodeApprovalsPending = "approvals_pending"

const (
	Move          AdmissionKind = "move"
	SetState      AdmissionKind = "set_state"
	FinalStepDone AdmissionKind = "final_step_done"
	Fields        AdmissionKind = "fields"
)

// Scope records the task placement and state that the human entry point
// authorized before it began the mutation.
type Scope struct {
	WorkspaceID string
	WorkflowID  string
	StepID      string
	TaskState   string
}

// Admission carries trusted human authority to one repository write. It is
// created by the task service after authorization and is never request data.
type Admission struct {
	TaskID   string
	Actor    wfmodels.StepTransitionActor
	Kind     AdmissionKind
	Expected Scope
}

// Querier is the read-only SQL surface required by completion policies. Both
// a transaction and the repository read pool satisfy it.
type Querier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Policy contributes domain-specific admission checks to task completion.
type Policy interface {
	AdmitCompletion(ctx context.Context, q Querier, current, candidate *models.Task, admission Admission) error
}

// RefusedError reports a typed policy refusal and the subjects that explain
// it without carrying presentation-layer names into the repository.
type RefusedError struct {
	Code     string
	Subjects []string
}

func (e *RefusedError) Error() string {
	if e == nil || e.Code == "" {
		return "completion refused"
	}
	return fmt.Sprintf("completion refused: %s", e.Code)
}
