package coordinator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"go.uber.org/zap"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

const maxStandingOrderIDs = 5

// startsAgentReporter is implemented by a kind whose proposal row stores
// whether approving it starts an agent.
type startsAgentReporter interface {
	ProposeStartsAgent(ctx context.Context, spec json.RawMessage) (bool, error)
}

// checkOrderIDsShape refuses a citation list that is too long, holds an empty
// id, or repeats one. It reads nothing.
func checkOrderIDsShape(ids []string) error {
	if len(ids) > maxStandingOrderIDs {
		return &FieldError{Field: fieldStandingOrderIDs, Message: "standing_order_ids may hold at most 5 ids"}
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			return &FieldError{Field: fieldStandingOrderIDs, Message: "standing_order_ids entries must be non-empty"}
		}
		if _, dup := seen[id]; dup {
			return &FieldError{Field: fieldStandingOrderIDs, Message: "standing_order_ids must not repeat an id"}
		}
		seen[id] = struct{}{}
	}
	return nil
}

// ProposeKind validates and stores one resume, message or move proposal. The
// bool reports a deduplicated result: an open proposal for the same target
// already existed and nothing was inserted.
func (s *Service) ProposeKind(ctx context.Context, coordinatorID, kind string, args json.RawMessage, orderIDs []string) (*Proposal, bool, error) {
	exec := s.kindExecutor(kind)
	if exec == nil {
		return nil, false, fmt.Errorf("%w: %q", ErrUnknownProposalKind, kind)
	}
	c, err := s.store.GetCoordinatorByID(ctx, coordinatorID)
	if err != nil {
		return nil, false, err
	}
	if err := checkOrderIDsShape(orderIDs); err != nil {
		return nil, false, err
	}
	taskID, err := proposeTaskID(args)
	if err != nil {
		return nil, false, err
	}
	if open, err := s.store.OpenTargetProposal(ctx, s.store.ro, coordinatorID, kind, taskID); err != nil {
		return nil, false, err
	} else if open != nil {
		return open, true, nil
	}
	p, err := s.buildKindProposal(ctx, exec, c, args, taskID, orderIDs)
	if err != nil {
		return nil, false, err
	}
	spec := json.RawMessage(p.RawSpec)
	deduped := false
	pre := func(ctx context.Context, tx coordinatorExec) (*Proposal, error) {
		open, err := s.store.OpenTargetProposal(ctx, tx, coordinatorID, kind, taskID)
		if err != nil {
			return nil, err
		}
		if open != nil {
			deduped = true
			return open, nil
		}
		return nil, s.checkOrdersActive(ctx, tx, coordinatorID, orderIDs)
	}
	turnID := s.currentUnattendedTurn(ctx, coordinatorID)
	inTx := func(ctx context.Context, tx coordinatorExec, p *Proposal) error {
		if err := s.recordKindProposed(ctx, tx, exec, p, string(spec), turnID); err != nil {
			return err
		}
		return s.store.MarkApplied(ctx, tx, coordinatorID, orderIDs, p.CreatedAt)
	}
	if err := s.store.InsertProposalWith(ctx, p, s.phase2, pre, inTx); err != nil {
		return nil, false, err
	}
	if !deduped {
		s.logger.Info("proposal created", zap.String("coordinator_id", coordinatorID),
			zap.String("proposal_id", p.ID), zap.String("kind", kind))
		s.publishCoordinatorUpdated(ctx, c.WorkspaceID, coordinatorID)
	}
	return p, deduped, nil
}

