package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/office/models"
)

// CommentWriter is the comment mutation dependency used by runtime actions.
type CommentWriter interface {
	CreateComment(ctx context.Context, comment *models.TaskComment) error
}

// TaskCreator is the task mutation dependency used by runtime actions.
type TaskCreator interface {
	CreateOfficeSubtaskAsAgent(
		ctx context.Context,
		callerAgentID string,
		parentTaskID string,
		assigneeAgentID string,
		title string,
		description string,
	) (string, error)
}

// TaskStatusUpdater is the task status mutation dependency used by runtime actions.
type TaskStatusUpdater interface {
	UpdateTaskStatusAsAgent(ctx context.Context, update TaskStatusUpdate) error
}

// AgentCreator is the agent mutation dependency used by runtime actions.
type AgentCreator interface {
	CreateAgentInstanceWithCaller(
		ctx context.Context,
		agent *models.AgentInstance,
		callerAgent *models.AgentInstance,
		reason string,
	) error
}

// ApprovalRequester is the approval creation dependency used by runtime actions.
type ApprovalRequester interface {
	CreateApprovalWithActivity(ctx context.Context, approval *models.Approval) error
}

// RunSpawner is the run queue dependency used by runtime actions.
type RunSpawner interface {
	QueueRun(ctx context.Context, agentInstanceID, reason, payload, idempotencyKey string) error
}

// AgentModifier is the agent update dependency used by runtime actions.
type AgentModifier interface {
	GetAgentInstance(ctx context.Context, idOrName string) (*models.AgentInstance, error)
	UpdateAgentInstance(ctx context.Context, agent *models.AgentInstance) error
}

// SkillManager is the skill mutation dependency used by runtime actions.
type SkillManager interface {
	GetSkill(ctx context.Context, id string) (*models.Skill, error)
	DeleteSkill(ctx context.Context, id string) error
}

// ActionDependencies groups service dependencies for runtime actions.
type ActionDependencies struct {
	Comments      CommentWriter
	Tasks         TaskCreator
	TaskStatus    TaskStatusUpdater
	Agents        AgentCreator
	Approvals     ApprovalRequester
	Runs          RunSpawner
	AgentModifier AgentModifier
	Skills        SkillManager
}

// Actions exposes the syscall-style mutation surface for an Office agent run.
type Actions struct {
	deps ActionDependencies
}

// NewActions creates an agent runtime action surface.
func NewActions(deps ActionDependencies) *Actions {
	return &Actions{deps: deps}
}

// PostComment records an agent-authored task comment when the run is scoped for it.
func (a *Actions) PostComment(ctx context.Context, runCtx RunContext, taskID, body string) error {
	if !runCtx.Capabilities.Allows(CapabilityPostComment) {
		return ErrCapabilityDenied
	}
	if !runCtx.CanMutateTask(taskID) {
		return ErrTaskOutOfScope
	}
	if a.deps.Comments == nil {
		return fmt.Errorf("%w: comments", ErrRuntimeDependencyMissing)
	}
	comment := &models.TaskComment{
		ID:         uuid.New().String(),
		TaskID:     taskID,
		AuthorType: "agent",
		AuthorID:   runCtx.AgentID,
		Body:       body,
		Source:     "agent",
		CreatedAt:  time.Now(),
	}
	return a.deps.Comments.CreateComment(ctx, comment)
}

// TaskStatusUpdate carries a status mutation plus the originating run identity.
type TaskStatusUpdate struct {
	TaskID       string
	NewStatus    string
	Comment      string
	ActorAgentID string
	RunID        string
	SessionID    string
}

// UpdateTaskStatus updates a task status when the run is scoped for that task.
func (a *Actions) UpdateTaskStatus(
	ctx context.Context,
	runCtx RunContext,
	taskID string,
	status string,
	comment string,
) error {
	if !runCtx.Capabilities.Allows(CapabilityUpdateTaskStatus) {
		return ErrCapabilityDenied
	}
	if !runCtx.CanMutateTask(taskID) {
		return ErrTaskOutOfScope
	}
	if a.deps.TaskStatus == nil {
		return fmt.Errorf("%w: task status", ErrRuntimeDependencyMissing)
	}
	return a.deps.TaskStatus.UpdateTaskStatusAsAgent(ctx, TaskStatusUpdate{
		TaskID:       taskID,
		NewStatus:    status,
		Comment:      comment,
		ActorAgentID: runCtx.AgentID,
		RunID:        runCtx.RunID,
		SessionID:    runCtx.SessionID,
	})
}

// CreateSubtaskInput contains the fields an agent may supply for a new subtask.
type CreateSubtaskInput struct {
	ParentTaskID    string `json:"parent_task_id"`
	AssigneeAgentID string `json:"assignee_agent_id"`
	Title           string `json:"title"`
	Description     string `json:"description"`
}

