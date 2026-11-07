package coordinator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// neededClasses lists the classes a proposal needs to read automatic: its
// kind's class, then start_agent when it starts an agent. It reports false
// when any of them is not raisable.
func neededClasses(p *Proposal) ([]Action, bool) {
	classes := []Action{kindAction(p.Kind)}
	if p.StartsAgent && classes[0] != ActionStartAgent {
		classes = append(classes, ActionStartAgent)
	}
	for _, class := range classes {
		if !raisableClasses[class] {
			return nil, false
		}
	}
	return classes, true
}

func needsAdmission(classes []Action) bool {
	for _, class := range classes {
		if class == ActionResume || class == ActionStartAgent {
			return true
		}
	}
	return false
}

// automaticPlan is what the locked section and the post-commit tail need.
type automaticPlan struct {
	proposal  *Proposal
	classes   []Action
	changeIDs map[Action]string
	actor     string
	actorCtx  context.Context
	spec      ProposalSpec    // create path
	exec      KindExecutor    // kind path; nil for create_task
	final     json.RawMessage // kind path frozen spec
}

// TryAutomaticApproval runs the automatic path for a freshly inserted pending
// proposal. It returns nil when a needed class is not automatic, so the
// phase 1 result stands.
func (s *Service) TryAutomaticApproval(ctx context.Context, proposal *Proposal) (*AutomaticResult, error) {
	if !s.phase3 || s.decisionTasks == nil {
		return nil, nil
	}
	classes, ok := neededClasses(proposal)
	if !ok {
		return nil, nil
	}
	settings, ok := s.automaticSettings(ctx, proposal.CoordinatorID, classes)
	if !ok {
		return nil, nil
	}
	if paused, gateErr := s.gate.Active(ctx, proposal.CoordinatorID); paused {
		if gateErr != nil {
			return s.pendingResult(proposal, unavailableNote), nil
		}
		return s.pendingResult(proposal, pausedNote), nil
	}
	plan := &automaticPlan{proposal: proposal, classes: classes, changeIDs: make(map[Action]string, len(classes))}
	for _, class := range classes {
		plan.changeIDs[class] = settings[class].ChangeID
	}
	if res := s.validateRaisers(ctx, plan, settings); res != nil {
		return res, nil
	}
	if res := s.admitAutomatic(ctx, proposal, classes); res != nil {
		return res, nil
	}
	if res := s.prepareAutomatic(plan); res != nil {
		return res, nil
	}
	return s.approveAutomatically(ctx, plan)
}

// automaticSettings reads every needed class's setting. It reports false
// unless all of them read automatic with a recorded raise.
func (s *Service) automaticSettings(ctx context.Context, coordinatorID string, classes []Action) (map[Action]ClassSetting, bool) {
	out := make(map[Action]ClassSetting, len(classes))
	for _, class := range classes {
		setting, err := s.actionSettings().Setting(ctx, coordinatorID, string(class))
		if err != nil || setting.Value != SettingAutomatic {
			return nil, false
		}
		if setting.ChangeID == "" {
			s.logger.Error("automatic class has no raise record", zap.String("coordinator_id", coordinatorID), zap.String("class", string(class)))
			return nil, false
		}
		out[class] = setting
	}
	return out, true
}

// validateRaisers checks every distinct raiser without stopping at the first
// invalid one and lowers the classes of each invalid raiser, guarded by the
// raise it validated. A check error lowers nothing for that raiser. A non-nil
// result leaves the proposal pending.
func (s *Service) validateRaisers(ctx context.Context, plan *automaticPlan, settings map[Action]ClassSetting) *AutomaticResult {
	byRaiser := map[string][]Action{}
	for _, class := range plan.classes {
		by := settings[class].ChangedBy
		byRaiser[by] = append(byRaiser[by], class)
	}
	raisers := make([]string, 0, len(byRaiser))
	for r := range byRaiser {
		raisers = append(raisers, r)
	}
	sort.Strings(raisers)
	ctxs := make(map[string]context.Context, len(raisers))
	unavailable, invalid := false, false
	for _, raiser := range raisers {
		raiserCtx, verdict, err := s.raiserContext(ctx, plan.proposal.WorkspaceID, raiser)
		switch {
		case err != nil:
			s.logger.Warn("automatic approval: raiser check failed", zap.String("proposal_id", plan.proposal.ID), zap.Error(err))
			unavailable = true
		case verdict == raiserInvalid:
			invalid = true
			s.lowerForInvalidRaiser(ctx, plan, byRaiser[raiser])
		default:
			ctxs[raiser] = raiserCtx
		}
	}
	if invalid {
		automaticApprovalTotal.Add(automaticRaiserInvalid, 1)
	}
	if invalid || unavailable {
		return s.pendingResult(plan.proposal, unavailableNote)
	}
	newest := newestRaise(plan.classes, settings)
	plan.actor = settings[newest].ChangedBy
	plan.actorCtx = ctxs[plan.actor]
	return nil
}

