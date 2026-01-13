package websocket

// Action constants for WebSocket messages
const (
	// Health
	ActionHealthCheck = "health.check"

	// Board actions
	ActionBoardList   = "board.list"
	ActionBoardCreate = "board.create"
	ActionBoardGet    = "board.get"
	ActionBoardUpdate = "board.update"
	ActionBoardDelete = "board.delete"

	// Workspace actions
	ActionWorkspaceList   = "workspace.list"
	ActionWorkspaceCreate = "workspace.create"
	ActionWorkspaceGet    = "workspace.get"
	ActionWorkspaceUpdate = "workspace.update"
	ActionWorkspaceDelete = "workspace.delete"

	// Column actions
	ActionColumnList   = "column.list"
	ActionColumnCreate = "column.create"
	ActionColumnGet    = "column.get"
	ActionColumnUpdate = "column.update"
	ActionColumnDelete = "column.delete"

	// Repository actions
	ActionRepositoryList   = "repository.list"
	ActionRepositoryCreate = "repository.create"
	ActionRepositoryGet    = "repository.get"
	ActionRepositoryUpdate = "repository.update"
	ActionRepositoryDelete = "repository.delete"

	// Repository Script actions
	ActionRepositoryScriptList   = "repository.script.list"
	ActionRepositoryScriptCreate = "repository.script.create"
	ActionRepositoryScriptGet    = "repository.script.get"
	ActionRepositoryScriptUpdate = "repository.script.update"
	ActionRepositoryScriptDelete = "repository.script.delete"

	// Executor actions
	ActionExecutorList   = "executor.list"
	ActionExecutorCreate = "executor.create"
	ActionExecutorGet    = "executor.get"
	ActionExecutorUpdate = "executor.update"
	ActionExecutorDelete = "executor.delete"

	// Environment actions
	ActionEnvironmentList   = "environment.list"
	ActionEnvironmentCreate = "environment.create"
	ActionEnvironmentGet    = "environment.get"
	ActionEnvironmentUpdate = "environment.update"
	ActionEnvironmentDelete = "environment.delete"

	// Task actions
	ActionTaskList      = "task.list"
	ActionTaskCreate    = "task.create"
	ActionTaskGet       = "task.get"
	ActionTaskUpdate    = "task.update"
	ActionTaskDelete    = "task.delete"
	ActionTaskMove      = "task.move"
	ActionTaskState     = "task.state"
	ActionTaskExecution = "task.execution"

	// Agent actions
	ActionAgentList    = "agent.list"
	ActionAgentLaunch  = "agent.launch"
	ActionAgentStatus  = "agent.status"
	ActionAgentLogs    = "agent.logs"
	ActionAgentStop    = "agent.stop"
	ActionAgentPrompt  = "agent.prompt"
	ActionAgentCancel  = "agent.cancel"
	ActionAgentSession = "agent.session"
	ActionAgentTypes   = "agent.types"

	// Orchestrator actions
	ActionOrchestratorStatus   = "orchestrator.status"
	ActionOrchestratorQueue    = "orchestrator.queue"
	ActionOrchestratorTrigger  = "orchestrator.trigger"
	ActionOrchestratorStart    = "orchestrator.start"
	ActionOrchestratorStop     = "orchestrator.stop"
	ActionOrchestratorPause    = "orchestrator.pause"
	ActionOrchestratorResume   = "orchestrator.resume"
	ActionOrchestratorPrompt   = "orchestrator.prompt"
	ActionOrchestratorComplete = "orchestrator.complete"

	// Subscription actions
	ActionTaskSubscribe   = "task.subscribe"
	ActionTaskUnsubscribe = "task.unsubscribe"

	// Comment actions
	ActionCommentAdd  = "comment.add"
	ActionCommentGet  = "comment.get"
	ActionCommentList = "comment.list"

	// Notification actions (server -> client)
	ActionACPProgress             = "acp.progress"
	ActionACPLog                  = "acp.log"
	ActionACPResult               = "acp.result"
	ActionACPError                = "acp.error"
	ActionACPStatus               = "acp.status"
	ActionACPHeartbeat            = "acp.heartbeat"
	ActionTaskCreated             = "task.created"
	ActionTaskUpdated             = "task.updated"
	ActionTaskDeleted             = "task.deleted"
	ActionTaskStateChanged        = "task.state_changed"
	ActionAgentUpdated            = "agent.updated"
	ActionWorkspaceCreated        = "workspace.created"
	ActionWorkspaceUpdated        = "workspace.updated"
	ActionWorkspaceDeleted        = "workspace.deleted"
	ActionBoardCreated            = "board.created"
	ActionBoardUpdated            = "board.updated"
	ActionBoardDeleted            = "board.deleted"
	ActionColumnCreated           = "column.created"
	ActionColumnUpdated           = "column.updated"
	ActionColumnDeleted           = "column.deleted"
	ActionCommentAdded            = "comment.added"
	ActionInputRequested          = "input.requested"
	ActionRepositoryCreated       = "repository.created"
	ActionRepositoryUpdated       = "repository.updated"
	ActionRepositoryDeleted       = "repository.deleted"
	ActionRepositoryScriptCreated = "repository.script.created"
	ActionRepositoryScriptUpdated = "repository.script.updated"
	ActionRepositoryScriptDeleted = "repository.script.deleted"
	ActionExecutorCreated         = "executor.created"
	ActionExecutorUpdated         = "executor.updated"
	ActionExecutorDeleted         = "executor.deleted"
	ActionEnvironmentCreated      = "environment.created"
	ActionEnvironmentUpdated      = "environment.updated"
	ActionEnvironmentDeleted      = "environment.deleted"

	ActionAgentProfileDeleted = "agent.profile.deleted"
	ActionAgentProfileCreated = "agent.profile.created"
	ActionAgentProfileUpdated = "agent.profile.updated"

	// Permission request actions (agent -> user -> agent)
	ActionPermissionRequested = "permission.requested" // Agent requesting permission
	ActionPermissionRespond   = "permission.respond"   // User responding to permission request
)

// Error codes
const (
	ErrorCodeBadRequest    = "BAD_REQUEST"
	ErrorCodeNotFound      = "NOT_FOUND"
	ErrorCodeInternalError = "INTERNAL_ERROR"
	ErrorCodeUnauthorized  = "UNAUTHORIZED"
	ErrorCodeForbidden     = "FORBIDDEN"
	ErrorCodeValidation    = "VALIDATION_ERROR"
	ErrorCodeUnknownAction = "UNKNOWN_ACTION"
)