// CreateSubtask creates a child task using the current agent as the caller.
func (a *Actions) CreateSubtask(
	ctx context.Context,
	runCtx RunContext,
	input CreateSubtaskInput,
) (string, error) {
	if !runCtx.Capabilities.Allows(CapabilityCreateSubtask) {
		return "", ErrCapabilityDenied
	}
	parentTaskID := input.ParentTaskID
	if parentTaskID == "" {
		parentTaskID = runCtx.TaskID
	}
	if !runCtx.CanMutateTask(parentTaskID) {
		return "", ErrTaskOutOfScope
	}
	if a.deps.Tasks == nil {
		return "", fmt.Errorf("%w: tasks", ErrRuntimeDependencyMissing)
	}
	return a.deps.Tasks.CreateOfficeSubtaskAsAgent(
		ctx,
		runCtx.AgentID,
		parentTaskID,
		input.AssigneeAgentID,
		input.Title,
		input.Description,
	)
}

// CreateAgentInput contains fields an agent may provide when creating a new agent.
type CreateAgentInput struct {
	Name                  string `json:"name"`
	Role                  string `json:"role"`
	ReportsTo             string `json:"reports_to"`
	Reason                string `json:"reason"`
	Permissions           string `json:"permissions"`
	DesiredSkills         string `json:"desired_skills"`
	ExecutorPreference    string `json:"executor_preference"`
	BudgetMonthlyCents    int    `json:"budget_monthly_cents"`
	MaxConcurrentSessions int    `json:"max_concurrent_sessions"`
}

// CreateAgent creates an Office agent using the current run agent as caller.
func (a *Actions) CreateAgent(
	ctx context.Context,
	runCtx RunContext,
	caller *models.AgentInstance,
	input CreateAgentInput,
) (*models.AgentInstance, error) {
	if !runCtx.Capabilities.Allows(CapabilityCreateAgent) {
		return nil, ErrCapabilityDenied
	}
	if a.deps.Agents == nil {
		return nil, fmt.Errorf("%w: agents", ErrRuntimeDependencyMissing)
	}
	reportsTo := input.ReportsTo
	if reportsTo == "" {
		reportsTo = runCtx.AgentID
	}
	agent := &models.AgentInstance{
		WorkspaceID:           runCtx.WorkspaceID,
		Name:                  input.Name,
		Role:                  models.AgentRole(input.Role),
		ReportsTo:             reportsTo,
		Permissions:           input.Permissions,
		DesiredSkills:         input.DesiredSkills,
		ExecutorPreference:    input.ExecutorPreference,
		BudgetMonthlyCents:    input.BudgetMonthlyCents,
		MaxConcurrentSessions: input.MaxConcurrentSessions,
	}
	if err := a.deps.Agents.CreateAgentInstanceWithCaller(ctx, agent, caller, input.Reason); err != nil {
		return nil, err
	}
	return agent, nil
}

// RequestApprovalInput contains fields for a runtime-created approval request.
type RequestApprovalInput struct {
	Type       string                 `json:"type"`
	TargetType string                 `json:"target_type"`
	TargetID   string                 `json:"target_id"`
	Reason     string                 `json:"reason"`
	Payload    map[string]interface{} `json:"payload"`
}

// RequestApproval creates a pending approval under the current run identity.
func (a *Actions) RequestApproval(
	ctx context.Context,
	runCtx RunContext,
	input RequestApprovalInput,
) (*models.Approval, error) {
	if !runCtx.Capabilities.Allows(CapabilityRequestApproval) {
		return nil, ErrCapabilityDenied
	}
	if a.deps.Approvals == nil {
		return nil, fmt.Errorf("%w: approvals", ErrRuntimeDependencyMissing)
	}
	payload, err := approvalPayload(runCtx, input)
	if err != nil {
		return nil, err
	}
	approval := &models.Approval{
		WorkspaceID:               runCtx.WorkspaceID,
		Type:                      input.Type,
		RequestedByAgentProfileID: runCtx.AgentID,
		Payload:                   payload,
	}
	if err := a.deps.Approvals.CreateApprovalWithActivity(ctx, approval); err != nil {
		return nil, err
	}
	return approval, nil
}

// SpawnAgentRunInput contains fields for queuing another agent run.
type SpawnAgentRunInput struct {
	AgentID        string                 `json:"agent_id"`
	Reason         string                 `json:"reason"`
	Payload        map[string]interface{} `json:"payload"`
	IdempotencyKey string                 `json:"idempotency_key"`
}

