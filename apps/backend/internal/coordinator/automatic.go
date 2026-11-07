package coordinator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
)

// raisableClasses is the closed allowlist of classes that may be automatic.
var raisableClasses = map[Action]bool{ActionCreateTask: true, ActionResume: true, ActionStartAgent: true}

// Eligibility condition names, class-scoped notes and fixed windows. The five
// gate values themselves are the coordinator's EligibilitySettings.
const (
	conditionHistory30d   = "history_30d"
	conditionVolume       = "volume"
	conditionUnedited     = "unedited_rate"
	conditionNoUndo       = "no_undo"
	conditionReviewed7d   = "reviewed_7d"
	decisionLogField      = "decision_log"
	classField            = "class"
	unavailableNote       = "automatic approval unavailable; a manager will decide"
	pausedNote            = "Paused; a manager will decide"
	limitReachedNote      = "automatic limit reached; a manager will decide"
	raiserLoweredReason   = "raising manager no longer a manager"
	undoLoweredReason     = "undo of an automatic create"
	failureLoweredReason  = "automatic action failed"
	overrideLoweredReason = "manager overrode an automatic class"
	automaticLimitWindow  = 24 * time.Hour
)

// EligibilityCondition is one eligibility condition and its current value.
// Value is a count or percentage (number), a time (RFC 3339 string) or nil.
type EligibilityCondition struct {
	Name  string
	Met   bool
	Value any
}

// EligibilityResult is the outcome of the five ordered conditions.
type EligibilityResult struct {
	Eligible   bool
	Conditions []EligibilityCondition
}

// FirstUnmet names the first condition that is not met, or "".
func (r EligibilityResult) FirstUnmet() string {
	for _, c := range r.Conditions {
		if !c.Met {
			return c.Name
		}
	}
	return ""
}

// RaiseRefusedError is the 409 a raise answers with when the class is not
// eligible: Condition names the first unmet condition.
type RaiseRefusedError struct {
	Class     Action
	Condition string
}

func (e *RaiseRefusedError) Error() string {
	return fmt.Sprintf("policy.actions.%s: not eligible for automatic (%s)", e.Class, e.Condition)
}

// ClassError is a 400 naming a class no class-scoped route accepts.
type ClassError struct{ Class string }

func (e *ClassError) Error() string { return "class " + e.Class + " is not a raisable class" }

// DecisionLogUnavailableError is the 503 a class review answers with when the
// decision log cannot be read.
type DecisionLogUnavailableError struct{ Err error }

func (e *DecisionLogUnavailableError) Error() string {
	return "decision log unavailable: " + e.Err.Error()
}
func (e *DecisionLogUnavailableError) Unwrap() error { return e.Err }

// automaticState holds the automatic path's injectable seams.
type automaticState struct {
	mu         sync.Mutex
	settings   ActionSettings
	log        DecisionLog
	identities IdentityResolver
	// afterClaim is a test-only hook run inside the automatic claim section
	// right after the claim; an error it returns aborts the section.
	afterClaim func(ctx context.Context) error
}

// IdentityResolver resolves a stored user id to the identity that user
// carries on an authenticated request; ok is false when the account is
// missing or disabled, and an error means the lookup could not decide.
type IdentityResolver interface {
	ResolveUserIdentity(ctx context.Context, userID string) (authn.Identity, bool, error)
}

// SetAutomaticIdentities registers the resolver the automatic path uses to
// check the raising manager and to create the task under their identity.
func (s *Service) SetAutomaticIdentities(r IdentityResolver) {
	s.automatic.mu.Lock()
	defer s.automatic.mu.Unlock()
	s.automatic.identities = r
}

func (s *Service) identityResolver() IdentityResolver {
	s.automatic.mu.Lock()
	defer s.automatic.mu.Unlock()
	return s.automatic.identities
}

