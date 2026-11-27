package service

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// AutomationCredentialRunAccessChecker identifies protected automation runs
// from their durable task association and evaluates the fixed creator.
type AutomationCredentialRunAccessChecker func(context.Context, string, string) (bool, bool, error)

// AutomationCredentialRunCreatorLookup returns a protected run's fixed creator.
type AutomationCredentialRunCreatorLookup func(context.Context, string) (string, error)

// SetAutomationCredentialRunAccessChecker wires the automation-owned access
// record into task, session, and environment authorization.
func (s *Service) SetAutomationCredentialRunAccessChecker(checker AutomationCredentialRunAccessChecker) {
	s.automationCredentialRunAccess = checker
}

// SetAutomationCredentialRunUseChecker wires current grant validation into
// prompt and runtime attachment admission for an already-running automation.
func (s *Service) SetAutomationCredentialRunUseChecker(checker AutomationCredentialRunAccessChecker) {
	s.automationCredentialRunUse = checker
}

// SetAutomationCredentialRunCreatorLookup wires run-id protection for tasks
// that have not yet been bound back to their run row.
func (s *Service) SetAutomationCredentialRunCreatorLookup(lookup AutomationCredentialRunCreatorLookup) {
	s.automationCredentialRunCreator = lookup
}

// IsProtectedAutomationRun reports whether runID carries credential authority.
func (s *Service) IsProtectedAutomationRun(ctx context.Context, runID string) (bool, error) {
	if s.automationCredentialRunCreator == nil {
		return false, nil
	}
	creatorID, err := s.automationCredentialRunCreator(ctx, runID)
	return creatorID != "", err
}

func (s *Service) IsProtectedAutomationTask(ctx context.Context, taskID string) (bool, error) {
	protected, _, err := s.automationCredentialRunAccessForTask(ctx, taskID, "")
	return protected, err
}

func (s *Service) authorizeAutomationCredentialRun(ctx context.Context, taskID string) error {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.Synthetic {
		return nil
	}
	protected, allowed, err := s.automationCredentialRunAccessForTask(ctx, taskID, identity.UserID)
	if err != nil {
		return err
	}
	if protected && !allowed {
		return repoerrors.ErrTaskNotFound
	}
	return nil
}

func (s *Service) automationCredentialRunAccessForTask(
	ctx context.Context,
	taskID, userID string,
) (bool, bool, error) {
	if s.automationCredentialRunAccess != nil {
		protected, allowed, err := s.automationCredentialRunAccess(ctx, taskID, userID)
		if err != nil || protected {
			return protected, allowed, err
		}
	}
	if s.automationCredentialRunCreator == nil || s.tasks == nil {
		return false, false, nil
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if errors.Is(err, repoerrors.ErrTaskNotFound) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if task == nil {
		return false, false, nil
	}
	runID, _ := task.Metadata[models.MetaKeyAutomationRunID].(string)
	if runID == "" {
		return false, false, nil
	}
	creatorID, err := s.automationCredentialRunCreator(ctx, runID)
	if err != nil || creatorID == "" {
		return false, false, err
	}
	return true, creatorID == userID, nil
}

func (s *Service) authorizeAutomationCredentialRunUse(ctx context.Context, taskID string) error {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.Synthetic || s.automationCredentialRunUse == nil {
		return nil
	}
	protected, allowed, err := s.automationCredentialRunUse(ctx, taskID, identity.UserID)
	if err != nil {
		return err
	}
	if protected && !allowed {
		return repoerrors.ErrTaskNotFound
	}
	return nil
}
