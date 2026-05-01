package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	v1 "github.com/kandev/kandev/pkg/api/v1"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
)

// PublishTaskUpdated publishes a task.updated event for the given task.
// Used when task metadata changes (e.g., primary session assignment) that
// don't go through the normal UpdateTask path.
func (s *Service) PublishTaskUpdated(ctx context.Context, task *models.Task) {
	s.publishTaskEvent(ctx, events.TaskUpdated, task, nil)
}

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
		"is_ephemeral":     task.IsEphemeral,
	}

	s.addTaskSessionEventFields(ctx, task.ID, data)

	if task.ParentID != "" {
		data["parent_id"] = task.ParentID
	}
	if task.ArchivedAt != nil {
		data["archived_at"] = task.ArchivedAt.Format(time.RFC3339)
	}
	// Orchestrator-originated events fetch the task via the raw repo.GetTask,
	// which does not populate Repositories. Load on demand so the payload
	// carries the full per-task repository list — matching the HTTP DTO and
	// preventing the frontend from collapsing multi-repo tasks down to the
	// primary repo on WS updates.
	repos := taskRepositoriesForEvent(ctx, s, task)
	if len(repos) > 0 {
		data["repository_id"] = repos[0].RepositoryID
		data["repositories"] = serializeTaskRepositories(repos)
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

// addTaskSessionEventFields merges session count, primary session info, and
// primary executor details into the task event payload. Extracted to keep
// publishTaskEvent under the project's function-length limit.
func (s *Service) addTaskSessionEventFields(ctx context.Context, taskID string, data map[string]interface{}) {
	if sessionCountMap, err := s.GetSessionCountsForTasks(ctx, []string{taskID}); err == nil {
		if count, ok := sessionCountMap[taskID]; ok {
			data["session_count"] = count
		}
	}

	primarySessionInfoMap, err := s.GetPrimarySessionInfoForTasks(ctx, []string{taskID})
	if err != nil {
		return
	}
	sessionInfo, ok := primarySessionInfoMap[taskID]
	if !ok || sessionInfo == nil {
		return
	}
	data["primary_session_id"] = sessionInfo.ID
	if sessionInfo.ReviewStatus != nil {
		data["review_status"] = *sessionInfo.ReviewStatus
	}
	if sessionInfo.State != "" {
		data["primary_session_state"] = string(sessionInfo.State)
	}
	if sessionInfo.ExecutorID != "" {
		data["primary_executor_id"] = sessionInfo.ExecutorID
	}
	var execType string
	if sessionInfo.ExecutorSnapshot != nil {
		if t, ok := sessionInfo.ExecutorSnapshot["executor_type"].(string); ok && t != "" {
			execType = t
			data["primary_executor_type"] = t
		}
		if n, ok := sessionInfo.ExecutorSnapshot["executor_name"].(string); ok && n != "" {
			data["primary_executor_name"] = n
		}
	}
	if execType != "" {
		data["is_remote_executor"] = models.IsRemoteExecutorType(models.ExecutorType(execType))
	}
}

// taskRepositoriesForEvent returns the task's full repository list, ordered by
// position. Prefers Task.Repositories when already loaded; falls back to a
// lookup so publishers that pass a task without eagerly loaded repositories
// (e.g. the orchestrator's raw repo.GetTask) still emit per-repo data.
func taskRepositoriesForEvent(ctx context.Context, s *Service, task *models.Task) []*models.TaskRepository {
	if len(task.Repositories) > 0 {
		return task.Repositories
	}
	repos, err := s.taskRepos.ListTaskRepositories(ctx, task.ID)
	if err != nil {
		return nil
	}
	return repos
}

// serializeTaskRepositories returns the WS-shaped repositories array. Mirrors
// the HTTP DTO's TaskRepositoryDTO so the frontend can use the same parser
// across SSR snapshot and WS update paths.
func serializeTaskRepositories(repos []*models.TaskRepository) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(repos))
	for _, r := range repos {
		out = append(out, map[string]interface{}{
			"id":              r.ID,
			"task_id":         r.TaskID,
			"repository_id":   r.RepositoryID,
			"base_branch":     r.BaseBranch,
			"checkout_branch": r.CheckoutBranch,
			"position":        r.Position,
		})
	}
	return out
}

// publishTaskMovedEvent publishes a task.moved event so the orchestrator can process
// on_exit/on_enter actions for the new workflow step.
func (s *Service) publishTaskMovedEvent(ctx context.Context, task *models.Task, fromStepID, toStepID, sessionID string) {
	if s.eventBus == nil {
		return
	}
	data := map[string]interface{}{
		"task_id":          task.ID,
		"from_step_id":     fromStepID,
		"to_step_id":       toStepID,
		"session_id":       sessionID,
		"workflow_id":      task.WorkflowID,
		"task_description": task.Description,
	}
	event := bus.NewEvent(events.TaskMoved, "task-service", data)
	if err := s.eventBus.Publish(ctx, events.TaskMoved, event); err != nil {
		s.logger.Error("failed to publish task.moved event",
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
		"id":                              workspace.ID,
		"name":                            workspace.Name,
		"description":                     workspace.Description,
		"owner_id":                        workspace.OwnerID,
		"default_executor_id":             workspace.DefaultExecutorID,
		"default_environment_id":          workspace.DefaultEnvironmentID,
		"default_agent_profile_id":        workspace.DefaultAgentProfileID,
		"default_config_agent_profile_id": workspace.DefaultConfigAgentProfileID,
		"created_at":                      workspace.CreatedAt.Format(time.RFC3339),
		"updated_at":                      workspace.UpdatedAt.Format(time.RFC3339),
	}

	s.publishEventToBus(ctx, eventType, "workspace", workspace.ID, data)
}

func (s *Service) publishWorkflowEvent(ctx context.Context, eventType string, workflow *models.Workflow) {
	if s.eventBus == nil || workflow == nil {
		return
	}

	data := map[string]interface{}{
		"id":               workflow.ID,
		"workspace_id":     workflow.WorkspaceID,
		"name":             workflow.Name,
		"description":      workflow.Description,
		"agent_profile_id": workflow.AgentProfileID,
		"created_at":       workflow.CreatedAt.Format(time.RFC3339),
		"updated_at":       workflow.UpdatedAt.Format(time.RFC3339),
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
		"id":             profile.ID,
		"executor_id":    profile.ExecutorID,
		"name":           profile.Name,
		"mcp_policy":     profile.McpPolicy,
		"config":         profile.Config,
		"prepare_script": profile.PrepareScript,
		"cleanup_script": profile.CleanupScript,
		"created_at":     profile.CreatedAt.Format(time.RFC3339),
		"updated_at":     profile.UpdatedAt.Format(time.RFC3339),
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
// Only true system-injected content (wrapped in <kandev-system> tags) is stripped
// from the visible message content delivered to clients.
func (s *Service) publishMessageEvent(ctx context.Context, eventType string, message *models.Message) {
	if s.eventBus == nil {
		s.logger.Warn("publishMessageEvent: eventBus is nil, skipping")
		return
	}

	messageType := string(message.Type)
	if messageType == "" {
		messageType = "message"
	}

	hasHidden := sysprompt.HasSystemContent(message.Content)
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

	if hasHidden {
		data["raw_content"] = message.Content
	}

	meta := message.Metadata
	if hasHidden {
		if meta == nil {
			meta = make(map[string]interface{})
		} else {
			cp := make(map[string]interface{}, len(meta))
			for k, v := range meta {
				cp[k] = v
			}
			meta = cp
		}
		meta["has_hidden_prompts"] = true
	}
	if meta != nil {
		data["metadata"] = meta
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