// Eligibility computes the five ordered conditions for create_task at now
// under the coordinator's stored eligibility settings.
func (s *Service) Eligibility(ctx context.Context, coordinatorID string, now time.Time) (EligibilityResult, error) {
	c, err := s.store.GetCoordinatorByID(ctx, coordinatorID)
	if err != nil {
		return EligibilityResult{}, err
	}
	return s.eligibilityFor(ctx, coordinatorID, ActionCreateTask, now, s.policyFor(c).EffectiveEligibility())
}

func (s *Service) eligibilityFor(ctx context.Context, coordinatorID string, class Action, now time.Time, cfg EligibilitySettings) (EligibilityResult, error) {
	log := s.decisionLog()
	since := now.Add(-cfg.evidenceWindow())
	var conds []EligibilityCondition

	earliest, ok, err := log.EarliestDecision(ctx, coordinatorID, string(class))
	if err != nil {
		return EligibilityResult{}, err
	}
	history := EligibilityCondition{Name: conditionHistory30d}
	if ok {
		history.Met = !earliest.After(since)
		history.Value = earliest.UTC().Format(time.RFC3339)
	}
	conds = append(conds, history)

	rows, err := log.Decisions(ctx, coordinatorID, string(class), since, now)
	if err != nil {
		return EligibilityResult{}, err
	}
	total, approved := len(rows), 0
	for _, r := range rows {
		if r.Outcome == DecisionApproved {
			approved++
		}
	}
	percent := 0
	if total > 0 {
		percent = approved * 100 / total
	}
	conds = append(conds,
		EligibilityCondition{Name: conditionVolume, Met: total >= cfg.MinDecisions, Value: total},
		EligibilityCondition{Name: conditionUnedited, Met: total > 0 && approved*100 >= total*cfg.UneditedFloorPct, Value: percent})

	noUndo, err := s.noUndoCount(ctx, coordinatorID, class, since, now)
	if err != nil {
		return EligibilityResult{}, err
	}
	conds = append(conds, EligibilityCondition{Name: conditionNoUndo, Met: noUndo == 0, Value: noUndo})

	review, err := s.store.NewestClassReview(ctx, coordinatorID, class)
	if err != nil {
		return EligibilityResult{}, err
	}
	reviewed := EligibilityCondition{Name: conditionReviewed7d}
	if review != nil {
		reviewed.Met = !review.ReviewedAt.Before(now.Add(-cfg.reviewMaxAge()))
		reviewed.Value = review.ReviewedAt.UTC().Format(time.RFC3339)
	}
	conds = append(conds, reviewed)

	res := EligibilityResult{Eligible: true, Conditions: conds}
	for _, c := range conds {
		res.Eligible = res.Eligible && c.Met
	}
	return res, nil
}

// noUndoCount is the no_undo value: undone tasks for create_task, and failed
// or system-lowered automatic actions for the classes that cannot be undone.
func (s *Service) noUndoCount(ctx context.Context, coordinatorID string, class Action, since, now time.Time) (int, error) {
	if class == ActionCreateTask {
		undone, err := s.decisionLog().UndoneTaskIDs(ctx, coordinatorID, since, now)
		return len(undone), err
	}
	return s.store.FailedOrLoweredCount(ctx, coordinatorID, class, since, now)
}

// checkRaise is the change hook SaveSettings calls inside its locked
// transaction. Only a raise of a raisable class to automatic is checked, under
// the eligibility settings stored before the save; a read error counts as not
// eligible.
func (s *Service) checkRaise(ctx context.Context, coordinatorID string, stored, final Policy) error {
	if !s.phase3 {
		return nil
	}
	cfg := stored.EffectiveEligibility()
	for _, class := range AllActions {
		if !raisableClasses[class] || final.Actions[class] != SettingAutomatic || stored.Actions[class] == SettingAutomatic {
			continue
		}
		res, err := s.eligibilityFor(ctx, coordinatorID, class, s.store.now().UTC(), cfg)
		if err != nil {
			s.logger.Warn("coordinator raise refused: decision log unreadable", zap.String("coordinator_id", coordinatorID), zap.Error(err))
			return &RaiseRefusedError{Class: class, Condition: decisionLogField}
		}
		if !res.Eligible {
			s.logger.Info("coordinator raise refused", zap.String("coordinator_id", coordinatorID),
				zap.String("class", string(class)), zap.String("condition", res.FirstUnmet()))
			return &RaiseRefusedError{Class: class, Condition: res.FirstUnmet()}
		}
	}
	return nil
}

