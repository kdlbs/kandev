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

// Event types for comments
const (
	MessageAdded   = "message.added"
	MessageUpdated = "message.updated"
)

// Event types for task sessions
const (
	TaskSessionStateChanged = "task_session.state_changed"
)

// Event types for task plans
const (
	TaskPlanCreated = "task_plan.created"
	TaskPlanUpdated = "task_plan.updated"
	TaskPlanDeleted = "task_plan.deleted"
)

// Event types for session turns
const (
	TurnStarted   = "turn.started"
	TurnCompleted = "turn.completed"
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
	GitEvent           = "git.event"            // Internal git events (agentctl -> orchestrator)
	GitWSEvent         = "git.ws"               // WebSocket git events (orchestrator -> frontend)
	FileChangeNotified = "file.change.notified" // File changed in workspace
)

// Event types for shell I/O
const (
	ShellOutput = "shell.output" // Shell output data
	ShellExit   = "shell.exit"   // Shell process exited
)

// Event types for dev server I/O
const (
	ProcessOutput = "process.output" // Process output data
	ProcessStatus = "process.status" // Process status updates
)

// Event types for context window
const (
	ContextWindowUpdated = "context_window.updated" // Context window usage updated
)

// Event types for available commands
const (
	AvailableCommandsUpdated = "available_commands.updated" // Available slash commands updated
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

// BuildProcessOutputSubject creates a process output subject for a specific session
func BuildProcessOutputSubject(sessionID string) string {
	return ProcessOutput + "." + sessionID
}

// BuildProcessOutputWildcardSubject creates a wildcard subject for all process output events
func BuildProcessOutputWildcardSubject() string {
	return ProcessOutput + ".*"
}

// BuildProcessStatusSubject creates a process status subject for a specific session
func BuildProcessStatusSubject(sessionID string) string {
	return ProcessStatus + "." + sessionID
}

// BuildProcessStatusWildcardSubject creates a wildcard subject for all process status events
func BuildProcessStatusWildcardSubject() string {
	return ProcessStatus + ".*"
}

// BuildAgentStreamSubject creates an agent stream subject for a specific session
func BuildAgentStreamSubject(sessionID string) string {
	return AgentStream + "." + sessionID
}

// BuildAgentStreamWildcardSubject creates a wildcard subscription for all agent stream events
func BuildAgentStreamWildcardSubject() string {
	return AgentStream + ".*"
}

// BuildGitEventSubject creates a git event subject for a specific session
func BuildGitEventSubject(sessionID string) string {
	return GitEvent + "." + sessionID
}

// BuildGitEventWildcardSubject creates a wildcard subscription for all internal git events
func BuildGitEventWildcardSubject() string {
	return GitEvent + ".*"
}

// BuildGitWSEventSubject creates a git WebSocket event subject for a specific session
// These events are sent from orchestrator to frontend via WebSocket gateway
func BuildGitWSEventSubject(sessionID string) string {
	return GitWSEvent + "." + sessionID
}

// BuildGitWSEventWildcardSubject creates a wildcard subscription for all git WebSocket events
func BuildGitWSEventWildcardSubject() string {
	return GitWSEvent + ".*"
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

// BuildContextWindowSubject creates a context window subject for a specific session
func BuildContextWindowSubject(sessionID string) string {
	return ContextWindowUpdated + "." + sessionID
}

// BuildContextWindowWildcardSubject creates a wildcard subscription for all context window events
func BuildContextWindowWildcardSubject() string {
	return ContextWindowUpdated + ".*"
}

// BuildAvailableCommandsSubject creates an available commands subject for a specific session
func BuildAvailableCommandsSubject(sessionID string) string {
	return AvailableCommandsUpdated + "." + sessionID
}

// BuildAvailableCommandsWildcardSubject creates a wildcard subscription for all available commands events
func BuildAvailableCommandsWildcardSubject() string {
	return AvailableCommandsUpdated + ".*"
}

