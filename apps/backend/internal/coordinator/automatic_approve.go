package coordinator

import (
	"context"
	"errors"
	"expvar"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// Outcomes of the automatic path, the closed label set of
// coordinator_automatic_approval_total.
const (
	automaticApproved      = "approved"
	automaticFailed        = "failed"
	automaticLimited       = "limited"
	automaticRaiserInvalid = "raiser_invalid"
)

var automaticApprovalTotal = expvar.NewMap("coordinator_automatic_approval_total")

// automaticTurnKey carries the unattended turn an automatic approval written
// inside a propose call belongs to.
type automaticTurnKey struct{}

func withAutomaticTurn(ctx context.Context, turnID *string) context.Context {
	return context.WithValue(ctx, automaticTurnKey{}, turnID)
}

func automaticTurnFrom(ctx context.Context) *string {
	id, _ := ctx.Value(automaticTurnKey{}).(*string)
	return id
}

// AutomaticResult is what propose_task_kandev returns for a proposal the
// automatic path handled. A nil result means the phase 1 result stands.
type AutomaticResult struct {
	Status ProposalStatus
	TaskID string
	Note   string
}

type raiserVerdict int

const (
	raiserValid raiserVerdict = iota
	raiserInvalid
)

// raiserContext returns the context the automatic approval runs under: the
// raising manager's identity, after checking they are an active user who
// still holds workspace.manage. An empty raiser is the auth-disabled case and
// keeps the caller's context. An error means the check could not decide.
func (s *Service) raiserContext(ctx context.Context, workspaceID, raiser string) (context.Context, raiserVerdict, error) {
	if raiser == "" {
		if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceManage); err != nil {
			return ctx, raiserInvalid, raiserCheckError(err)
		}
		return ctx, raiserValid, nil
	}
	resolver := s.identityResolver()
	if resolver == nil {
		return ctx, raiserInvalid, fmt.Errorf("%w: no identity resolver", errRaiserUnavailable)
	}
	identity, ok, err := resolver.ResolveUserIdentity(ctx, raiser)
	if err != nil {
		return ctx, raiserInvalid, fmt.Errorf("%w: %v", errRaiserUnavailable, err)
	}
	if !ok {
		return ctx, raiserInvalid, nil
	}
	raiserCtx := authn.WithIdentity(ctx, identity)
	if err := s.authz.AuthorizeWorkspaceScope(raiserCtx, workspaceID, authz.ScopeWorkspaceManage); err != nil {
		return ctx, raiserInvalid, raiserCheckError(err)
	}
	return raiserCtx, raiserValid, nil
}

// raiserCheckError maps a refusal to a nil error (the manager no longer
// qualifies) and any other failure to errRaiserUnavailable.
func raiserCheckError(err error) error {
	if errors.Is(err, taskservice.ErrForbidden) || errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		return nil
	}
	return fmt.Errorf("%w: %v", errRaiserUnavailable, err)
}

// TryAutomaticApproval runs the automatic path for a freshly inserted pending
// create_task proposal. It returns nil when the class is not automatic, so the
// phase 1 result stands.
func (s *Service) TryAutomaticApproval(ctx context.Context, proposal *Proposal) (*AutomaticResult, error) {
	if !s.phase3 || s.decisionTasks == nil || proposal.Kind != ProposalKindCreateTask || proposal.StartsAgent {
		return nil, nil
	}
	setting, err := s.actionSettings().Setting(ctx, proposal.CoordinatorID, string(ActionCreateTask))
	if err != nil || setting.Value != SettingAutomatic {
		return nil, nil
	}
	raiser := setting.ChangedBy
	raiserCtx, verdict, err := s.raiserContext(ctx, proposal.WorkspaceID, raiser)
	if err != nil {
		s.logger.Warn("automatic approval: raiser check failed", zap.String("proposal_id", proposal.ID), zap.Error(err))
		return s.pendingResult(proposal, unavailableNote), nil
	}
	if verdict == raiserInvalid {
		automaticApprovalTotal.Add(automaticRaiserInvalid, 1)
		if lerr := s.LowerClass(ctx, proposal.CoordinatorID, ActionCreateTask, raiserLoweredReason); lerr != nil {
			s.logger.Error("automatic approval: lowering after invalid raiser failed", zap.String("coordinator_id", proposal.CoordinatorID), zap.Error(lerr))
		}
		return s.pendingResult(proposal, unavailableNote), nil
	}
	spec, err := s.prepareApproval(raiserCtx, proposal.WorkspaceID, proposal, nil)
	if err != nil {
		s.logger.Warn("automatic approval: spec no longer valid", zap.String("proposal_id", proposal.ID), zap.Error(err))
		return s.pendingResult(proposal, unavailableNote), nil
	}
	return s.approveAutomatically(ctx, raiserCtx, proposal, raiser, spec)
}

func (s *Service) pendingResult(p *Proposal, note string) *AutomaticResult {
	return &AutomaticResult{Status: ProposalStatusPending, Note: note}
}

type automaticClaim struct {
	stale   bool
	limited bool
	matched bool
	token   string
}

