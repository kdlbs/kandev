// Package mcp provides MCP server functionality for agentctl.
// It exposes MCP tools that forward requests to the Kandev backend via WebSocket.
package mcp

import (
	"context"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/server/wsclient"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"
)

// Server wraps the MCP server with WebSocket client for backend communication.
type Server struct {
	wsClient   *wsclient.Client
	sessionID  string
	mcpServer  *server.MCPServer
	sseServer  *server.SSEServer
	httpServer *server.StreamableHTTPServer
	logger     *logger.Logger
	mu         sync.Mutex
	running    bool
}

// New creates a new MCP server for agentctl.
func New(wsClient *wsclient.Client, sessionID string, log *logger.Logger) *Server {
	s := &Server{
		wsClient:  wsClient,
		sessionID: sessionID,
		logger:    log.WithFields(zap.String("component", "mcp-server")),
	}

	// Create MCP server
	s.mcpServer = server.NewMCPServer(
		"kandev-mcp",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	// Register tools
	s.registerTools()

	// Create SSE server for Claude Desktop, Cursor, etc.
	s.sseServer = server.NewSSEServer(s.mcpServer)

	// Create Streamable HTTP server for Codex
	s.httpServer = server.NewStreamableHTTPServer(s.mcpServer,
		server.WithEndpointPath("/mcp"),
	)

	return s
}

// RegisterRoutes adds MCP routes to the gin router.
func (s *Server) RegisterRoutes(router gin.IRouter) {
	// SSE transport routes
	router.GET("/sse", gin.WrapH(s.sseServer.SSEHandler()))
	router.POST("/message", gin.WrapH(s.sseServer.MessageHandler()))

	// Streamable HTTP transport route
	router.Any("/mcp", gin.WrapH(s.httpServer))

	s.logger.Info("registered MCP routes", zap.String("sse", "/sse"), zap.String("http", "/mcp"))
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

	return nil
}

// registerTools registers all MCP tools.
func (s *Server) registerTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("list_workspaces",
			mcp.WithDescription("List all workspaces. Use this first to get workspace IDs."),
		),
		s.listWorkspacesHandler(),
	)

	s.mcpServer.AddTool(
		mcp.NewTool("list_boards",
			mcp.WithDescription("List all boards in a workspace."),
			mcp.WithString("workspace_id", mcp.Required(), mcp.Description("The workspace ID")),
		),
		s.listBoardsHandler(),
	)

	s.mcpServer.AddTool(
		mcp.NewTool("list_workflow_steps",
			mcp.WithDescription("List all workflow steps in a board."),
			mcp.WithString("board_id", mcp.Required(), mcp.Description("The board ID")),
		),
		s.listWorkflowStepsHandler(),
	)

	s.mcpServer.AddTool(
		mcp.NewTool("list_tasks",
			mcp.WithDescription("List all tasks on a board."),
			mcp.WithString("board_id", mcp.Required(), mcp.Description("The board ID")),
		),
		s.listTasksHandler(),
	)

	s.mcpServer.AddTool(
		mcp.NewTool("create_task",
			mcp.WithDescription("Create a new task on a board."),
			mcp.WithString("workspace_id", mcp.Required(), mcp.Description("The workspace ID")),
			mcp.WithString("board_id", mcp.Required(), mcp.Description("The board ID")),
			mcp.WithString("workflow_step_id", mcp.Required(), mcp.Description("The workflow step ID")),
			mcp.WithString("title", mcp.Required(), mcp.Description("The task title")),
			mcp.WithString("description", mcp.Description("The task description")),
		),
		s.createTaskHandler(),
	)

	s.mcpServer.AddTool(
		mcp.NewTool("update_task",
			mcp.WithDescription("Update an existing task."),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task ID")),
			mcp.WithString("title", mcp.Description("New title")),
			mcp.WithString("description", mcp.Description("New description")),
			mcp.WithString("state", mcp.Description("New state: not_started, in_progress, etc.")),
		),
		s.updateTaskHandler(),
	)

	s.mcpServer.AddTool(
		mcp.NewTool("ask_user_question",
			mcp.WithDescription("Ask the user a clarifying question."),
			mcp.WithString("prompt", mcp.Required(), mcp.Description("The question to ask")),
			mcp.WithArray("options", mcp.Required(), mcp.Description("Options for the user")),
			mcp.WithString("context", mcp.Description("Context for the question")),
		),
		s.askUserQuestionHandler(),
	)

	s.logger.Info("registered MCP tools", zap.Int("count", 7))
}

// WrapHandler wraps a handler function with error handling for gin.
func WrapHandler(h http.Handler) gin.HandlerFunc {
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

