// Package models defines data types for the orchestrate domain.
package models

import "time"

// AgentRole represents the role of an agent instance.
type AgentRole string

const (
	AgentRoleCEO        AgentRole = "ceo"
	AgentRoleWorker     AgentRole = "worker"
	AgentRoleSpecialist AgentRole = "specialist"
	AgentRoleAssistant  AgentRole = "assistant"
)

// AgentStatus represents the runtime status of an agent instance.
type AgentStatus string

const (
	AgentStatusIdle            AgentStatus = "idle"
	AgentStatusWorking         AgentStatus = "working"
	AgentStatusPaused          AgentStatus = "paused"
	AgentStatusStopped         AgentStatus = "stopped"
	AgentStatusPendingApproval AgentStatus = "pending_approval"
)

// AgentInstance represents an orchestrate agent instance.
type AgentInstance struct {
	ID                    string      `json:"id" db:"id"`
	WorkspaceID           string      `json:"workspace_id" db:"workspace_id"`
	Name                  string      `json:"name" db:"name"`
	AgentProfileID        string      `json:"agent_profile_id" db:"agent_profile_id"`
	Role                  AgentRole   `json:"role" db:"role"`
	Icon                  string      `json:"icon" db:"icon"`
	Status                AgentStatus `json:"status" db:"status"`
	ReportsTo             string      `json:"reports_to" db:"reports_to"`
	Permissions           string      `json:"permissions" db:"permissions"`
	BudgetMonthlyCents    int         `json:"budget_monthly_cents" db:"budget_monthly_cents"`
	MaxConcurrentSessions int         `json:"max_concurrent_sessions" db:"max_concurrent_sessions"`
	DesiredSkills         string      `json:"desired_skills" db:"desired_skills"`
	ExecutorPreference    string      `json:"executor_preference" db:"executor_preference"`
	PauseReason           string      `json:"pause_reason" db:"pause_reason"`
	CreatedAt             time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt             time.Time   `json:"updated_at" db:"updated_at"`
}

// Skill represents a reusable skill definition.
type Skill struct {
	ID                       string    `json:"id" db:"id"`
	WorkspaceID              string    `json:"workspace_id" db:"workspace_id"`
	Name                     string    `json:"name" db:"name"`
	Slug                     string    `json:"slug" db:"slug"`
	Description              string    `json:"description" db:"description"`
	SourceType               string    `json:"source_type" db:"source_type"`
	SourceLocator            string    `json:"source_locator" db:"source_locator"`
	Content                  string    `json:"content" db:"content"`
	FileInventory            string    `json:"file_inventory" db:"file_inventory"`
	CreatedByAgentInstanceID string    `json:"created_by_agent_instance_id" db:"created_by_agent_instance_id"`
	CreatedAt                time.Time `json:"created_at" db:"created_at"`
	UpdatedAt                time.Time `json:"updated_at" db:"updated_at"`
}

// ProjectStatus represents the status of a project.
type ProjectStatus string

const (
	ProjectStatusActive    ProjectStatus = "active"
	ProjectStatusCompleted ProjectStatus = "completed"
	ProjectStatusOnHold    ProjectStatus = "on_hold"
	ProjectStatusArchived  ProjectStatus = "archived"
)

// Project represents an orchestrate project.
type Project struct {
	ID                  string        `json:"id" db:"id"`
	WorkspaceID         string        `json:"workspace_id" db:"workspace_id"`
	Name                string        `json:"name" db:"name"`
	Description         string        `json:"description" db:"description"`
	Status              ProjectStatus `json:"status" db:"status"`
	LeadAgentInstanceID string        `json:"lead_agent_instance_id" db:"lead_agent_instance_id"`
	Color               string        `json:"color" db:"color"`
	BudgetCents         int           `json:"budget_cents" db:"budget_cents"`
	Repositories        string        `json:"repositories" db:"repositories"`
	ExecutorConfig      string        `json:"executor_config" db:"executor_config"`
	CreatedAt           time.Time     `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time     `json:"updated_at" db:"updated_at"`
}

// CostEvent represents a cost tracking event.
type CostEvent struct {
	ID              string    `json:"id" db:"id"`
	SessionID       string    `json:"session_id" db:"session_id"`
	TaskID          string    `json:"task_id" db:"task_id"`
	AgentInstanceID string    `json:"agent_instance_id" db:"agent_instance_id"`
	ProjectID       string    `json:"project_id" db:"project_id"`
	Model           string    `json:"model" db:"model"`
	Provider        string    `json:"provider" db:"provider"`
	TokensIn        int       `json:"tokens_in" db:"tokens_in"`
	TokensCachedIn  int       `json:"tokens_cached_in" db:"tokens_cached_in"`
	TokensOut       int       `json:"tokens_out" db:"tokens_out"`
	CostCents       int       `json:"cost_cents" db:"cost_cents"`
	OccurredAt      time.Time `json:"occurred_at" db:"occurred_at"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
}

