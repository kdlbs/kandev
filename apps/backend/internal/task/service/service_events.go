package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	v1 "github.com/kandev/kandev/pkg/api/v1"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
)

// publishTaskEvent publishes task events to the event bus
func (s *Service) publishTaskEvent(ctx context.Context, eventType string, task *models.Task, oldState *v1.TaskState) {
	if s.eventBus == nil {
		return
	}

	data := map[string]interface{}{
		"task_id":          task.ID,
		"workflow_id":      task.WorkflowID,
		"workflow_step_id": task.WorkflowStepID,
		"title":            task.Title,
		"description":      task.Description,
		"state":            string(task.State),
		"priority":         task.Priority,
		"position":         task.Position,
		"created_at":       task.CreatedAt.Format(time.RFC3339),
		"updated_at":       task.UpdatedAt.Format(time.RFC3339),
	}

	// Fetch session count and primary session info for the task
	sessionCountMap, err := s.GetSessionCountsForTasks(ctx, []string{task.ID})
	if err == nil {
		if count, ok := sessionCountMap[task.ID]; ok {
			data["session_count"] = count
		}
	}

	primarySessionInfoMap, err := s.GetPrimarySessionInfoForTasks(ctx, []string{task.ID})
	if err == nil {
		if sessionInfo, ok := primarySessionInfoMap[task.ID]; ok && sessionInfo != nil {
			data["primary_session_id"] = sessionInfo.ID
			if sessionInfo.ReviewStatus != nil {
				data["review_status"] = *sessionInfo.ReviewStatus
			}
		}
	}

	if task.ArchivedAt != nil {
		data["archived_at"] = task.ArchivedAt.Format(time.RFC3339)
	}

	if len(task.Repositories) > 0 {
		data["repository_id"] = task.Repositories[0].RepositoryID
	}

	if task.Metadata != nil {
		data["metadata"] = task.Metadata
	}

	if oldState != nil {
		data["old_state"] = string(*oldState)
		data["new_state"] = string(task.State)
	}

	event := bus.NewEvent(eventType, "task-service", data)

	if err := s.eventBus.Publish(ctx, eventType, event); err != nil {
		s.logger.Error("failed to publish task event",
			zap.String("event_type", eventType),
			zap.String("task_id", task.ID),
			zap.Error(err))
	}
}

func (s *Service) publishEventToBus(ctx context.Context, eventType, resourceType, resourceID string, data map[string]interface{}) {
	event := bus.NewEvent(eventType, "task-service", data)
	if err := s.eventBus.Publish(ctx, eventType, event); err != nil {
		s.logger.Error("failed to publish "+resourceType+" event",
			zap.String("event_type", eventType),
			zap.String(resourceType+"_id", resourceID),
			zap.Error(err))
	}
}

func (s *Service) publishWorkspaceEvent(ctx context.Context, eventType string, workspace *models.Workspace) {
	if s.eventBus == nil || workspace == nil {
		return
	}

	data := map[string]interface{}{
		"id":                       workspace.ID,
		"name":                     workspace.Name,
		"description":              workspace.Description,
		"owner_id":                 workspace.OwnerID,
		"default_executor_id":      workspace.DefaultExecutorID,
		"default_environment_id":   workspace.DefaultEnvironmentID,
		"default_agent_profile_id": workspace.DefaultAgentProfileID,
		"created_at":               workspace.CreatedAt.Format(time.RFC3339),
		"updated_at":               workspace.UpdatedAt.Format(time.RFC3339),
	}

	s.publishEventToBus(ctx, eventType, "workspace", workspace.ID, data)
}

func (s *Service) publishWorkflowEvent(ctx context.Context, eventType string, workflow *models.Workflow) {
	if s.eventBus == nil || workflow == nil {
		return
	}

	data := map[string]interface{}{
		"id":           workflow.ID,
		"workspace_id": workflow.WorkspaceID,
		"name":         workflow.Name,
		"description":  workflow.Description,
		"created_at":   workflow.CreatedAt.Format(time.RFC3339),
		"updated_at":   workflow.UpdatedAt.Format(time.RFC3339),
	}

	s.publishEventToBus(ctx, eventType, "workflow", workflow.ID, data)
}

func (s *Service) publishExecutorEvent(ctx context.Context, eventType string, executor *models.Executor) {
	if s.eventBus == nil || executor == nil {
		return
	}

	data := map[string]interface{}{
		"id":         executor.ID,
		"name":       executor.Name,
		"type":       executor.Type,
		"status":     executor.Status,
		"is_system":  executor.IsSystem,
		"resumable":  executor.Resumable,
		"config":     executor.Config,
		"created_at": executor.CreatedAt.Format(time.RFC3339),
		"updated_at": executor.UpdatedAt.Format(time.RFC3339),
	}

	s.publishEventToBus(ctx, eventType, "executor", executor.ID, data)
}

