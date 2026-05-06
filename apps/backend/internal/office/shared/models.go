// Package shared contains cross-cutting types and utilities used by two or more
// office feature packages. It has no dependency on any sibling feature package.
package shared

import "time"

// AgentRole represents the role of an agent instance.
type AgentRole string

const (
	AgentRoleCEO        AgentRole = "ceo"
	AgentRoleWorker     AgentRole = "worker"
	AgentRoleSpecialist AgentRole = "specialist"
	AgentRoleAssistant  AgentRole = "assistant"
	AgentRoleSecurity   AgentRole = "security"
	AgentRoleQA         AgentRole = "qa"
	AgentRoleDevOps     AgentRole = "devops"
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

// ActivityEntry represents an entry in the activity log.
// Used by activity logging across all features, dashboard, and inbox.
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

// TaskBlocker represents a blocker relationship between tasks.
// Used by the tasks and scheduler features.
type TaskBlocker struct {
	TaskID        string    `json:"task_id" db:"task_id"`
	BlockerTaskID string    `json:"blocker_task_id" db:"blocker_task_id"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

// TaskComment represents an asynchronous comment on a task.
// Used by channels and tasks features.
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

// InboxItem represents a computed inbox entry for the user.
// Used by inbox and dashboard features.
type InboxItem struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	Status      string         `json:"status"`
	EntityID    string         `json:"entity_id,omitempty"`
	EntityType  string         `json:"entity_type,omitempty"`
	Payload     map[string]any `json:"payload,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

// DashboardData represents aggregated dashboard information.
// Used by the dashboard feature.
type DashboardData struct {
	AgentCount       int              `json:"agent_count"`
	RunningCount     int              `json:"running_count"`
	PausedCount      int              `json:"paused_count"`
	ErrorCount       int              `json:"error_count"`
	TasksInProgress  int              `json:"tasks_in_progress"`
	OpenTasks        int              `json:"open_tasks"`
	BlockedTasks     int              `json:"blocked_tasks"`
	MonthSpendCents  int              `json:"month_spend_cents"`
	PendingApprovals int              `json:"pending_approvals"`
	RecentActivity   []*ActivityEntry `json:"recent_activity"`
	RecentIssues     []any            `json:"recent_issues,omitempty"`
}