// BudgetPolicy represents a budget limit policy.
type BudgetPolicy struct {
	ID                string    `json:"id" db:"id"`
	WorkspaceID       string    `json:"workspace_id" db:"workspace_id"`
	ScopeType         string    `json:"scope_type" db:"scope_type"`
	ScopeID           string    `json:"scope_id" db:"scope_id"`
	LimitCents        int       `json:"limit_cents" db:"limit_cents"`
	Period            string    `json:"period" db:"period"`
	AlertThresholdPct int       `json:"alert_threshold_pct" db:"alert_threshold_pct"`
	ActionOnExceed    string    `json:"action_on_exceed" db:"action_on_exceed"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time `json:"updated_at" db:"updated_at"`
}

// WakeupRequest represents a wakeup queue entry.
type WakeupRequest struct {
	ID              string     `json:"id" db:"id"`
	AgentInstanceID string     `json:"agent_instance_id" db:"agent_instance_id"`
	Reason          string     `json:"reason" db:"reason"`
	Payload         string     `json:"payload" db:"payload"`
	Status          string     `json:"status" db:"status"`
	CoalescedCount  int        `json:"coalesced_count" db:"coalesced_count"`
	IdempotencyKey  string     `json:"idempotency_key" db:"idempotency_key"`
	ContextSnapshot string     `json:"context_snapshot" db:"context_snapshot"`
	RequestedAt     time.Time  `json:"requested_at" db:"requested_at"`
	ClaimedAt       *time.Time `json:"claimed_at" db:"claimed_at"`
	FinishedAt      *time.Time `json:"finished_at" db:"finished_at"`
}

// Routine represents a recurring task definition.
type Routine struct {
	ID                      string     `json:"id" db:"id"`
	WorkspaceID             string     `json:"workspace_id" db:"workspace_id"`
	Name                    string     `json:"name" db:"name"`
	Description             string     `json:"description" db:"description"`
	TaskTemplate            string     `json:"task_template" db:"task_template"`
	AssigneeAgentInstanceID string     `json:"assignee_agent_instance_id" db:"assignee_agent_instance_id"`
	Status                  string     `json:"status" db:"status"`
	ConcurrencyPolicy       string     `json:"concurrency_policy" db:"concurrency_policy"`
	Variables               string     `json:"variables" db:"variables"`
	LastRunAt               *time.Time `json:"last_run_at" db:"last_run_at"`
	CreatedAt               time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at" db:"updated_at"`
}

