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
var raisableClasses = map[Action]bool{ActionCreateTask: true}

// Eligibility thresholds for raising a class to automatic.
const (
	evidenceWindow       = 30 * 24 * time.Hour
	reviewMaxAge         = 7 * 24 * time.Hour
	eligibilityMinRows   = 20
	eligibilityApproved  = 9 // approved * 10 >= total * 9, a 90% floor in integer math
	eligibilityUnedited  = 10
	conditionHistory30d  = "history_30d"
	conditionVolume      = "volume"
	conditionUnedited    = "unedited_rate"
	conditionNoUndo      = "no_undo"
	conditionReviewed7d  = "reviewed_7d"
	decisionLogField     = "decision_log"
	classField           = "class"
	unavailableNote      = "automatic approval unavailable; a manager will decide"
	limitReachedNote     = "automatic limit reached; a manager will decide"
	raiserLoweredReason  = "raising manager no longer a manager"
	undoLoweredReason    = "undo of an automatic create"
	automaticDailyLimit  = 10
	automaticLimitWindow = 24 * time.Hour
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
// missing or disabled.
type IdentityResolver interface {
	IdentityForUser(ctx context.Context, userID string) (authn.Identity, bool)
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

// Eligibility computes the five ordered conditions for class at now.
func (s *Service) Eligibility(ctx context.Context, coordinatorID string, now time.Time) (EligibilityResult, error) {
	return s.eligibilityFor(ctx, coordinatorID, ActionCreateTask, now)
}

func (s *Service) eligibilityFor(ctx context.Context, coordinatorID string, class Action, now time.Time) (EligibilityResult, error) {
	log := s.decisionLog()
	since := now.Add(-evidenceWindow)
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
		EligibilityCondition{Name: conditionVolume, Met: total >= eligibilityMinRows, Value: total},
		EligibilityCondition{Name: conditionUnedited, Met: total > 0 && approved*eligibilityUnedited >= total*eligibilityApproved, Value: percent})

	undone, err := log.UndoneTaskIDs(ctx, coordinatorID, since, now)
	if err != nil {
		return EligibilityResult{}, err
	}
	conds = append(conds, EligibilityCondition{Name: conditionNoUndo, Met: len(undone) == 0, Value: len(undone)})

	review, err := s.store.NewestClassReview(ctx, coordinatorID, class)
	if err != nil {
		return EligibilityResult{}, err
	}
	reviewed := EligibilityCondition{Name: conditionReviewed7d}
	if review != nil {
		reviewed.Met = !review.ReviewedAt.Before(now.Add(-reviewMaxAge))
		reviewed.Value = review.ReviewedAt.UTC().Format(time.RFC3339)
	}
	conds = append(conds, reviewed)

	res := EligibilityResult{Eligible: true, Conditions: conds}
	for _, c := range conds {
		res.Eligible = res.Eligible && c.Met
	}
	return res, nil
}

// checkRaise is the change hook SaveSettings calls inside its locked
// transaction. Only a raise of a raisable class to automatic is checked; a
// read error counts as not eligible.
func (s *Service) checkRaise(ctx context.Context, coordinatorID string, stored, final Policy) error {
	if !s.phase3 {
		return nil
	}
	for _, class := range AllActions {
		if !raisableClasses[class] || final.Actions[class] != SettingAutomatic || stored.Actions[class] == SettingAutomatic {
			continue
		}
		res, err := s.eligibilityFor(ctx, coordinatorID, class, s.store.now().UTC())
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
	Result  EligibilityResult
	Setting Setting
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
	if _, err := s.store.GetCoordinator(ctx, workspaceID, coordinatorID); err != nil {
		return nil, err
	}
	if Action(class) != ActionCreateTask {
		return nil, &ClassError{Class: class}
	}
	setting, err := s.actionSettings().Setting(ctx, coordinatorID, class)
	if err != nil {
		return nil, err
	}
	res, err := s.eligibilityFor(ctx, coordinatorID, ActionCreateTask, s.store.now().UTC())
	if err != nil {
		return nil, &DecisionLogUnavailableError{Err: err}
	}
	return &EligibilityView{Result: res, Setting: setting.Value, ChangedBy: setting.ChangedBy, ChangedAt: setting.ChangedAt}, nil
}

// RecordClassReview stores a manager's review of the last 30 days of class.
// Every field but the reviewer is computed here.
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
	if Action(class) != ActionCreateTask {
		return nil, &ClassError{Class: class}
	}
	now := s.store.now().UTC()
	rows, err := s.decisionLog().Decisions(ctx, coordinatorID, class, now.Add(-evidenceWindow), now)
	if err != nil {
		return nil, &DecisionLogUnavailableError{Err: err}
	}
	review := ClassReview{
		ID: uuid.NewString(), CoordinatorID: coordinatorID, Class: ActionCreateTask, ReviewedBy: decidingUserID(ctx),
		ReviewedAt: now, WindowStart: now.Add(-evidenceWindow), WindowEnd: now, RowCount: len(rows),
	}
	if err := s.store.InsertClassReview(ctx, review); err != nil {
		return nil, err
	}
	return &review, nil
}

// errRaiserUnavailable marks a raiser check that could not decide.
var errRaiserUnavailable = errors.New("coordinator: raiser check unavailable")
