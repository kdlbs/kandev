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
	MessageAdded   = "message.added"
	MessageUpdated = "message.updated"
)

// Event types for task sessions
const (
	TaskSessionStateChanged = "task_session.state_changed"
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

// Event types for executors
const (
	ExecutorCreated = "executor.created"
	ExecutorUpdated = "executor.updated"
	ExecutorDeleted = "executor.deleted"
)

// Event types for users
const (
	UserSettingsUpdated = "user.settings.updated"
)

// Event types for environments
const (
	EnvironmentCreated = "environment.created"
	EnvironmentUpdated = "environment.updated"
	EnvironmentDeleted = "environment.deleted"
)

// Event types for agents
const (
	AgentStarted           = "agent.started"
	AgentRunning           = "agent.running"
	AgentReady             = "agent.ready" // Agent finished prompt, ready for follow-up
	AgentCompleted         = "agent.completed"
	AgentFailed            = "agent.failed"
	AgentStopped           = "agent.stopped"
	AgentACPSessionCreated = "agent.acp_session_created"
)

// Event types for ACP messages
const (
	ACPMessage = "acp.message" // Base subject for ACP messages
)

// Event types for agent prompts
const (
	PromptComplete   = "prompt.complete"    // Agent finished responding to a prompt
	ToolCallStarted  = "tool_call.started"  // Agent started a tool call
	ToolCallComplete = "tool_call.complete" // Agent finished a tool call
)

// Event types for workspace/git status
const (
	GitStatusUpdated   = "git.status.updated"   // Git status changed in workspace
	FileChangeNotified = "file.change.notified" // File changed in workspace
)

// BuildACPSubject creates an ACP subject for a specific task
func BuildACPSubject(taskID string) string {
	return ACPMessage + "." + taskID
}

// BuildACPWildcardSubject creates a wildcard subscription for all ACP messages
func BuildACPWildcardSubject() string {
	return ACPMessage + ".*"
}

// BuildGitStatusSubject creates a git status subject for a specific task
func BuildGitStatusSubject(taskID string) string {
	return GitStatusUpdated + "." + taskID
}

// BuildGitStatusWildcardSubject creates a wildcard subscription for all git status updates
func BuildGitStatusWildcardSubject() string {
	return GitStatusUpdated + ".*"
}

// BuildFileChangeSubject creates a file change subject for a specific task
func BuildFileChangeSubject(taskID string) string {
	return FileChangeNotified + "." + taskID
}

// BuildFileChangeWildcardSubject creates a wildcard subscription for all file change notifications
func BuildFileChangeWildcardSubject() string {
	return FileChangeNotified + ".*"
}
