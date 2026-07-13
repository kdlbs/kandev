// Package mcp provides MCP server functionality for agentctl.
// It exposes MCP tools that forward requests to the Kandev backend via the agent stream.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// BackendClient is the interface for communicating with the Kandev backend.
// MCP tool handlers use this to forward requests to the backend.
type BackendClient interface {
	// RequestPayload sends a request to the backend and unmarshals the response.
	RequestPayload(ctx context.Context, action string, payload, result interface{}) error
}

// MCP mode constants control which tools are registered.
const (
	// ModeTask registers kanban, plan, and interaction tools (default for task-solving agents).
	ModeTask = "task"
	// ModeConfig registers configuration tools for workflows, agents, and MCP servers.
	ModeConfig = "config"
	// ModeExternal registers config tools plus create_task_kandev for external coding agents
	// (Claude Code, Cursor, etc.) that connect to the backend's MCP endpoint.
	// No session-scoped tools (plan, ask_user_question) since there is no live session.
	ModeExternal = "external"
	// ModeOffice registers plan and interaction tools for office agents.
	// Kanban tools are excluded because office agents use CLI commands instead.
	ModeOffice = "office"
)

// MCP payload keys reused across tool registrations. Extracted so a future
// wire-protocol rename touches every tool in one place AND so goconst
// doesn't flag the literals as repeated string occurrences.
const (
	mcpKeyTaskID           = "task_id"
	mcpKeyRepositoryID     = "repository_id"
	mcpKeyTaskRepositoryID = "task_repository_id"
	mcpKeyRepositoryURL    = "repository_url"
	mcpKeyLocalPath        = "local_path"
	mcpKeyGitHubURL        = "github_url"
	mcpKeyBaseBranch       = "base_branch"
	mcpKeyCheckoutBranch   = "checkout_branch"
)

// locatorCount returns how many of the supplied repository-locator strings
// are non-empty. Used by add_branch / create_task mutual-exclusion checks
// so a chain of `if a != "" && b != "" { ... }` doesn't repeat at each call
// site.
func locatorCount(locators ...string) int {
	n := 0
	for _, s := range locators {
		if s != "" {
			n++
		}
	}
	return n
}

// normalizeMode returns a valid MCP mode, defaulting unknown values to ModeTask.
func normalizeMode(mode string) string {
	switch mode {
	case ModeConfig, ModeExternal, ModeOffice:
		return mode
	default:
		return ModeTask
	}
}

// Server wraps the MCP server with backend client for communication.
type Server struct {
	backend            BackendClient
	sessionID          string
	taskID             string
	disableAskQuestion bool
	mode               string // "task" (default), "config", or "office"
	mcpServer          *server.MCPServer
	sseServer          *server.SSEServer
	httpServer         *server.StreamableHTTPServer
	logger             *logger.Logger
	mcpLogger          *zap.Logger // optional file logger for MCP debug traces
	mu                 sync.Mutex
	running            bool
}

// New creates a new MCP server for agentctl.
// port is the HTTP server port used to build the SSE base URL (http://localhost:<port>).
// mcpLogFile is an optional file path for MCP debug logging; pass "" to disable.
func New(backend BackendClient, sessionID, taskID string, port int, log *logger.Logger, mcpLogFile string, disableAskQuestion bool, mcpMode string) *Server {
	s := newServer(backend, sessionID, taskID, log, mcpLogFile, disableAskQuestion, mcpMode)

	// Create SSE server for Claude Desktop, Cursor, etc.
	// WithBaseURL ensures the SSE endpoint event includes the full message URL
	// (e.g. http://localhost:10005/message?sessionId=xxx) so MCP clients can POST back.
	s.sseServer = server.NewSSEServer(s.mcpServer,
		server.WithBaseURL(fmt.Sprintf("http://localhost:%d", port)),
	)

	// Create Streamable HTTP server for Codex
	s.httpServer = server.NewStreamableHTTPServer(s.mcpServer,
		server.WithEndpointPath("/mcp"),
	)

	return s
}

// NewExternal creates an MCP server for the Kandev backend's external endpoint.
// External coding agents (Claude Code, Cursor, etc.) connect here to manage Kandev
// configuration and create tasks. Routes are mounted under /mcp on the backend.
func NewExternal(backend BackendClient, log *logger.Logger, mcpLogFile string) *Server {
	// External mode has no live session, so disable ask-question and use empty IDs.
	s := newServer(backend, "", "", log, mcpLogFile, true, ModeExternal)

	// SSE handlers are mounted at /mcp/sse and /mcp/message — the static base path
	// makes the SSE endpoint event emit /mcp/message. Keeping the message endpoint
	// path-only lets remote clients resolve it against the URL they used to reach
	// Kandev instead of a server-guessed localhost URL.
	s.sseServer = server.NewSSEServer(s.mcpServer,
		server.WithStaticBasePath("/mcp"),
		server.WithUseFullURLForMessageEndpoint(false),
	)

	// Streamable HTTP transport handler — mounted at /mcp on the backend.
	s.httpServer = server.NewStreamableHTTPServer(s.mcpServer,
		server.WithEndpointPath("/mcp"),
	)

	return s
}