// RoutineTrigger represents a trigger for a routine.
type RoutineTrigger struct {
	ID             string     `json:"id" db:"id"`
	RoutineID      string     `json:"routine_id" db:"routine_id"`
	Kind           string     `json:"kind" db:"kind"`
	CronExpression string     `json:"cron_expression" db:"cron_expression"`
	Timezone       string     `json:"timezone" db:"timezone"`
	PublicID       string     `json:"public_id" db:"public_id"`
	SigningMode    string     `json:"signing_mode" db:"signing_mode"`
	Secret         string     `json:"secret" db:"secret"`
	NextRunAt      *time.Time `json:"next_run_at" db:"next_run_at"`
	LastFiredAt    *time.Time `json:"last_fired_at" db:"last_fired_at"`
	Enabled        bool       `json:"enabled" db:"enabled"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}

// RoutineRun represents a single run of a routine.
type RoutineRun struct {
	ID                  string     `json:"id" db:"id"`
	RoutineID           string     `json:"routine_id" db:"routine_id"`
	TriggerID           string     `json:"trigger_id" db:"trigger_id"`
	Source              string     `json:"source" db:"source"`
	Status              string     `json:"status" db:"status"`
	TriggerPayload      string     `json:"trigger_payload" db:"trigger_payload"`
	LinkedTaskID        string     `json:"linked_task_id" db:"linked_task_id"`
	CoalescedIntoRunID  string     `json:"coalesced_into_run_id" db:"coalesced_into_run_id"`
	DispatchFingerprint string     `json:"dispatch_fingerprint" db:"dispatch_fingerprint"`
	StartedAt           *time.Time `json:"started_at" db:"started_at"`
	CompletedAt         *time.Time `json:"completed_at" db:"completed_at"`
	CreatedAt           time.Time  `json:"created_at" db:"created_at"`
}

// Approval represents a pending or resolved approval request.
type Approval struct {
	ID                         string     `json:"id" db:"id"`
	WorkspaceID                string     `json:"workspace_id" db:"workspace_id"`
	Type                       string     `json:"type" db:"type"`
	RequestedByAgentInstanceID string     `json:"requested_by_agent_instance_id" db:"requested_by_agent_instance_id"`
	Status                     string     `json:"status" db:"status"`
	Payload                    string     `json:"payload" db:"payload"`
	DecisionNote               string     `json:"decision_note" db:"decision_note"`
	DecidedBy                  string     `json:"decided_by" db:"decided_by"`
	DecidedAt                  *time.Time `json:"decided_at" db:"decided_at"`
	CreatedAt                  time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt                  time.Time  `json:"updated_at" db:"updated_at"`
}

// ActivityEntry represents an entry in the activity log.
type ActivityEntry struct {
	ID          string    `json:"id" db:"id"`
	WorkspaceID string    `json:"workspace_id" db:"workspace_id"`
	ActorType   string    `json:"actor_type" db:"actor_type"`
	ActorID     string    `json:"actor_id" db:"actor_id"`
	Action      string    `json:"action" db:"action"`
	TargetType  string    `json:"target_type" db:"target_type"`
	TargetID    string    `json:"target_id" db:"target_id"`
	Details     string    `json:"details" db:"details"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// AgentMemory represents a memory entry for an agent.
type AgentMemory struct {
	ID              string    `json:"id" db:"id"`
	AgentInstanceID string    `json:"agent_instance_id" db:"agent_instance_id"`
	Layer           string    `json:"layer" db:"layer"`
	Key             string    `json:"key" db:"key"`
	Content         string    `json:"content" db:"content"`
	Metadata        string    `json:"metadata" db:"metadata"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
}

// Channel represents a communication channel for an agent.
type Channel struct {
	ID              string    `json:"id" db:"id"`
	WorkspaceID     string    `json:"workspace_id" db:"workspace_id"`
	AgentInstanceID string    `json:"agent_instance_id" db:"agent_instance_id"`
	Platform        string    `json:"platform" db:"platform"`
	Config          string    `json:"config" db:"config"`
	Status          string    `json:"status" db:"status"`
	TaskID          string    `json:"task_id" db:"task_id"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
}

// TaskBlocker represents a blocker relationship between tasks.
type TaskBlocker struct {
	TaskID        string    `json:"task_id" db:"task_id"`
	BlockerTaskID string    `json:"blocker_task_id" db:"blocker_task_id"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

// TaskComment represents an asynchronous comment on a task.
type TaskComment struct {
	ID             string    `json:"id" db:"id"`
	TaskID         string    `json:"task_id" db:"task_id"`
	AuthorType     string    `json:"author_type" db:"author_type"`
	AuthorID       string    `json:"author_id" db:"author_id"`
	Body           string    `json:"body" db:"body"`
	Source         string    `json:"source" db:"source"`
	ReplyChannelID string    `json:"reply_channel_id" db:"reply_channel_id"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}

// CostBreakdown represents an aggregated cost entry.
type CostBreakdown struct {
	GroupKey   string `json:"group_key" db:"group_key"`
	TotalCents int    `json:"total_cents" db:"total_cents"`
	Count      int    `json:"count" db:"count"`
}
