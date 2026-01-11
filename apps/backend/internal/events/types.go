// Package events provides event types and utilities for the Kandev event system.
package events

// Event types for tasks
const (
	TaskCreated      = "task.created"
	TaskUpdated      = "task.updated"
	TaskStateChanged = "task.state_changed"
	TaskDeleted      = "task.deleted"
)

// Event types for workspaces
const (
	WorkspaceCreated = "workspace.created"
	WorkspaceUpdated = "workspace.updated"
	WorkspaceDeleted = "workspace.deleted"
)

// Event types for boards
const (
	BoardCreated = "board.created"
	BoardUpdated = "board.updated"
	BoardDeleted = "board.deleted"
)

// Event types for columns
const (
	ColumnCreated = "column.created"
	ColumnUpdated = "column.updated"
	ColumnDeleted = "column.deleted"
)

// Event types for comments
const (
	CommentAdded = "comment.added"
)

// Event types for repositories
const (
	RepositoryCreated = "repository.created"
	RepositoryUpdated = "repository.updated"
	RepositoryDeleted = "repository.deleted"
)

// Event types for repository scripts
const (
	RepositoryScriptCreated = "repository.script.created"
	RepositoryScriptUpdated = "repository.script.updated"
	RepositoryScriptDeleted = "repository.script.deleted"
)

// Event types for agents
const (
	AgentStarted   = "agent.started"
	AgentRunning   = "agent.running"
	AgentReady     = "agent.ready" // Agent finished prompt, ready for follow-up
	AgentCompleted = "agent.completed"
	AgentFailed    = "agent.failed"
	AgentStopped   = "agent.stopped"
)

// Event types for ACP messages
const (
	ACPMessage = "acp.message" // Base subject for ACP messages
)

// BuildACPSubject creates an ACP subject for a specific task
func BuildACPSubject(taskID string) string {
	return ACPMessage + "." + taskID
}

// BuildACPWildcardSubject creates a wildcard subscription for all ACP messages
func BuildACPWildcardSubject() string {
	return ACPMessage + ".*"
}