// EligibilityView is the eligibility route's result.
type EligibilityView struct {
	Result   EligibilityResult
	Settings EligibilitySettings
	Setting  Setting
	// ChangedBy and ChangedAt describe the newest change to the current value.
	ChangedBy string
	ChangedAt *time.Time
}

// GetEligibility returns create_task's eligibility and current setting.
func (s *Service) GetEligibility(ctx context.Context, workspaceID, coordinatorID, class string) (*EligibilityView, error) {
	if !s.phase3 {
		return nil, ErrNotFound
	}
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	coord, err := s.store.GetCoordinator(ctx, workspaceID, coordinatorID)
	if err != nil {
		return nil, err
	}
	if !raisableClasses[Action(class)] {
		return nil, &ClassError{Class: class}
	}
	setting, err := s.actionSettings().Setting(ctx, coordinatorID, class)
	if err != nil {
		return nil, err
	}
	cfg := s.policyFor(coord).EffectiveEligibility()
	res, err := s.eligibilityFor(ctx, coordinatorID, Action(class), s.store.now().UTC(), cfg)
	if err != nil {
		return nil, &DecisionLogUnavailableError{Err: err}
	}
	return &EligibilityView{Result: res, Settings: cfg, Setting: setting.Value, ChangedBy: setting.ChangedBy, ChangedAt: setting.ChangedAt}, nil
}

// RecordClassReview stores a manager's review of the evidence window of class.
// The evidence window and the decided-row count are read inside the coordinator
// lock, so the review and a concurrent settings save are ordered. Every field
// but the reviewer is computed here.
func (s *Service) RecordClassReview(ctx context.Context, workspaceID, coordinatorID, class string) (*ClassReview, error) {
	if !s.phase3 {
		return nil, ErrNotFound
	}
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceManage); err != nil {
		return nil, err
	}
	if _, err := s.store.GetCoordinator(ctx, workspaceID, coordinatorID); err != nil {
		return nil, err
	}
	if !raisableClasses[Action(class)] {
		return nil, &ClassError{Class: class}
	}
	var review ClassReview
	err := s.store.withCoordinatorLock(ctx, coordinatorID, func(tx coordinatorExec) error {
		var raw *string
		if err := tx.QueryRowContext(ctx, s.store.db.Rebind(`SELECT policy_json FROM coordinators WHERE id = ?`), coordinatorID).Scan(&raw); err != nil {
			return fmt.Errorf("read coordinator policy: %w", err)
		}
		stored, _ := ParsePolicy(raw)
		window := stored.EffectiveEligibility().evidenceWindow()
		now := s.store.now().UTC()
		count, err := s.store.CountDecidedRowsTx(ctx, tx, coordinatorID, Action(class), now.Add(-window), now)
		if err != nil {
			return err
		}
		review = ClassReview{
			ID: uuid.NewString(), CoordinatorID: coordinatorID, Class: Action(class), ReviewedBy: decidingUserID(ctx),
			ReviewedAt: now, WindowStart: now.Add(-window), WindowEnd: now, RowCount: count,
		}
		return s.store.InsertClassReviewTx(ctx, tx, review)
	})
	if err != nil {
		return nil, &DecisionLogUnavailableError{Err: err}
	}
	return &review, nil
}

// errRaiserUnavailable marks a raiser check that could not decide.
var errRaiserUnavailable = errors.New("coordinator: raiser check unavailable")
