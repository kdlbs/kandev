// Package mcp provides MCP server functionality for agentctl.
// It exposes MCP tools that forward requests to the Kandev backend via the agent stream.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
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
)

// normalizeMode returns a valid MCP mode, defaulting unknown values to ModeTask.
func normalizeMode(mode string) string {
	switch mode {
	case ModeConfig, ModeExternal:
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
	mode               string // "task" (default) or "config"
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
//
// baseURL is the publicly reachable backend URL (e.g. "http://localhost:38429").
// It is used to build the message endpoint URL emitted in SSE events.
func NewExternal(backend BackendClient, baseURL string, log *logger.Logger, mcpLogFile string) *Server {
	// External mode has no live session, so disable ask-question and use empty IDs.
	s := newServer(backend, "", "", log, mcpLogFile, true, ModeExternal)

	// SSE handlers are mounted at /mcp/sse and /mcp/message — the static base path
	// makes the SSE endpoint event emit the correct full message URL.
	s.sseServer = server.NewSSEServer(s.mcpServer,
		server.WithBaseURL(baseURL),
		server.WithStaticBasePath("/mcp"),
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
		count += 11
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
		count += 11
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
	default: // ModeTask
		s.registerKanbanTools()
		count += 11
		if !s.disableAskQuestion {
			s.registerInteractionTools()
			count++
		}
		s.registerPlanTools()
		count += 4
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
			mcp.WithDescription("List all tasks in a workflow."),
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
			mcp.WithDescription("Move a task to a different workflow step. Optionally send a hand-off prompt to the receiving agent — required only when handing the task off mid-turn (e.g. QA → review) with specific instructions. Plain admin/config moves can omit prompt."),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task ID")),
			mcp.WithString("workflow_id", mcp.Required(), mcp.Description("Target workflow ID")),
			mcp.WithString("workflow_step_id", mcp.Required(), mcp.Description("Target workflow step ID")),
			mcp.WithNumber("position", mcp.Description("Position within the step (0-based)")),
			mcp.WithString("prompt", mcp.Description("Optional hand-off message for the receiving agent. When supplied AND the source session is mid-turn, the move is deferred to the agent's turn-end and the prompt is delivered at the new step (concatenated after the step's own auto_start prompt, if any). Omit for plain admin/config moves where there's no agent to address.")),
		),
		s.wrapHandler("move_task_kandev", s.moveTaskHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("message_task_kandev",
			mcp.WithDescription(`Send a follow-up prompt (message) to an existing task's primary session.

Use this to communicate with a sibling task, a parent task, or any task you know the ID of — for example to ask a delegated subtask for clarification, hand it new context, or nudge a paused task forward.

Behaviour by session state:
- Running/starting: the message is queued and delivered when the current turn ends.
- Idle (waiting for input or completed): the message is sent immediately as a new turn.
- Created (not yet started): the agent is started with this message as its first prompt.
- Failed/cancelled: an error is returned (use create_task_kandev to start fresh).

Returns the dispatch status: "queued", "sent", or "started".`),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The target task ID")),
			mcp.WithString("prompt", mcp.Required(), mcp.Description("The message to deliver to the task's agent")),
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
- Creating tasks from a plan → use parent_id='self' (inherits repo, workspace, workflow)
- Delegating work to another agent → use parent_id='self'

WHEN TO OMIT parent_id (top-level task):
- Creating an unrelated, standalone task
- Provide a repository via repository_url, repository_id, or local_path
- workspace_id and workflow_id are auto-resolved if only one exists; provide explicitly if ambiguous

IMPORTANT:
- Subtasks inherit repositories, workspace, workflow, agent profile, and executor from parent
- Top-level tasks need a repository via repository_url, repository_id, or local_path
- 'description' is the sub-agent's initial prompt — be specific and detailed
- Set start_agent=false to create without starting an agent`
	parentDesc := "Parent task ID for subtasks. Use 'self' to create a subtask of your current task (RECOMMENDED for plan phases, delegated work). Omit only for unrelated top-level tasks."

	if s.mode == ModeExternal {
		toolDesc = `Create a new top-level task and auto-start an agent on it.

IMPORTANT:
- Provide a repository via repository_url, repository_id, or local_path
- workspace_id and workflow_id are auto-resolved if only one exists; provide explicitly if ambiguous
- 'description' is the agent's initial prompt — be specific and detailed
- Set start_agent=false to create without starting an agent
- Use parent_id only when delegating to a known existing task by its ID`
		parentDesc = "Optional parent task ID. Omit for top-level tasks; provide an existing task ID only to create a subtask of that task."
	}

	s.mcpServer.AddTool(
		mcp.NewTool("create_task_kandev",
			mcp.WithDescription(toolDesc),
			mcp.WithString("parent_id", mcp.Description(parentDesc)),
			mcp.WithString("workspace_id", mcp.Description("The workspace ID. Auto-resolved if only one workspace exists. Inherited from parent for subtasks.")),
			mcp.WithString("workflow_id", mcp.Description("The workflow ID. Auto-resolved if the workspace has only one workflow. Inherited from parent for subtasks.")),
			mcp.WithString("workflow_step_id", mcp.Description("The workflow step ID (optional, auto-resolved if omitted)")),
			mcp.WithString("title", mcp.Required(), mcp.Description("The task title")),
			mcp.WithString("description", mcp.Description("The initial prompt for the sub-agent. This is the ONLY context the agent receives when it starts — treat it as the agent's first user message. REQUIRED for subtasks: without a description the sub-agent starts with no context and cannot do useful work. Be specific and detailed.")),
			mcp.WithString("agent_profile_id", mcp.Description("Agent profile ID to use. For subtasks, inherited from the parent session. For top-level tasks, ask the user which agent profile they want (e.g. Claude Code, OpenCode) if not already known.")),
			mcp.WithString("executor_profile_id", mcp.Description("Executor profile ID to use (determines the runtime environment: local, worktree, docker, etc.). For subtasks, inherited from the parent session. For top-level tasks, ask the user which executor profile they want if not already known.")),
			mcp.WithBoolean("start_agent", mcp.Description("Whether to auto-start an agent on the created task. Default: true. Set to false to create the task without starting an agent.")),
			mcp.WithString("repository_id", mcp.Description("Repository ID for top-level tasks. Not needed for subtasks (inherited from parent).")),
			mcp.WithString("local_path", mcp.Description("Local repository folder path (e.g. '/Users/me/projects/myrepo') for top-level tasks. Will create/find the repository automatically. Preferred for local worktree flow.")),
			mcp.WithString("repository_url", mcp.Description("GitHub repository URL (e.g. 'https://github.com/owner/repo') for top-level tasks. The repository will be cloned automatically on first use. Not needed for subtasks (inherited from parent).")),
			mcp.WithString("base_branch", mcp.Description("Base branch for the repository (e.g. 'main'). Optional, defaults to repository's default branch.")),
		),
		s.wrapHandler("create_task_kandev", s.createTaskHandler()),
	)
}

func (s *Server) registerInteractionTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("ask_user_question_kandev",
			mcp.WithDescription(`Ask the user a clarifying question with multiple choice options.

Use this tool when you need user input to proceed. The user will see the prompt and can select one of the options or provide a custom text response.

IMPORTANT: Each option must be a concrete, actionable choice - NOT meta-text like "Answer questions below".

Options format - array of objects with:
- "label": Short text (1-5 words) shown as the clickable option
- "description": Brief explanation of what this option means

Example usage:
{
  "prompt": "Which database should I use for this project?",
  "options": [
    {"label": "PostgreSQL", "description": "Relational database, good for complex queries"},
    {"label": "MongoDB", "description": "Document database, flexible schema"},
    {"label": "SQLite", "description": "Embedded database, simple setup"}
  ],
  "context": "The project requires storing user profiles and relationships between entities."
}

Another example:
{
  "prompt": "How should I handle the existing user data during migration?",
  "options": [
    {"label": "Migrate all", "description": "Keep all existing records"},
    {"label": "Archive old", "description": "Archive records older than 1 year"},
    {"label": "Fresh start", "description": "Delete existing data and start fresh"}
  ]
}`),
			mcp.WithString("prompt", mcp.Required(), mcp.Description("The question to ask the user. Be specific and clear.")),
			mcp.WithArray("options", mcp.Required(), mcp.Description(`Array of option objects. Each option must have: "label" (1-5 words, the clickable choice) and "description" (explanation of the option). Provide 2-4 concrete, actionable options.`),
				mcp.Items(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"label": map[string]any{
							"type":        "string",
							"description": "Short text (1-5 words) shown as the clickable option",
						},
						"description": map[string]any{
							"type":        "string",
							"description": "Brief explanation of what this option means",
						},
					},
					"required": []string{"label", "description"},
				}),
			),
			mcp.WithString("context", mcp.Description("Optional background information to help the user understand why you're asking this question.")),
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
