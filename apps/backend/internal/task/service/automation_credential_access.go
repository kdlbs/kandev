package service

import (
	"context"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// AutomationCredentialRunAccessChecker identifies protected automation runs
// from their durable task association and evaluates the fixed creator.
type AutomationCredentialRunAccessChecker func(context.Context, string, string) (bool, bool, error)

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

func (s *Service) IsProtectedAutomationTask(ctx context.Context, taskID string) (bool, error) {
	if s.automationCredentialRunAccess == nil {
		return false, nil
	}
	protected, _, err := s.automationCredentialRunAccess(ctx, taskID, "")
	return protected, err
}

func (s *Service) authorizeAutomationCredentialRun(ctx context.Context, taskID string) error {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.Synthetic || s.automationCredentialRunAccess == nil {
		return nil
	}
	protected, allowed, err := s.automationCredentialRunAccess(ctx, taskID, identity.UserID)
	if err != nil {
		return err
	}
	if protected && !allowed {
		return repoerrors.ErrTaskNotFound
	}
	return nil
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
