// Package dto defines HTTP request/response types for the orchestrate API.
package dto

import "github.com/kandev/kandev/internal/orchestrate/models"

// -- Agent Instances --

// CreateAgentRequest is the request body for creating an agent instance.
type CreateAgentRequest struct {
	Name                  string `json:"name"`
	AgentProfileID        string `json:"agent_profile_id"`
	Role                  string `json:"role"`
	Icon                  string `json:"icon"`
	ReportsTo             string `json:"reports_to"`
	Permissions           string `json:"permissions"`
	BudgetMonthlyCents    int    `json:"budget_monthly_cents"`
	MaxConcurrentSessions int    `json:"max_concurrent_sessions"`
	DesiredSkills         string `json:"desired_skills"`
	ExecutorPreference    string `json:"executor_preference"`
}

// UpdateAgentRequest is the request body for updating an agent instance.
type UpdateAgentRequest struct {
	Name                  *string `json:"name,omitempty"`
	AgentProfileID        *string `json:"agent_profile_id,omitempty"`
	Role                  *string `json:"role,omitempty"`
	Icon                  *string `json:"icon,omitempty"`
	Status                *string `json:"status,omitempty"`
	ReportsTo             *string `json:"reports_to,omitempty"`
	Permissions           *string `json:"permissions,omitempty"`
	BudgetMonthlyCents    *int    `json:"budget_monthly_cents,omitempty"`
	MaxConcurrentSessions *int    `json:"max_concurrent_sessions,omitempty"`
	DesiredSkills         *string `json:"desired_skills,omitempty"`
	ExecutorPreference    *string `json:"executor_preference,omitempty"`
	PauseReason           *string `json:"pause_reason,omitempty"`
}

// UpdateAgentStatusRequest is the request body for changing agent status.
type UpdateAgentStatusRequest struct {
	Status      string `json:"status"`
	PauseReason string `json:"pause_reason"`
}

// AgentResponse wraps a single agent instance.
type AgentResponse struct {
	Agent *models.AgentInstance `json:"agent"`
}

// AgentListResponse wraps a list of agent instances.
type AgentListResponse struct {
	Agents []*models.AgentInstance `json:"agents"`
}

// -- Skills --

// CreateSkillRequest is the request body for creating a skill.
type CreateSkillRequest struct {
	Name                     string `json:"name"`
	Slug                     string `json:"slug"`
	Description              string `json:"description"`
	SourceType               string `json:"source_type"`
	SourceLocator            string `json:"source_locator"`
	Content                  string `json:"content"`
	FileInventory            string `json:"file_inventory"`
	CreatedByAgentInstanceID string `json:"created_by_agent_instance_id"`
}

// UpdateSkillRequest is the request body for updating a skill.
type UpdateSkillRequest struct {
	Name          *string `json:"name,omitempty"`
	Slug          *string `json:"slug,omitempty"`
	Description   *string `json:"description,omitempty"`
	SourceType    *string `json:"source_type,omitempty"`
	SourceLocator *string `json:"source_locator,omitempty"`
	Content       *string `json:"content,omitempty"`
	FileInventory *string `json:"file_inventory,omitempty"`
}

// SkillResponse wraps a single skill.
type SkillResponse struct {
	Skill *models.Skill `json:"skill"`
}

// SkillListResponse wraps a list of skills.
type SkillListResponse struct {
	Skills []*models.Skill `json:"skills"`
}

// ImportSkillRequest is the request body for importing a skill from a URL or path.
type ImportSkillRequest struct {
	Source string `json:"source"`
}

// ImportSkillResponse wraps the result of a skill import.
type ImportSkillResponse struct {
	Skills   []*models.Skill `json:"skills"`
	Warnings []string        `json:"warnings"`
}

// SkillFileResponse wraps the content of a skill file.
type SkillFileResponse struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// -- Projects --

// CreateProjectRequest is the request body for creating a project.
type CreateProjectRequest struct {
	Name                string `json:"name"`
	Description         string `json:"description"`
	LeadAgentInstanceID string `json:"lead_agent_instance_id"`
	Color               string `json:"color"`
	BudgetCents         int    `json:"budget_cents"`
	Repositories        string `json:"repositories"`
	ExecutorConfig      string `json:"executor_config"`
}