func (s *Service) publishExecutorProfileEvent(ctx context.Context, eventType string, profile *models.ExecutorProfile) {
	if s.eventBus == nil || profile == nil {
		return
	}
	data := map[string]interface{}{
		"id":           profile.ID,
		"executor_id":  profile.ExecutorID,
		"name":         profile.Name,
		"is_default":   profile.IsDefault,
		"config":       profile.Config,
		"setup_script": profile.SetupScript,
		"created_at":   profile.CreatedAt.Format(time.RFC3339),
		"updated_at":   profile.UpdatedAt.Format(time.RFC3339),
	}
	s.publishEventToBus(ctx, eventType, "executor_profile", profile.ID, data)
}

func (s *Service) publishEnvironmentEvent(ctx context.Context, eventType string, environment *models.Environment) {
	if s.eventBus == nil || environment == nil {
		return
	}

	data := map[string]interface{}{
		"id":            environment.ID,
		"name":          environment.Name,
		"kind":          environment.Kind,
		"is_system":     environment.IsSystem,
		"worktree_root": environment.WorktreeRoot,
		"image_tag":     environment.ImageTag,
		"dockerfile":    environment.Dockerfile,
		"build_config":  environment.BuildConfig,
		"created_at":    environment.CreatedAt.Format(time.RFC3339),
		"updated_at":    environment.UpdatedAt.Format(time.RFC3339),
	}

	s.publishEventToBus(ctx, eventType, "environment", environment.ID, data)
}

// publishMessageEvent publishes message events to the event bus.
// System-injected content (wrapped in <kandev-system> tags) is stripped from the content
// so users don't see workflow step prompt modifications in the UI.
func (s *Service) publishMessageEvent(ctx context.Context, eventType string, message *models.Message) {
	if s.eventBus == nil {
		s.logger.Warn("publishMessageEvent: eventBus is nil, skipping")
		return
	}

	messageType := string(message.Type)
	if messageType == "" {
		messageType = "message"
	}

	data := map[string]interface{}{
		"message_id":     message.ID,
		"session_id":     message.TaskSessionID,
		"task_id":        message.TaskID,
		"turn_id":        message.TurnID,
		"author_type":    string(message.AuthorType),
		"author_id":      message.AuthorID,
		"content":        sysprompt.StripSystemContent(message.Content),
		"type":           messageType,
		"requests_input": message.RequestsInput,
		"created_at":     message.CreatedAt.Format(time.RFC3339),
	}

	if message.Metadata != nil {
		data["metadata"] = message.Metadata
	}

	event := bus.NewEvent(eventType, "task-service", data)

	if err := s.eventBus.Publish(ctx, eventType, event); err != nil {
		s.logger.Error("failed to publish message event",
			zap.String("event_type", eventType),
			zap.String("message_id", message.ID),
			zap.Error(err))
	}
}

func (s *Service) publishRepositoryEvent(ctx context.Context, eventType string, repository *models.Repository) {
	if s.eventBus == nil || repository == nil {
		return
	}
	data := map[string]interface{}{
		"id":                     repository.ID,
		"workspace_id":           repository.WorkspaceID,
		"name":                   repository.Name,
		"source_type":            repository.SourceType,
		"local_path":             repository.LocalPath,
		"provider":               repository.Provider,
		"provider_repo_id":       repository.ProviderRepoID,
		"provider_owner":         repository.ProviderOwner,
		"provider_name":          repository.ProviderName,
		"default_branch":         repository.DefaultBranch,
		"worktree_branch_prefix": repository.WorktreeBranchPrefix,
		"pull_before_worktree":   repository.PullBeforeWorktree,
		"setup_script":           repository.SetupScript,
		"cleanup_script":         repository.CleanupScript,
		"created_at":             repository.CreatedAt.Format(time.RFC3339),
		"updated_at":             repository.UpdatedAt.Format(time.RFC3339),
	}
	event := bus.NewEvent(eventType, "task-service", data)
	if err := s.eventBus.Publish(ctx, eventType, event); err != nil {
		s.logger.Error("failed to publish repository event",
			zap.String("event_type", eventType),
			zap.String("repository_id", repository.ID),
			zap.Error(err))
	}
}

func (s *Service) publishRepositoryScriptEvent(ctx context.Context, eventType string, script *models.RepositoryScript) {
	if s.eventBus == nil || script == nil {
		return
	}
	data := map[string]interface{}{
		"id":            script.ID,
		"repository_id": script.RepositoryID,
		"name":          script.Name,
		"command":       script.Command,
		"position":      script.Position,
		"created_at":    script.CreatedAt.Format(time.RFC3339),
		"updated_at":    script.UpdatedAt.Format(time.RFC3339),
	}
	event := bus.NewEvent(eventType, "task-service", data)
	if err := s.eventBus.Publish(ctx, eventType, event); err != nil {
		s.logger.Error("failed to publish repository script event",
			zap.String("event_type", eventType),
			zap.String("script_id", script.ID),
			zap.Error(err))
	}
}