// newServer builds the shared parts of a Server (logger, mcp-go server, tools).
// Callers are responsible for constructing sseServer and httpServer with the
// transport configuration appropriate for their hosting environment.
func newServer(backend BackendClient, sessionID, taskID string, log *logger.Logger, mcpLogFile string, disableAskQuestion bool, mcpMode string) *Server {
	mcpMode = normalizeMode(mcpMode)
	s := &Server{
		backend:            backend,
		sessionID:          sessionID,
		taskID:             taskID,
		disableAskQuestion: disableAskQuestion,
		mode:               mcpMode,
		logger:             log.WithFields(zap.String("component", "mcp-server")),
	}

	// Set up optional file logger for MCP debug traces
	if mcpLogFile != "" {
		fileCfg := zap.NewProductionConfig()
		fileCfg.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
		fileCfg.OutputPaths = []string{mcpLogFile}
		fileCfg.ErrorOutputPaths = []string{mcpLogFile}
		if fl, err := fileCfg.Build(); err == nil {
			s.mcpLogger = fl
			log.Info("MCP file logger enabled", zap.String("path", mcpLogFile))
		} else {
			log.Warn("failed to create MCP file logger", zap.Error(err))
		}
	}

	s.mcpServer = server.NewMCPServer(
		"kandev-mcp",
		"1.0.0",
		server.WithToolCapabilities(true),
	)
	s.registerTools()
	s.running = true
	return s
}

// RegisterRoutes adds MCP routes to the gin router at the root.
// Used by agentctl which serves the MCP transport at /sse, /message, /mcp.
func (s *Server) RegisterRoutes(router gin.IRouter) {
	router.GET("/sse", gin.WrapH(s.sseServer.SSEHandler()))
	router.POST("/message", gin.WrapH(s.sseServer.MessageHandler()))
	router.Any("/mcp", gin.WrapH(s.httpServer))

	s.logger.Info("registered MCP routes", zap.String("sse", "/sse"), zap.String("http", "/mcp"))
}

// RegisterBackendRoutes adds MCP routes namespaced under /mcp to the gin router.
// Used by the Kandev backend so that all MCP endpoints (/mcp, /mcp/sse, /mcp/message)
// share a clean URL prefix on the multi-purpose backend HTTP server.
func (s *Server) RegisterBackendRoutes(router gin.IRouter) {
	router.GET("/mcp/sse", gin.WrapH(s.sseServer.SSEHandler()))
	router.POST("/mcp/message", gin.WrapH(s.sseServer.MessageHandler()))
	router.Any("/mcp", gin.WrapH(s.httpServer))

	s.logger.Info("registered MCP backend routes",
		zap.String("sse", "/mcp/sse"),
		zap.String("message", "/mcp/message"),
		zap.String("http", "/mcp"))
}

// Close shuts down the MCP server.
func (s *Server) Close(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}
	s.running = false

	if s.sseServer != nil {
		if err := s.sseServer.Shutdown(ctx); err != nil {
			s.logger.Warn("failed to shutdown SSE server", zap.Error(err))
		}
	}
	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(ctx); err != nil {
			s.logger.Warn("failed to shutdown HTTP server", zap.Error(err))
		}
	}
	if s.mcpLogger != nil {
		_ = s.mcpLogger.Sync()
	}

	return nil
}

// wrapHandler wraps a tool handler with debug logging for tracing MCP calls.
func (s *Server) wrapHandler(toolName string, handler server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		args := req.GetArguments()

		s.logger.Debug("MCP tool call",
			zap.String("tool", toolName),
			zap.Any("args", args))
		if s.mcpLogger != nil {
			s.mcpLogger.Debug("MCP tool call",
				zap.String("tool", toolName),
				zap.String("session_id", s.sessionID),
				zap.Any("args", args))
		}

		result, err := handler(ctx, req)
		duration := time.Since(start)

		switch {
		case err != nil:
			s.logger.Debug("MCP tool error",
				zap.String("tool", toolName),
				zap.Duration("duration", duration),
				zap.Error(err))
			if s.mcpLogger != nil {
				s.mcpLogger.Debug("MCP tool error",
					zap.String("tool", toolName),
					zap.String("session_id", s.sessionID),
					zap.Duration("duration", duration),
					zap.Error(err))
			}
		case result != nil && result.IsError:
			s.logger.Debug("MCP tool returned error",
				zap.String("tool", toolName),
				zap.Duration("duration", duration),
				zap.Any("result", result.Content))
			if s.mcpLogger != nil {
				s.mcpLogger.Debug("MCP tool returned error",
					zap.String("tool", toolName),
					zap.String("session_id", s.sessionID),
					zap.Duration("duration", duration),
					zap.Any("result", result.Content))
			}
		default:
			s.logger.Debug("MCP tool success",
				zap.String("tool", toolName),
				zap.Duration("duration", duration))
			if s.mcpLogger != nil {
				s.mcpLogger.Debug("MCP tool success",
					zap.String("tool", toolName),
					zap.String("session_id", s.sessionID),
					zap.Duration("duration", duration))
			}
		}

		return result, err
	}
}

// SetMode changes the MCP server mode and re-registers tools accordingly.
// This allows reconfiguring the tool set after initial creation (e.g., when
// a session transitions to plan/config mode on a pre-existing workspace).
func (s *Server) SetMode(mode string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.mode = normalizeMode(mode)
	// Clear all existing tools and re-register for the new mode.
	s.mcpServer.SetTools() // empty call clears all tools
	s.registerTools()
}