// UpdateProjectRequest is the request body for updating a project.
type UpdateProjectRequest struct {
	Name                *string `json:"name,omitempty"`
	Description         *string `json:"description,omitempty"`
	Status              *string `json:"status,omitempty"`
	LeadAgentInstanceID *string `json:"lead_agent_instance_id,omitempty"`
	Color               *string `json:"color,omitempty"`
	BudgetCents         *int    `json:"budget_cents,omitempty"`
	Repositories        *string `json:"repositories,omitempty"`
	ExecutorConfig      *string `json:"executor_config,omitempty"`
}

// ProjectResponse wraps a single project.
type ProjectResponse struct {
	Project *models.Project `json:"project"`
}

// ProjectListResponse wraps a list of projects.
type ProjectListResponse struct {
	Projects []*models.ProjectWithCounts `json:"projects"`
}

// -- Costs --

// CostListResponse wraps a list of cost events.
type CostListResponse struct {
	Costs []*models.CostEvent `json:"costs"`
}

// CostBreakdownResponse wraps aggregated cost data.
type CostBreakdownResponse struct {
	Breakdown []*models.CostBreakdown `json:"breakdown"`
}

// -- Budgets --

// CreateBudgetRequest is the request body for creating a budget policy.
type CreateBudgetRequest struct {
	ScopeType         string `json:"scope_type"`
	ScopeID           string `json:"scope_id"`
	LimitCents        int    `json:"limit_cents"`
	Period            string `json:"period"`
	AlertThresholdPct int    `json:"alert_threshold_pct"`
	ActionOnExceed    string `json:"action_on_exceed"`
}

// UpdateBudgetRequest is the request body for updating a budget policy.
type UpdateBudgetRequest struct {
	ScopeType         *string `json:"scope_type,omitempty"`
	ScopeID           *string `json:"scope_id,omitempty"`
	LimitCents        *int    `json:"limit_cents,omitempty"`
	Period            *string `json:"period,omitempty"`
	AlertThresholdPct *int    `json:"alert_threshold_pct,omitempty"`
	ActionOnExceed    *string `json:"action_on_exceed,omitempty"`
}

// BudgetListResponse wraps a list of budget policies.
type BudgetListResponse struct {
	Budgets []*models.BudgetPolicy `json:"budgets"`
}

// -- Routines --

// CreateRoutineRequest is the request body for creating a routine.
type CreateRoutineRequest struct {
	Name                    string `json:"name"`
	Description             string `json:"description"`
	TaskTemplate            string `json:"task_template"`
	AssigneeAgentInstanceID string `json:"assignee_agent_instance_id"`
	ConcurrencyPolicy       string `json:"concurrency_policy"`
	Variables               string `json:"variables"`
}

// UpdateRoutineRequest is the request body for updating a routine.
type UpdateRoutineRequest struct {
	Name                    *string `json:"name,omitempty"`
	Description             *string `json:"description,omitempty"`
	TaskTemplate            *string `json:"task_template,omitempty"`
	AssigneeAgentInstanceID *string `json:"assignee_agent_instance_id,omitempty"`
	Status                  *string `json:"status,omitempty"`
	ConcurrencyPolicy       *string `json:"concurrency_policy,omitempty"`
	Variables               *string `json:"variables,omitempty"`
}

// RoutineResponse wraps a single routine.
type RoutineResponse struct {
	Routine *models.Routine `json:"routine"`
}

// RoutineListResponse wraps a list of routines.
type RoutineListResponse struct {
	Routines []*models.Routine `json:"routines"`
}

// RunRoutineRequest is the request body for manually firing a routine.
type RunRoutineRequest struct {
	Variables map[string]string `json:"variables"`
}

// RoutineRunResponse wraps a single routine run.
type RoutineRunResponse struct {
	Run *models.RoutineRun `json:"run"`
}

// RunListResponse wraps a list of routine runs.
type RunListResponse struct {
	Runs []*models.RoutineRun `json:"runs"`
}

// CreateTriggerRequest is the request body for creating a routine trigger.
type CreateTriggerRequest struct {
	Kind           string `json:"kind"`
	CronExpression string `json:"cron_expression"`
	Timezone       string `json:"timezone"`
	PublicID       string `json:"public_id"`
	SigningMode    string `json:"signing_mode"`
	Secret         string `json:"secret"`
}

// TriggerResponse wraps a single routine trigger.
type TriggerResponse struct {
	Trigger *models.RoutineTrigger `json:"trigger"`
}

// TriggerListResponse wraps a list of routine triggers.
type TriggerListResponse struct {
	Triggers []*models.RoutineTrigger `json:"triggers"`
}

// -- Approvals --

