package coordinator

import (
	"context"
	"encoding/json"
	"errors"

	"go.uber.org/zap"
)

// ErrImprovementsUnavailable answers a propose while phase 3 is not effective.
var ErrImprovementsUnavailable = errors.New("coordinator: improvements are unavailable")

// ProposeImprovement validates and stores one improvement proposal. The
// context it replaces is read in the same locked transaction as the open
// proposal count, so the stored before never equals the after.
func (s *Service) ProposeImprovement(ctx context.Context, coordinatorID string, args json.RawMessage) (*Proposal, error) {
	if !s.phase3 {
		return nil, ErrImprovementsUnavailable
	}
	exec := s.kindExecutor(ProposalKindImprovement)
	c, err := s.store.GetCoordinatorByID(ctx, coordinatorID)
	if err != nil {
		return nil, err
	}
	validated, err := exec.ValidatePropose(ctx, c, args)
	if err != nil {
		return nil, err
	}
	var spec improvementSpec
	if err := json.Unmarshal(validated, &spec); err != nil {
		return nil, err
	}
	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: c.WorkspaceID, Kind: ProposalKindImprovement}
	pre := func(ctx context.Context, tx coordinatorExec) (*Proposal, error) {
		before, err := s.store.contextOn(ctx, tx, c.ID)
		if err != nil {
			return nil, err
		}
		spec.ContextBefore = before
		raw, err := json.Marshal(spec)
		if err != nil {
			return nil, err
		}
		p.RawSpec = string(raw)
		return nil, nil
	}
	turnID := s.currentUnattendedTurn(ctx, coordinatorID)
	inTx := func(ctx context.Context, tx coordinatorExec, p *Proposal) error {
		if spec.ContextBefore == spec.ContextAfter {
			return &FieldError{Field: fieldContext, Message: "context is the same as the current context"}
		}
		return s.recordKindProposed(ctx, tx, exec, p, p.RawSpec, turnID)
	}
	if err := s.store.InsertProposalWith(ctx, p, s.phase2, pre, inTx); err != nil {
		return nil, err
	}
	countImprovement(improvementProposed)
	s.logger.Info("proposal created", zap.String("coordinator_id", coordinatorID),
		zap.String("proposal_id", p.ID), zap.String("kind", ProposalKindImprovement))
	s.publishCoordinatorUpdated(ctx, c.WorkspaceID, coordinatorID)
	return p, nil
}

// contextOn reads a coordinator's stored context on exec.
func (s *Store) contextOn(ctx context.Context, exec coordinatorExec, coordinatorID string) (string, error) {
	var v string
	if err := exec.QueryRowContext(ctx, s.db.Rebind(`SELECT context FROM coordinators WHERE id = ?`), coordinatorID).Scan(&v); err != nil {
		return "", err
	}
	return v, nil
}
