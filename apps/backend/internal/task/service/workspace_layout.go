package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

const (
	WorkspaceLayoutRepository      = "repository"
	WorkspaceLayoutTaskRoot        = "task_root"
	WorkspaceLayoutCurrentRoot     = "current_root"
	WorkspaceLayoutKandevDirectory = "kandev_directory"
)

var ErrInvalidInitialWorkspaceLayout = fmt.Errorf("invalid initial workspace layout")

func (s *Service) resolveInitialWorkspaceExecutorType(ctx context.Context, req *CreateTaskRequest) (string, error) {
	if executorType := strings.TrimSpace(req.ExecutorType); executorType != "" {
		return executorType, nil
	}
	profileID, executorID := requestExecutorSelectionIDs(req)
	if profileID != "" {
		resolvedID, err := s.executorIDFromProfile(ctx, profileID)
		if err != nil {
			return "", err
		}
		executorID = resolvedID
	}
	if executorID == "" {
		resolvedID, err := s.workspaceDefaultExecutorID(ctx, req.WorkspaceID)
		if err != nil {
			return "", err
		}
		executorID = resolvedID
	}
	if executorID == "" {
		return workspaceExecutorTypeWorktree, nil
	}
	return s.executorTypeFromID(ctx, executorID)
}

func requestExecutorSelectionIDs(req *CreateTaskRequest) (string, string) {
	profileID := strings.TrimSpace(req.ExecutorProfileID)
	if profileID == "" {
		profileID = requestExecutorSelectionValue(req.DeferredLaunch, "executor_profile_id")
	}
	if profileID == "" {
		profileID = requestExecutorSelectionValue(req.Metadata, models.MetaKeyExecutorProfileID)
	}
	executorID := strings.TrimSpace(req.ExecutorID)
	if executorID == "" {
		executorID = requestExecutorSelectionValue(req.DeferredLaunch, "executor_id")
	}
	if executorID == "" {
		executorID = requestExecutorSelectionValue(req.Metadata, models.MetaKeyExecutorID)
	}
	return profileID, executorID
}

func (s *Service) executorIDFromProfile(ctx context.Context, profileID string) (string, error) {
	if s.executors == nil {
		return "", fmt.Errorf("%w: executor profile lookup is unavailable", ErrInvalidInitialWorkspaceLayout)
	}
	profile, err := s.executors.GetExecutorProfile(ctx, profileID)
	if err != nil {
		return "", fmt.Errorf("%w: resolve executor profile %q: %v", ErrInvalidInitialWorkspaceLayout, profileID, err)
	}
	if profile == nil || strings.TrimSpace(profile.ExecutorID) == "" {
		return "", fmt.Errorf("%w: executor profile %q has no executor", ErrInvalidInitialWorkspaceLayout, profileID)
	}
	return strings.TrimSpace(profile.ExecutorID), nil
}

func (s *Service) workspaceDefaultExecutorID(ctx context.Context, workspaceID string) (string, error) {
	if s.workspaces == nil {
		return "", nil
	}
	workspace, err := s.workspaces.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return "", fmt.Errorf("%w: resolve workspace executor: %v", ErrInvalidInitialWorkspaceLayout, err)
	}
	if workspace == nil || workspace.DefaultExecutorID == nil {
		return "", nil
	}
	return strings.TrimSpace(*workspace.DefaultExecutorID), nil
}

func (s *Service) executorTypeFromID(ctx context.Context, executorID string) (string, error) {
	if s.executors == nil {
		return "", fmt.Errorf("%w: executor lookup is unavailable", ErrInvalidInitialWorkspaceLayout)
	}
	executor, err := s.executors.GetExecutor(ctx, executorID)
	if err != nil {
		return "", fmt.Errorf("%w: resolve executor %q: %v", ErrInvalidInitialWorkspaceLayout, executorID, err)
	}
	if executor == nil || strings.TrimSpace(string(executor.Type)) == "" {
		return "", fmt.Errorf("%w: executor %q has no type", ErrInvalidInitialWorkspaceLayout, executorID)
	}
	return string(executor.Type), nil
}

func requestExecutorSelectionValue(values map[string]interface{}, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

const workspaceExecutorTypeWorktree = "worktree"

func NormalizeInitialWorkspaceLayout(requested string, repositoryCount int, executorTypes ...string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" && requested != WorkspaceLayoutRepository && requested != WorkspaceLayoutTaskRoot {
		return "", fmt.Errorf("%w: %q", ErrInvalidInitialWorkspaceLayout, requested)
	}
	if repositoryCount == 0 {
		if requested == WorkspaceLayoutTaskRoot {
			return "", fmt.Errorf("%w: task_root requires at least one repository", ErrInvalidInitialWorkspaceLayout)
		}
		return "", nil
	}
	executorType := workspaceExecutorTypeWorktree
	if len(executorTypes) > 0 && strings.TrimSpace(executorTypes[0]) != "" {
		executorType = strings.TrimSpace(executorTypes[0])
	}
	if requested == WorkspaceLayoutTaskRoot && executorType != workspaceExecutorTypeWorktree {
		return "", fmt.Errorf("%w: task_root is supported only by the Worktree executor (got %q)", ErrInvalidInitialWorkspaceLayout, executorType)
	}
	if repositoryCount > 1 {
		if executorType != workspaceExecutorTypeWorktree {
			return WorkspaceLayoutRepository, nil
		}
		return WorkspaceLayoutTaskRoot, nil
	}
	if requested == "" {
		return WorkspaceLayoutRepository, nil
	}
	return requested, nil
}