// DecideApprovalRequest is the request body for deciding an approval.
type DecideApprovalRequest struct {
	Status       string `json:"status"`
	DecisionNote string `json:"decision_note"`
	DecidedBy    string `json:"decided_by"`
}

// ApprovalListResponse wraps a list of approvals.
type ApprovalListResponse struct {
	Approvals []*models.Approval `json:"approvals"`
}

// -- Activity --

// ActivityListResponse wraps a list of activity entries.
type ActivityListResponse struct {
	Activity []*models.ActivityEntry `json:"activity"`
}

// -- Memory --

// UpsertMemoryRequest is the request body for creating/updating agent memory.
type UpsertMemoryRequest struct {
	Entries []MemoryEntry `json:"entries"`
}

// MemoryEntry represents a single memory entry for upsert.
type MemoryEntry struct {
	Layer    string `json:"layer"`
	Key      string `json:"key"`
	Content  string `json:"content"`
	Metadata string `json:"metadata"`
}

// MemoryListResponse wraps a list of memory entries.
type MemoryListResponse struct {
	Memory []*models.AgentMemory `json:"memory"`
}

// MemorySummaryResponse wraps a summary of memory entries.
type MemorySummaryResponse struct {
	Count int `json:"count"`
}

// -- Inbox --

// InboxResponse wraps inbox items.
type InboxResponse struct {
	Items []*models.InboxItem `json:"items"`
}

// InboxCountResponse wraps the inbox item count.
type InboxCountResponse struct {
	Count int `json:"count"`
}

// -- Dashboard --

// DashboardResponse wraps the full dashboard data.
type DashboardResponse struct {
	AgentCount       int                     `json:"agent_count"`
	RunningCount     int                     `json:"running_count"`
	PausedCount      int                     `json:"paused_count"`
	ErrorCount       int                     `json:"error_count"`
	TasksInProgress  int                     `json:"tasks_in_progress"`
	OpenTasks        int                     `json:"open_tasks"`
	BlockedTasks     int                     `json:"blocked_tasks"`
	MonthSpendCents  int                     `json:"month_spend_cents"`
	PendingApprovals int                     `json:"pending_approvals"`
	RecentActivity   []*models.ActivityEntry `json:"recent_activity"`
}

// -- Channels --

// CreateChannelRequest is the request body for creating a channel.
type CreateChannelRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Platform    string `json:"platform"`
	Config      string `json:"config"`
	Status      string `json:"status"`
}

// ChannelListResponse wraps a list of channels.
type ChannelListResponse struct {
	Channels []*models.Channel `json:"channels"`
}

// -- Task Search --

// TaskSearchResultDTO represents a single task in search results.
type TaskSearchResultDTO struct {
	ID                      string `json:"id"`
	WorkspaceID             string `json:"workspaceId"`
	Identifier              string `json:"identifier"`
	Title                   string `json:"title"`
	Description             string `json:"description,omitempty"`
	Status                  string `json:"status"`
	Priority                int    `json:"priority"`
	ParentID                string `json:"parentId,omitempty"`
	ProjectID               string `json:"projectId,omitempty"`
	AssigneeAgentInstanceID string `json:"assigneeAgentInstanceId,omitempty"`
	Labels                  string `json:"labels,omitempty"`
	CreatedAt               string `json:"createdAt"`
	UpdatedAt               string `json:"updatedAt"`
}

// TaskSearchResponse wraps search results.
type TaskSearchResponse struct {
	Tasks []*TaskSearchResultDTO `json:"tasks"`
}

// -- Wakeups --

// WakeupListResponse wraps a list of wakeup requests.
type WakeupListResponse struct {
	Wakeups []*models.WakeupRequest `json:"wakeups"`
}

// -- Git --

// GitCloneRequest is the request body for cloning a workspace from a git repo.
type GitCloneRequest struct {
	RepoURL       string `json:"repoUrl"`
	Branch        string `json:"branch"`
	WorkspaceName string `json:"workspaceName"`
}

// GitPushRequest is the request body for pushing workspace changes.
type GitPushRequest struct {
	Message string `json:"message"`
}

// GitStatusResponse wraps the git status for a workspace.
type GitStatusResponse struct {
	IsGit       bool   `json:"is_git"`
	Branch      string `json:"branch,omitempty"`
	IsDirty     bool   `json:"is_dirty"`
	HasRemote   bool   `json:"has_remote"`
	Ahead       int    `json:"ahead"`
	Behind      int    `json:"behind"`
	CommitCount int    `json:"commit_count"`
}
