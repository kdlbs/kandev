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
	ActionTaskList          = "task.list"
	ActionTaskCreate        = "task.create"
	ActionTaskGet           = "task.get"
	ActionTaskUpdate        = "task.update"
	ActionTaskDelete        = "task.delete"
	ActionTaskMove          = "task.move"
	ActionTaskState         = "task.state"
	ActionTaskPlanCreate    = "task.plan.create"
	ActionTaskPlanGet       = "task.plan.get"
	ActionTaskPlanUpdate    = "task.plan.update"
	ActionTaskPlanDelete    = "task.plan.delete"

	ActionTaskSessionList   = "task.session.list"
	ActionTaskSessionResume = "task.session.resume"
	ActionTaskSessionStatus = "task.session.status"

	// Agent actions
	ActionAgentList   = "agent.list"
	ActionAgentLaunch = "agent.launch"
	ActionAgentStatus = "agent.status"
	ActionAgentLogs   = "agent.logs"
	ActionAgentStop   = "agent.stop"
	ActionAgentPrompt = "agent.prompt"
	ActionAgentCancel = "agent.cancel"
	ActionTaskSession = "task.session"
	ActionAgentTypes  = "agent.types"

	// Agent passthrough actions
	ActionAgentStdin  = "agent.stdin"  // Send input to agent process stdin (passthrough mode)
	ActionAgentStdout = "agent.stdout" // Agent stdout notification (passthrough mode)
	ActionAgentResize = "agent.resize" // Resize agent PTY (passthrough mode)

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

	// Workflow actions
	ActionWorkflowTemplateList   = "workflow.template.list"
	ActionWorkflowTemplateGet    = "workflow.template.get"
	ActionWorkflowStepList       = "workflow.step.list"
	ActionWorkflowStepGet        = "workflow.step.get"
	ActionWorkflowStepCreate  = "workflow.step.create"
	ActionWorkflowHistoryList = "workflow.history.list"

	// Subscription actions
	ActionTaskSubscribe      = "task.subscribe"
	ActionTaskUnsubscribe    = "task.unsubscribe"
	ActionSessionSubscribe   = "session.subscribe"
	ActionSessionUnsubscribe = "session.unsubscribe"
	ActionUserSubscribe      = "user.subscribe"
	ActionUserUnsubscribe    = "user.unsubscribe"

	// Message actions
	ActionMessageAdd  = "message.add"
	ActionMessageGet  = "message.get"
	ActionMessageList = "message.list"

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
	ActionTaskPlanCreated         = "task.plan.created"
	ActionTaskPlanUpdated         = "task.plan.updated"
	ActionTaskPlanDeleted         = "task.plan.deleted"
	ActionAgentUpdated            = "agent.updated"
	ActionAgentAvailableUpdated   = "agent.available.updated"
	ActionWorkspaceCreated        = "workspace.created"
	ActionWorkspaceUpdated        = "workspace.updated"
	ActionWorkspaceDeleted        = "workspace.deleted"
	ActionBoardCreated            = "board.created"
	ActionBoardUpdated            = "board.updated"
	ActionBoardDeleted            = "board.deleted"
	ActionSessionMessageAdded     = "session.message.added"
	ActionSessionMessageUpdated   = "session.message.updated"
	ActionSessionStateChanged     = "session.state_changed"
	ActionSessionWaitingForInput  = "session.waiting_for_input"
	ActionSessionAgentctlStarting = "session.agentctl_starting"
	ActionSessionAgentctlReady    = "session.agentctl_ready"
	ActionSessionAgentctlError    = "session.agentctl_error"
	ActionSessionTurnStarted      = "session.turn.started"
	ActionSessionTurnCompleted    = "session.turn.completed"
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

	// Workspace file operations
	ActionWorkspaceFileTreeGet    = "workspace.tree.get"
	ActionWorkspaceFileContentGet = "workspace.file.get"
	ActionWorkspaceFilesSearch    = "workspace.files.search"
	ActionWorkspaceFileChanges    = "session.workspace.file.changes" // Notification

	// Shell actions
	ActionShellStatus        = "session.shell.status" // Get shell status
	ActionShellSubscribe     = "shell.subscribe"      // Subscribe to shell output
	ActionShellInput         = "shell.input"          // Send input to shell
	ActionSessionShellOutput = "session.shell.output" // Shell output notification (also used for exit with type: "exit")

	// Session git actions (requests)
	ActionSessionGitSnapshots   = "session.git.snapshots"   // Get git snapshots for a session
	ActionSessionGitCommits     = "session.git.commits"     // Get commits for a session
	ActionSessionCumulativeDiff = "session.cumulative_diff" // Get cumulative diff from base branch
	ActionSessionCommitDiff     = "session.commit_diff"     // Get diff for a specific commit

	// Session git event (unified notification)
	ActionSessionGitEvent = "session.git.event" // Notification: unified git event

	// Process runner actions
	ActionSessionProcessOutput = "session.process.output"
	ActionSessionProcessStatus = "session.process.status"

	// Git worktree actions
	ActionWorktreePull     = "worktree.pull"      // Pull from remote
	ActionWorktreePush     = "worktree.push"      // Push to remote
	ActionWorktreeRebase   = "worktree.rebase"    // Rebase onto base branch
	ActionWorktreeMerge    = "worktree.merge"     // Merge base branch into worktree
	ActionWorktreeAbort    = "worktree.abort"     // Abort in-progress merge or rebase
	ActionWorktreeCommit   = "worktree.commit"    // Commit changes
	ActionWorktreeStage    = "worktree.stage"     // Stage files for commit
	ActionWorktreeUnstage  = "worktree.unstage"   // Unstage files from index
	ActionWorktreeCreatePR = "worktree.create_pr" // Create a pull request

	// User actions
	ActionUserGet             = "user.get"
	ActionUserSettingsUpdate  = "user.settings.update"
	ActionUserSettingsUpdated = "user.settings.updated"

	// MCP tool actions (agentctl -> backend via WS tunnel)
	ActionMCPListWorkspaces    = "mcp.list_workspaces"
	ActionMCPListBoards        = "mcp.list_boards"
	ActionMCPListWorkflowSteps = "mcp.list_workflow_steps"
	ActionMCPListTasks         = "mcp.list_tasks"
	ActionMCPCreateTask        = "mcp.create_task"
	ActionMCPUpdateTask        = "mcp.update_task"
	ActionMCPAskUserQuestion   = "mcp.ask_user_question"
	ActionMCPCreateTaskPlan    = "mcp.create_task_plan"
	ActionMCPGetTaskPlan       = "mcp.get_task_plan"
	ActionMCPUpdateTaskPlan    = "mcp.update_task_plan"
	ActionMCPDeleteTaskPlan    = "mcp.delete_task_plan"
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