// registerTools registers MCP tools based on the server mode.
func (s *Server) registerTools() {
	count := 0
	switch s.mode {
	case ModeConfig:
		s.registerConfigWorkflowTools()
		count += 12
		s.registerConfigAgentTools()
		count += 4
		s.registerConfigMcpTools()
		count += 4
		s.registerConfigExecutorTools()
		count += 5
		s.registerConfigTaskTools()
		count += 6
		if !s.disableAskQuestion {
			s.registerInteractionTools()
			count++
		}
	case ModeExternal:
		// External coding agents get config tools plus create_task_kandev so
		// they can both manage Kandev configuration and spawn new tasks.
		// No interaction or plan tools (no live session to attach them to).
		s.registerConfigWorkflowTools()
		count += 12
		s.registerConfigAgentTools()
		count += 4
		s.registerConfigMcpTools()
		count += 4
		s.registerConfigExecutorTools()
		count += 5
		s.registerConfigTaskTools()
		count += 6
		s.registerCreateTaskTool()
		count++
	case ModeOffice:
		// Office agents use `agentctl kandev …` subcommands for every
		// office mutation (create task, delegate, comment, …). The MCP
		// surface for office mode only keeps:
		//   - ask_user_question — interactive prompt path
		//   - plan tools        — structured plan capture
		//   - related-tasks     — discover parent/child/sibling IDs
		//   - task-document tools — parent/child coordination docs
		// delegate_task was removed in favour of
		// `agentctl kandev tasks create --parent $KANDEV_TASK_ID …`.
		if !s.disableAskQuestion {
			s.registerInteractionTools()
			count++
		}
		s.registerPlanTools()
		count += 4
		s.registerRelatedTasksTool()
		count++
		s.registerTaskDocumentTools()
		count += 3
	default: // ModeTask
		// Kanban tasks get list_related_tasks_kandev (useful for finding
		// a sibling to message_task_kandev) but NOT the task-document
		// tools — those are office coordination plumbing.
		s.registerKanbanTools()
		count += 13
		if !s.disableAskQuestion {
			s.registerInteractionTools()
			count++
		}
		s.registerPlanTools()
		count += 4
		s.registerWalkthroughTools()
		count += 3
		s.registerRelatedTasksTool()
		count++
		// Task-mode only: requires a live session to attach the new
		// (repository, branch) to. External mode has no such context.
		s.registerAddBranchToTaskTool()
		s.registerUpdateRepositoryBaseBranchTool()
		count += 2
		// Task-mode only: ADR 0015 explicit step-completion signal. The
		// tool targets the current (task, session, step) the MCP server
		// was bound to, so it has no meaningful semantics outside a task
		// session.
		s.registerStepCompleteTool()
		count++
	}
	s.logger.Info("registered MCP tools",
		zap.String("mode", s.mode),
		zap.Int("count", count),
		zap.Bool("disable_ask_question", s.disableAskQuestion))
}