// approveAutomatically claims the proposal inside one coordinator-locked
// section and, after the commit, runs the ordinary approve tail under the
// raiser's identity.
func (s *Service) approveAutomatically(ctx, raiserCtx context.Context, proposal *Proposal, raiser string, spec ProposalSpec) (*AutomaticResult, error) {
	var claim automaticClaim
	err := s.guardedLock(ctx, proposal.CoordinatorID, func(tx coordinatorExec) error {
		var err error
		claim, err = s.automaticClaimSection(ctx, tx, proposal, raiser, spec)
		return err
	})
	switch {
	case errors.Is(err, ErrNotFound):
		return nil, ErrNotFound
	case err != nil:
		s.logger.Error("automatic approval: claim section failed", zap.String("proposal_id", proposal.ID), zap.Error(err))
		return s.pendingResult(proposal, unavailableNote), nil
	case claim.stale:
		return nil, nil
	case claim.limited:
		automaticApprovalTotal.Add(automaticLimited, 1)
		return s.pendingResult(proposal, limitReachedNote), nil
	case !claim.matched:
		current, err := s.store.GetProposal(ctx, proposal.WorkspaceID, proposal.CoordinatorID, proposal.ID, s.phase2)
		if err != nil {
			return nil, err
		}
		return &AutomaticResult{Status: current.Status}, nil
	}
	turnCtx := withAutomaticTurn(raiserCtx, s.currentUnattendedTurn(ctx, proposal.CoordinatorID))
	done, err := s.finishClaim(turnCtx, proposal.WorkspaceID, proposal.CoordinatorID, proposal.ID, claim.token, spec, false, nil)
	if err != nil {
		s.logger.Error("automatic approval: completion failed after claim", zap.String("proposal_id", proposal.ID), zap.Error(err))
		return s.afterFailedFinish(ctx, proposal), nil
	}
	return automaticResultFor(done), nil
}

func (s *Service) afterFailedFinish(ctx context.Context, proposal *Proposal) *AutomaticResult {
	current, err := s.store.GetProposal(ctx, proposal.WorkspaceID, proposal.CoordinatorID, proposal.ID, s.phase2)
	if err != nil {
		return s.pendingResult(proposal, unavailableNote)
	}
	return &AutomaticResult{Status: current.Status, Note: unavailableNote}
}

func automaticResultFor(p *Proposal) *AutomaticResult {
	res := &AutomaticResult{Status: p.Status}
	switch p.Status {
	case ProposalStatusApproved:
		automaticApprovalTotal.Add(automaticApproved, 1)
		if p.TaskID != nil {
			res.TaskID = *p.TaskID
		}
	case ProposalStatusFailed:
		automaticApprovalTotal.Add(automaticFailed, 1)
	}
	return res
}

// guardedLock is withCoordinatorLock that turns a panic in fn into an error
// after the deferred rollback ran.
func (s *Service) guardedLock(ctx context.Context, coordinatorID string, fn func(tx coordinatorExec) error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("automatic approval section panicked: %v", r)
		}
	}()
	return s.store.withCoordinatorLock(ctx, coordinatorID, fn)
}

// automaticClaimSection re-reads the setting under the lock, applies the
// 24-hour limit and takes the claim on tx.
func (s *Service) automaticClaimSection(ctx context.Context, tx coordinatorExec, proposal *Proposal, raiser string, spec ProposalSpec) (automaticClaim, error) {
	var raw *string
	if err := tx.QueryRowContext(ctx, s.store.db.Rebind(`SELECT policy_json FROM coordinators WHERE id = ?`), proposal.CoordinatorID).Scan(&raw); err != nil {
		return automaticClaim{}, fmt.Errorf("read coordinator policy: %w", err)
	}
	stored, _ := ParsePolicy(raw)
	if stored.Actions[ActionCreateTask] != SettingAutomatic {
		return automaticClaim{stale: true}, nil
	}
	change, err := s.store.NewestClassChangeTx(ctx, tx, proposal.CoordinatorID, ActionCreateTask, SettingAutomatic)
	if err != nil {
		return automaticClaim{}, err
	}
	changedBy := ""
	if change != nil {
		changedBy = change.ChangedBy
	}
	if changedBy != raiser {
		return automaticClaim{stale: true}, nil
	}
	now := s.store.now().UTC()
	count, err := s.store.CountAutomaticDecidedTx(ctx, tx, proposal.CoordinatorID, now.Add(-automaticLimitWindow))
	if err != nil {
		return automaticClaim{}, err
	}
	if count >= automaticDailyLimit {
		return automaticClaim{limited: true}, nil
	}
	token := uuid.New().String()
	matched, err := s.store.ClaimProposalTx(ctx, tx, proposal.ID, token, spec, raiser, now, &now)
	if err != nil {
		return automaticClaim{}, err
	}
	if s.automatic.afterClaim != nil {
		if err := s.automatic.afterClaim(ctx); err != nil {
			return automaticClaim{}, err
		}
	}
	return automaticClaim{matched: matched, token: token}, nil
}
