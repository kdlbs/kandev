package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

var ErrWorkspaceCloneName = errors.New("a non-reserved workspace name is required")

// WorkspaceClonePersistence commits the complete configuration and returns
// workflows for ordinary post-commit creation events.
type WorkspaceClonePersistence interface {
	CloneWorkspace(context.Context, *models.Workspace, *models.Workspace) ([]*models.Workflow, error)
}

func (s *Service) SetWorkspaceCloner(cloner WorkspaceClonePersistence) { s.workspaceCloner = cloner }

func (s *Service) CloneWorkspace(ctx context.Context, sourceID, name string) (*models.Workspace, error) {
	name = strings.TrimSpace(name)
	if name == "" || (&models.Workspace{Name: name}).IsImproveKandev() {
		return nil, ErrWorkspaceCloneName
	}
	source, err := s.GetWorkspace(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	if err := s.requireWorkspaceManage(ctx, source); err != nil {
		return nil, err
	}
	if !s.workspaceDecision(ctx, source).Has(authz.ScopeSecretManage) {
		return nil, ErrForbidden
	}
	if source.OfficeWorkflowID != "" || source.IsImproveKandev() {
		return nil, repoerrors.ErrWorkspaceCloneConfiguration
	}
	target := *source
	target.ID, target.Name = uuid.NewString(), name
	target.OwnerID, _ = callerScope(ctx)
	target.OrgID = callerOrgID(ctx)
	target.UnitID, err = s.placementFor(ctx, target.OwnerID, target.OrgID)
	if err != nil {
		return nil, err
	}
	target.OfficeWorkflowID, target.TaskSequence = "", 0
	target.CreatedAt, target.UpdatedAt = time.Time{}, time.Time{}
	if err := s.validateCloneDefaults(ctx, &target); err != nil {
		return nil, err
	}
	if s.workspaceCloner == nil {
		return nil, errors.New("workspace cloner is not configured")
	}
	workflows, err := s.workspaceCloner.CloneWorkspace(ctx, source, &target)
	if err != nil {
		return nil, err
	}
	s.publishWorkspaceEvent(ctx, events.WorkspaceCreated, &target)
	for _, workflow := range workflows {
		s.publishWorkflowEvent(ctx, events.WorkflowCreated, workflow)
	}
	return &target, nil
}

func (s *Service) validateCloneDefaults(ctx context.Context, target *models.Workspace) error {
	if id := normalizeOptionalID(target.DefaultExecutorID); id != nil {
		executor, err := s.GetExecutor(ctx, *id)
		if err != nil || executor == nil || executor.DeletedAt != nil || executor.Status == models.ExecutorStatusDisabled {
			return repoerrors.ErrWorkspaceCloneConfiguration
		}
	}
	if id := normalizeOptionalID(target.DefaultEnvironmentID); id != nil {
		if _, err := s.GetEnvironment(ctx, *id); err != nil {
			return repoerrors.ErrWorkspaceCloneConfiguration
		}
	}
	for _, id := range []*string{target.DefaultAgentProfileID, target.DefaultConfigAgentProfileID} {
		if id = normalizeOptionalID(id); id == nil {
			continue
		}
		if _, err := s.loadWorkflowChangeProfile(ctx, target.ID, *id, ""); err != nil {
			return repoerrors.ErrWorkspaceCloneConfiguration
		}
	}
	return nil
}

// ValidateWorkspaceCloneProfile admits only shared, enabled profiles. The
// profile reader uses the separate reader pool while the clone writer is held.
func (s *Service) ValidateWorkspaceCloneProfile(ctx context.Context, id string) error {
	if _, err := s.loadWorkflowChangeProfile(ctx, "", id, ""); err != nil {
		return repoerrors.ErrWorkspaceCloneConfiguration
	}
	return nil
}