func (s *Server) registerKanbanTools() {
	// Use NewToolWithRawSchema for parameter-less tools to ensure the schema
	// includes "properties": {}. The default ToolInputSchema type in mcp-go uses
	// omitempty which drops empty properties maps, causing OpenAI API validation
	// errors ("object schema missing properties").
	s.mcpServer.AddTool(
		mcp.NewToolWithRawSchema("list_workspaces_kandev",
			"List all workspaces. Use this first to get workspace IDs.",
			json.RawMessage(`{"type":"object","properties":{}}`),
		),
		s.wrapHandler("list_workspaces_kandev", s.listWorkspacesHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("list_workflows_kandev",
			mcp.WithDescription("List all workflows in a workspace."),
			mcp.WithString("workspace_id", mcp.Required(), mcp.Description("The workspace ID")),
		),
		s.wrapHandler("list_workflows_kandev", s.listWorkflowsHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("list_workflow_steps_kandev",
			mcp.WithDescription("List all workflow steps in a workflow."),
			mcp.WithString("workflow_id", mcp.Required(), mcp.Description("The workflow ID")),
		),
		s.wrapHandler("list_workflow_steps_kandev", s.listWorkflowStepsHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("list_tasks_kandev",
			mcp.WithDescription("List all tasks in a workflow. Each task includes its associated GitHub pull requests (number, url, title, state) under the \"prs\" field when any exist — use the PR state (open/closed/merged) to find tasks whose work has landed."),
			mcp.WithString("workflow_id", mcp.Required(), mcp.Description("The workflow ID")),
		),
		s.wrapHandler("list_tasks_kandev", s.listTasksHandler()),
	)
	s.registerCreateTaskTool()
	s.mcpServer.AddTool(
		mcp.NewToolWithRawSchema("list_agents_kandev",
			"List all configured agents with their profiles. Use this to find available agent_profile_ids for create_task_kandev.",
			json.RawMessage(`{"type":"object","properties":{}}`),
		),
		s.wrapHandler("list_agents_kandev", s.listAgentsHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("list_executor_profiles_kandev",
			mcp.WithDescription("List all profiles for an executor. Use this to find available executor_profile_ids for create_task_kandev. Standard executor IDs: exec-local (standalone process), exec-worktree (git worktree), exec-local-docker (Docker container), exec-sprites (cloud)."),
			mcp.WithString("executor_id", mcp.Required(), mcp.Description("The executor ID (e.g. exec-local, exec-worktree, exec-local-docker, exec-sprites)")),
		),
		s.wrapHandler("list_executor_profiles_kandev", s.listExecutorProfilesHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("update_task_kandev",
			mcp.WithDescription("Update an existing task."),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task ID")),
			mcp.WithString("title", mcp.Description("New title")),
			mcp.WithString("description", mcp.Description("New description")),
			mcp.WithString("state", mcp.Description("New state: not_started, in_progress, etc.")),
		),
		s.wrapHandler("update_task_kandev", s.updateTaskHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("move_task_kandev",
			mcp.WithDescription("Move a task to a different workflow step. When the source session is mid-turn (RUNNING), the move is deferred to turn-end automatically — prompt is optional (use it for cross-agent hand-offs). Idle-session and admin moves apply immediately."),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task ID")),
			mcp.WithString("workflow_id", mcp.Required(), mcp.Description("Target workflow ID")),
			mcp.WithString("workflow_step_id", mcp.Required(), mcp.Description("Target workflow step ID")),
			mcp.WithNumber("position", mcp.Description("Position within the step (0-based)")),
			mcp.WithString("prompt", mcp.Description("Optional hand-off message for the receiving agent at the new step. Mid-turn moves are always deferred; include a prompt when the next agent needs context (e.g. QA → review). Omit for self-moves like Work → Done.")),
		),
		s.wrapHandler("move_task_kandev", s.moveTaskHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("delete_task_kandev",
			mcp.WithDescription("Delete a task permanently. Use to clean up orphaned, duplicate, or test tasks you no longer need. This cannot be undone — prefer archive_task_kandev when the task may still be wanted. Restoring an archived task is a user action done from the UI, not via MCP."),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task ID to delete")),
		),
		s.wrapHandler("delete_task_kandev", s.deleteTaskHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("archive_task_kandev",
			mcp.WithDescription("Archive a task. The task is hidden from active board views but kept in the database. Use to tidy up finished or abandoned tasks. Unarchiving is a user action done from the UI, not via MCP."),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task ID to archive")),
		),
		s.wrapHandler("archive_task_kandev", s.archiveTaskHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("message_task_kandev",
			mcp.WithDescription(`Send a follow-up prompt (message) to an existing task's primary session.

Use this to communicate with a sibling task, a parent task, or any task you know the ID of — for example to ask a delegated subtask for clarification, hand it new context, or nudge a paused task forward.

Behaviour by session state:
- Running/starting: the message is queued and delivered when the current turn ends. Pass delivery_mode="interrupt" to instead interrupt the target's current turn immediately so the message is delivered without waiting — keeps a parent's steering/stop messages from piling up behind a long-running child's turn. Only honored when you are the target task's direct parent; a non-parent sender requesting "interrupt" gets a hard error instead of being silently downgraded to "queued".
- Idle (waiting for input or completed): the message is sent immediately as a new turn (delivery_mode has no effect).
- Created (not yet started): the agent is started with this message as its first prompt (delivery_mode has no effect).
- Failed/cancelled: an error is returned (use create_task_kandev to start fresh).

Returns the dispatch status: "queued", "sent", or "started".`),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The target task's full UUID (not a truncated prefix)")),
			mcp.WithString("prompt", mcp.Required(), mcp.Description("The message to deliver to the task's agent")),
			mcp.WithString("delivery_mode",
				mcp.Enum("queued", "interrupt"),
				mcp.DefaultString("queued"),
				mcp.Description(`How to deliver this message if the target is currently running/starting. "queued" (default): wait for the current turn to finish, like any other peer message. "interrupt": cancel the target's current turn now and deliver this message immediately instead — only allowed when you are the target task's direct parent; requesting "interrupt" as a non-parent is rejected with an error rather than silently queued.`),
			),
		),
		s.wrapHandler("message_task_kandev", s.messageTaskHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("get_task_conversation_kandev",
			mcp.WithDescription("Get conversation history for a task. If session_id is omitted, the primary session is used."),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task ID")),
			mcp.WithString("session_id", mcp.Description("Optional session ID (must belong to task_id)")),
			mcp.WithNumber("limit", mcp.Description("Optional page size (defaults to backend setting, max backend-capped)")),
			mcp.WithString("before", mcp.Description("Optional cursor message ID to fetch messages before this ID")),
			mcp.WithString("after", mcp.Description("Optional cursor message ID to fetch messages after this ID")),
			mcp.WithString("sort", mcp.Description("Optional sort order: asc or desc")),
			mcp.WithArray("message_types", mcp.Description("Optional message type filters (e.g. message, tool_call, error)"), mcp.Items(map[string]any{"type": "string"})),
		),
		s.wrapHandler("get_task_conversation_kandev", s.getTaskConversationHandler()),
	)
}

// registerCreateTaskTool registers the create_task_kandev tool. Shared between
// kanban (task) mode and external mode. The tool description and parent_id
// guidance differ by mode: in external mode there is no current task, so the
// 'self' shorthand is omitted.
func (s *Server) registerCreateTaskTool() {
	toolDesc := `Create a new task or subtask and auto-start an agent on it.

WHEN TO USE parent_id='self':
- Breaking down your current task into phases/steps → use parent_id='self'
- Creating tasks from a plan → use parent_id='self' (inherits repo, task workspace, workflow, and materialized workspace by default)
- Delegating work to another agent → use parent_id='self'
- Delegating work that lives in a sibling repo → use parent_id='self' AND pass repository_url / repository_id / local_path to point the subtask at that repo

WHEN TO OMIT parent_id (top-level task):
- Creating an unrelated, standalone task
- Provide a repository via repository_url, repository_id, or local_path
- workspace_id and workflow_id are auto-resolved if only one exists; provide explicitly if ambiguous

IMPORTANT:
- Subtasks inherit task workspace, workflow, agent profile, executor, and materialized workspace from the parent by default. Pass workspace_id/workflow_id only when deliberately targeting a different task workspace/workflow; any supplied workflow_id must belong to the effective workspace_id. Pass workspace_mode='new_workspace' when the subtask needs its own materialized workspace/worktree.
- Agent profile precedence is explicit agent_profile_id > current/source task or parent task > workflow defaults > workspace default
- When launched from a current task, omitting parent_id still uses the current task as the source for profile inheritance before workflow/workspace defaults. Do not rely on workspace defaults for follow-up work from an active task.
- Every created task must have a resolvable agent profile. start_agent=false still records the profile for a later manual start.
- Subtasks inherit the parent's repository unless you supply repository_url, repository_id, or local_path — in which case the subtask targets that repo instead
- base_branch behaviour:
  - Same repo as parent (no repo args): subtask inherits the parent's base_branch (sibling branches off the same starting point — useful for PR stacks)
  - Different repo (you passed repository_url / repository_id / local_path): subtask defaults to that repo's default_branch
  - Pass base_branch explicitly to override either default. Use list_repositories_kandev to see each repo's default_branch.
- Top-level tasks need a repository via repository_url, repository_id, or local_path
- 'description' is the sub-agent's initial prompt — be specific and detailed
- start_agent defaults to true and is what you want in nearly every case — the new task auto-launches an agent that immediately works on the description. Pass start_agent=false ONLY for an explicit placeholder (e.g. queuing work the user will start later, or creating a tracking task with no immediate work), and still pass agent_profile_id unless it can be inherited. When in doubt, leave it true.
- Kanban subtasks cannot have their own subtasks (max nesting depth is 1). To break work down further, create a sibling under the same parent. (Office task trees are exempt.)`
	parentDesc := "Parent task ID for subtasks. Use 'self' to create a subtask of your current task (RECOMMENDED for plan phases, delegated work). Omit only for unrelated top-level tasks."
	agentProfileDesc := "Agent profile ID to use. Precedence: explicit value > current/source task or parent task > workflow defaults > workspace default. Required unless one of those inheritance sources can resolve it. start_agent=false still needs a profile for later manual start."

	if s.mode == ModeExternal {
		toolDesc = `Create a new top-level task and auto-start an agent on it.

IMPORTANT:
- Provide a repository via repository_url, repository_id, or local_path
- workspace_id and workflow_id are auto-resolved if only one exists; provide explicitly if ambiguous
- Agent profile precedence is explicit agent_profile_id > parent task > workflow defaults > workspace default. External mode has no current/source task.
- Every created task must have a resolvable agent profile. start_agent=false still records the profile for a later manual start.
- 'description' is the agent's initial prompt — be specific and detailed
- start_agent defaults to true and is what you want in nearly every case — the new task auto-launches an agent that immediately works on the description. Pass start_agent=false ONLY for an explicit placeholder (e.g. queuing work the user will start later), and still pass agent_profile_id unless a default exists. When in doubt, leave it true.
- Use parent_id only when delegating to a known existing task by its ID`
		parentDesc = "Optional parent task ID. Omit for top-level tasks; provide an existing task ID only to create a subtask of that task."
		agentProfileDesc = "Agent profile ID to use. Precedence: explicit value > parent task > workflow defaults > workspace default. External mode has no current/source task. Required unless one of those inheritance sources can resolve it. start_agent=false still needs a profile for later manual start."
	}

	s.mcpServer.AddTool(
		mcp.NewTool("create_task_kandev",
			mcp.WithDescription(toolDesc),
			mcp.WithString("parent_id", mcp.Description(parentDesc)),
			mcp.WithString("workspace_id", mcp.Description("The workspace ID. Auto-resolved if only one workspace exists. Defaulted from parent for subtasks when omitted.")),
			mcp.WithString("workflow_id", mcp.Description("The workflow ID. Auto-resolved if the workspace has only one workflow. Defaulted from parent for subtasks when workspace_id is also omitted; if supplied, it must belong to the effective workspace_id.")),
			mcp.WithString("workflow_step_id", mcp.Description("The workflow step ID (optional, auto-resolved if omitted; for subtasks, pass only with an explicit workflow_id)")),
			mcp.WithString("workspace_mode", mcp.Description("Subtask materialized-workspace mode: inherit_parent reuses the parent's worktree/materialized workspace (default for subtasks); new_workspace launches the subtask in its own workspace/worktree.")),
			mcp.WithString("title", mcp.Required(), mcp.Description("The task title")),
			mcp.WithString("description", mcp.Description("The initial prompt for the sub-agent. This is the ONLY context the agent receives when it starts — treat it as the agent's first user message. REQUIRED for subtasks: without a description the sub-agent starts with no context and cannot do useful work. Be specific and detailed.")),
			mcp.WithString("agent_profile_id", mcp.Description(agentProfileDesc)),
			mcp.WithString("executor_profile_id", mcp.Description("Executor profile ID to use (determines the runtime environment: local, worktree, docker, etc.). For subtasks, inherited from the parent session. For top-level tasks, ask the user which executor profile they want if not already known.")),
			mcp.WithBoolean("start_agent", mcp.Description("Whether to auto-start an agent on the created task. Default: true — leave it true unless you specifically want a placeholder task with no agent running. Setting false leaves the task waiting for the user to click 'Start agent' in the UI; the description is preserved but no work happens automatically.")),
			mcp.WithString("repository_id", mcp.Description("Repository ID. Required for top-level tasks unless local_path or repository_url is provided. For subtasks: optional — supply only when the subtask should target a different repo than the parent.")),
			mcp.WithString("local_path", mcp.Description("Local repository folder path (e.g. '/Users/me/projects/myrepo'). Will create/find the repository automatically. Preferred for local worktree flow. For subtasks: supply only when the subtask should target a different repo than the parent.")),
			mcp.WithString("repository_url", mcp.Description("GitHub repository URL (e.g. 'https://github.com/owner/repo'). The repository will be cloned automatically on first use. For subtasks: supply only when the subtask should target a different repo than the parent.")),
			mcp.WithString("base_branch", mcp.Description("Base branch for the repository (e.g. 'main'). Optional. Defaults: same-repo subtasks inherit the parent's base_branch; cross-repo subtasks and top-level tasks fall back to the repository's default_branch (visible via list_repositories_kandev).")),
		),
		s.wrapHandler("create_task_kandev", s.createTaskHandler()),
	)
}

// registerAddBranchToTaskTool registers add_branch_to_task_kandev. Scoped to
// task mode only — external coding agents have no live session context to
// attach the new worktree to, and shipping this tool through the shared
// create-task path would silently widen the external surface.
func (s *Server) registerAddBranchToTaskTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("add_branch_to_task_kandev",
			mcp.WithDescription(`Attach an additional (repository, branch) worktree to an existing task.

Use this when the task should open more than one PR — same repo with different branches, or a second repository entirely. The new branch gets its own worktree under the task directory and behaves like any other multi-repo entry for changes, PRs, and review surfaces.

IMPORTANT:
- Only works on tasks running the WORKTREE executor. Tasks on docker / sprites / local-pc / SSH / remote_docker reject this tool because sibling worktrees are a git-worktree-specific layout — other executors bind one workspace path per task and the new branch would silently never appear on disk.
- task_id defaults to your CURRENT task when omitted — pass it explicitly only to target a different task.
- Repository selection (matches create_task_kandev): pass exactly one of repository_id / repository_url / local_path. For single-repo tasks all three are optional — the service auto-resolves to the task's only repository. Multi-repo tasks must identify the target repo explicitly.
- checkout_branch is the branch the new worktree will check out. Leave empty to create a fresh feature branch from base_branch.
- base_branch is optional; defaults to the repository's default_branch.
- The (task_id, repository_id, base_branch, checkout_branch) tuple must be unique on the task — re-adding the same combination is an error, not a no-op.`),
			mcp.WithString("task_id", mcp.Description("The task to attach the branch to. Defaults to the current task when omitted.")),
			mcp.WithString("repository_id", mcp.Description("Repository UUID. Optional for single-repo tasks (auto-resolved). Required for multi-repo tasks unless repository_url or local_path is supplied.")),
			mcp.WithString("repository_url", mcp.Description("GitHub repository URL (e.g. 'https://github.com/owner/repo'). Alternative to repository_id when you don't have the UUID handy. The repository is found-or-created in the task's workspace.")),
			mcp.WithString("local_path", mcp.Description("Local repository folder path (e.g. '/Users/me/projects/myrepo'). Alternative to repository_id for the local worktree flow. The repository is found-or-created in the task's workspace.")),
			mcp.WithString("checkout_branch", mcp.Description("Existing branch to check out in the new worktree (e.g. a PR head branch). Empty to create a fresh feature branch from base_branch.")),
			mcp.WithString("base_branch", mcp.Description("Branch to base the worktree on. Defaults to the repository's default_branch.")),
		),
		s.wrapHandler("add_branch_to_task_kandev", s.addBranchToTaskHandler()),
	)
}

func (s *Server) addBranchToTaskHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID := req.GetString(mcpKeyTaskID, "")
		if taskID == "" {
			taskID = s.taskID
		}
		if taskID == "" {
			return mcp.NewToolResultError("task_id is required (no current task context to default to)"), nil
		}
		// Mutual-exclusion gate at the MCP tier so the error names the
		// agent-facing alias (repository_url) instead of the WS wire field
		// (github_url). The WS handler still re-validates for direct WS
		// callers that don't go through this tool.
		repositoryID := req.GetString(mcpKeyRepositoryID, "")
		repositoryURL := req.GetString(mcpKeyRepositoryURL, "")
		localPath := req.GetString(mcpKeyLocalPath, "")
		if locatorCount(repositoryID, repositoryURL, localPath) > 1 {
			return mcp.NewToolResultError("pass at most one of repository_id, repository_url, local_path"), nil
		}
		// repository_url is the tool-facing alias used by create_task_kandev;
		// translate to github_url on the wire so the WS handler can reuse the
		// same field name as the rest of the multi-repo payloads.
		payload := map[string]interface{}{
			mcpKeyTaskID:         taskID,
			mcpKeyRepositoryID:   repositoryID,
			mcpKeyLocalPath:      localPath,
			mcpKeyGitHubURL:      repositoryURL,
			mcpKeyCheckoutBranch: req.GetString(mcpKeyCheckoutBranch, ""),
			mcpKeyBaseBranch:     req.GetString(mcpKeyBaseBranch, ""),
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPAddBranchToTask, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

// registerUpdateRepositoryBaseBranchTool registers
// update_repository_base_branch_kandev. Lets an agent or the UI change the
// base branch used for diff stats / changes panel comparison after a task
// has already been created — used by promotion-chain users who branched
// from a release branch instead of `main`.
func (s *Server) registerUpdateRepositoryBaseBranchTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("update_repository_base_branch_kandev",
			mcp.WithDescription(`Change the base branch used by a task repository for diff stats and the Changes panel.

Use when a task was created against the wrong base (e.g. picked up `+"`main`"+` when the work was forked from a release / QA / staging branch). The Changes panel and per-task +/- counts compare HEAD against this branch.

Scope: this updates the value the WorkspaceTracker uses for diff comparison (BaseCommit / Ahead / Behind / cumulative diff). It does NOT auto-set the PR target on push; the PR target is whatever value the caller passes to the create-PR endpoint at push time. Callers that want both to move together should pass the new base_branch on the next PR-create call.

The agentctl tracker is updated live: a successful call refreshes BaseCommit / Ahead / Behind without needing a session restart.`),
			mcp.WithString("task_id", mcp.Description("The task whose repository to update. Defaults to the current task when omitted.")),
			mcp.WithString("task_repository_id", mcp.Description("UUID of the task_repositories row to update. Required — disambiguates multi-repo tasks. Find it via list_tasks_kandev's repositories[] field.")),
			mcp.WithString("base_branch", mcp.Description("New base branch name (e.g. 'staging', 'release/v2.4'). Required.")),
		),
		s.wrapHandler("update_repository_base_branch_kandev", s.updateRepositoryBaseBranchHandler()),
	)
}

func (s *Server) updateRepositoryBaseBranchHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID := req.GetString(mcpKeyTaskID, "")
		if taskID == "" {
			taskID = s.taskID
		}
		if taskID == "" {
			return mcp.NewToolResultError("task_id is required (no current task context to default to)"), nil
		}
		taskRepositoryID := req.GetString(mcpKeyTaskRepositoryID, "")
		if taskRepositoryID == "" {
			return mcp.NewToolResultError("task_repository_id is required"), nil
		}
		baseBranch := req.GetString(mcpKeyBaseBranch, "")
		if baseBranch == "" {
			return mcp.NewToolResultError("base_branch is required"), nil
		}
		payload := map[string]interface{}{
			mcpKeyTaskID:           taskID,
			mcpKeyTaskRepositoryID: taskRepositoryID,
			mcpKeyBaseBranch:       baseBranch,
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPUpdateRepositoryBaseBranch, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

// registerStepCompleteTool registers step_complete_kandev — the ADR 0015
// explicit completion signal. The tool is bound to the current (task, session)
// and writes a pending-signal entry on the session's metadata bag; the
// orchestrator consumes that signal to drive the workflow's on_turn_complete
// transitions. Steps with `auto_advance_requires_signal=false` (the legacy
// default) ignore the signal entirely.
func (s *Server) registerStepCompleteTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("step_complete_kandev",
			mcp.WithDescription(`Signal that every user-stated requirement for the CURRENT workflow step is satisfied.

WHEN TO CALL:
- All work for the current step is finished and the task is ready to move forward in the workflow.
- This is the LAST thing you do in the step — call it after the final tool call / commit / answer that completes the requested work.

WHEN NOT TO CALL:
- You are about to ask the user a question (use ask_user_question_kandev instead and wait).
- The work is partially done or you ran into a blocker you couldn't resolve.
- You are mid-conversation and expect the user to reply with more direction.

BEHAVIOUR:
- The call is idempotent within a step: subsequent calls return accepted=false with reason="already_signaled" and have no other effect.
- The call returns immediately. The workflow transition (if the step is configured to auto-advance) is driven asynchronously by the orchestrator on turn-end.
- If the user sends another message before the transition fires, the signal is cancelled and the conversation continues on the current step. Call again at the end of the new turn if appropriate.
- For steps that do NOT have auto-advance enabled, the call succeeds (accepted=true) but the workflow does not move automatically. The signal is discarded on the next turn start; there is no separate audit history to query later.

The summary you provide is shown to the user in chat and may be forwarded to the next step's agent as a hand-off note.`),
			mcp.WithString("summary", mcp.Required(), mcp.Description("One-paragraph plain-text summary of what was done in this step. Shown to the user.")),
			mcp.WithString("handoff", mcp.Description("Optional context the next step's agent will need to pick up where you left off (decisions, open files, follow-ups).")),
			mcp.WithString("blockers", mcp.Description("Optional list of known unresolved issues. Use sparingly — only when the step is complete in the sense that you cannot make further progress without input, not for normal partial work.")),
		),
		s.wrapHandler("step_complete_kandev", s.stepCompleteHandler()),
	)
}

func (s *Server) stepCompleteHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if s.taskID == "" || s.sessionID == "" {
			return mcp.NewToolResultError("step_complete_kandev requires a bound task and session"), nil
		}
		summary := strings.TrimSpace(req.GetString("summary", ""))
		if summary == "" {
			return mcp.NewToolResultError("summary is required"), nil
		}
		payload := map[string]interface{}{
			"task_id":    s.taskID,
			"session_id": s.sessionID,
			"summary":    summary,
			"handoff":    req.GetString("handoff", ""),
			"blockers":   req.GetString("blockers", ""),
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPStepComplete, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) registerInteractionTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("ask_user_question_kandev",
			mcp.WithDescription(`Ask the user one or more clarifying questions in a single tool call.

Use this tool when you need user input to proceed. Bundle related questions
together in one call so the user answers them all in one back-and-forth instead
of sequential round-trips. Each question is rendered as its own card; the user
selects an option or provides a custom text response per question, and the
agent receives a map keyed by question id once every question has been answered.

IMPORTANT:
- Provide 1 to 4 questions per call.
- Each question must have 2 to 6 concrete, actionable options.
- Each option must have a short "label" (1-5 words) and a "description"
  explaining what selecting it means. NEVER use meta-text like "Answer below".
- Only call this tool when you genuinely need information you cannot infer.

Example usage:
{
  "questions": [
    {
      "id": "db",
      "prompt": "Which database should I use for this project?",
      "options": [
        {"label": "PostgreSQL", "description": "Relational, good for complex queries"},
        {"label": "MongoDB", "description": "Document database, flexible schema"},
        {"label": "SQLite", "description": "Embedded, simple setup"}
      ]
    },
    {
      "id": "migration",
      "prompt": "How should I handle the existing user data during migration?",
      "options": [
        {"label": "Migrate all", "description": "Keep all existing records"},
        {"label": "Archive old", "description": "Archive records older than 1 year"},
        {"label": "Fresh start", "description": "Delete existing data and start fresh"}
      ]
    }
  ],
  "context": "Backend redesign — picking the persistence layer and migration policy together."
}

The response is a JSON object keyed by each question id. Each entry may include
"selected_option" (the option_id the user picked), "custom_text" (the user's
free-form answer; can co-exist with a selected option), or "answered": false
when the user did not respond to that question. When the user skipped the entire
bundle, the envelope also carries "rejected": true and an optional
"reject_reason". Example success response:
{
  "db": {"selected_option": "q1_opt1"},
  "migration": {"custom_text": "Migrate all but flag rows older than 2 years"}
}
Example rejection:
{
  "rejected": true,
  "reject_reason": "User skipped",
  "db": {"answered": false, "rejected": true},
  "migration": {"answered": false, "rejected": true}
}`),
			mcp.WithArray(questionsArg, mcp.Required(),
				mcp.Description(`Array of 1-4 question objects. Each question must have a "prompt" (the question text) and an "options" array (2-6 entries with label + description). Optional fields: "id" (stable identifier in the response map; auto-generated if omitted), "title" (≤12 chars short label).`),
				mcp.MinItems(1),
				mcp.MaxItems(4),
				mcp.Items(buildQuestionSchemaItem()),
			),
			mcp.WithString("context", mcp.Description("Optional shared background information to help the user understand why you're asking these questions.")),
		),
		s.wrapHandler("ask_user_question_kandev", s.askUserQuestionHandler()),
	)
}

func (s *Server) registerPlanTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("create_task_plan_kandev",
			mcp.WithDescription("Create or save a task plan. Use this to save your implementation plan for the current task."),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task ID to create a plan for")),
			mcp.WithString("content", mcp.Required(), mcp.Description("The plan content in markdown format")),
			mcp.WithString("title", mcp.Description("Optional title for the plan (default: 'Plan')")),
		),
		s.wrapHandler("create_task_plan_kandev", s.createTaskPlanHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("get_task_plan_kandev",
			mcp.WithDescription("Get the current plan for a task. Use this to retrieve an existing plan, including any user edits."),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task ID to get the plan for")),
		),
		s.wrapHandler("get_task_plan_kandev", s.getTaskPlanHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("update_task_plan_kandev",
			mcp.WithDescription("Update an existing task plan. Use this to modify the plan during implementation."),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task ID to update the plan for")),
			mcp.WithString("content", mcp.Required(), mcp.Description("The updated plan content in markdown format")),
			mcp.WithString("title", mcp.Description("Optional new title for the plan")),
		),
		s.wrapHandler("update_task_plan_kandev", s.updateTaskPlanHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("delete_task_plan_kandev",
			mcp.WithDescription("Delete a task plan."),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task ID to delete the plan for")),
		),
		s.wrapHandler("delete_task_plan_kandev", s.deleteTaskPlanHandler()),
	)
}

// registerWalkthroughTools registers the agent-authored code-walkthrough tools.
// show_walkthrough is the JetBrains-style "walk a person through the code" tool:
// the agent supplies ordered, file+line-anchored steps that the user cycles
// through as popovers over the review diff with Previous/Next.
func (s *Server) registerWalkthroughTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("show_walkthrough_kandev",
			mcp.WithDescription(
				"Show and store a guided code walkthrough for this task. Accepts an ordered list of "+
					"steps; each step anchors a short markdown explanation to a specific file line or "+
					"line range, and renders as a popover over the review diff/editor. The user cycles "+
					"through steps with Previous and Next. The walkthrough is saved to the task and "+
					"replaces any prior one. Only reference files that exist in the task's local worktree "+
					"or current review diff; for PR-only files, do not assume the PR head is checked out "+
					"locally. Use line_end when a logical explanation spans multiple lines. "+
					"Use this after producing a change to narrate the diff (what each hunk does and why), "+
					"or to explain how a part of the codebase works. Order steps to follow the reader's "+
					"natural path through the code (entry point first, then the call chain). Keep text "+
					"concise and do not add a 'Justification:' preamble."),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task ID to attach the walkthrough to")),
			mcp.WithString("title", mcp.Description("Optional title for the walkthrough (default: 'Walkthrough')")),
			mcp.WithArray("steps", mcp.Required(),
				mcp.Description("Ordered list of walkthrough steps, each anchored to a file line or range."),
				mcp.Items(buildWalkthroughStepSchemaItem()),
			),
		),
		s.wrapHandler("show_walkthrough_kandev", s.showWalkthroughHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("get_walkthrough_kandev",
			mcp.WithDescription("Get the current code walkthrough for a task, including any steps."),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task ID to get the walkthrough for")),
		),
		s.wrapHandler("get_walkthrough_kandev", s.getWalkthroughHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("delete_walkthrough_kandev",
			mcp.WithDescription("Delete the code walkthrough for a task."),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task ID to delete the walkthrough for")),
		),
		s.wrapHandler("delete_walkthrough_kandev", s.deleteWalkthroughHandler()),
	)
}

// buildWalkthroughStepSchemaItem describes one step object in the
// show_walkthrough_kandev tool schema.
func buildWalkthroughStepSchemaItem() map[string]any {
	const typeKey = "type"
	str := func(desc string) map[string]any {
		return map[string]any{typeKey: "string", descriptionArg: desc}
	}
	num := func(desc string) map[string]any {
		return map[string]any{typeKey: "integer", descriptionArg: desc}
	}
	return map[string]any{
		typeKey: "object",
		"properties": map[string]any{
			titleArg: str("Optional short heading for this step."),
			"repo":   str("Optional repository name; disambiguates in multi-repo reviews."),
			"file": str(
				"Path to a file present in the task worktree or current review diff, relative to the repo root.",
			),
			"line": num("1-based start line to anchor the popover to."),
			"line_end": num(
				"Optional 1-based end line. Use this for multi-line ranges instead of adjacent single-line steps.",
			),
			"text": str(
				"Concise markdown explanation shown in the step popover. Do not start with 'Justification:'.",
			),
		},
		"required": []string{"file", "line", "text"},
	}
}

// buildQuestionSchemaItem describes the shape of a single question object in
// the ask_user_question_kandev tool schema. Hoisted out of registerInteractionTools
// to keep the registration body short and to deduplicate the JSON-schema
// keyword strings (linter goconst rules).
func buildQuestionSchemaItem() map[string]any {
	const typeKey = "type"
	const propsKey = "properties"
	const reqKey = "required"
	const objType = "object"
	const stringType = "string"

	str := func(desc string) map[string]any {
		return map[string]any{typeKey: stringType, descriptionArg: desc}
	}

	return map[string]any{
		typeKey: objType,
		propsKey: map[string]any{
			idArg:     str("Stable identifier used as the key in the response map. Auto-assigned (q1, q2, ...) if omitted."),
			titleArg:  str("Optional short label (≤12 chars) shown above the prompt."),
			promptArg: str("The question text shown to the user."),
			optionsArg: map[string]any{
				typeKey:        "array",
				descriptionArg: "2-6 concrete, actionable choices.",
				"minItems":     2,
				"maxItems":     6,
				"items": map[string]any{
					typeKey: objType,
					propsKey: map[string]any{
						labelArg:       str("Short text (1-5 words) shown as the clickable option."),
						descriptionArg: str("Brief explanation of what this option means."),
					},
					reqKey: []string{labelArg, descriptionArg},
				},
			},
		},
		reqKey: []string{promptArg, optionsArg},
	}
}
