package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// moveSpec is the stored spec of a move proposal.
type moveSpec struct {
	TaskID     string `json:"task_id"`
	WorkflowID string `json:"workflow_id"`
	FromStepID string `json:"from_step_id"`
	ToStepID   string `json:"to_step_id"`
	Rationale  string `json:"rationale"`
}

const detailAlreadyThere = "It was already there"
const detailMoveQueued = "It is queued behind the step's limit"

type moveKind struct{ svc *Service }

func (k *moveKind) Kind() string             { return ProposalKindMove }
func (k *moveKind) Action() Action           { return ActionMove }
func (k *moveKind) ReRunsOnStaleClaim() bool { return false }

func (k *moveKind) ValidateEdits(base, edits json.RawMessage) (json.RawMessage, error) {
	return refuseAllEdits(base, edits)
}

// Execute runs the ordered checks against the task as it is now, records
// from_step_id under the claim's fence, then moves. Every error before the
// fenced write is a definite failure: nothing has been moved.
func (k *moveKind) Execute(ctx context.Context, claim Claim) (Outcome, error) {
	var spec moveSpec
	if err := json.Unmarshal(claim.Spec, &spec); err != nil {
		return Outcome{}, failWith(fmt.Sprintf("stored move spec is unreadable: %v", err))
	}
	u := k.svc.undoTasks
	if u == nil {
		return Outcome{}, failWith("task moves are not wired")
	}
	taskID := claim.TargetTaskID
	if deps := k.svc.kindDeps; deps.Tasks != nil {
		target, err := deps.Tasks.GetTarget(ctx, taskID)
		if errors.Is(err, ErrTaskNotFound) {
			return Outcome{}, failWith(failTaskArchived)
		}
		if err != nil {
			return Outcome{}, failWith(err.Error())
		}
		if err := checkExecuteTarget(target, claim, spec.TaskID); err != nil {
			return Outcome{}, err
		}
	} else if spec.TaskID != taskID {
		return Outcome{}, failWith(failTaskArchived)
	}
	task, err := u.GetTask(ctx, taskID)
	if errors.Is(err, ErrTaskNotFound) {
		return Outcome{}, failWith(failTaskArchived)
	}
	if err != nil {
		return Outcome{}, failWith(err.Error())
	}
	if failure := k.checkTask(ctx, u, claim, &spec, task); failure != nil {
		if errors.Is(failure, errNoop) {
			return noopOutcome(taskID, spec.ToStepID), nil
		}
		return Outcome{}, failure
	}

	k.svc.logger.Info("move approval found task step",
		zap.String("proposal_id", claim.ProposalID), zap.String("task_id", taskID), zap.String("from_step_id", task.WorkflowStepID))
	outcomeJSON := moveOutcomeJSON(task.WorkflowStepID, spec.ToStepID, false)
	fenced, err := k.svc.store.SetOutcomeFenced(ctx, claim.ProposalID, claim.Token, outcomeJSON, time.Now())
	if err != nil {
		return Outcome{}, failWith(err.Error())
	}
	if !fenced {
		return Outcome{}, errSettleFenced
	}
	return k.move(ctx, u, taskID, &spec, task.WorkflowStepID)
}

var errNoop = errors.New("coordinator: move destination is the current step")

// checkTask runs checks 1 to 7. It returns errNoop for check 4, a failure for
// the others, and nil when the move may proceed.
func (k *moveKind) checkTask(ctx context.Context, u UndoTaskService, claim Claim, spec *moveSpec, task *UndoTask) error {
	if task.ArchivedAt != nil {
		return failWith(failTaskArchived)
	}
	if task.WorkflowID != spec.WorkflowID {
		return failWith(failTaskLeftWF)
	}
	step, err := u.GetStep(ctx, spec.ToStepID)
	if errors.Is(err, ErrStepNotFound) || (err == nil && step.WorkflowID != spec.WorkflowID) {
		return failWith(failStepMissing)
	}
	if err != nil {
		return failWith(err.Error())
	}
	if task.WorkflowStepID == spec.ToStepID {
		return errNoop
	}
	if step.CompletesOnEnter {
		return failWith(failStepIsDone)
	}
	nodes, err := u.ListSteps(ctx, spec.WorkflowID)
	if err != nil {
		return failWith(err.Error())
	}
	if StartsAgentOnEnter(nodes, spec.ToStepID) && !claim.StartsAgent {
		return failWith(failStepStartsAgent)
	}
	active, err := u.HasActiveSession(ctx, claim.TargetTaskID)
	if err != nil {
		return failWith(err.Error())
	}
	if active {
		return failWith(failAgentRunning)
	}
	return nil
}

func (k *moveKind) move(ctx context.Context, u UndoTaskService, taskID string, spec *moveSpec, fromStepID string) (Outcome, error) {
	admitted, err := u.MoveTaskWithOptions(ctx, taskID, spec.WorkflowID, spec.ToStepID, 0, UndoMoveOptions{ExpectedWorkflowID: spec.WorkflowID})
	switch {
	case errors.Is(err, ErrWIPLimitExceeded):
		return Outcome{}, failWith(failStepFull)
	case errors.Is(err, ErrMoveConflict):
		return Outcome{}, failWith(failMoved)
	case errors.Is(err, ErrTaskNotFound):
		return Outcome{}, failWith(failTaskArchived)
	case err != nil:
		return Outcome{}, err
	}
	out := Outcome{TaskID: taskID, OutcomeJSON: moveOutcomeJSON(fromStepID, spec.ToStepID, !admitted), Detail: "Moved"}
	if !admitted {
		out.Detail = detailMoveQueued
	}
	return out, nil
}

func noopOutcome(taskID, toStepID string) Outcome {
	raw, _ := json.Marshal(map[string]any{"from_step_id": toStepID, "to_step_id": toStepID, "noop": true})
	return Outcome{TaskID: taskID, OutcomeJSON: string(raw), Detail: detailAlreadyThere, Noop: true}
}

func moveOutcomeJSON(from, to string, queued bool) string {
	m := map[string]any{"from_step_id": from, "to_step_id": to}
	if queued {
		m["queued"] = true
	}
	raw, _ := json.Marshal(m)
	return string(raw)
}

// editFieldNames are the approve-body names a non-create kind inspects.
var editFieldNames = [...]string{ApproveFieldTitle, ApproveFieldDescription, ApproveFieldWorkflowID, ApproveFieldStepID, ApproveFieldRepositoryID, fieldText}

// carriesKindEdits reports whether edits names any of the six edit fields,
// whatever the value, null included.
func carriesKindEdits(edits ApproveProposalRequest) bool {
	for _, name := range editFieldNames {
		if _, ok := edits[name]; ok {
			return true
		}
	}
	return false
}

// refuseAllEdits is ValidateEdits for a kind with no editable field.
func refuseAllEdits(base, edits json.RawMessage) (json.RawMessage, error) {
	var body map[string]json.RawMessage
	if len(edits) > 0 {
		if err := json.Unmarshal(edits, &body); err != nil {
			return nil, &FieldError{Field: fieldBody, Message: "body must be a JSON object"}
		}
	}
	for _, name := range editFieldNames {
		if _, ok := body[name]; ok {
			return nil, &FieldError{Field: name, Message: "not_editable"}
		}
	}
	return base, nil
}
