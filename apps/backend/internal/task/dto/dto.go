package dto

import (
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type WorkflowDTO struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type WorkspaceDTO struct {
	ID                    string    `json:"id"`
	Name                  string    `json:"name"`
	Description           *string   `json:"description,omitempty"`
	OwnerID               string    `json:"owner_id"`
	DefaultExecutorID     *string   `json:"default_executor_id,omitempty"`
	DefaultEnvironmentID  *string   `json:"default_environment_id,omitempty"`
	DefaultAgentProfileID *string   `json:"default_agent_profile_id,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type RepositoryDTO struct {
	ID                   string                `json:"id"`
	WorkspaceID          string                `json:"workspace_id"`
	Name                 string                `json:"name"`
	SourceType           string                `json:"source_type"`
	LocalPath            string                `json:"local_path"`
	Provider             string                `json:"provider"`
	ProviderRepoID       string                `json:"provider_repo_id"`
	ProviderOwner        string                `json:"provider_owner"`
	ProviderName         string                `json:"provider_name"`
	DefaultBranch        string                `json:"default_branch"`
	WorktreeBranchPrefix string                `json:"worktree_branch_prefix"`
	PullBeforeWorktree   bool                  `json:"pull_before_worktree"`
	SetupScript          string                `json:"setup_script"`
	CleanupScript        string                `json:"cleanup_script"`
	DevScript            string                `json:"dev_script"`
	CreatedAt            time.Time             `json:"created_at"`
	UpdatedAt            time.Time             `json:"updated_at"`
	Scripts              []RepositoryScriptDTO `json:"scripts,omitempty"`
}

type RepositoryScriptDTO struct {
	ID           string    `json:"id"`
	RepositoryID string    `json:"repository_id"`
	Name         string    `json:"name"`
	Command      string    `json:"command"`
	Position     int       `json:"position"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ExecutorDTO struct {
	ID        string                `json:"id"`
	Name      string                `json:"name"`
	Type      models.ExecutorType   `json:"type"`
	Status    models.ExecutorStatus `json:"status"`
	IsSystem  bool                  `json:"is_system"`
	Resumable bool                  `json:"resumable"`
	Config    map[string]string     `json:"config,omitempty"`
	CreatedAt time.Time             `json:"created_at"`
	UpdatedAt time.Time             `json:"updated_at"`
}

type EnvironmentDTO struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Kind         models.EnvironmentKind `json:"kind"`
	IsSystem     bool                   `json:"is_system"`
	WorktreeRoot string                 `json:"worktree_root,omitempty"`
	ImageTag     string                 `json:"image_tag,omitempty"`
	Dockerfile   string                 `json:"dockerfile,omitempty"`
	BuildConfig  map[string]string      `json:"build_config,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

type TaskDTO struct {
	ID               string                 `json:"id"`
	WorkspaceID      string                 `json:"workspace_id"`
	WorkflowID       string                 `json:"workflow_id"`
	WorkflowStepID   string                 `json:"workflow_step_id"`
	Title            string                 `json:"title"`
	Description      string                 `json:"description"`
	State            v1.TaskState           `json:"state"`
	Priority         int                    `json:"priority"`
	Repositories     []TaskRepositoryDTO    `json:"repositories,omitempty"`
	Position         int                    `json:"position"`
	PrimarySessionID *string                `json:"primary_session_id,omitempty"`
	SessionCount     *int                   `json:"session_count,omitempty"`
	ReviewStatus     *string                `json:"review_status,omitempty"`
	ArchivedAt       *time.Time             `json:"archived_at,omitempty"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

type TaskRepositoryDTO struct {
	ID           string                 `json:"id"`
	TaskID       string                 `json:"task_id"`
	RepositoryID string                 `json:"repository_id"`
	BaseBranch   string                 `json:"base_branch"`
	Position     int                    `json:"position"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

type TaskSessionDTO struct {
	ID                   string                  `json:"id"`
	TaskID               string                  `json:"task_id"`
	AgentExecutionID     string                  `json:"agent_execution_id,omitempty"`
	ContainerID          string                  `json:"container_id,omitempty"`
	AgentProfileID       string                  `json:"agent_profile_id,omitempty"`
	ExecutorID           string                  `json:"executor_id,omitempty"`
	EnvironmentID        string                  `json:"environment_id,omitempty"`
	RepositoryID         string                  `json:"repository_id,omitempty"`
	BaseBranch           string                  `json:"base_branch,omitempty"`
	WorktreeID           string                  `json:"worktree_id,omitempty"`
	WorktreePath         string                  `json:"worktree_path,omitempty"`
	WorktreeBranch       string                  `json:"worktree_branch,omitempty"`
	State                models.TaskSessionState `json:"state"`
	ErrorMessage         string                  `json:"error_message,omitempty"`
	Metadata             map[string]interface{}  `json:"metadata,omitempty"`
	AgentProfileSnapshot map[string]interface{}  `json:"agent_profile_snapshot,omitempty"`
	ExecutorSnapshot     map[string]interface{}  `json:"executor_snapshot,omitempty"`
	EnvironmentSnapshot  map[string]interface{}  `json:"environment_snapshot,omitempty"`
	RepositorySnapshot   map[string]interface{}  `json:"repository_snapshot,omitempty"`
	StartedAt            time.Time               `json:"started_at"`
	CompletedAt          *time.Time              `json:"completed_at,omitempty"`
	UpdatedAt            time.Time               `json:"updated_at"`
	// Workflow fields
	IsPrimary      bool    `json:"is_primary"`
	IsPassthrough  bool    `json:"is_passthrough"`
	WorkflowStepID *string `json:"workflow_step_id,omitempty"`
	ReviewStatus   *string `json:"review_status,omitempty"`
}

type GetTaskSessionResponse struct {
	Session TaskSessionDTO `json:"session"`
}

type ListTaskSessionsResponse struct {
	Sessions []TaskSessionDTO `json:"sessions"`
	Total    int              `json:"total"`
}

type WorkflowSnapshotDTO struct {
	Workflow WorkflowDTO       `json:"workflow"`
	Steps    []WorkflowStepDTO `json:"steps"`
	Tasks    []TaskDTO         `json:"tasks"`
}

type ListMessagesResponse struct {
	Messages []*v1.Message `json:"messages"`
	Total    int           `json:"total"`
	HasMore  bool          `json:"has_more"`
	Cursor   string        `json:"cursor"`
}

type TurnDTO struct {
	ID          string                 `json:"id"`
	SessionID   string                 `json:"session_id"`
	TaskID      string                 `json:"task_id"`
	StartedAt   string                 `json:"started_at"`
	CompletedAt *string                `json:"completed_at,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt   string                 `json:"created_at"`
	UpdatedAt   string                 `json:"updated_at"`
}

type ListTurnsResponse struct {
	Turns []TurnDTO `json:"turns"`
	Total int       `json:"total"`
}

type ListWorkflowsResponse struct {
	Workflows []WorkflowDTO `json:"workflows"`
	Total     int           `json:"total"`
}

type ListWorkspacesResponse struct {
	Workspaces []WorkspaceDTO `json:"workspaces"`
	Total      int            `json:"total"`
}

type ListRepositoriesResponse struct {
	Repositories []RepositoryDTO `json:"repositories"`
	Total        int             `json:"total"`
}

type ListRepositoryScriptsResponse struct {
	Scripts []RepositoryScriptDTO `json:"scripts"`
	Total   int                   `json:"total"`
}

type ListExecutorsResponse struct {
	Executors []ExecutorDTO `json:"executors"`
	Total     int           `json:"total"`
}

type ListEnvironmentsResponse struct {
	Environments []EnvironmentDTO `json:"environments"`
	Total        int              `json:"total"`
}

type BranchDTO struct {
	Name   string `json:"name"`
	Type   string `json:"type"`   // "local" or "remote"
	Remote string `json:"remote"` // remote name (e.g., "origin") for remote branches
}

type RepositoryBranchesResponse struct {
	Branches []BranchDTO `json:"branches"`
	Total    int         `json:"total"`
}

type LocalRepositoryDTO struct {
	Path          string `json:"path"`
	Name          string `json:"name"`
	DefaultBranch string `json:"default_branch,omitempty"`
}

type RepositoryDiscoveryResponse struct {
	Roots        []string             `json:"roots"`
	Repositories []LocalRepositoryDTO `json:"repositories"`
	Total        int                  `json:"total"`
}

type RepositoryPathValidationResponse struct {
	Path          string `json:"path"`
	Exists        bool   `json:"exists"`
	IsGitRepo     bool   `json:"is_git"`
	Allowed       bool   `json:"allowed"`
	DefaultBranch string `json:"default_branch,omitempty"`
	Message       string `json:"message,omitempty"`
}

type ListTasksResponse struct {
	Tasks []TaskDTO `json:"tasks"`
	Total int       `json:"total"`
}

type SuccessResponse struct {
	Success bool `json:"success"`
}

func FromWorkflow(workflow *models.Workflow) WorkflowDTO {
	var description *string
	if workflow.Description != "" {
		description = &workflow.Description
	}

	return WorkflowDTO{
		ID:          workflow.ID,
		WorkspaceID: workflow.WorkspaceID,
		Name:        workflow.Name,
		Description: description,
		CreatedAt:   workflow.CreatedAt,
		UpdatedAt:   workflow.UpdatedAt,
	}
}

func FromWorkspace(workspace *models.Workspace) WorkspaceDTO {
	var description *string
	if workspace.Description != "" {
		description = &workspace.Description
	}

	return WorkspaceDTO{
		ID:                    workspace.ID,
		Name:                  workspace.Name,
		Description:           description,
		OwnerID:               workspace.OwnerID,
		DefaultExecutorID:     workspace.DefaultExecutorID,
		DefaultEnvironmentID:  workspace.DefaultEnvironmentID,
		DefaultAgentProfileID: workspace.DefaultAgentProfileID,
		CreatedAt:             workspace.CreatedAt,
		UpdatedAt:             workspace.UpdatedAt,
	}
}

func FromRepository(repository *models.Repository) RepositoryDTO {
	return RepositoryDTO{
		ID:                   repository.ID,
		WorkspaceID:          repository.WorkspaceID,
		Name:                 repository.Name,
		SourceType:           repository.SourceType,
		LocalPath:            repository.LocalPath,
		Provider:             repository.Provider,
		ProviderRepoID:       repository.ProviderRepoID,
		ProviderOwner:        repository.ProviderOwner,
		ProviderName:         repository.ProviderName,
		DefaultBranch:        repository.DefaultBranch,
		WorktreeBranchPrefix: repository.WorktreeBranchPrefix,
		PullBeforeWorktree:   repository.PullBeforeWorktree,
		SetupScript:          repository.SetupScript,
		CleanupScript:        repository.CleanupScript,
		DevScript:            repository.DevScript,
		CreatedAt:            repository.CreatedAt,
		UpdatedAt:            repository.UpdatedAt,
	}
}

func FromRepositoryScript(script *models.RepositoryScript) RepositoryScriptDTO {
	return RepositoryScriptDTO{
		ID:           script.ID,
		RepositoryID: script.RepositoryID,
		Name:         script.Name,
		Command:      script.Command,
		Position:     script.Position,
		CreatedAt:    script.CreatedAt,
		UpdatedAt:    script.UpdatedAt,
	}
}

func FromExecutor(executor *models.Executor) ExecutorDTO {
	return ExecutorDTO{
		ID:        executor.ID,
		Name:      executor.Name,
		Type:      executor.Type,
		Status:    executor.Status,
		IsSystem:  executor.IsSystem,
		Resumable: executor.Resumable,
		Config:    executor.Config,
		CreatedAt: executor.CreatedAt,
		UpdatedAt: executor.UpdatedAt,
	}
}

func FromEnvironment(environment *models.Environment) EnvironmentDTO {
	return EnvironmentDTO{
		ID:           environment.ID,
		Name:         environment.Name,
		Kind:         environment.Kind,
		IsSystem:     environment.IsSystem,
		WorktreeRoot: environment.WorktreeRoot,
		ImageTag:     environment.ImageTag,
		Dockerfile:   environment.Dockerfile,
		BuildConfig:  environment.BuildConfig,
		CreatedAt:    environment.CreatedAt,
		UpdatedAt:    environment.UpdatedAt,
	}
}

func FromLocalRepository(repo service.LocalRepository) LocalRepositoryDTO {
	return LocalRepositoryDTO{
		Path:          repo.Path,
		Name:          repo.Name,
		DefaultBranch: repo.DefaultBranch,
	}
}

func FromTask(task *models.Task) TaskDTO {
	return FromTaskWithPrimarySession(task, nil)
}

// FromTaskWithPrimarySession converts a task model to a TaskDTO, including the primary session ID.
func FromTaskWithPrimarySession(task *models.Task, primarySessionID *string) TaskDTO {
	return FromTaskWithSessionInfo(task, primarySessionID, nil, nil)
}

// FromTaskWithSessionInfo converts a task model to a TaskDTO, including session information.
func FromTaskWithSessionInfo(task *models.Task, primarySessionID *string, sessionCount *int, reviewStatus *string) TaskDTO {
	// Convert repositories
	var repositories []TaskRepositoryDTO
	for _, repo := range task.Repositories {
		repositories = append(repositories, TaskRepositoryDTO{
			ID:           repo.ID,
			TaskID:       repo.TaskID,
			RepositoryID: repo.RepositoryID,
			BaseBranch:   repo.BaseBranch,
			Position:     repo.Position,
			Metadata:     repo.Metadata,
			CreatedAt:    repo.CreatedAt,
			UpdatedAt:    repo.UpdatedAt,
		})
	}

	return TaskDTO{
		ID:               task.ID,
		WorkspaceID:      task.WorkspaceID,
		WorkflowID:       task.WorkflowID,
		WorkflowStepID:   task.WorkflowStepID,
		Title:            task.Title,
		Description:      task.Description,
		State:            task.State,
		Priority:         task.Priority,
		Repositories:     repositories,
		Position:         task.Position,
		PrimarySessionID: primarySessionID,
		SessionCount:     sessionCount,
		ReviewStatus:     reviewStatus,
		ArchivedAt:       task.ArchivedAt,
		CreatedAt:        task.CreatedAt,
		UpdatedAt:        task.UpdatedAt,
		Metadata:         task.Metadata,
	}
}

func FromTaskSession(session *models.TaskSession) TaskSessionDTO {
	result := TaskSessionDTO{
		ID:                   session.ID,
		TaskID:               session.TaskID,
		AgentExecutionID:     session.AgentExecutionID,
		ContainerID:          session.ContainerID,
		AgentProfileID:       session.AgentProfileID,
		ExecutorID:           session.ExecutorID,
		EnvironmentID:        session.EnvironmentID,
		RepositoryID:         session.RepositoryID,
		BaseBranch:           session.BaseBranch,
		State:                session.State,
		ErrorMessage:         session.ErrorMessage,
		Metadata:             session.Metadata,
		AgentProfileSnapshot: session.AgentProfileSnapshot,
		ExecutorSnapshot:     session.ExecutorSnapshot,
		EnvironmentSnapshot:  session.EnvironmentSnapshot,
		RepositorySnapshot:   session.RepositorySnapshot,
		StartedAt:            session.StartedAt,
		CompletedAt:          session.CompletedAt,
		UpdatedAt:            session.UpdatedAt,
		// Workflow fields
		IsPrimary:      session.IsPrimary,
		IsPassthrough:  session.IsPassthrough,
		WorkflowStepID: session.WorkflowStepID,
		ReviewStatus:   session.ReviewStatus,
	}
	if len(session.Worktrees) > 0 {
		result.WorktreeID = session.Worktrees[0].WorktreeID
		result.WorktreePath = session.Worktrees[0].WorktreePath
		result.WorktreeBranch = session.Worktrees[0].WorktreeBranch
	}
	return result
}

// WorkflowStepDTO represents a workflow step for API responses
type WorkflowStepDTO struct {
	ID                    string         `json:"id"`
	WorkflowID            string         `json:"workflow_id"`
	Name                  string         `json:"name"`
	Position              int            `json:"position"`
	Color                 string         `json:"color"`
	Prompt                string         `json:"prompt,omitempty"`
	Events                *StepEventsDTO `json:"events,omitempty"`
	AllowManualMove       bool           `json:"allow_manual_move"`
	AutoArchiveAfterHours int            `json:"auto_archive_after_hours,omitempty"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
}

// StepEventsDTO represents step events for API responses
type StepEventsDTO struct {
	OnEnter        []StepActionDTO `json:"on_enter,omitempty"`
	OnTurnComplete []StepActionDTO `json:"on_turn_complete,omitempty"`
}

// StepActionDTO represents a step action for API responses
type StepActionDTO struct {
	Type   string                 `json:"type"`
	Config map[string]interface{} `json:"config,omitempty"`
}

// MoveTaskResponse includes the task and the target workflow step info
type MoveTaskResponse struct {
	Task         TaskDTO         `json:"task"`
	WorkflowStep WorkflowStepDTO `json:"workflow_step"`
}

// Session Workflow Review DTOs

// ApproveSessionRequest is the request to approve a session's current step
type ApproveSessionRequest struct {
	SessionID string `json:"-"` // From URL path
}

// ApproveSessionResponse is the response after approving a session
type ApproveSessionResponse struct {
	Success      bool            `json:"success"`
	Session      TaskSessionDTO  `json:"session"`
	WorkflowStep WorkflowStepDTO `json:"workflow_step,omitempty"` // New step after approval
}

// TaskPlanDTO represents a task plan for API responses
type TaskPlanDTO struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TaskPlanFromModel converts a TaskPlan model to a TaskPlanDTO.
func TaskPlanFromModel(plan *models.TaskPlan) *TaskPlanDTO {
	if plan == nil {
		return nil
	}
	return &TaskPlanDTO{
		ID:        plan.ID,
		TaskID:    plan.TaskID,
		Title:     plan.Title,
		Content:   plan.Content,
		CreatedBy: plan.CreatedBy,
		CreatedAt: plan.CreatedAt,
		UpdatedAt: plan.UpdatedAt,
	}
}

// FromTurn converts a Turn model to a TurnDTO.
func FromTurn(turn *models.Turn) TurnDTO {
	var completedAt *string
	if turn.CompletedAt != nil {
		formatted := turn.CompletedAt.UTC().Format(time.RFC3339)
		completedAt = &formatted
	}

	return TurnDTO{
		ID:          turn.ID,
		SessionID:   turn.TaskSessionID,
		TaskID:      turn.TaskID,
		StartedAt:   turn.StartedAt.UTC().Format(time.RFC3339),
		CompletedAt: completedAt,
		Metadata:    turn.Metadata,
		CreatedAt:   turn.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   turn.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