// buildKindProposal validates args and returns the unsaved proposal. The stored
// target is the task the validated spec names.
func (s *Service) buildKindProposal(ctx context.Context, exec KindExecutor, c *Coordinator, args json.RawMessage, taskID string, orderIDs []string) (*Proposal, error) {
	spec, err := exec.ValidatePropose(ctx, c, args)
	if err != nil {
		return nil, err
	}
	if specID, err := proposeTaskID(spec); err != nil || specID != taskID {
		return nil, notFoundTarget()
	}
	p := &Proposal{
		CoordinatorID: c.ID, WorkspaceID: c.WorkspaceID, Kind: exec.Kind(), TargetTaskID: &taskID,
		RawSpec: string(spec), StandingOrderIDs: append([]string{}, orderIDs...),
	}
	if r, ok := exec.(startsAgentReporter); ok {
		if p.StartsAgent, err = r.ProposeStartsAgent(ctx, spec); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func (s *Service) recordKindProposed(ctx context.Context, tx coordinatorExec, exec KindExecutor, p *Proposal, spec string, turnID *string) error {
	id := p.ID
	return s.Record(ctx, tx, ActivityRow{
		CoordinatorID: p.CoordinatorID, WorkspaceID: p.WorkspaceID, ActionClass: exec.Action(),
		Outcome: ActivityProposed, Authorization: AuthRequiresApproval, TargetTaskID: p.TargetTaskID,
		ProposalID: &id, Detail: proposedDetail(spec), UnattendedTurnID: turnID,
	})
}

// proposedDetail is the rationale of a stored spec, the one-line reason a
// proposed row shows.
func proposedDetail(spec string) string {
	var v struct {
		Rationale string `json:"rationale"`
	}
	_ = json.Unmarshal([]byte(spec), &v)
	return truncateRunes(v.Rationale, 200)
}

// checkOrdersActive refuses a citation of an order that is missing, another
// coordinator's, or retired, reading inside the propose transaction.
func (s *Service) checkOrdersActive(ctx context.Context, tx coordinatorExec, coordinatorID string, ids []string) error {
	for _, id := range ids {
		o, err := s.store.standingOrderOn(ctx, tx, coordinatorID, id)
		if errors.Is(err, ErrNotFound) || (err == nil && o.RetiredAt != nil) {
			return &FieldError{Field: fieldStandingOrderIDs, Message: "standing_order_ids must cite active orders of this coordinator"}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// OpenTargetProposal returns the open (pending, approving or failed) proposal
// of the coordinator for a kind and target task, or nil.
func (s *Store) OpenTargetProposal(ctx context.Context, exec coordinatorExec, coordinatorID, kind, taskID string) (*Proposal, error) {
	rows, err := exec.QueryContext(ctx, s.db.Rebind(`SELECT `+proposalColumns+` FROM coordinator_proposals
		WHERE coordinator_id = ? AND kind = ? AND target_task_id = ? AND status IN ('pending','approving','failed')`),
		coordinatorID, kind, taskID)
	if err != nil {
		return nil, fmt.Errorf("read open proposal: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, rows.Err()
	}
	var r proposalRow
	if err := scanProposalRow(rows, &r); err != nil {
		return nil, err
	}
	return r.toProposal()
}

func proposeTaskID(args json.RawMessage) (string, error) {
	var in struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(args, &in); err != nil || strings.TrimSpace(in.TaskID) == "" {
		return "", &FieldError{Field: fieldTaskID, Message: "task_id is required"}
	}
	return strings.TrimSpace(in.TaskID), nil
}

func checkRationale(r string) error {
	if utf8.RuneCountInString(r) > proposalTextMaxRunes {
		return &FieldError{Field: fieldRationale, Message: fmt.Sprintf("rationale must be at most %d characters", proposalTextMaxRunes)}
	}
	return nil
}

// proposeTarget applies the target checks every kind shares.
func (s *Service) proposeTarget(ctx context.Context, c *Coordinator, taskID string) (*TargetTask, error) {
	tasks := s.kindDeps.Tasks
	if tasks == nil {
		return nil, errors.New("coordinator: task reads are not wired")
	}
	t, err := tasks.GetTarget(ctx, taskID)
	if errors.Is(err, ErrTaskNotFound) {
		return nil, notFoundTarget()
	}
	if err != nil {
		return nil, err
	}
	if t.WorkspaceID != c.WorkspaceID {
		return nil, notFoundTarget()
	}
	set, err := s.EffectiveWatchSet(ctx, c.ID)
	if err != nil {
		return nil, fmt.Errorf("read watch set: %w", err)
	}
	if !set.Contains(t.WorkflowID) {
		return nil, notFoundTarget()
	}
	switch {
	case t.ArchivedAt != nil:
		return nil, &FieldError{Field: fieldTaskID, Message: "task is archived"}
	case t.Origin == string(taskmodels.TaskOriginCoordinator):
		return nil, &FieldError{Field: fieldTaskID, Message: "task is a coordinator conversation"}
	}
	return t, nil
}

// notFoundTarget is the single answer for a missing, foreign or unwatched task,
// so a caller cannot tell them apart.
func notFoundTarget() error {
	return &FieldError{Field: fieldTaskID, Message: "task not found"}
}

func parseArgs(args json.RawMessage, into any) error {
	if err := json.Unmarshal(args, into); err != nil {
		return &FieldError{Field: fieldBody, Message: "arguments are malformed"}
	}
	return nil
}

func (k *resumeKind) ValidatePropose(ctx context.Context, c *Coordinator, args json.RawMessage) (json.RawMessage, error) {
	var in resumeSpec
	if err := parseArgs(args, &in); err != nil {
		return nil, err
	}
	if err := checkRationale(in.Rationale); err != nil {
		return nil, err
	}
	t, err := k.svc.proposeTarget(ctx, c, in.TaskID)
	if err != nil {
		return nil, err
	}
	if t.Primary == nil || !sessionResumable(t.Primary, k.svc.kindDeps.Tasks.HasLiveExecution(ctx, t.Primary.ID)) {
		return nil, &FieldError{Field: fieldTaskID, Message: "task has no resumable session"}
	}
	return json.Marshal(resumeSpec{TaskID: in.TaskID, Rationale: in.Rationale})
}

func (k *messageKind) ValidatePropose(ctx context.Context, c *Coordinator, args json.RawMessage) (json.RawMessage, error) {
	var in messageSpec
	if err := parseArgs(args, &in); err != nil {
		return nil, err
	}
	if err := checkRationale(in.Rationale); err != nil {
		return nil, err
	}
	text, err := checkMessageText(in.Text)
	if err != nil {
		return nil, err
	}
	t, err := k.svc.proposeTarget(ctx, c, in.TaskID)
	if err != nil {
		return nil, err
	}
	if t.Primary == nil || !messageAcceptsSession(t.Primary.State) {
		return nil, &FieldError{Field: fieldTaskID, Message: "task's session is not accepting messages"}
	}
	return json.Marshal(messageSpec{TaskID: in.TaskID, Text: text, Rationale: in.Rationale})
}

type moveArgs struct {
	TaskID    string `json:"task_id"`
	StepID    string `json:"step_id"`
	Rationale string `json:"rationale"`
}

func (k *moveKind) ValidatePropose(ctx context.Context, c *Coordinator, args json.RawMessage) (json.RawMessage, error) {
	var in moveArgs
	if err := parseArgs(args, &in); err != nil {
		return nil, err
	}
	if err := checkRationale(in.Rationale); err != nil {
		return nil, err
	}
	t, err := k.svc.proposeTarget(ctx, c, in.TaskID)
	if err != nil {
		return nil, err
	}
	u := k.svc.undoTasks
	if u == nil {
		return nil, errors.New("coordinator: task moves are not wired")
	}
	step, err := u.GetStep(ctx, in.StepID)
	if errors.Is(err, ErrStepNotFound) || (err == nil && step.WorkflowID != t.WorkflowID) {
		return nil, &FieldError{Field: fieldStepID, Message: "step not found in the task's workflow"}
	}
	if err != nil {
		return nil, err
	}
	if in.StepID == t.WorkflowStepID {
		return nil, &FieldError{Field: fieldStepID, Message: "task is already on this step"}
	}
	if step.CompletesOnEnter {
		return nil, &FieldError{Field: fieldStepID, Message: "step completes the task"}
	}
	starts, err := k.startsAgent(ctx, t.WorkflowID, in.StepID)
	if err != nil {
		return nil, err
	}
	if starts && !k.svc.policyFor(c).Allows(ActionStartAgent) {
		return nil, &FieldError{Field: fieldStepID, Message: "step starts an agent"}
	}
	return json.Marshal(moveSpec{TaskID: in.TaskID, WorkflowID: t.WorkflowID, FromStepID: t.WorkflowStepID, ToStepID: in.StepID, Rationale: in.Rationale})
}

func (k *moveKind) startsAgent(ctx context.Context, workflowID, stepID string) (bool, error) {
	nodes, err := k.svc.undoTasks.ListSteps(ctx, workflowID)
	if err != nil {
		return false, err
	}
	return StartsAgentOnEnter(nodes, stepID), nil
}

// ProposeStartsAgent is the value stored as the row's starts_agent.
func (k *moveKind) ProposeStartsAgent(ctx context.Context, spec json.RawMessage) (bool, error) {
	var s moveSpec
	if err := json.Unmarshal(spec, &s); err != nil {
		return false, err
	}
	return k.startsAgent(ctx, s.WorkflowID, s.ToStepID)
}

func scanProposalRow(rows *sql.Rows, r *proposalRow) error {
	if err := rows.Scan(&r.ID, &r.CoordinatorID, &r.WorkspaceID, &r.Status, &r.SpecJSON, &r.FinalSpecJSON, &r.ClaimedAt,
		&r.ClaimToken, &r.TaskID, &r.Error, &r.RejectReason, &r.DecidedBy, &r.CreatedAt, &r.UpdatedAt, &r.Kind,
		&r.TargetTaskID, &r.StandingIDs, &r.StartsAgent, &r.OutcomeJSON, &r.ReplyText, &r.ReplyDeliveredAt,
		&r.ReplyDeliveryClaimedAt, &r.InReplyTo, &r.ChangeStatus); err != nil {
		return fmt.Errorf("scan proposal: %w", err)
	}
	return nil
}