// newestRaise returns the class raised last; the class name ascending breaks
// a tie.
func newestRaise(classes []Action, settings map[Action]ClassSetting) Action {
	newest := classes[0]
	for _, class := range classes[1:] {
		a, b := settings[class].ChangedAt, settings[newest].ChangedAt
		if a != nil && b != nil && (a.After(*b) || (a.Equal(*b) && class < newest)) {
			newest = class
		}
	}
	return newest
}

func (s *Service) lowerForInvalidRaiser(ctx context.Context, plan *automaticPlan, classes []Action) {
	for _, class := range classes {
		if _, err := s.LowerClass(ctx, plan.proposal.CoordinatorID, class, raiserLoweredReason, LowerGuard{RaiseID: plan.changeIDs[class]}); err != nil {
			automaticLowerFailedTotal.Add(lowerTriggerRaiserInvalid, 1)
			s.logger.Error("automatic approval: lowering after invalid raiser failed",
				zap.String("coordinator_id", plan.proposal.CoordinatorID), zap.String("class", string(class)), zap.Error(err))
		}
	}
}

// admitAutomatic holds an agent-starting action while containment or the cost
// ceiling would refuse it. A non-nil result leaves the proposal pending and
// counts nothing toward the limit.
func (s *Service) admitAutomatic(ctx context.Context, proposal *Proposal, classes []Action) *AutomaticResult {
	if !needsAdmission(classes) {
		return nil
	}
	admission := s.AdmitAction(ctx, proposal.CoordinatorID)
	if admission.OK {
		return nil
	}
	token := admission.Reason
	if admission.Detail == admitDetailReadError {
		token = admitDetailReadError
	}
	automaticApprovalTotal.Add(automaticHeld, 1)
	automaticHeldTotal.Add(token, 1)
	return s.pendingResult(proposal, fmt.Sprintf("automatic action held (%s); a manager will decide", token))
}

// prepareAutomatic validates the frozen spec on the actor's authority.
func (s *Service) prepareAutomatic(plan *automaticPlan) *AutomaticResult {
	p := plan.proposal
	var err error
	if p.Kind == ProposalKindCreateTask {
		plan.spec, err = s.prepareApproval(plan.actorCtx, p.WorkspaceID, p, nil)
	} else {
		plan.exec = s.kindExecutor(p.Kind)
		if plan.exec == nil {
			err = fmt.Errorf("%w: %q", ErrUnknownProposalKind, p.Kind)
		} else {
			plan.final, err = s.prepareKindApproval(plan.actorCtx, plan.exec, p, ApproveProposalRequest{})
		}
	}
	if err != nil {
		s.logger.Warn("automatic approval: spec no longer valid", zap.String("proposal_id", p.ID), zap.Error(err))
		return s.pendingResult(p, unavailableNote)
	}
	return nil
}

func (s *Service) pendingResult(p *Proposal, note string) *AutomaticResult {
	return &AutomaticResult{Status: ProposalStatusPending, Note: note}
}

type automaticClaim struct {
	paused  bool
	stale   bool
	limited bool
	matched bool
	token   string
}

// approveAutomatically claims the proposal inside one coordinator-locked
// section and, after the commit, runs the ordinary approve tail under the
// actor's identity.
func (s *Service) approveAutomatically(ctx context.Context, plan *automaticPlan) (*AutomaticResult, error) {
	proposal := plan.proposal
	var claim automaticClaim
	err := s.guardedLock(ctx, proposal.CoordinatorID, func(tx coordinatorExec) error {
		var err error
		claim, err = s.automaticClaimSection(ctx, tx, plan)
		return err
	})
	switch {
	case errors.Is(err, ErrNotFound):
		return nil, ErrNotFound
	case err != nil:
		s.logger.Error("automatic approval: claim section failed", zap.String("proposal_id", proposal.ID), zap.Error(err))
		return s.pendingResult(proposal, unavailableNote), nil
	case claim.paused:
		return s.pendingResult(proposal, pausedNote), nil
	case claim.stale:
		return nil, nil
	case claim.limited:
		automaticApprovalTotal.Add(automaticLimited, 1)
		return s.pendingResult(proposal, limitReachedNote), nil
	case !claim.matched:
		current, err := s.store.GetProposalAnyKind(ctx, proposal.WorkspaceID, proposal.CoordinatorID, proposal.ID)
		if err != nil {
			return nil, err
		}
		return &AutomaticResult{Status: current.Status}, nil
	}
	turnCtx := withAutomaticTurn(plan.actorCtx, s.currentUnattendedTurn(ctx, proposal.CoordinatorID))
	var done *Proposal
	if plan.exec != nil {
		done, err = s.runClaimedKind(turnCtx, plan.exec, proposal, claim.token, plan.final, plan.actor)
	} else {
		done, err = s.finishClaim(turnCtx, proposal.WorkspaceID, proposal.CoordinatorID, proposal.ID, claim.token, plan.spec, proposal.StartsAgent, nil)
	}
	if err != nil {
		s.logger.Error("automatic approval: completion failed after claim", zap.String("proposal_id", proposal.ID), zap.Error(err))
		return s.afterFailedFinish(ctx, proposal), nil
	}
	return automaticResultFor(done), nil
}

