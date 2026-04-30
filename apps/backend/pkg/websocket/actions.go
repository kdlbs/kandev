package websocket

// Action constants for WebSocket messages
const (
	// Health
	ActionHealthCheck = "health.check"

	// Workflow actions
	ActionWorkflowList    = "workflow.list"
	ActionWorkflowCreate  = "workflow.create"
	ActionWorkflowGet     = "workflow.get"
	ActionWorkflowUpdate  = "workflow.update"
	ActionWorkflowDelete  = "workflow.delete"
	ActionWorkflowReorder = "workflow.reorder"

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

	// Executor profile actions
	ActionExecutorProfileList    = "executor.profile.list"
	ActionExecutorProfileListAll = "executor.profile.list_all"
	ActionExecutorProfileCreate  = "executor.profile.create"
	ActionExecutorProfileGet     = "executor.profile.get"
	ActionExecutorProfileUpdate  = "executor.profile.update"
	ActionExecutorProfileDelete  = "executor.profile.delete"

	// Environment actions
	ActionEnvironmentList   = "environment.list"
	ActionEnvironmentCreate = "environment.create"
	ActionEnvironmentGet    = "environment.get"
	ActionEnvironmentUpdate = "environment.update"
	ActionEnvironmentDelete = "environment.delete"

	// Task actions
	ActionTaskList              = "task.list"
	ActionTaskCreate            = "task.create"
	ActionTaskGet               = "task.get"
	ActionTaskUpdate            = "task.update"
	ActionTaskDelete            = "task.delete"
	ActionTaskMove              = "task.move"
	ActionTaskState             = "task.state"
	ActionTaskArchive           = "task.archive"
	ActionTaskPlanCreate        = "task.plan.create"
	ActionTaskPlanGet           = "task.plan.get"
	ActionTaskPlanUpdate        = "task.plan.update"
	ActionTaskPlanDelete        = "task.plan.delete"
	ActionTaskPlanRevisionsList = "task.plan.revisions.list"
	ActionTaskPlanRevisionGet   = "task.plan.revision.get"
	ActionTaskPlanRevert        = "task.plan.revert"

	ActionTaskSessionList    = "task.session.list"
	ActionTaskSessionResume  = "task.session.resume"
	ActionTaskSessionStatus  = "task.session.status"
	ActionTaskSessionPrepare = "task.session.prepare"

	// Unified session launch
	ActionSessionLaunch       = "session.launch"
	ActionSessionRecover      = "session.recover"
	ActionSessionResetContext = "session.reset_context"
	ActionSessionStop         = "session.stop"
	ActionSessionDelete       = "session.delete"
	ActionSessionSetPrimary   = "session.set_primary"

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

	// Message Queue actions
	ActionMessageQueueAdd           = "message.queue.add"
	ActionMessageQueueCancel        = "message.queue.cancel"
	ActionMessageQueueGet           = "message.queue.get"
	ActionMessageQueueUpdate        = "message.queue.update"
	ActionMessageQueueAppend        = "message.queue.append"
	ActionMessageQueueStatusChanged = "message.queue.status_changed" // Notification: queue status changed

	// Workflow template/step actions
	ActionWorkflowTemplateList = "workflow.template.list"
	ActionWorkflowTemplateGet  = "workflow.template.get"
	ActionWorkflowStepList     = "workflow.step.list"
	ActionWorkflowStepGet      = "workflow.step.get"
	ActionWorkflowStepCreate   = "workflow.step.create"
	ActionWorkflowHistoryList  = "workflow.history.list"

	// Subscription actions
	ActionTaskSubscribe      = "task.subscribe"
	ActionTaskUnsubscribe    = "task.unsubscribe"
	ActionSessionSubscribe   = "session.subscribe"
	ActionSessionUnsubscribe = "session.unsubscribe"
	// Focus signals are layered on top of subscriptions to indicate which
	// session the user is actively viewing (task details page or task panel),
	// vs merely subscribed (sidebar diff badges). Drives backend polling tier.
	ActionSessionFocus    = "session.focus"
	ActionSessionUnfocus  = "session.unfocus"
	ActionUserSubscribe   = "user.subscribe"
	ActionUserUnsubscribe = "user.unsubscribe"

	// Message actions
	ActionMessageAdd    = "message.add"
	ActionMessageGet    = "message.get"
	ActionMessageList   = "message.list"
	ActionMessageSearch = "message.search"

	// Notification actions (server -> client)
	ActionACPProgress              = "acp.progress"
	ActionACPLog                   = "acp.log"
	ActionACPResult                = "acp.result"
	ActionACPError                 = "acp.error"
	ActionACPStatus                = "acp.status"
	ActionACPHeartbeat             = "acp.heartbeat"
	ActionTaskCreated              = "task.created"
	ActionTaskUpdated              = "task.updated"
	ActionTaskDeleted              = "task.deleted"
	ActionTaskStateChanged         = "task.state_changed"
	ActionTaskPlanCreated          = "task.plan.created"
	ActionTaskPlanUpdated          = "task.plan.updated"
	ActionTaskPlanDeleted          = "task.plan.deleted"
	ActionTaskPlanRevisionCreated  = "task.plan.revision.created"
	ActionTaskPlanReverted         = "task.plan.reverted"
	ActionAgentUpdated             = "agent.updated"
	ActionAgentAvailableUpdated    = "agent.available.updated"
	ActionWorkspaceCreated         = "workspace.created"
	ActionWorkspaceUpdated         = "workspace.updated"
	ActionWorkspaceDeleted         = "workspace.deleted"
	ActionWorkflowCreated          = "workflow.created"
	ActionWorkflowUpdated          = "workflow.updated"
	ActionWorkflowDeleted          = "workflow.deleted"
	ActionWorkflowStepCreated      = "workflow.step.created"
	ActionWorkflowStepUpdated      = "workflow.step.updated"
	ActionWorkflowStepDeleted      = "workflow.step.deleted"
	ActionSessionMessageAdded      = "session.message.added"
	ActionSessionMessageUpdated    = "session.message.updated"
	ActionSessionStateChanged      = "session.state_changed"
	ActionSessionWaitingForInput   = "session.waiting_for_input"
	ActionSessionAgentctlStarting  = "session.agentctl_starting"
	ActionSessionAgentctlReady     = "session.agentctl_ready"
	ActionSessionAgentctlError     = "session.agentctl_error"
	ActionSessionTurnStarted       = "session.turn.started"
	ActionSessionTurnCompleted     = "session.turn.completed"
	ActionSessionAvailableCommands = "session.available_commands"
	ActionSessionModeChanged       = "session.mode_changed"
	ActionSessionAgentCapabilities = "session.agent_capabilities"
	ActionSessionModelsUpdated     = "session.models_updated"
	ActionSessionSetMode           = "session.set_mode"
	ActionSessionTodosUpdated      = "session.todos_updated"
	ActionSessionPromptUsage       = "session.prompt_usage"
	ActionSessionPollModeChanged   = "session.poll_mode_changed"
	ActionInputRequested           = "input.requested"
	ActionRepositoryCreated        = "repository.created"
	ActionRepositoryUpdated        = "repository.updated"
	ActionRepositoryDeleted        = "repository.deleted"
	ActionRepositoryScriptCreated  = "repository.script.created"
	ActionRepositoryScriptUpdated  = "repository.script.updated"
	ActionRepositoryScriptDeleted  = "repository.script.deleted"
	ActionExecutorCreated          = "executor.created"
	ActionExecutorUpdated          = "executor.updated"
	ActionExecutorDeleted          = "executor.deleted"
	ActionEnvironmentCreated       = "environment.created"
	ActionEnvironmentUpdated       = "environment.updated"
	ActionEnvironmentDeleted       = "environment.deleted"
	ActionExecutorProfileCreated   = "executor.profile.created"
	ActionExecutorProfileUpdated   = "executor.profile.updated"
	ActionExecutorProfileDeleted   = "executor.profile.deleted"
	ActionExecutorPrepareProgress  = "executor.prepare.progress"
	ActionExecutorPrepareCompleted = "executor.prepare.completed"

	ActionAgentProfileDeleted = "agent.profile.deleted"
	ActionAgentProfileCreated = "agent.profile.created"
	ActionAgentProfileUpdated = "agent.profile.updated"

	// Permission request actions (agent -> user -> agent)
	ActionPermissionRequested = "permission.requested" // Agent requesting permission
	ActionPermissionRespond   = "permission.respond"   // User responding to permission request

	// Workspace file operations
	ActionWorkspaceFileTreeGet       = "workspace.tree.get"
	ActionWorkspaceFileContentGet    = "workspace.file.get"
	ActionWorkspaceFileContentGetRef = "workspace.file.get_at_ref"
	ActionWorkspaceFileContentUpdate = "workspace.file.update"
	ActionWorkspaceFileCreate        = "workspace.file.create"
	ActionWorkspaceFileDelete        = "workspace.file.delete"
	ActionWorkspaceFileRename        = "workspace.file.rename"
	ActionWorkspaceFilesSearch       = "workspace.files.search"
	ActionWorkspaceFileChanges       = "session.workspace.file.changes" // Notification

	// Shell actions
	ActionShellStatus        = "session.shell.status" // Get shell status
	ActionShellSubscribe     = "shell.subscribe"      // Subscribe to shell output
	ActionShellInput         = "shell.input"          // Send input to shell
	ActionSessionShellOutput = "session.shell.output" // Shell output notification (also used for exit with type: "exit")

	// User shell actions (independent terminal tabs)
	ActionUserShellList   = "user_shell.list"   // List running user shells for a session
	ActionUserShellCreate = "user_shell.create" // Create a new user shell terminal (assigns ID and label)
	ActionUserShellStop   = "user_shell.stop"   // Stop a user shell terminal

	// Session file review actions
	ActionSessionFileReviewGet    = "session.file_review.get"    // Get all file reviews for a session
	ActionSessionFileReviewUpdate = "session.file_review.update" // Upsert a single file review
	ActionSessionFileReviewReset  = "session.file_review.reset"  // Delete all reviews for a session

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
	ActionWorktreePull         = "worktree.pull"          // Pull from remote
	ActionWorktreePush         = "worktree.push"          // Push to remote
	ActionWorktreeRebase       = "worktree.rebase"        // Rebase onto base branch
	ActionWorktreeMerge        = "worktree.merge"         // Merge base branch into worktree
	ActionWorktreeAbort        = "worktree.abort"         // Abort in-progress merge or rebase
	ActionWorktreeCommit       = "worktree.commit"        // Commit changes
	ActionWorktreeStage        = "worktree.stage"         // Stage files for commit
	ActionWorktreeUnstage      = "worktree.unstage"       // Unstage files from index
	ActionWorktreeDiscard      = "worktree.discard"       // Discard changes to files
	ActionWorktreeCreatePR     = "worktree.create_pr"     // Create a pull request
	ActionWorktreeRevertCommit = "worktree.revert_commit" // Revert a commit (staged, no new commit)
	ActionWorktreeRenameBranch = "worktree.rename_branch" // Rename the current branch
	ActionWorktreeReset        = "worktree.reset"         // Reset HEAD to a commit (soft/hard)

	// User actions
	ActionUserGet             = "user.get"
	ActionUserSettingsUpdate  = "user.settings.update"
	ActionUserSettingsUpdated = "user.settings.updated"

	// VS Code server actions
	ActionVscodeStart    = "vscode.start"    // Start code-server for a session
	ActionVscodeStop     = "vscode.stop"     // Stop code-server for a session
	ActionVscodeStatus   = "vscode.status"   // Get code-server status for a session
	ActionVscodeOpenFile = "vscode.openFile" // Open a file in code-server for a session

	// Port actions
	ActionPortList        = "port.list"         // List listening ports on a remote executor
	ActionPortTunnelStart = "port.tunnel.start" // Start a port tunnel
	ActionPortTunnelStop  = "port.tunnel.stop"  // Stop a port tunnel
	ActionPortTunnelList  = "port.tunnel.list"  // List active port tunnels

	// Secret actions
	ActionSecretList   = "secrets.list"
	ActionSecretCreate = "secrets.create"
	ActionSecretUpdate = "secrets.update"
	ActionSecretDelete = "secrets.delete"
	ActionSecretReveal = "secrets.reveal"

	// Sprites actions
	ActionSpritesStatus              = "sprites.status"
	ActionSpritesInstancesList       = "sprites.instances.list"
	ActionSpritesInstancesDestroy    = "sprites.instances.destroy"
	ActionSpritesTest                = "sprites.test"
	ActionSpritesNetworkPolicyGet    = "sprites.network_policy.get"
	ActionSpritesNetworkPolicyUpdate = "sprites.network_policy.update"

	// MCP tool actions (agentctl -> backend via WS tunnel)
	ActionMCPListWorkspaces       = "mcp.list_workspaces"
	ActionMCPListWorkflows        = "mcp.list_workflows"
	ActionMCPListWorkflowSteps    = "mcp.list_workflow_steps"
	ActionMCPListTasks            = "mcp.list_tasks"
	ActionMCPCreateTask           = "mcp.create_task"
	ActionMCPUpdateTask           = "mcp.update_task"
	ActionMCPAskUserQuestion      = "mcp.ask_user_question"
	ActionMCPCreateTaskPlan       = "mcp.create_task_plan"
	ActionMCPGetTaskPlan          = "mcp.get_task_plan"
	ActionMCPUpdateTaskPlan       = "mcp.update_task_plan"
	ActionMCPDeleteTaskPlan       = "mcp.delete_task_plan"
	ActionMCPClarificationTimeout = "mcp.clarification_timeout"

	// Config-mode MCP actions (agent-native configuration)
	ActionMCPCreateWorkflow = "mcp.create_workflow"
	ActionMCPUpdateWorkflow = "mcp.update_workflow"
	ActionMCPDeleteWorkflow = "mcp.delete_workflow"

	ActionMCPCreateWorkflowStep  = "mcp.create_workflow_step"
	ActionMCPUpdateWorkflowStep  = "mcp.update_workflow_step"
	ActionMCPDeleteWorkflowStep  = "mcp.delete_workflow_step"
	ActionMCPReorderWorkflowStep = "mcp.reorder_workflow_steps"

	ActionMCPListAgents  = "mcp.list_agents"
	ActionMCPUpdateAgent = "mcp.update_agent"

	ActionMCPListAgentProfiles  = "mcp.list_agent_profiles"
	ActionMCPCreateAgentProfile = "mcp.create_agent_profile"
	ActionMCPUpdateAgentProfile = "mcp.update_agent_profile"
	ActionMCPDeleteAgentProfile = "mcp.delete_agent_profile"
	ActionMCPGetMcpConfig       = "mcp.get_mcp_config"
	ActionMCPUpdateMcpConfig    = "mcp.update_mcp_config"

	ActionMCPListExecutors         = "mcp.list_executors"
	ActionMCPListExecutorProfiles  = "mcp.list_executor_profiles"
	ActionMCPCreateExecutorProfile = "mcp.create_executor_profile"
	ActionMCPUpdateExecutorProfile = "mcp.update_executor_profile"
	ActionMCPDeleteExecutorProfile = "mcp.delete_executor_profile"

	ActionMCPMoveTask        = "mcp.move_task"
	ActionMCPDeleteTask      = "mcp.delete_task"
	ActionMCPArchiveTask     = "mcp.archive_task"
	ActionMCPUpdateTaskState = "mcp.update_task_state"
	ActionMCPMessageTask     = "mcp.message_task"
)

