package service

import (
	"context"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
)

// ConfigChatRestartSelection captures the persisted launch choices, without
// consulting defaults that may have changed since this conversation began.
type ConfigChatRestartSelection struct {
	AgentProfileID    string
	ExecutorID        string
	ExecutorProfileID string
}

type ConfigChatRestartValidationError struct{ Code string }

func (e *ConfigChatRestartValidationError) Error() string { return e.Code }

func invalidConfigChatRestart(code string) error {
	return &ConfigChatRestartValidationError{Code: code}
}

func (s *Service) ValidateConfigChatRestart(ctx context.Context, workspaceID, taskID, sessionID string) (*ConfigChatRestartSelection, error) {
	if err := s.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeTaskWrite); err != nil {
		return nil, err
	}
	if err := s.AuthorizeTaskSessionPromptAccess(ctx, taskID, sessionID); err != nil {
		return nil, err
	}
	if err := s.AuthorizeSessionScope(ctx, sessionID, authz.ScopeSessionControl); err != nil {
		return nil, err
	}
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if err := validateConfigChatRestartTask(task, workspaceID); err != nil {
		return nil, err
	}
	session, err := s.configChatRestartSession(ctx, taskID, sessionID)
	if err != nil {
		return nil, err
	}
	selection := &ConfigChatRestartSelection{
		AgentProfileID:    firstNonEmptyWorkflowAgentOverrideValue(session.AgentProfileID, models.StringFromAny(task.Metadata[models.MetaKeyAgentProfileID])),
		ExecutorID:        firstNonEmptyWorkflowAgentOverrideValue(session.ExecutorID, models.StringFromAny(task.Metadata[models.MetaKeyExecutorID])),
		ExecutorProfileID: firstNonEmptyWorkflowAgentOverrideValue(session.ExecutorProfileID, models.StringFromAny(task.Metadata[models.MetaKeyExecutorProfileID])),
	}
	// An unselected executor is persisted as empty for the implicit host runtime.
	// Pin that choice explicitly so changed workspace defaults cannot redirect it.
	if selection.ExecutorID == "" && selection.ExecutorProfileID == "" {
		selection.ExecutorID = models.ExecutorIDLocal
	}
	return s.validateConfigChatRestartSelection(ctx, workspaceID, selection)
}

func validateConfigChatRestartTask(task *models.Task, workspaceID string) error {
	configMode, _ := task.Metadata["config_mode"].(bool)
	if !IsRestorableQuickChatTask(task) || !configMode || task.WorkspaceID != workspaceID ||
		task.ArchivedAt != nil || task.ParentID != "" || task.ProjectID != "" || task.IsFromOffice ||
		task.Origin == models.TaskOriginRoutine || task.Origin == models.TaskOriginAutomationTask || len(task.WorkspaceFolders) != 0 {
		return invalidConfigChatRestart("config_chat_restart_invalid_target")
	}
	return nil
}

func (s *Service) configChatRestartSession(ctx context.Context, taskID, sessionID string) (*models.TaskSession, error) {
	repositories, err := s.taskRepos.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return nil, err
	}
	sessions, err := s.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if len(repositories) != 0 || len(sessions) != 1 || sessions[0] == nil {
		return nil, invalidConfigChatRestart("config_chat_restart_invalid_target")
	}
	session := sessions[0]
	if session.ID != sessionID || session.TaskID != taskID || !session.IsPrimary || session.RepositoryID != "" {
		return nil, invalidConfigChatRestart("config_chat_restart_invalid_target")
	}
	return session, nil
}

func (s *Service) validateConfigChatRestartSelection(ctx context.Context, workspaceID string, selection *ConfigChatRestartSelection) (*ConfigChatRestartSelection, error) {
	profile, err := s.loadWorkflowChangeProfile(ctx, workspaceID, selection.AgentProfileID, "")
	if selection.AgentProfileID == "" || err != nil {
		return nil, invalidConfigChatRestart("config_chat_restart_profile_unavailable")
	}
	if s.executors == nil {
		return nil, invalidConfigChatRestart("config_chat_restart_executor_unavailable")
	}
	executorProfile, executorID, err := s.resolveWorkflowAgentOverrideExecutorProfile(ctx, selection.ExecutorID, selection.ExecutorProfileID)
	if err != nil || executorID == "" {
		return nil, invalidConfigChatRestart("config_chat_restart_executor_unavailable")
	}
	executor, err := s.executors.GetExecutor(ctx, executorID)
	if err != nil || executor == nil || executor.DeletedAt != nil || executor.Status == models.ExecutorStatusDisabled {
		return nil, invalidConfigChatRestart("config_chat_restart_executor_unavailable")
	}
	if s.agentProfileExecutorValidator == nil || s.agentProfileExecutorValidator.ValidateAgentProfileForExecutor(ctx, profile, executor, executorProfile) != nil {
		return nil, invalidConfigChatRestart("config_chat_restart_incompatible")
	}
	selection.ExecutorID = executorID
	return selection, nil
}
