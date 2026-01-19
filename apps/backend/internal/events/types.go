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
	AgentctlStarting       = "agentctl.starting"
	AgentctlReady          = "agentctl.ready"
	AgentctlError          = "agentctl.error"
)

// Event types for agent stream messages
const (
	AgentStream = "agent.stream" // Base subject for agent stream events
)

// Event types for agent prompts
const (
	PermissionRequestReceived = "permission_request.received" // Agent requested permission
)

// Event types for workspace/git status
const (
	GitStatusUpdated   = "git.status.updated"   // Git status changed in workspace
	FileChangeNotified = "file.change.notified" // File changed in workspace
)

// Event types for shell I/O
const (
	ShellOutput = "shell.output" // Shell output data
	ShellExit   = "shell.exit"   // Shell process exited
)

// BuildShellOutputSubject creates a shell output subject for a specific session
func BuildShellOutputSubject(sessionID string) string {
	return ShellOutput + "." + sessionID
}

// BuildShellOutputWildcardSubject creates a wildcard subscription for all shell output events
func BuildShellOutputWildcardSubject() string {
	return ShellOutput + ".*"
}

// BuildShellExitSubject creates a shell exit subject for a specific session
func BuildShellExitSubject(sessionID string) string {
	return ShellExit + "." + sessionID
}

// BuildShellExitWildcardSubject creates a wildcard subscription for all shell exit events
func BuildShellExitWildcardSubject() string {
	return ShellExit + ".*"
}

// BuildAgentStreamSubject creates an agent stream subject for a specific session
func BuildAgentStreamSubject(sessionID string) string {
	return AgentStream + "." + sessionID
}

// BuildAgentStreamWildcardSubject creates a wildcard subscription for all agent stream events
func BuildAgentStreamWildcardSubject() string {
	return AgentStream + ".*"
}

// BuildGitStatusSubject creates a git status subject for a specific session
func BuildGitStatusSubject(sessionID string) string {
	return GitStatusUpdated + "." + sessionID
}

// BuildGitStatusWildcardSubject creates a wildcard subscription for all git status updates
func BuildGitStatusWildcardSubject() string {
	return GitStatusUpdated + ".*"
}

// BuildFileChangeSubject creates a file change subject for a specific session
func BuildFileChangeSubject(sessionID string) string {
	return FileChangeNotified + "." + sessionID
}

// BuildFileChangeWildcardSubject creates a wildcard subscription for all file change notifications
func BuildFileChangeWildcardSubject() string {
	return FileChangeNotified + ".*"
}

// BuildPermissionRequestSubject creates a permission request subject for a specific session
func BuildPermissionRequestSubject(sessionID string) string {
	return PermissionRequestReceived + "." + sessionID
}

// BuildPermissionRequestWildcardSubject creates a wildcard subscription for all permission request events
func BuildPermissionRequestWildcardSubject() string {
	return PermissionRequestReceived + ".*"
}