// GitHub integration actions
const (
	ActionGitHubStatus            = "github.status"
	ActionGitHubTaskPRsList       = "github.task_prs.list"
	ActionGitHubTaskPRGet         = "github.task_pr.get"
	ActionGitHubPRFeedbackGet     = "github.pr_feedback.get"
	ActionGitHubReviewWatchesList = "github.review_watches.list"
	ActionGitHubReviewWatchCreate = "github.review_watches.create"
	ActionGitHubReviewWatchUpdate = "github.review_watches.update"
	ActionGitHubReviewWatchDelete = "github.review_watches.delete"
	ActionGitHubReviewTrigger     = "github.review_watches.trigger"
	ActionGitHubReviewTriggerAll  = "github.review_watches.trigger_all"
	ActionGitHubPRWatchesList     = "github.pr_watches.list"
	ActionGitHubPRWatchDelete     = "github.pr_watches.delete"
	ActionGitHubPRFilesGet        = "github.pr_files.get"
	ActionGitHubPRCommitsGet      = "github.pr_commits.get"
	ActionGitHubTaskPRUpdated     = "github.task_pr.updated"      // Notification
	ActionGitHubPRFeedbackNotify  = "github.pr_feedback.notify"   // Notification
	ActionGitHubNewReviewPRNotify = "github.new_review_pr.notify" // Notification
	ActionGitHubTaskPRSync        = "github.task_pr.sync"
	ActionGitHubStats             = "github.stats"
	ActionGitHubCheckSessionPR    = "github.check_session_pr"

	// Issue watch actions
	ActionGitHubIssueWatchesList = "github.issue_watches.list"
	ActionGitHubIssueWatchCreate = "github.issue_watches.create"
	ActionGitHubIssueWatchUpdate = "github.issue_watches.update"
	ActionGitHubIssueWatchDelete = "github.issue_watches.delete"
	ActionGitHubIssueTrigger     = "github.issue_watches.trigger"
	ActionGitHubIssueTriggerAll  = "github.issue_watches.trigger_all"
	ActionGitHubNewIssueNotify   = "github.new_issue.notify" // Notification

	// Action preset actions for the /github page quick-launch prompts.
	ActionGitHubActionPresetsList   = "github.action_presets.list"
	ActionGitHubActionPresetsUpdate = "github.action_presets.update"
	ActionGitHubActionPresetsReset  = "github.action_presets.reset"
)

// Jira integration actions
const (
	ActionJiraConfigGet        = "jira.config.get"
	ActionJiraConfigSet        = "jira.config.set"
	ActionJiraConfigDelete     = "jira.config.delete"
	ActionJiraConfigTest       = "jira.config.test"
	ActionJiraTicketGet        = "jira.ticket.get"
	ActionJiraTicketTransition = "jira.ticket.transition"
	ActionJiraProjectsList     = "jira.projects.list"
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