func (s *Service) afterFailedFinish(ctx context.Context, proposal *Proposal) *AutomaticResult {
	current, err := s.store.GetProposalAnyKind(ctx, proposal.WorkspaceID, proposal.CoordinatorID, proposal.ID)
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
		} else if p.TargetTaskID != nil {
			res.TaskID = *p.TargetTaskID
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

// automaticClaimSection re-reads every needed class and the pause state under
// the lock, applies the 24-hour limit and takes the claim on tx.
func (s *Service) automaticClaimSection(ctx context.Context, tx coordinatorExec, plan *automaticPlan) (automaticClaim, error) {
	proposal := plan.proposal
	var raw *string
	var pausedAt sql.NullTime
	if err := tx.QueryRowContext(ctx, s.store.db.Rebind(`SELECT policy_json, paused_at FROM coordinators WHERE id = ?`), proposal.CoordinatorID).Scan(&raw, &pausedAt); err != nil {
		return automaticClaim{}, fmt.Errorf("read coordinator policy: %w", err)
	}
	stored, _ := ParsePolicy(raw)
	for _, class := range plan.classes {
		if stored.Actions[class] != SettingAutomatic {
			return automaticClaim{stale: true}, nil
		}
		change, err := s.store.NewestClassChangeTx(ctx, tx, proposal.CoordinatorID, class, SettingAutomatic)
		if err != nil {
			return automaticClaim{}, err
		}
		if change == nil || change.ID != plan.changeIDs[class] {
			return automaticClaim{stale: true}, nil
		}
	}
	if pausedAt.Valid {
		return automaticClaim{paused: true}, nil
	}
	now := s.store.now().UTC()
	count, err := s.store.CountAutomaticDecidedTx(ctx, tx, proposal.CoordinatorID, now.Add(-automaticLimitWindow))
	if err != nil {
		return automaticClaim{}, err
	}
	if count >= stored.EffectiveEligibility().DailyLimit {
		return automaticClaim{limited: true}, nil
	}
	token := uuid.New().String()
	var matched bool
	if plan.exec != nil {
		matched, err = s.store.ClaimProposalRawTx(ctx, tx, proposal.ID, token, string(plan.final), plan.actor, now, &now)
	} else {
		matched, err = s.store.ClaimProposalTx(ctx, tx, proposal.ID, token, plan.spec, plan.actor, now, &now)
	}
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

// automaticAttempt is the snapshot of an automatic claim taken in the
// transaction that settles it, so a later manager retry cannot change what the
// failure hook acts on.
type automaticAttempt struct {
	coordinatorID string
	classes       []Action
	automaticAt   *time.Time
}

// stampAutomatic marks row as the automatic path's when the proposal's
// current claim is automatic, and returns the attempt snapshot. A manager's
// claim, or a row that cannot be read, leaves row untouched and returns nil.
func (s *Service) stampAutomatic(ctx context.Context, p *Proposal, row *ActivityRow) *automaticAttempt {
	current, err := s.store.GetProposalAnyKind(ctx, p.WorkspaceID, p.CoordinatorID, p.ID)
	if err != nil || !current.ClaimedAutomatically {
		return nil
	}
	row.Authorization, row.UnattendedTurnID = AuthAutomatic, automaticTurnFrom(ctx)
	if current.DecidedBy != nil && *current.DecidedBy != "" {
		row.ActorUserID = current.DecidedBy
	}
	classes, _ := neededClasses(current)
	return &automaticAttempt{coordinatorID: current.CoordinatorID, classes: classes, automaticAt: current.AutomaticAt}
}

// afterAutomaticFailure lowers the agent-starting classes of an automatic
// attempt that settled failed, guarded by the attempt's own date so a re-raise
// after it is never lowered. A failure is counted and left to the backstop.
func (s *Service) afterAutomaticFailure(ctx context.Context, attempt *automaticAttempt) {
	if attempt == nil || !s.phase3 {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("automatic failure lowering panicked", zap.String("coordinator_id", attempt.coordinatorID), zap.Any("panic", r))
		}
	}()
	lctx := context.WithoutCancel(ctx)
	for _, class := range attempt.classes {
		if class != ActionResume && class != ActionStartAgent {
			continue
		}
		if _, err := s.LowerClass(lctx, attempt.coordinatorID, class, failureLoweredReason, LowerGuard{After: attempt.automaticAt}); err != nil {
			automaticLowerFailedTotal.Add(lowerTriggerFailure, 1)
			s.logger.Error("automatic failure lowering failed", zap.String("coordinator_id", attempt.coordinatorID), zap.String("class", string(class)), zap.Error(err))
		}
	}
}