// SpawnAgentRun queues a run for an agent in the same workspace.
func (a *Actions) SpawnAgentRun(
	ctx context.Context,
	runCtx RunContext,
	input SpawnAgentRunInput,
) error {
	if !runCtx.Capabilities.Allows(CapabilitySpawnAgentRun) {
		return ErrCapabilityDenied
	}
	if a.deps.Runs == nil || a.deps.AgentModifier == nil {
		return fmt.Errorf("%w: runs", ErrRuntimeDependencyMissing)
	}
	target, err := a.deps.AgentModifier.GetAgentInstance(ctx, input.AgentID)
	if err != nil {
		return err
	}
	if target.WorkspaceID != runCtx.WorkspaceID {
		return ErrWorkspaceOutOfScope
	}
	payloadMap := input.Payload
	if payloadMap == nil {
		payloadMap = map[string]interface{}{}
	}
	payload, err := json.Marshal(payloadMap)
	if err != nil {
		return err
	}
	return a.deps.Runs.QueueRun(ctx, target.ID, input.Reason, string(payload), input.IdempotencyKey)
}

// ModifyAgentInput contains agent fields an authorized runtime may update.
type ModifyAgentInput struct {
	Name                  *string `json:"name,omitempty"`
	Role                  *string `json:"role,omitempty"`
	ReportsTo             *string `json:"reports_to,omitempty"`
	Permissions           *string `json:"permissions,omitempty"`
	DesiredSkills         *string `json:"desired_skills,omitempty"`
	ExecutorPreference    *string `json:"executor_preference,omitempty"`
	BudgetMonthlyCents    *int    `json:"budget_monthly_cents,omitempty"`
	MaxConcurrentSessions *int    `json:"max_concurrent_sessions,omitempty"`
}

// ModifyAgent updates another agent in the same workspace.
func (a *Actions) ModifyAgent(
	ctx context.Context,
	runCtx RunContext,
	agentID string,
	input ModifyAgentInput,
) (*models.AgentInstance, error) {
	if !runCtx.Capabilities.Allows(CapabilityModifyAgents) {
		return nil, ErrCapabilityDenied
	}
	if a.deps.AgentModifier == nil {
		return nil, fmt.Errorf("%w: agents", ErrRuntimeDependencyMissing)
	}
	agent, err := a.deps.AgentModifier.GetAgentInstance(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if agent.WorkspaceID != runCtx.WorkspaceID {
		return nil, ErrWorkspaceOutOfScope
	}
	applyAgentPatch(agent, input)
	if err := a.deps.AgentModifier.UpdateAgentInstance(ctx, agent); err != nil {
		return nil, err
	}
	return agent, nil
}

// DeleteSkill removes a skill from the current workspace.
func (a *Actions) DeleteSkill(ctx context.Context, runCtx RunContext, skillID string) error {
	if !runCtx.Capabilities.Allows(CapabilityDeleteSkills) {
		return ErrCapabilityDenied
	}
	if a.deps.Skills == nil {
		return fmt.Errorf("%w: skills", ErrRuntimeDependencyMissing)
	}
	skill, err := a.deps.Skills.GetSkill(ctx, skillID)
	if err != nil {
		return err
	}
	if skill.WorkspaceID != runCtx.WorkspaceID {
		return ErrWorkspaceOutOfScope
	}
	return a.deps.Skills.DeleteSkill(ctx, skillID)
}

func approvalPayload(runCtx RunContext, input RequestApprovalInput) (string, error) {
	payload := map[string]interface{}{}
	for k, v := range input.Payload {
		payload[k] = v
	}
	payload["target_type"] = input.TargetType
	payload["target_id"] = input.TargetID
	payload["reason"] = input.Reason
	payload["task_id"] = runCtx.TaskID
	payload["run_id"] = runCtx.RunID
	payload["session_id"] = runCtx.SessionID
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func applyAgentPatch(agent *models.AgentInstance, input ModifyAgentInput) {
	if input.Name != nil {
		agent.Name = *input.Name
	}
	if input.Role != nil {
		agent.Role = models.AgentRole(*input.Role)
	}
	if input.ReportsTo != nil {
		agent.ReportsTo = *input.ReportsTo
	}
	if input.Permissions != nil {
		agent.Permissions = *input.Permissions
	}
	if input.DesiredSkills != nil {
		agent.DesiredSkills = *input.DesiredSkills
	}
	if input.ExecutorPreference != nil {
		agent.ExecutorPreference = *input.ExecutorPreference
	}
	if input.BudgetMonthlyCents != nil {
		agent.BudgetMonthlyCents = *input.BudgetMonthlyCents
	}
	if input.MaxConcurrentSessions != nil {
		agent.MaxConcurrentSessions = *input.MaxConcurrentSessions
	}
}
