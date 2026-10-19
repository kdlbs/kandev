package service

import (
	"context"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const executorActiveUserStatus = "active"

// authorizeExecutorOwners composes parent and profile ownership without treating
// an internal, unscoped context as authority over private compute.
func (s *Service) authorizeExecutorOwners(ctx context.Context, owners ...string) error {
	for _, owner := range owners {
		if owner == "" {
			continue
		}
		identity, ok := s.executorCaller(ctx)
		if !ok || identity.UserID == "" {
			return repoerrors.ErrExecutorProfileNotFound
		}
		if identity.Synthetic {
			continue
		}

		if owner == identity.UserID {
			continue
		}
		if !authz.OrgScopes(authz.NormalizeOrgRole(string(identity.Role))).Has(authz.ScopeOrgConfigManage) {
			return repoerrors.ErrExecutorProfileNotFound
		}
		if tenancyEnforced {
			if s.userOrgs == nil || identity.OrgID == "" {
				return repoerrors.ErrExecutorProfileNotFound
			}
			orgID, err := s.userOrgs(ctx, owner)
			if err != nil || orgID != identity.OrgID {
				return repoerrors.ErrExecutorProfileNotFound
			}
		}
	}
	return nil
}

// AuthorizeExecutorProfile checks the resolved profile and its actual parent.
// Callers must resolve defaults before this check and before reading secrets.
func (s *Service) AuthorizeExecutorProfile(ctx context.Context, profileID string) error {
	if profileID == "" {
		return nil
	}
	_, err := s.GetExecutorProfile(ctx, profileID)
	return err
}

func (s *Service) AuthorizeExecutorConfiguration(ctx context.Context, executorID, profileID string) error {
	if executorID == "" && profileID == "" {
		return nil
	}
	if profileID != "" {
		return s.AuthorizeExecutorProfile(ctx, profileID)
	}
	executorID, err := s.executorAdmissionID(ctx, executorID)
	if err != nil {
		return err
	}
	executor, err := s.executors.GetExecutor(ctx, executorID)
	if err != nil {
		return err
	}
	return s.authorizeExecutorOwners(ctx, executor.OwnerUserID)
}

func (s *Service) authorizeExecutorProfile(ctx context.Context, profile *models.ExecutorProfile) error {
	if profile == nil {
		return repoerrors.ErrExecutorProfileNotFound
	}
	executor, err := s.executors.GetExecutor(ctx, profile.ExecutorID)
	if err != nil {
		return err
	}
	return s.authorizeExecutorOwners(ctx, executor.OwnerUserID, profile.OwnerUserID)
}

func (s *Service) executorOwnershipIntent(ctx context.Context, current string, requested models.OptionalOwner) (*models.ExecutorOwnershipIntent, error) {
	intent := &models.ExecutorOwnershipIntent{ExpectedOwnerUserID: current}
	if !requested.Set {
		return intent, nil
	}
	identity, ok := s.executorCaller(ctx)
	if !ok || identity.UserID == "" {
		return nil, ErrForbidden
	}
	manager := s.executorConfigurationManager(ctx)
	if !manager && (current == "" || current != identity.UserID || (requested.Value != "" && requested.Value != identity.UserID)) {
		return nil, ErrForbidden
	}
	if requested.Value != "" {
		if identity.Synthetic {
			return nil, models.ErrExecutorOwnershipInvalid
		}
		status, _, found, err := s.lookupUser(ctx, requested.Value)
		if err != nil || !found || status != executorActiveUserStatus {
			return nil, models.ErrExecutorOwnershipInvalid
		}
		if err := s.requireAssignableWorkspaceUser(ctx, requested.Value, identity.OrgID); err != nil {
			return nil, models.ErrExecutorOwnershipInvalid
		}
	}
	intent.OwnerUserID = &requested.Value
	return intent, nil
}

func (s *Service) validateExecutorCreateOwner(ctx context.Context, owner string) error {
	if owner == "" {
		return nil
	}
	identity, ok := s.executorCaller(ctx)
	if !ok || identity.Synthetic || identity.UserID == "" {
		return ErrForbidden
	}
	if identity.UserID != owner && !s.executorConfigurationManager(ctx) {
		return ErrForbidden
	}
	status, _, found, err := s.lookupUser(ctx, owner)
	if err != nil || !found || status != executorActiveUserStatus {
		return models.ErrExecutorOwnershipInvalid
	}
	return s.requireAssignableWorkspaceUser(ctx, owner, identity.OrgID)
}

func executorProfileOwnershipOnly(req *UpdateExecutorProfileRequest) bool {
	return req.OwnerUserID.Set && req.Name == nil && req.McpPolicy == nil && req.Config == nil && req.PrepareScript == nil && req.CleanupScript == nil && req.EnvVars == nil
}

func (s *Service) visibleExecutorProfiles(ctx context.Context, profiles []*models.ExecutorProfile) []*models.ExecutorProfile {
	visible := make([]*models.ExecutorProfile, 0, len(profiles))
	for _, profile := range profiles {
		if s.authorizeExecutorProfile(ctx, profile) == nil {
			visible = append(visible, profile)
		}
	}
	return visible
}

func (s *Service) executorConfigurationManager(ctx context.Context) bool {
	identity, ok := s.executorCaller(ctx)
	return ok && (identity.Synthetic || authz.OrgScopes(authz.NormalizeOrgRole(string(identity.Role))).Has(authz.ScopeOrgConfigManage))
}

func (s *Service) executorCaller(ctx context.Context) (authn.Identity, bool) {
	identity, ok := authn.IdentityFromContext(ctx)
	if actor, bound := models.ExecutorActorFromContext(ctx); bound {
		if s.executorIdentityResolver != nil {
			identity, ok = s.executorIdentityResolver(ctx, actor)
		} else if !ok || actor == "" || identity.UserID != actor {
			return authn.Identity{}, false
		}
	}
	if !ok || identity.UserID == "" {
		return authn.Identity{}, false
	}
	if !identity.Synthetic && s.userDirectory != nil {
		status, role, found, err := s.userDirectory.LookupStatus(ctx, identity.UserID)
		if err != nil || !found || status != executorActiveUserStatus {
			return authn.Identity{}, false
		}
		identity.Role = authn.Role(role)
	}
	return identity, true
}

func executorUseScope(scope authz.Scope) bool {
	return scope == authz.ScopeSessionExec || scope == authz.ScopeSessionPrompt
}

func (s *Service) authorizeSessionExecutor(ctx context.Context, session *models.TaskSession) error {
	if err := s.AuthorizeExecutorConfiguration(ctx, session.ExecutorID, session.ExecutorProfileID); err != nil {
		return err
	}
	if session.TaskEnvironmentID != "" {
		env, err := s.taskEnvironments.GetTaskEnvironment(ctx, session.TaskEnvironmentID)
		if err != nil {
			return err
		}
		if env != nil {
			return s.AuthorizeExecutorConfiguration(ctx, env.ExecutorID, env.ExecutorProfileID)
		}
	}
	return nil
}

func executorOwnershipOnly(req *UpdateExecutorRequest) bool {
	return req.OwnerUserID.Set && req.Name == nil && req.Type == nil && req.Status == nil && req.Resumable == nil && req.Config == nil
}

func (s *Service) saveExecutorProfileOwnership(ctx context.Context, profile *models.ExecutorProfile, req *UpdateExecutorProfileRequest) (*models.ExecutorProfile, error) {
	var err error
	if req.ExpectedUpdatedAt != nil {
		err = s.executors.UpdateExecutorProfileIfUnmodified(ctx, profile, *req.ExpectedUpdatedAt)
	} else {
		err = s.executors.UpdateExecutorProfile(ctx, profile)
	}
	if err != nil {
		return nil, err
	}
	s.publishExecutorProfileEvent(ctx, events.ExecutorProfileUpdated, profile)
	return profile, nil
}

func (s *Service) requireExecutorTypeAdmin(ctx context.Context, types ...models.ExecutorType) error {
	if identity, ok := s.executorCaller(ctx); ok {
		ctx = authn.WithIdentity(ctx, identity)
	} else if _, bound := models.ExecutorActorFromContext(ctx); bound {
		ctx = authn.WithIdentity(ctx, authn.Identity{})
	}
	return requireExecutorTypeAdmin(ctx, types...)
}
