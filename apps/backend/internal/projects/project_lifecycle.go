package projects

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/authz"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

func (s *Service) Archive(ctx context.Context, workspaceID, projectID string) (*ProjectView, error) {
	return s.setProjectArchived(ctx, workspaceID, projectID, true)
}

func (s *Service) Restore(ctx context.Context, workspaceID, projectID string) (*ProjectView, error) {
	return s.setProjectArchived(ctx, workspaceID, projectID, false)
}

func (s *Service) setProjectArchived(ctx context.Context, workspaceID, projectID string, archived bool) (*ProjectView, error) {
	if err := s.requireEnabled(); err != nil {
		return nil, err
	}
	if err := s.tasks.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeTaskWrite); err != nil {
		return nil, err
	}
	if s.lifecycle == nil {
		return nil, errors.New("project task lifecycle is unavailable")
	}
	s.workerMu.Lock()
	defer s.workerMu.Unlock()
	project, err := s.store.Get(ctx, workspaceID, projectID)
	if err != nil {
		return nil, err
	}
	if project.MainTaskID == "" {
		return nil, ErrCoordinatorConflict
	}
	if archived {
		_, err = s.lifecycle.ArchiveAgentProjectTree(ctx, projectID, project.MainTaskID)
	} else {
		_, err = s.lifecycle.UnarchiveAgentProjectTree(ctx, projectID, project.MainTaskID)
	}
	if err != nil {
		return nil, err
	}
	if err := s.store.SetArchived(ctx, workspaceID, projectID, archived); err != nil {
		return nil, err
	}
	return s.Get(ctx, workspaceID, projectID)
}

func (s *Service) Delete(ctx context.Context, workspaceID, projectID string, discardWorktreeChanges, deleteContext bool) error {
	if err := s.requireEnabled(); err != nil {
		return err
	}
	if err := s.tasks.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeTaskWrite); err != nil {
		return err
	}
	if s.lifecycle == nil {
		return errors.New("project task lifecycle is unavailable")
	}
	s.workerMu.Lock()
	defer s.workerMu.Unlock()
	project, err := s.store.Get(ctx, workspaceID, projectID)
	if err != nil {
		return err
	}
	if project.MainTaskID == "" {
		return ErrCoordinatorConflict
	}
	if err := s.preflightProjectDelete(ctx, projectID); err != nil {
		return err
	}
	if err := s.deleteProjectTaskTree(ctx, project, discardWorktreeChanges); err != nil {
		return err
	}
	if err := s.store.SetArchived(ctx, workspaceID, projectID, true); err != nil {
		return err
	}
	if err := s.removeProjectContext(projectID, deleteContext); err != nil {
		return err
	}
	return s.store.Delete(ctx, workspaceID, projectID)
}

func (s *Service) preflightProjectDelete(ctx context.Context, projectID string) error {
	running, err := s.store.HasRunningTaskSessions(ctx, projectID)
	if err != nil {
		return err
	}
	if running {
		return ErrProjectRunning
	}
	return nil
}

func (s *Service) deleteProjectTaskTree(ctx context.Context, project *Project, discardWorktreeChanges bool) error {
	coordinator, err := s.tasks.GetTask(ctx, project.MainTaskID)
	if errors.Is(err, taskrepo.ErrTaskNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if coordinator == nil {
		return taskrepo.ErrTaskNotFound
	}
	_, err = s.lifecycle.DeleteAgentProjectTree(ctx, project.ID, project.MainTaskID, taskservice.DeleteTaskOptions{
		DiscardWorktreeChanges: discardWorktreeChanges,
	})
	return err
}

func (s *Service) removeProjectContext(projectID string, remove bool) error {
	if !remove {
		return nil
	}
	if s.context == nil {
		return ErrContextUnavailable
	}
	if err := s.context.RemoveProject(projectID); err != nil {
		return fmt.Errorf("remove project context: %w", err)
	}
	return nil
}
