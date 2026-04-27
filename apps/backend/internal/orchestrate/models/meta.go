package models

// StatusMeta describes a task status for frontend rendering.
type StatusMeta struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Order int    `json:"order"`
	Color string `json:"color"`
}

// PriorityMeta describes a task priority for frontend rendering.
type PriorityMeta struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Order int    `json:"order"`
	Color string `json:"color"`
	Value int    `json:"value"`
}

// RoleMeta describes an agent role for frontend rendering.
type RoleMeta struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Color string `json:"color"`
}

// ExecutorTypeMeta describes an executor type for frontend rendering.
type ExecutorTypeMeta struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// SkillSourceTypeMeta describes a skill source type for frontend rendering.
type SkillSourceTypeMeta struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	ReadOnly       bool   `json:"readOnly"`
	ReadOnlyReason string `json:"readOnlyReason,omitempty"`
}

// ProjectStatusMeta describes a project status for frontend rendering.
type ProjectStatusMeta struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Color string `json:"color"`
}

// AgentStatusMeta describes an agent runtime status for frontend rendering.
type AgentStatusMeta struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Color string `json:"color"`
}

// RoutineRunStatusMeta describes a routine run status for frontend rendering.
type RoutineRunStatusMeta struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Color string `json:"color"`
}

// InboxItemTypeMeta describes an inbox item type for frontend rendering.
type InboxItemTypeMeta struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Icon  string `json:"icon"`
}

// AllStatuses returns the ordered list of task statuses.
func AllStatuses() []StatusMeta {
	return []StatusMeta{
		{ID: "backlog", Label: "Backlog", Order: 0, Color: "text-muted-foreground"},
		{ID: "todo", Label: "Todo", Order: 1, Color: "text-blue-600"},
		{ID: "in_progress", Label: "In Progress", Order: 2, Color: "text-yellow-600"},
		{ID: "in_review", Label: "In Review", Order: 3, Color: "text-violet-600"},
		{ID: "blocked", Label: "Blocked", Order: 4, Color: "text-red-600"},
		{ID: "done", Label: "Done", Order: 5, Color: "text-green-600"},
		{ID: "cancelled", Label: "Cancelled", Order: 6, Color: "text-neutral-500"},
	}
}

// AllPriorities returns the ordered list of task priorities.
func AllPriorities() []PriorityMeta {
	return []PriorityMeta{
		{ID: "critical", Label: "Critical", Order: 0, Color: "text-red-600", Value: 4},
		{ID: "high", Label: "High", Order: 1, Color: "text-orange-600", Value: 3},
		{ID: "medium", Label: "Medium", Order: 2, Color: "text-yellow-600", Value: 2},
		{ID: "low", Label: "Low", Order: 3, Color: "text-blue-600", Value: 1},
		{ID: "none", Label: "None", Order: 4, Color: "text-muted-foreground", Value: 0},
	}
}

// AllRoles returns the list of agent roles.
func AllRoles() []RoleMeta {
	return []RoleMeta{
		{ID: "ceo", Label: "CEO", Color: "bg-purple-100 text-purple-700 dark:bg-purple-900/50 dark:text-purple-300"},
		{ID: "worker", Label: "Worker", Color: "bg-blue-100 text-blue-700 dark:bg-blue-900/50 dark:text-blue-300"},
		{ID: "specialist", Label: "Specialist", Color: "bg-emerald-100 text-emerald-700 dark:bg-emerald-900/50 dark:text-emerald-300"},
		{ID: "assistant", Label: "Assistant", Color: "bg-amber-100 text-amber-700 dark:bg-amber-900/50 dark:text-amber-300"},
	}
}

// AllExecutorTypes returns the list of executor types.
func AllExecutorTypes() []ExecutorTypeMeta {
	return []ExecutorTypeMeta{
		{ID: "local_pc", Label: "Local (standalone)", Description: "Run on host machine"},
		{ID: "local_docker", Label: "Local Docker", Description: "Run in a local Docker container"},
		{ID: "sprites", Label: "Sprites (remote sandbox)", Description: "Run in a Sprites cloud environment"},
		{ID: "remote_docker", Label: "Remote Docker", Description: "Run in a remote Docker host"},
	}
}

// AllSkillSourceTypes returns the list of skill source types.
func AllSkillSourceTypes() []SkillSourceTypeMeta {
	return []SkillSourceTypeMeta{
		{ID: "inline", Label: "Inline", ReadOnly: false},
		{ID: "local_path", Label: "Local Path", ReadOnly: true, ReadOnlyReason: "Local path skills are read-only"},
		{ID: "git", Label: "Git Repository", ReadOnly: true, ReadOnlyReason: "GitHub-managed skills are read-only"},
		{ID: "skills_sh", Label: "skills.sh", ReadOnly: true, ReadOnlyReason: "skills.sh-managed skills are read-only"},
	}
}

// AllProjectStatuses returns the list of project statuses.
func AllProjectStatuses() []ProjectStatusMeta {
	return []ProjectStatusMeta{
		{ID: "active", Label: "Active", Color: "bg-green-100 text-green-700 dark:bg-green-900/50 dark:text-green-300"},
		{ID: "completed", Label: "Completed", Color: "bg-blue-100 text-blue-700 dark:bg-blue-900/50 dark:text-blue-300"},
		{ID: "on_hold", Label: "On Hold", Color: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/50 dark:text-yellow-300"},
		{ID: "archived", Label: "Archived", Color: "bg-neutral-100 text-neutral-700 dark:bg-neutral-900/50 dark:text-neutral-300"},
	}
}

// AllAgentStatuses returns the list of agent runtime statuses.
func AllAgentStatuses() []AgentStatusMeta {
	return []AgentStatusMeta{
		{ID: "idle", Label: "Idle", Color: "bg-neutral-400"},
		{ID: "working", Label: "Working", Color: "bg-cyan-400 animate-pulse"},
		{ID: "paused", Label: "Paused", Color: "bg-yellow-400"},
		{ID: "stopped", Label: "Stopped", Color: "bg-neutral-400 opacity-50"},
		{ID: "pending_approval", Label: "Pending Approval", Color: "bg-orange-400"},
	}
}

// AllRoutineRunStatuses returns the list of routine run statuses.
func AllRoutineRunStatuses() []RoutineRunStatusMeta {
	return []RoutineRunStatusMeta{
		{ID: "received", Label: "Received", Color: "bg-blue-100 text-blue-700 dark:bg-blue-900/50 dark:text-blue-300"},
		{ID: "task_created", Label: "Task Created", Color: "bg-green-100 text-green-700 dark:bg-green-900/50 dark:text-green-300"},
		{ID: "skipped", Label: "Skipped", Color: "bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-400"},
		{ID: "coalesced", Label: "Coalesced", Color: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/50 dark:text-yellow-300"},
		{ID: "failed", Label: "Failed", Color: "bg-red-100 text-red-700 dark:bg-red-900/50 dark:text-red-300"},
		{ID: "done", Label: "Done", Color: "bg-green-100 text-green-700 dark:bg-green-900/50 dark:text-green-300"},
		{ID: "cancelled", Label: "Cancelled", Color: "bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-400"},
	}
}

// AllInboxItemTypes returns the list of inbox item types.
func AllInboxItemTypes() []InboxItemTypeMeta {
	return []InboxItemTypeMeta{
		{ID: "approval", Label: "Approval", Icon: "shield-check"},
		{ID: "budget_alert", Label: "Budget Alert", Icon: "alert-triangle"},
		{ID: "agent_error", Label: "Agent Error", Icon: "bug"},
		{ID: "task_review", Label: "Task Review", Icon: "eye"},
	}
}
