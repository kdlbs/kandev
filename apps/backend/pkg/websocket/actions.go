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

	// Column actions
	ActionColumnList   = "column.list"
	ActionColumnCreate = "column.create"
	ActionColumnGet    = "column.get"

	// Task actions
	ActionTaskList   = "task.list"
	ActionTaskCreate = "task.create"
	ActionTaskGet    = "task.get"
	ActionTaskUpdate = "task.update"
	ActionTaskDelete = "task.delete"
	ActionTaskMove   = "task.move"
	ActionTaskState  = "task.state"
	ActionTaskLogs   = "task.logs"

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

	// Notification actions (server -> client)
	ActionACPProgress   = "acp.progress"
	ActionACPLog        = "acp.log"
	ActionACPResult     = "acp.result"
	ActionACPError      = "acp.error"
	ActionACPStatus     = "acp.status"
	ActionACPHeartbeat  = "acp.heartbeat"
	ActionTaskUpdated   = "task.updated"
	ActionAgentUpdated  = "agent.updated"
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

