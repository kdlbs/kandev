// Package mcp provides MCP server functionality for agentctl.
// It exposes MCP tools that forward requests to the Kandev backend via the agent stream.
package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/mcp/plugintools"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpproviders "github.com/kandev/kandev/internal/mcp/providers"
	"github.com/kandev/kandev/internal/mcp/toolschema"
	"github.com/kandev/kandev/internal/mcp/tooltokens"
	"github.com/kandev/kandev/internal/task/service"
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
	// ModeTaskTitlePending registers the task-mode tools plus the one-shot
	// title tool used while a prompt-first task still has its provisional title.
	ModeTaskTitlePending = "task-title-pending"
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

const pluginToolArgumentsKey = "arguments"

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
	case ModeConfig, ModeExternal, ModeOffice, ModeTaskTitlePending:
		return mode
	default:
		return ModeTask
	}
}

// Server wraps the MCP server with backend client for communication.
type Server struct {
	backend             BackendClient
	sessionID           string
	taskID              string
	disableAskQuestion  bool
	mode                string // "task" (default), "task-title-pending", "config", or "office"
	mcpProviders        []string
	profile             mcpprofile.Context
	mcpServer           *server.MCPServer
	sseServer           *server.SSEServer
	httpServer          *server.StreamableHTTPServer
	logger              *logger.Logger
	mcpLogger           *zap.Logger // optional file logger for MCP debug traces
	mu                  sync.RWMutex
	running             bool
	attachmentMu        sync.RWMutex
	attachmentAttempt   streams.MCPAttachmentAttempt
	attachmentAttempts  map[string]streams.MCPAttachmentAttempt
	attachmentReporter  func(streams.MCPAttachmentEvidence)
	validatorMu         sync.RWMutex
	toolValidators      map[string]toolArgumentValidator
	pluginToolsUpdateMu sync.Mutex
	pluginToolsMu       sync.Mutex
	pluginTools         plugintools.Snapshot
	pluginToolsReady    bool
}

// New creates a new MCP server for agentctl.
// port is the HTTP server port used to build the SSE base URL (http://localhost:<port>).
// mcpLogFile is an optional file path for MCP debug logging; pass "" to disable.
func New(backend BackendClient, sessionID, taskID string, port int, log *logger.Logger, mcpLogFile string, disableAskQuestion bool, mcpMode string, mcpProviders ...[]string) *Server {
	var providers []string
	if len(mcpProviders) > 0 {
		providers = mcpProviders[0]
	}
	s := newServerWithProfile(backend, sessionID, taskID, log, mcpLogFile, mcpprofile.Legacy(mcpMode, disableAskQuestion, providers))

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

// NewWithProfile creates an MCP server from the backend-owned typed profile.
// The profile keeps base surfaces and additive capability groups separate so
// callers can add or remove one context-specific group without copying a full
// mode branch.
func NewWithProfile(backend BackendClient, sessionID, taskID string, port int, log *logger.Logger, mcpLogFile string, disableAskQuestion bool, profileContext mcpprofile.Context) *Server {
	if disableAskQuestion {
		profileContext = profileContext.WithoutCapability(mcpprofile.CapabilityUserQuestion)
	}
	s := newServerWithProfile(backend, sessionID, taskID, log, mcpLogFile, profileContext)
	s.sseServer = server.NewSSEServer(s.mcpServer,
		server.WithBaseURL(fmt.Sprintf("http://localhost:%d", port)),
	)
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
	s := newServerWithProfile(backend, "", "", log, mcpLogFile, mcpprofile.Legacy(ModeExternal, true, nil))

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
func newServer(backend BackendClient, sessionID, taskID string, log *logger.Logger, mcpLogFile string, disableAskQuestion bool, mcpMode string, mcpProviders []string) *Server {
	return newServerWithProfile(backend, sessionID, taskID, log, mcpLogFile, mcpprofile.Legacy(mcpMode, disableAskQuestion, mcpProviders))
}

func newServerWithProfile(backend BackendClient, sessionID, taskID string, log *logger.Logger, mcpLogFile string, profileContext mcpprofile.Context) *Server {
	profileContext = mcpprofile.New(profileContext.Surface, profileContext.Capabilities, profileContext.Providers)
	s := &Server{
		backend:            backend,
		sessionID:          sessionID,
		taskID:             taskID,
		disableAskQuestion: !profileContext.HasCapability(mcpprofile.CapabilityUserQuestion),
		mode:               modeForProfile(profileContext),
		mcpProviders:       mcpproviders.Normalize(profileContext.Providers),
		profile:            profileContext,
		logger:             log.WithFields(zap.String("component", "mcp-server")),
		attachmentAttempts: make(map[string]streams.MCPAttachmentAttempt),
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

	hooks := &server.Hooks{}
	s.mcpServer = server.NewMCPServer(
		"kandev-mcp",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithHooks(hooks),
	)
	hooks.AddOnRegisterSession(func(_ context.Context, session server.ClientSession) {
		s.registerMCPConnection(session.SessionID())
	})
	hooks.AddAfterInitialize(func(ctx context.Context, _ any, _ *mcp.InitializeRequest, _ *mcp.InitializeResult) {
		s.observeMCPConnection(mcpConnectionID(ctx), streams.MCPAttachmentEvidenceInitializeObserved, 0, "")
	})
	hooks.AddAfterListTools(func(ctx context.Context, _ any, _ *mcp.ListToolsRequest, result *mcp.ListToolsResult) {
		s.observeMCPToolsList(mcpConnectionID(ctx), result.Tools)
	})
	hooks.AddBeforeListTools(func(ctx context.Context, _ any, _ *mcp.ListToolsRequest) {
		s.syncPluginTools(ctx)
	})
	hooks.AddAfterCallTool(func(ctx context.Context, _ any, _ *mcp.CallToolRequest, _ *mcp.CallToolResult) {
		s.observeMCPConnection(mcpConnectionID(ctx), streams.MCPAttachmentEvidenceToolCallObserved, 0, "")
	})
	hooks.AddOnError(func(ctx context.Context, _ any, _ mcp.MCPMethod, _ any, err error) {
		s.observeMCPConnection(mcpConnectionID(ctx), streams.MCPAttachmentEvidenceExplicitError, 0, err.Error())
	})
	hooks.AddOnUnregisterSession(func(_ context.Context, session server.ClientSession) {
		s.unregisterMCPConnection(session.SessionID())
	})
	s.registerTools()
	s.running = true
	return s
}

func modeForProfile(profileContext mcpprofile.Context) string {
	switch profileContext.Surface {
	case mcpprofile.SurfaceConfiguration:
		return ModeConfig
	case mcpprofile.SurfaceExternal:
		return ModeExternal
	case mcpprofile.SurfaceOfficeTask:
		return ModeOffice
	case mcpprofile.SurfaceKanbanTask:
		if profileContext.HasCapability(mcpprofile.CapabilityTaskTitle) {
			return ModeTaskTitlePending
		}
		return ModeTask
	default:
		return ModeTask
	}
}

// SetAttachmentReporter routes safe MCP observations to the instance's
// existing agent update stream. It accepts a concrete callback to keep the MCP
// package independent of process-manager implementation details.
func (s *Server) SetAttachmentReporter(reporter func(streams.MCPAttachmentEvidence)) {
	s.attachmentMu.Lock()
	defer s.attachmentMu.Unlock()
	s.attachmentReporter = reporter
}

// SetAttachmentAttempt selects the backend-owned attempt to which subsequent
// MCP endpoint observations belong. It is called only by the local agentctl
// API before handing configuration to an agent adapter.
func (s *Server) SetAttachmentAttempt(attempt streams.MCPAttachmentAttempt) {
	s.attachmentMu.Lock()
	defer s.attachmentMu.Unlock()
	s.attachmentAttempt = attempt
}

func (s *Server) observeMCPConnection(connectionID string, kind streams.MCPAttachmentEvidenceKind, toolCount int, summary string) {
	s.observeMCPConnectionWithTools(connectionID, kind, toolCount, summary, nil)
}

func (s *Server) observeMCPToolsList(connectionID string, tools []mcp.Tool) {
	summaries := make([]streams.MCPToolSummary, 0, len(tools))
	for _, tool := range tools {
		summaries = append(summaries, summarizeMCPTool(tool))
	}
	s.observeMCPConnectionWithTools(connectionID, streams.MCPAttachmentEvidenceToolsListObserved, len(tools), "", summaries)
}

func summarizeMCPTool(tool mcp.Tool) streams.MCPToolSummary {
	summary := streams.MCPToolSummary{
		Name:        tool.Name,
		Description: tool.Description,
		InputSchema: mcpToolInputSchema(tool),
	}
	definition, err := json.Marshal(tool)
	if err != nil {
		return summary
	}
	summary.EstimatedTokens, _ = tooltokens.EstimateToolJSON(definition)
	return summary
}

func mcpToolInputSchema(tool mcp.Tool) json.RawMessage {
	if tool.RawInputSchema != nil {
		if !json.Valid(tool.RawInputSchema) {
			return nil
		}
		return append(json.RawMessage(nil), tool.RawInputSchema...)
	}
	schema, err := json.Marshal(tool.InputSchema)
	if err != nil {
		return nil
	}
	return schema
}

func (s *Server) observeMCPConnectionWithTools(
	connectionID string,
	kind streams.MCPAttachmentEvidenceKind,
	toolCount int,
	summary string,
	tools []streams.MCPToolSummary,
) {
	s.attachmentMu.RLock()
	attempt, ok := s.attachmentAttempts[connectionID]
	reporter := s.attachmentReporter
	s.attachmentMu.RUnlock()
	if !ok || reporter == nil || attempt.AttemptID == "" {
		return
	}
	s.reportMCPConnectionWithTools(reporter, attempt, connectionID, kind, toolCount, summary, tools)
}

func (s *Server) registerMCPConnection(connectionID string) {
	s.attachmentMu.Lock()
	attempt := s.attachmentAttempt
	reporter := s.attachmentReporter
	if connectionID != "" && attempt.AttemptID != "" {
		s.attachmentAttempts[connectionID] = attempt
	}
	s.attachmentMu.Unlock()
	if reporter == nil || attempt.AttemptID == "" {
		return
	}
	s.reportMCPConnection(reporter, attempt, connectionID, streams.MCPAttachmentEvidenceSessionAccepted, 0, "")
}

func (s *Server) unregisterMCPConnection(connectionID string) {
	s.attachmentMu.Lock()
	attempt, ok := s.attachmentAttempts[connectionID]
	reporter := s.attachmentReporter
	delete(s.attachmentAttempts, connectionID)
	s.attachmentMu.Unlock()
	if !ok || reporter == nil || attempt.AttemptID == "" {
		return
	}
	s.reportMCPConnection(reporter, attempt, connectionID, streams.MCPAttachmentEvidenceConnectionClosed, 0, "")
}

func (s *Server) reportMCPConnection(
	reporter func(streams.MCPAttachmentEvidence),
	attempt streams.MCPAttachmentAttempt,
	connectionID string,
	kind streams.MCPAttachmentEvidenceKind,
	toolCount int,
	summary string,
) {
	s.reportMCPConnectionWithTools(reporter, attempt, connectionID, kind, toolCount, summary, nil)
}

func (s *Server) reportMCPConnectionWithTools(
	reporter func(streams.MCPAttachmentEvidence),
	attempt streams.MCPAttachmentAttempt,
	connectionID string,
	kind streams.MCPAttachmentEvidenceKind,
	toolCount int,
	summary string,
	tools []streams.MCPToolSummary,
) {
	if kind == streams.MCPAttachmentEvidenceToolsListObserved {
		// Bound summaries at the publication boundary before the callback can
		// expose evidence to the agent update stream. Apply repeats the bound as
		// a defense for any other evidence producer.
		tools, _ = streams.NormalizeMCPToolCatalog(tools, toolCount)
	}
	reporter(streams.MCPAttachmentEvidence{
		AttemptID:          attempt.AttemptID,
		ServerName:         "kandev",
		Kind:               kind,
		OccurredAt:         time.Now().UTC(),
		Source:             streams.MCPServerSourceKandev,
		ConnectionID:       opaqueMCPConnectionID(connectionID),
		ToolCount:          toolCount,
		Tools:              tools,
		ToolTokenEstimator: toolTokenEstimator(tools),
		Summary:            streams.SanitizeMCPErrorSummary(summary),
	})
}

func toolTokenEstimator(tools []streams.MCPToolSummary) string {
	for _, tool := range tools {
		if tool.EstimatedTokens > 0 {
			return tooltokens.Estimator
		}
	}
	return ""
}

func mcpConnectionID(ctx context.Context) string {
	session := server.ClientSessionFromContext(ctx)
	if session == nil {
		return ""
	}
	return session.SessionID()
}

func opaqueMCPConnectionID(connectionID string) string {
	if connectionID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(connectionID))
	return fmt.Sprintf("mcp-%x", sum[:8])
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
	return s.wrapHandlerWithArgumentLogging(toolName, handler, true)
}

func (s *Server) wrapSensitiveHandler(toolName string, handler server.ToolHandlerFunc) server.ToolHandlerFunc {
	return s.wrapHandlerWithArgumentLogging(toolName, handler, false)
}

func (s *Server) wrapHandlerWithArgumentLogging(toolName string, handler server.ToolHandlerFunc, logArguments bool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()

		fields := []zap.Field{zap.String("tool", toolName)}
		if logArguments {
			fields = append(fields, zap.Any("args", req.GetArguments()))
		}
		s.logger.Debug("MCP tool call", fields...)
		if s.mcpLogger != nil {
			mcpFields := append([]zap.Field(nil), fields...)
			mcpFields = append(mcpFields, zap.String("session_id", s.sessionID))
			s.mcpLogger.Debug("MCP tool call", mcpFields...)
		}

		validatedReq, validationErr := s.validateToolArguments(toolName, req)
		var result *mcp.CallToolResult
		var err error
		if validationErr != nil {
			result = mcp.NewToolResultError(validationErr.Error())
		} else {
			result, err = handler(ctx, validatedReq)
		}
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
			resultFields := []zap.Field{
				zap.String("tool", toolName),
				zap.Duration("duration", duration),
			}
			if logArguments {
				resultFields = append(resultFields, zap.Any("result", result.Content))
			}
			s.logger.Debug("MCP tool returned error", resultFields...)
			if s.mcpLogger != nil {
				mcpResultFields := append([]zap.Field(nil), resultFields...)
				mcpResultFields = append(mcpResultFields, zap.String("session_id", s.sessionID))
				s.mcpLogger.Debug("MCP tool returned error", mcpResultFields...)
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

	normalizedMode := normalizeMode(mode)
	if s.mode == normalizedMode {
		return
	}
	s.mode = normalizedMode
	s.profile = mcpprofile.New(surfaceForMode(normalizedMode), s.profile.Capabilities, s.mcpProviders)
	if normalizedMode == ModeTaskTitlePending {
		s.profile = s.profile.WithCapability(mcpprofile.CapabilityTaskTitle)
	} else {
		s.profile = s.profile.WithoutCapability(mcpprofile.CapabilityTaskTitle)
	}
	s.rebuildTools()
}

func surfaceForMode(mode string) mcpprofile.Surface {
	switch mode {
	case ModeConfig:
		return mcpprofile.SurfaceConfiguration
	case ModeExternal:
		return mcpprofile.SurfaceExternal
	case ModeOffice:
		return mcpprofile.SurfaceOfficeTask
	default:
		return mcpprofile.SurfaceKanbanTask
	}
}

// SetProviders replaces the provider capabilities advertised by task mode.
// The MCP mode itself is preserved while the effective tool registry is rebuilt.
func (s *Server) SetProviders(providerValues []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalizedProviders := mcpproviders.Normalize(providerValues)
	if sameProviderSet(s.mcpProviders, normalizedProviders) {
		return
	}
	s.mcpProviders = normalizedProviders
	s.profile.Providers = normalizedProviders
	s.rebuildTools()
}

// Profile returns a copy of the effective backend-owned MCP profile.
func (s *Server) Profile() mcpprofile.Context {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return mcpprofile.New(s.profile.Surface, s.profile.Capabilities, s.profile.Providers)
}

// SetProfile replaces the complete profile and rebuilds the tool registry in
// one atomic operation. This is the preferred runtime update seam. Legacy
// SetMode and SetProviders remain available for older agentctl callers.
func (s *Server) SetProfile(profileContext mcpprofile.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	profileContext = mcpprofile.New(profileContext.Surface, profileContext.Capabilities, profileContext.Providers)
	if sameProfile(s.profile, profileContext) {
		return
	}
	s.profile = profileContext
	s.mode = modeForProfile(profileContext)
	s.disableAskQuestion = !profileContext.HasCapability(mcpprofile.CapabilityUserQuestion)
	s.mcpProviders = mcpproviders.Normalize(profileContext.Providers)
	s.rebuildTools()
}

func sameProfile(left, right mcpprofile.Context) bool {
	if left.Surface != right.Surface || len(left.Capabilities) != len(right.Capabilities) || len(left.Providers) != len(right.Providers) {
		return false
	}
	for i := range left.Capabilities {
		if left.Capabilities[i] != right.Capabilities[i] {
			return false
		}
	}
	for i := range left.Providers {
		if left.Providers[i] != right.Providers[i] {
			return false
		}
	}
	return true
}

func (s *Server) rebuildTools() {
	// Build against an isolated registry so the live server remains unchanged
	// until the complete replacement is ready. mcp-go emits one notification
	// for SetTools, while registering directly would expose every intermediate
	// AddTool state to initialized clients.
	s.mcpServer.SetTools(s.assembleTools()...)
}

// SetPluginTools validates and atomically replaces sideloaded tools. SetTools
// emits one tools/list_changed notification to initialized MCP clients.
func (s *Server) SetPluginTools(snapshot plugintools.Snapshot) error {
	s.pluginToolsUpdateMu.Lock()
	defer s.pluginToolsUpdateMu.Unlock()
	if err := validatePluginToolSnapshot(snapshot); err != nil {
		return err
	}
	normalized := plugintools.Normalize(snapshot)
	s.pluginToolsMu.Lock()
	if s.pluginToolsReady && normalized.Generation == s.pluginTools.Generation && normalized.Revision <= s.pluginTools.Revision {
		s.pluginToolsMu.Unlock()
		return nil
	}
	if s.pluginToolsReady && equivalentPluginToolCatalog(s.pluginTools, normalized) {
		s.pluginTools = normalized
		s.pluginToolsMu.Unlock()
		return nil
	}
	s.pluginTools = normalized
	s.pluginToolsReady = true
	s.pluginToolsMu.Unlock()
	s.mu.Lock()
	s.rebuildTools()
	s.mu.Unlock()
	return nil
}

func equivalentPluginToolCatalog(left, right plugintools.Snapshot) bool {
	left.Generation, right.Generation = "", ""
	left.Revision, right.Revision = 0, 0
	return plugintools.Equal(left, right)
}

func validatePluginToolSnapshot(snapshot plugintools.Snapshot) error {
	if snapshot.Generation == "" {
		return fmt.Errorf("plugin tool snapshot generation is required")
	}
	seen := make(map[string]struct{}, len(snapshot.Tools))
	for i, definition := range snapshot.Tools {
		name := fmt.Sprintf("plugin tool %d", i)
		if definition.PluginID == "" || definition.LocalName == "" || definition.ExposedName == "" || definition.Description == "" {
			return fmt.Errorf("%s has incomplete identity or description", name)
		}
		if expected := plugintools.ExposedName(definition.PluginID, definition.LocalName); definition.ExposedName != expected {
			return fmt.Errorf("%s exposed name %q does not match %q", name, definition.ExposedName, expected)
		}
		if _, ok := seen[definition.ExposedName]; ok {
			return fmt.Errorf("duplicate plugin tool exposed name %q", definition.ExposedName)
		}
		seen[definition.ExposedName] = struct{}{}
		if err := validatePluginToolSurfaces(name, definition.Surfaces); err != nil {
			return err
		}
		if err := validatePluginToolSchema(definition.ExposedName+"/input", definition.InputSchema); err != nil {
			return fmt.Errorf("%s input schema: %w", name, err)
		}
		if len(definition.OutputSchema) > 0 {
			if err := validatePluginToolSchema(definition.ExposedName+"/output", definition.OutputSchema); err != nil {
				return fmt.Errorf("%s output schema: %w", name, err)
			}
		}
	}
	return nil
}

func validatePluginToolSurfaces(name string, surfaces []string) error {
	if len(surfaces) == 0 {
		return fmt.Errorf("%s has no surfaces", name)
	}
	seen := make(map[string]struct{}, len(surfaces))
	for _, surface := range surfaces {
		if surface != plugintools.SurfaceKanban && surface != plugintools.SurfaceOffice {
			return fmt.Errorf("%s has unsupported surface %q", name, surface)
		}
		if _, ok := seen[surface]; ok {
			return fmt.Errorf("%s duplicates surface %q", name, surface)
		}
		seen[surface] = struct{}{}
	}
	return nil
}

func validatePluginToolSchema(name string, raw json.RawMessage) error {
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		return fmt.Errorf("decode schema: %w", err)
	}
	if _, err := toolschema.Compile(name, document); err != nil {
		return err
	}
	return nil
}

func (s *Server) syncPluginTools(ctx context.Context) {
	if s.backend == nil {
		return
	}
	requestCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	s.mu.RLock()
	surface := string(s.profile.Surface)
	s.mu.RUnlock()
	var snapshot plugintools.Snapshot
	if err := s.backend.RequestPayload(requestCtx, ws.ActionMCPListPluginTools, map[string]string{"surface": surface}, &snapshot); err != nil {
		return
	}
	if err := s.SetPluginTools(snapshot); err != nil {
		s.logger.Warn("ignoring invalid plugin tool catalog", zap.Error(err))
	}
}

func (s *Server) registerPluginTools() {
	s.pluginToolsMu.Lock()
	ready := s.pluginToolsReady
	snapshot := plugintools.Normalize(s.pluginTools)
	s.pluginToolsMu.Unlock()
	if !ready {
		return
	}
	for _, definition := range snapshot.Tools {
		if !pluginToolSupportsSurface(definition, string(s.profile.Surface)) {
			continue
		}
		tool := mcp.NewToolWithRawSchema(definition.ExposedName, definition.Description, definition.InputSchema)
		tool.RawOutputSchema = append(json.RawMessage(nil), definition.OutputSchema...)
		tool.Annotations = mcp.ToolAnnotation{
			ReadOnlyHint: &definition.ReadOnlyHint, DestructiveHint: &definition.DestructiveHint,
			IdempotentHint: &definition.IdempotentHint, OpenWorldHint: &definition.OpenWorldHint,
		}
		d := definition
		s.mcpServer.AddTool(tool, s.wrapSensitiveHandler(d.ExposedName, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			s.mu.RLock()
			surface := string(s.profile.Surface)
			s.mu.RUnlock()
			payload := map[string]any{
				"plugin_id": d.PluginID, "local_name": d.LocalName, pluginToolArgumentsKey: req.GetArguments(),
				"invocation_id": fmt.Sprintf("mcp-%d", time.Now().UnixNano()), "surface": surface,
			}
			var result struct {
				Text              string         `json:"text"`
				StructuredContent map[string]any `json:"structured_content,omitempty"`
				IsError           bool           `json:"is_error"`
			}
			if err := s.backend.RequestPayload(ctx, ws.ActionMCPInvokePluginTool, payload, &result); err != nil {
				return nil, err
			}
			if result.IsError {
				return mcp.NewToolResultError(result.Text), nil
			}
			if result.StructuredContent != nil {
				return mcp.NewToolResultStructured(result.StructuredContent, result.Text), nil
			}
			return mcp.NewToolResultText(result.Text), nil
		}))
	}
}

func pluginToolSupportsSurface(definition plugintools.Definition, surface string) bool {
	for _, allowed := range definition.Surfaces {
		if allowed == surface {
			return true
		}
	}
	return false
}

func sameProviderSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func (s *Server) assembleTools() []server.ServerTool {
	activeServer := s.mcpServer
	assemblyServer := server.NewMCPServer(
		"kandev-mcp",
		"1.0.0",
		server.WithToolCapabilities(true),
	)
	s.mcpServer = assemblyServer
	defer func() { s.mcpServer = activeServer }()

	s.registerTools()
	registered := assemblyServer.ListTools()
	tools := make([]server.ServerTool, 0, len(registered))
	for _, entry := range registered {
		tools = append(tools, *entry)
	}
	return tools
}

type profileToolGroup struct {
	name     string
	enabled  func(mcpprofile.Context) bool
	register func(*Server)
}

func surfaceEnabled(surface mcpprofile.Surface) func(mcpprofile.Context) bool {
	return func(ctx mcpprofile.Context) bool { return ctx.Surface == surface }
}

func capabilityEnabled(capability mcpprofile.Capability) func(mcpprofile.Context) bool {
	return func(ctx mcpprofile.Context) bool { return ctx.HasCapability(capability) }
}

func andProfilePredicates(predicates ...func(mcpprofile.Context) bool) func(mcpprofile.Context) bool {
	return func(ctx mcpprofile.Context) bool {
		for _, predicate := range predicates {
			if !predicate(ctx) {
				return false
			}
		}
		return true
	}
}

func (s *Server) profileToolGroups() []profileToolGroup {
	config := surfaceEnabled(mcpprofile.SurfaceConfiguration)
	external := surfaceEnabled(mcpprofile.SurfaceExternal)
	office := surfaceEnabled(mcpprofile.SurfaceOfficeTask)
	kanban := surfaceEnabled(mcpprofile.SurfaceKanbanTask)
	return []profileToolGroup{
		{name: "configuration-workflows", enabled: func(ctx mcpprofile.Context) bool { return config(ctx) || external(ctx) }, register: func(s *Server) { s.registerConfigWorkflowTools() }},
		{name: "configuration-agents", enabled: func(ctx mcpprofile.Context) bool { return config(ctx) || external(ctx) }, register: func(s *Server) { s.registerConfigAgentTools() }},
		{name: "configuration-mcp", enabled: func(ctx mcpprofile.Context) bool { return config(ctx) || external(ctx) }, register: func(s *Server) { s.registerConfigMcpTools() }},
		{name: "configuration-executors", enabled: func(ctx mcpprofile.Context) bool { return config(ctx) || external(ctx) }, register: func(s *Server) { s.registerConfigExecutorTools() }},
		{name: "configuration-tasks", enabled: func(ctx mcpprofile.Context) bool { return config(ctx) || external(ctx) }, register: func(s *Server) { s.registerConfigTaskTools() }},
		{name: "external-create-task", enabled: external, register: func(s *Server) { s.registerCreateTaskTool() }},
		{name: "external-questions", enabled: external, register: func(s *Server) { s.registerQuestionAnsweringTools() }},
		{name: "external-agent-permissions", enabled: external, register: func(s *Server) { s.registerAgentPermissionTools() }},
		// Dependency edges are manageable wherever a task can be created.
		{name: "task-dependencies", enabled: func(ctx mcpprofile.Context) bool { return kanban(ctx) || external(ctx) }, register: func(s *Server) { s.registerTaskDependencyTools() }},
		{name: "kanban-task", enabled: kanban, register: func(s *Server) { s.registerKanbanTools() }},
		{name: "github-pr", enabled: andProfilePredicates(kanban, func(ctx mcpprofile.Context) bool { return mcpproviders.Contains(ctx.Providers, mcpproviders.GitHub) }), register: func(s *Server) { s.registerPRAutomationTools() }},
		{name: "gitlab-mr", enabled: andProfilePredicates(kanban, func(ctx mcpprofile.Context) bool { return mcpproviders.Contains(ctx.Providers, mcpproviders.GitLab) }), register: func(s *Server) { s.registerMRAutomationTools() }},
		{name: "user-question", enabled: capabilityEnabled(mcpprofile.CapabilityUserQuestion), register: func(s *Server) { s.registerInteractionTools() }},
		{name: "parent-question", enabled: andProfilePredicates(kanban, capabilityEnabled(mcpprofile.CapabilityParentQuestion)), register: func(s *Server) { s.registerParentQuestionTool() }},
		{name: "plan", enabled: func(ctx mcpprofile.Context) bool { return kanban(ctx) || office(ctx) }, register: func(s *Server) { s.registerPlanTools() }},
		{name: "rich-output", enabled: func(ctx mcpprofile.Context) bool { return kanban(ctx) || office(ctx) }, register: func(s *Server) { s.registerRichOutputTool() }},
		{name: "walkthrough", enabled: kanban, register: func(s *Server) { s.registerWalkthroughTools() }},
		{name: "review", enabled: kanban, register: func(s *Server) { s.registerReviewTools() }},
		{name: "related-tasks", enabled: func(ctx mcpprofile.Context) bool { return kanban(ctx) || office(ctx) }, register: func(s *Server) { s.registerRelatedTasksTool() }},
		{name: "office-documents", enabled: office, register: func(s *Server) { s.registerTaskDocumentTools() }},
		{name: "office-decisions", enabled: office, register: func(s *Server) { s.registerRecordStepDecisionTool() }},
		{name: "task-branch-sources", enabled: kanban, register: func(s *Server) {
			s.registerAddBranchToTaskTool()
			s.registerAddWorkspaceSourcesTool()
			s.registerUpdateRepositoryBaseBranchTool()
		}},
		{name: "step-completion", enabled: kanban, register: func(s *Server) { s.registerStepCompleteTool() }},
		{name: "task-title", enabled: andProfilePredicates(kanban, capabilityEnabled(mcpprofile.CapabilityTaskTitle)), register: func(s *Server) { s.registerSetTaskTitleTool() }},
		{name: "diagnostics", enabled: kanban, register: func(s *Server) { s.registerDiagnosticBundleTool() }},
	}
}

func (s *Server) registerAgentPermissionTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("list_pending_agent_permissions_kandev",
			mcp.WithDescription("List live pending agent command and tool permission requests for an authorized task, optionally limited to one session. Returns safe action details and the exact immutable provider-offered option IDs and kinds needed for resolution."),
			mcp.WithString(mcpKeyTaskID, mcp.Required(), mcp.Description("The task ID whose live permission requests to list")),
			mcp.WithString("session_id", mcp.Description("Optional session ID, which must belong to task_id")),
		),
		s.wrapHandler("list_pending_agent_permissions_kandev", s.listPendingAgentPermissionsHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("resolve_agent_permission_kandev",
			mcp.WithDescription("Resolve one exact live agent permission request by selecting one option the provider originally offered. First call list_pending_agent_permissions_kandev and submit the returned task, session, request, pending, and option IDs unchanged. Reject choices are selected by their offered option ID; cancellation and caller-authored commands or options are not accepted."),
			mcp.WithString(mcpKeyTaskID, mcp.Required(), mcp.Description("The task ID returned by permission discovery")),
			mcp.WithString("session_id", mcp.Required(), mcp.Description("The session ID returned by permission discovery")),
			mcp.WithString("request_id", mcp.Required(), mcp.Description("The immutable Kandev request generation ID")),
			mcp.WithString("pending_id", mcp.Required(), mcp.Description("The provider pending request ID")),
			mcp.WithString("option_id", mcp.Required(), mcp.Description("One exact option ID returned for this request")),
		),
		s.wrapHandler("resolve_agent_permission_kandev", s.resolveAgentPermissionHandler()),
	)
}

// registerTools registers MCP tools from the declarative profile registry.
// Each group owns one additive capability or base surface. The registry is
// backend-owned: an agent receives the result, not an arbitrary tool list.
func (s *Server) registerTools() {
	for _, group := range s.profileToolGroups() {
		if group.enabled(s.profile) {
			group.register(s)
		}
	}
	s.registerPluginTools()
	s.logger.Info("registered MCP tools",
		zap.String("mode", s.mode),
		zap.Int("count", len(s.mcpServer.ListTools())),
		zap.Bool("disable_ask_question", s.disableAskQuestion))
	s.rebuildToolArgumentValidators()
}

func (s *Server) registerDiagnosticBundleTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("get_diagnostic_bundle_kandev",
			mcp.WithDescription("Collect a bounded diagnostic ZIP for the current task session and materialize it inside this execution workspace. Request backend first for backend/runtime issues, frontend for browser issues, or all only when correlation requires both."),
			mcp.WithString("source",
				mcp.Required(),
				mcp.Enum("backend", "frontend", "all"),
				mcp.Description("Diagnostic source to collect: backend, frontend, or all"),
			),
		),
		s.wrapHandler("get_diagnostic_bundle_kandev", s.getDiagnosticBundleHandler()),
	)
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
			"List all configured agents with their profiles. Use this to find available agent_profile_ids for create_task_kandev and spawn_session_kandev.",
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
			mcp.WithString("title", mcp.MaxLength(service.TaskTitleMaxLength), mcp.Description("New concise task title (maximum 60 characters)")),
			mcp.WithString("description", mcp.Description("New description")),
			mcp.WithString("state", mcp.Description("New state: not_started, in_progress, etc.")),
			mcp.WithString("deferred_launch_prompt", mcp.Description("Replace the prompt a not-yet-started task will launch with. Only valid for a task created with blocked_by (+ start_agent), whose launch is still waiting on its dependencies — use it to refresh a brief that went stale while the chain ran. Rejected once the task has started; send new context with message_task_kandev instead. When this is rejected, no other field in the same call is applied.")),
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
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithIdempotentHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task ID to delete")),
		),
		s.wrapHandler("delete_task_kandev", s.deleteTaskHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("archive_task_kandev",
			mcp.WithDescription("Archive a task. The task is hidden from active board views but kept in the database. Use to tidy up finished or abandoned tasks. Archiving an already-archived task is a no-op that succeeds with already_archived: true. Unarchiving is a user action done from the UI, not via MCP."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task ID to archive")),
		),
		s.wrapHandler("archive_task_kandev", s.archiveTaskHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("message_task_kandev",
			mcp.WithDescription(`Send a prompt to an existing Kandev task session, not a native subagent. Use delivery_mode="queued" (default) for information that can wait, or delivery_mode="interrupt" for urgent replacement work on a running direct child; non-parent interrupts fail and if cancellation is unsafe, the prompt remains queued. Halt-only work uses stop_task_kandev. The primary session is used by default; if it is terminal, Kandev tries the newest session that can accept messages, or names spawn_session_kandev when none can. Pass reply_to_question_id for an autopilot child question. Returns "queued", "sent", or "started".`),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The target task's full UUID (not a truncated prefix)")),
			mcp.WithString("session_id", mcp.Description("Optional target session ID (must belong to task_id). Omit to message the task's primary session. Required when messaging a sibling session on your OWN task (task_id may then be your own task ID) — e.g. a session you spawned with spawn_session_kandev.")),
			mcp.WithString("prompt", mcp.Required(), mcp.Description("The message to deliver to the task's agent")),
			mcp.WithString("delivery_mode",
				mcp.Enum("queued", "interrupt"),
				mcp.DefaultString("queued"),
				mcp.Description(`How to deliver this message if the target is currently running/starting. "queued" (default): wait for the current turn to finish, like any other peer message. "interrupt": cancel the target's current turn now and deliver this message immediately instead — only allowed when you are the target task's direct parent; requesting "interrupt" as a non-parent is rejected with an error rather than silently queued.`),
			),
			mcp.WithString("reply_to_question_id", mcp.Description("Optional question ID from an autopilot child. When set, the direct parent answer is recorded against that pending question and the delivery is idempotent.")),
		),
		s.wrapHandler("message_task_kandev", s.messageTaskHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("stop_task_kandev",
			mcp.WithDescription(`Stop all live sessions on a direct child task. Only its direct parent may call this halt-only tool; self, sibling, parent, grandparent, unrelated, and cross-workspace requests fail. It does not send a prompt or start a replacement turn; use message_task_kandev with delivery_mode="interrupt" to stop and steer. Accepted sessions become CANCELLED and teardown runs asynchronously; an eligible active task moves to REVIEW. If nothing is running, returns status="not_running" without changing state. Worktrees, commits, records, descendants, and queued messages are preserved. CANCELLED sessions cannot be resumed; use spawn_session_kandev with a new prompt to restart in the same workspace.`),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString(mcpKeyTaskID, mcp.Required(), mcp.Description("The direct child task's full UUID (not a truncated prefix)")),
		),
		s.wrapHandler("stop_task_kandev", s.stopTaskHandler()),
	)
	s.registerSpawnSessionTool()
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
	s.registerListTaskSessionsTool()
}

func (s *Server) registerPRAutomationTools() {
	s.mcpServer.AddTool(
		mcp.NewToolWithRawSchema("get_task_pr_automation_kandev",
			"Get the current task's GitHub PR automation settings, including lifecycle notification switches. "+
				"The five automation switches are scoped per linked PR; pr_options carries one entry per PR "+
				"(repository_id, pr_number, and the five booleans). The top-level booleans are an aggregate "+
				"that reports true only when every linked PR has that switch on and at least one PR is linked "+
				"— use them to check whether a task-wide enable fully took, and use pr_options for anything "+
				"PR-specific.",
			json.RawMessage(`{"type":"object","properties":{}}`),
		),
		s.wrapHandler("get_task_pr_automation_kandev", s.getTaskPRAutomationHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("update_task_pr_automation_kandev",
			mcp.WithDescription(
				"Update this task's PR automation options (auto-fix, auto-merge, and lifecycle notifications). "+
					"The five switches are scoped per linked PR: pass both repository_id and pr_number to target "+
					"one linked PR, or omit both to apply the change to every PR currently linked to the task "+
					"(unchanged default behavior). auto_fix_prompt_override applies task-wide regardless of PR identity.",
			),
			mcp.WithString("repository_id", mcp.Description("Target one linked PR's repository_id; must be paired with pr_number. Omit both to apply to every linked PR.")),
			mcp.WithNumber("pr_number", mcp.Description("Target one linked PR's number; must be paired with repository_id. Omit both to apply to every linked PR.")),
			mcp.WithBoolean("auto_fix_enabled", mcp.Description("Enable or disable auto-fix when CI checks fail")),
			mcp.WithBoolean("auto_merge_enabled", mcp.Description("Enable or disable auto-merge when PR passes all checks")),
			mcp.WithString("auto_fix_prompt_override", mcp.Description("Custom prompt for auto-fix (empty string clears the override). Task-wide; not affected by repository_id/pr_number.")),
			mcp.WithBoolean("prompt_on_review_requested", mcp.Description("Prompt this task's agent when a review is requested for the authenticated user")),
			mcp.WithBoolean("prompt_on_merged", mcp.Description("Prompt this task's agent once when the linked PR becomes merged")),
			mcp.WithBoolean("prompt_on_closed", mcp.Description("Prompt this task's agent once when the linked PR becomes closed without merge")),
		),
		s.wrapHandler("update_task_pr_automation_kandev", s.updateTaskPRAutomationHandler()),
	)
}

func (s *Server) registerMRAutomationTools() {
	s.mcpServer.AddTool(
		mcp.NewToolWithRawSchema("get_task_mr_automation_kandev",
			"Get the current task's GitLab MR automation settings, including lifecycle notification switches.",
			json.RawMessage(`{"type":"object","properties":{}}`),
		),
		s.wrapHandler("get_task_mr_automation_kandev", s.getTaskMRAutomationHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("update_task_mr_automation_kandev",
			mcp.WithDescription("Update this task's GitLab merge request automation options (auto-fix, auto-merge, "+
				"and lifecycle notifications). The five switches are per merge request: pass repository_id, "+
				"project_path and mr_iid together to target one linked MR, or omit all three to apply them to "+
				"every MR linked to this task. auto_fix_prompt_override applies to every linked MR regardless "+
				"of MR identity."),
			mcp.WithString("repository_id", mcp.Description("Repository ID of the linked MR to target (omit to target every linked MR)")),
			mcp.WithString("project_path", mcp.Description("Project path of the linked MR to target, e.g. group/project")),
			mcp.WithNumber("mr_iid", mcp.Description("IID of the linked MR to target")),
			mcp.WithBoolean("auto_fix_enabled", mcp.Description("Enable or disable auto-fix when the linked MR's pipeline fails")),
			mcp.WithBoolean("auto_merge_enabled", mcp.Description("Enable or disable auto-merge when the linked MR is ready")),
			mcp.WithString("auto_fix_prompt_override", mcp.Description("Task-level custom prompt for auto-fix; valid without linked MRs and not scoped by MR identity (empty string clears the override)")),
			mcp.WithBoolean("prompt_on_review_requested", mcp.Description("Prompt this task's agent when a review is requested for the authenticated user")),
			mcp.WithBoolean("prompt_on_merged", mcp.Description("Prompt this task's agent once when the linked MR becomes merged")),
			mcp.WithBoolean("prompt_on_closed", mcp.Description("Prompt this task's agent once when the linked MR becomes closed without merge")),
		),
		s.wrapHandler("update_task_mr_automation_kandev", s.updateTaskMRAutomationHandler()),
	)
}

// registerCreateTaskTool registers the create_task_kandev tool. Shared between
// kanban (task) mode and external mode. The tool description and parent_id
// guidance differ by mode: in external mode there is no current task, so the
// 'self' shorthand is omitted.
func (s *Server) registerCreateTaskTool() {
	toolDesc := `Create a persistent Kandev task or subtask and optionally start its agent. Use only for user-requested Kandev-tracked work; use the host's native subagent mechanism for ordinary delegation. Set parent_id="self" for a subtask of the current task and omit it for an unrelated top-level task. Subtasks inherit the parent's workspace, workflow, repository, profiles, executor, and materialized workspace unless the matching input overrides them. external_id provides create-if-absent idempotency and is not a lookup.`
	parentDesc := "Parent task ID for subtasks. Use 'self' for a user-requested Kandev subtask or plan phase. Omit only for unrelated top-level tasks."
	agentProfileDesc := "Agent profile ID to use. On a workflow step, the step's launch profile (its pinned profile, or the workflow default when unpinned) outranks it; otherwise an explicit agent_profile_id wins. When both are absent, current_task uses the verified creating session's profile and effective model, mode, and dynamic options for a session-bound call, then falls back to the current/source or parent profile without verified session context. workspace_default skips those task profiles and the creating session, then uses workflow profiles before the target workspace default. Explicit profiles do not copy creator-session runtime values. start_agent=false still needs a resolvable profile for later manual start."

	if s.mode == ModeExternal {
		toolDesc = `Create a persistent Kandev task and optionally start its agent from an external client. Use only for user-requested Kandev-tracked work; use the host's native subagent mechanism for ordinary delegation. Provide a repository for a top-level task; parent_id may target a known task when the user requested a subtask. external_id provides create-if-absent idempotency and is not a lookup.`
		parentDesc = "Optional parent task ID. Omit for top-level tasks; provide an existing task ID only to create a subtask of that task."
		agentProfileDesc = "Agent profile ID to use. On a workflow step, the step's launch profile (its pinned profile, or the workflow default when unpinned) outranks it; otherwise an explicit agent_profile_id wins. When both are absent, current_task uses the parent task profile because external mode has no creating session or current/source task context; workspace_default skips the parent profile, then uses workflow profiles before the target workspace default. External mode never copies creator-session runtime values. start_agent=false still needs a resolvable profile for later manual start."
	}

	s.mcpServer.AddTool(
		mcp.NewTool("create_task_kandev",
			mcp.WithDescription(toolDesc),
			mcp.WithString("parent_id", mcp.Description(parentDesc)),
			mcp.WithString("workspace_id", mcp.Description("The workspace ID. Auto-resolved if only one workspace exists. Defaulted from parent for subtasks when omitted.")),
			mcp.WithString("workflow_id", mcp.Description("The workflow ID. Auto-resolved if the workspace has only one workflow. Defaulted from parent for subtasks when workspace_id is also omitted; if supplied, it must belong to the effective workspace_id.")),
			mcp.WithString("workflow_step_id", mcp.Description("The workflow step ID (optional, auto-resolved if omitted; for subtasks, pass only with an explicit workflow_id)")),
			mcp.WithString("workspace_mode", mcp.Description("Subtask materialized-workspace mode: inherit_parent reuses the parent's worktree/materialized workspace (default for subtasks); new_workspace launches the subtask in its own workspace/worktree.")),
			mcp.WithString("title", mcp.Required(), mcp.MaxLength(service.TaskTitleMaxLength), mcp.Description("A concise, few-word task title (maximum 60 characters).")),
			mcp.WithString("prompt", mcp.Description("The initial prompt for the task agent. This is the ONLY context the agent receives when it starts — treat it as the agent's first user message. For auto-started subtasks, provide a specific and detailed prompt; omitting it starts the task agent without task-specific context.")),
			mcp.WithBoolean("autopilot", mcp.Description("Start this task in autopilot mode. Default: false. The value is fixed at creation and is not inherited by subtasks. The agent does not ask the user directly; it asks its direct parent only for critical decisions.")),
			mcp.WithString("agent_profile_id", mcp.Description(agentProfileDesc)),
			mcp.WithString("executor_profile_id", mcp.Description("Executor profile ID to use (determines the runtime environment: local, worktree, docker, etc.). For subtasks, inherited from the parent session. For top-level tasks, ask the user which executor profile they want if not already known.")),
			mcp.WithBoolean("start_agent", mcp.Description("Whether to auto-start an agent on the created task. Default: true — leave it true unless you specifically want a placeholder task with no agent running. Setting false leaves the task waiting for the user to click 'Start agent' in the UI; the prompt is preserved but no work happens automatically.")),
			mcp.WithString("repository_id", mcp.Description("Repository ID. Required for top-level tasks unless local_path or repository_url is provided. For subtasks: optional — supply only when the subtask should target a different repo than the parent.")),
			mcp.WithString("local_path", mcp.Description("Local repository folder path (e.g. '/Users/me/projects/myrepo'). Will create/find the repository automatically. Preferred for local worktree flow. For subtasks: supply only when the subtask should target a different repo than the parent.")),
			mcp.WithString("repository_url", mcp.Description("Repository URL, GitHub pull request URL, or GitLab merge request URL (for example 'https://github.com/owner/repo'). A contribution URL attaches the task to that existing contribution and prepares its source branch. For subtasks: supply only when the subtask should target a different repo than the parent.")),
			mcp.WithString("base_branch", mcp.Description("Base branch for the repository (e.g. 'main'). Optional. Defaults: same-repo subtasks inherit the parent's base_branch; cross-repo subtasks and top-level tasks fall back to the repository's default_branch (visible via list_repositories_kandev).")),
			mcp.WithString("external_id", mcp.Description("A stable identifier from your own system (issue key, webhook delivery ID, a UUID you generated). Creating a task twice with the same external_id in the same workspace returns the first task instead of making a duplicate — use it when a retry or restart could re-run this call. Replay the same arguments you sent the first time. This creates the task when nothing holds the identity yet — it is not a lookup.")),
			mcp.WithArray("blocked_by",
				mcp.Description(blockedByParamDesc),
				mcp.Items(map[string]any{"type": "string"}),
			),
			mcp.WithBoolean("start_when_unblocked", mcp.Description(startWhenUnblockedParamDesc)),
		),
		s.wrapHandler("create_task_kandev", s.createTaskHandler()),
	)
}

// Dependency parameter descriptions for create_task_kandev. Extracted as
// constants because the same guidance has to appear on the two dependency tools
// below and must not drift between them.
const (
	blockedByParamDesc = "Task IDs this task depends on: it will not start until every one of them completes SUCCESSFULLY. " +
		"Use this — not parent_id — to express ordering. A subtask means \"part of\"; a dependency means \"not until\". " +
		"Decomposing a plan into ordered phases is N sibling tasks chained with blocked_by, NOT N subtasks started at once. " +
		"A predecessor that ends FAILED or CANCELLED halts the chain and needs human action; it will not retry itself."

	startWhenUnblockedParamDesc = "Whether to start this task automatically once every task in blocked_by completes successfully. " +
		"Defaults to true when blocked_by is non-empty, which is what chains ordered work: with blocked_by set, " +
		"start_agent=true records this intent instead of launching now, so the whole chain does not start at once. " +
		"Pass false to create the dependency edges with no automatic start at all."
)

// registerTaskDependencyTools registers add/remove for task dependency edges.
// Mirrors the two HTTP routes one-to-one and shares the single edge validator
// (self-edge, cross-workspace, cycle-with-path) in the task service. The read
// side already exists as list_related_tasks_kandev.
func (s *Server) registerTaskDependencyTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("add_task_dependency_kandev",
			mcp.WithDescription(`Block a task until another task completes successfully. Use dependencies for execution order and parent_id for hierarchy. task_id defaults to the current task. Self-dependencies, cross-workspace dependencies, and cycles are rejected. A failed or cancelled predecessor leaves the task blocked for human action. Returns the resulting depends_on list.`),
			mcp.WithString(mcpKeyTaskID, mcp.Description("The blocked task. Defaults to your current task when omitted.")),
			mcp.WithString("depends_on_task_id", mcp.Required(), mcp.Description("The task that must complete first (the predecessor).")),
		),
		s.wrapHandler("add_task_dependency_kandev", s.addTaskDependencyHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("remove_task_dependency_kandev",
			mcp.WithDescription(`Remove a task dependency edge. Removing an edge that does not exist succeeds.

Removing the last edge unblocks the task but does NOT start it: an automatic start is triggered by a dependency RESOLVING, not by the edge going away. Removing the edge means you are taking manual control.`),
			mcp.WithString(mcpKeyTaskID, mcp.Description("The blocked task. Defaults to your current task when omitted.")),
			mcp.WithString("depends_on_task_id", mcp.Required(), mcp.Description("The predecessor task to unlink.")),
		),
		s.wrapHandler("remove_task_dependency_kandev", s.removeTaskDependencyHandler()),
	)
}

// registerSpawnSessionTool registers spawn_session_kandev. Spawns an ADDITIONAL
// agent session on an existing task (usually the caller's own) — unlike
// create_task_kandev, no new task is created: the spawned session shares the
// task's workspace, conversation surface (as a separate tab), and lifecycle.
func (s *Server) registerSpawnSessionTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("spawn_session_kandev",
			mcp.WithDescription(`Start an additional Kandev session on an existing task. This creates a persistent Kandev session/tab, not a native subagent; call it only when the user explicitly requests another Kandev session or a Kandev workflow requires session coordination. It does not create a task. Returns {task_id, session_id, state, agent_profile_id}; a workflow step may override the requested profile.`),
			mcp.WithString("prompt", mcp.Required(), mcp.Description("The Kandev session's initial prompt. This is the ONLY context the new agent receives — be specific and detailed.")),
			mcp.WithString("agent_profile_id", mcp.Description("Requested agent profile for the new session. Omit to inherit your session's profile; a workflow launch profile may override it.")),
			mcp.WithString("name", mcp.Description("Optional session name shown on the session tab (e.g. 'reviewer'). Helps the user tell concurrent sessions apart.")),
			mcp.WithString("task_id", mcp.Description("Task to spawn the session on. Omit to use your current task.")),
		),
		s.wrapHandler("spawn_session_kandev", s.spawnSessionHandler()),
	)
}

// registerListTaskSessionsTool registers list_task_sessions_kandev. It is the
// discovery half of the session-addressing tools: get_task_conversation_kandev
// and message_task_kandev both take an optional session_id and fall back to the
// primary session, so without this a sibling session (one spawned with
// spawn_session_kandev, or spawned by someone else) is unreachable unless the
// caller happened to create it. Registered wherever those two tools are.
func (s *Server) registerListTaskSessionsTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("list_task_sessions_kandev",
			mcp.WithDescription(`List a task's sessions, newest first. Use the returned session_id with get_task_conversation_kandev or message_task_kandev; those tools otherwise use the primary session. Entries include name, state, is_primary, is_current, agent_profile_id, and timestamps.`),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString(mcpKeyTaskID, mcp.Required(), mcp.Description("The task ID whose sessions to list")),
		),
		s.wrapHandler("list_task_sessions_kandev", s.listTaskSessionsHandler()),
	)
}

// registerAddBranchToTaskTool registers add_branch_to_task_kandev. Scoped to
// task mode only — external coding agents have no live session context to
// attach the new worktree to, and shipping this tool through the shared
// create-task path would silently widen the external surface.
func (s *Server) registerAddBranchToTaskTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("add_branch_to_task_kandev",
			mcp.WithDescription(`Attach another repository branch worktree to the current task for an additional PR or repository. This works only with the worktree executor. Single-repository tasks can omit the repository locator; multi-repository tasks must pass exactly one of repository_id, repository_url, or local_path. checkout_branch selects an existing branch; omit it to create a feature branch from base_branch. Returns worktree_path and task_workspace_path.`),
			mcp.WithString("task_id", mcp.Description("The current task. Defaults to the current task when omitted.")),
			mcp.WithString("repository_id", mcp.Description("Repository UUID. Optional for single-repo tasks (auto-resolved). Required for multi-repo tasks unless repository_url or local_path is supplied.")),
			mcp.WithString("repository_url", mcp.Description("GitHub repository URL (e.g. 'https://github.com/owner/repo'). Alternative to repository_id when you don't have the UUID handy. The repository is found-or-created in the task's workspace.")),
			mcp.WithString("local_path", mcp.Description("Local repository folder path (e.g. '/Users/me/projects/myrepo'). Alternative to repository_id for the local worktree flow. The repository is found-or-created in the task's workspace.")),
			mcp.WithString("checkout_branch", mcp.Description("Existing branch to check out in the new worktree (e.g. a PR head branch). Empty to create a fresh feature branch from base_branch.")),
			mcp.WithString("base_branch", mcp.Description("Branch to base the worktree on. Defaults to the repository's default_branch.")),
		),
		s.wrapHandler("add_branch_to_task_kandev", s.addBranchToTaskHandler()),
	)
}

// registerAddWorkspaceSourcesTool attaches a mixed repository/folder batch to
// the current task. Runtime adoption remains a backend concern; this tool only
// forwards the documented union unchanged to the shared mutation boundary.
func (s *Server) registerAddWorkspaceSourcesTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("add_workspace_sources_kandev",
			mcp.WithDescription("Attach repository and folder workspace sources to an idle task. A task may update itself or its same-workspace direct child. Exact retries are idempotent."),
			mcp.WithString(mcpKeyTaskID, mcp.Description("Task to update. Defaults to the current task.")),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithArray("sources", mcp.Required(), mcp.MinItems(1),
				mcp.Description("Ordered source objects. Each has kind repository or folder and the documented fields for that kind."),
				mcp.Items(map[string]any{"type": "object"}),
			),
		),
		s.wrapHandler("add_workspace_sources_kandev", s.addWorkspaceSourcesHandler()),
	)
}

func (s *Server) addWorkspaceSourcesHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID := req.GetString(mcpKeyTaskID, "")
		if taskID == "" {
			taskID = s.taskID
		}
		if taskID == "" {
			return mcp.NewToolResultError("task_id is required (no current task context to default to)"), nil
		}
		if s.taskID == "" || s.sessionID == "" {
			return mcp.NewToolResultError("task source provenance is unavailable in this session"), nil
		}
		arguments, ok := req.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("sources is required"), nil
		}
		sources, ok := arguments["sources"]
		if !ok {
			return mcp.NewToolResultError("sources is required"), nil
		}
		// Provenance is server-authored and intentionally absent from the callable schema.
		payload := map[string]interface{}{mcpKeyTaskID: taskID, "sources": sources, "caller_task_id": s.taskID, "caller_session_id": s.sessionID}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPAddWorkspaceSources, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
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
		// A task-mode MCP server is bound to one live task. Never let an agent
		// use this mutation tool as a cross-task capability: the backend tunnel
		// has no authenticated user principal to authorize arbitrary task IDs.
		if s.taskID == "" || taskID != s.taskID {
			return mcp.NewToolResultError("task_id is not available in this session"), nil
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
			mcp.WithDescription(`Change the base branch used for task diff statistics and the Changes panel. This updates BaseCommit, Ahead, Behind, and cumulative diff immediately without restarting the session. It does not change a pull request's target branch; pass the same base_branch when creating the PR if both must change.`),
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
			mcp.WithDescription(`Signal that every requirement for the current workflow step is complete. Call this as the step's final action; do not call before asking the user, during partial work, or with an unresolved blocker. The signal is idempotent within a step, and any configured transition runs asynchronously at turn end. A new user message cancels a pending signal. The summary is shown to the user and may be forwarded to the next step.`),
			mcp.WithString("summary", mcp.Required(), mcp.Description("One-paragraph plain-text summary of what was done in this step. Shown to the user.")),
			mcp.WithString("handoff", mcp.Description("Optional context the next step's agent will need to pick up where you left off (decisions, open files, follow-ups).")),
			mcp.WithString("blockers", mcp.Description("Optional list of known unresolved issues. Use sparingly — only when the step is complete in the sense that you cannot make further progress without input, not for normal partial work.")),
		),
		s.wrapHandler("step_complete_kandev", s.stepCompleteHandler()),
	)
}

// registerSetTaskTitleTool registers the one-shot title handoff used by
// prompt-first task sessions. The server is bound to the current task, so the
// agent only supplies the short user-facing title it wants to keep.
func (s *Server) registerSetTaskTitleTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("set_task_title_kandev",
			mcp.WithDescription(`Replace the current task's provisional title. Call this as the session's first action, even when the provisional title looks usable. Use a short title phrase in sentence case, targeting about 6 words and summarizing the requested outcome. Do not use a sentence or progress update (for example, "Improve task title casing"). Kandev may also use it for generated branch names.`),
			mcp.WithString(titleArg, mcp.Required(), mcp.Description("Short sentence-case task title targeting about 6 words.")),
		),
		s.wrapHandler("set_task_title_kandev", s.setTaskTitleHandler()),
	)
}

func (s *Server) setTaskTitleHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if s.taskID == "" {
			return mcp.NewToolResultError("set_task_title_kandev requires a bound task"), nil
		}
		title := strings.TrimSpace(req.GetString(titleArg, ""))
		if title == "" {
			return mcp.NewToolResultError("title is required"), nil
		}
		payload := map[string]interface{}{
			mcpKeyTaskID: s.taskID,
			"session_id": s.sessionID,
			titleArg:     title,
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPSetTaskTitle, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
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

const askUserQuestionOutputSchema = `{
  "type": "object",
  "properties": {
    "rejected": {"type": "boolean", "description": "True when the user rejected the question bundle."},
    "reject_reason": {"type": "string", "description": "Optional reason supplied when the bundle was rejected."}
  },
  "additionalProperties": {
    "type": "object",
    "description": "Answer keyed by question id.",
    "properties": {
      "selected_option": {"type": "string"},
      "custom_text": {"type": "string"},
      "answered": {"type": "boolean"},
      "rejected": {"type": "boolean"}
    },
    "additionalProperties": false
  }
}`

func (s *Server) registerInteractionTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("ask_user_question_kandev",
			mcp.WithDescription(`Ask the user 1-4 related questions and wait for all answers. Each question requires a prompt and 2-6 concrete options with short labels and descriptions; use a stable id when response keys matter. Call only when required input cannot be inferred, and bundle related questions in one call. Returns answers keyed by question id; a rejected bundle includes rejected=true and may include reject_reason.`),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithRawOutputSchema(json.RawMessage(askUserQuestionOutputSchema)),
			mcp.WithArray(questionsArg, mcp.Required(),
				mcp.Description(`Array of 1-4 question objects. Each question must have a "prompt" (the question text) and an "options" array (2-6 entries with label + description). Optional fields: "id" (stable identifier in the response map; auto-generated if omitted), "title" (≤12 chars short label).`),
				mcp.MinItems(1),
				mcp.MaxItems(4),
				mcp.Items(buildQuestionSchemaItem()),
			),
			mcp.WithString("context", mcp.Description("Optional single block of shared background information. Preserved verbatim; use context_paragraphs for multiple paragraphs.")),
			mcp.WithArray(contextParagraphsArg,
				mcp.Description("Optional shared background as separate paragraphs. Preferred for multiline context; when non-empty, overrides context and is joined with blank lines."),
				mcp.Items(map[string]any{"type": "string"}),
			),
		),
		s.wrapHandler("ask_user_question_kandev", s.askUserQuestionHandler()),
	)
}

func (s *Server) registerParentQuestionTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("ask_parent_question_kandev",
			mcp.WithDescription(`Ask the direct parent task one or more questions when an autopilot task reaches a critical decision that cannot be inferred safely.

This tool is available only to autopilot child tasks. It sends a durable question message to the direct parent, pauses this child turn, and returns without waiting for the answer. The parent answers with message_task_kandev using the returned question_id as reply_to_question_id. Do not use ask_user_question_kandev for this task, and do not ask unless the decision is truly blocking.`),
			mcp.WithArray(questionsArg, mcp.Required(),
				mcp.Description(`Array of 1-4 question objects. Each question must have a "prompt" (the question text) and an "options" array (2-6 entries with label + description). Optional fields: "id" (stable identifier), "title" (≤12 chars short label).`),
				mcp.MinItems(1),
				mcp.MaxItems(4),
				mcp.Items(buildQuestionSchemaItem()),
			),
			mcp.WithString("context", mcp.Description("Optional single block of shared background information for the direct parent. Preserved verbatim; use context_paragraphs for multiple paragraphs.")),
			mcp.WithArray(contextParagraphsArg,
				mcp.Description("Optional shared background for the direct parent as separate paragraphs. When non-empty, overrides context and is joined with blank lines."),
				mcp.Items(map[string]any{"type": "string"}),
			),
		),
		s.wrapHandler("ask_parent_question_kandev", s.askParentQuestionHandler()),
	)
}

func (s *Server) registerPlanTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("create_task_plan_kandev",
			mcp.WithDescription("Create or save a task plan. task_id addresses the plan's task: pass your own task ID for your current task, or another task's ID to write that task's plan (allowed only within your reach — same workspace / task tree; a task outside it is rejected, never silently redirected to your own)."),
			mcp.WithString("task_id", mcp.Description("The task ID to create a plan for. Defaults to your current task when omitted; pass another task's ID to target it directly.")),
			mcp.WithString("content", mcp.Required(), mcp.Description("The plan content in markdown format")),
			mcp.WithString("title", mcp.Description("Optional title for the plan (default: 'Plan')")),
		),
		s.wrapHandler("create_task_plan_kandev", s.createTaskPlanHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("get_task_plan_kandev",
			mcp.WithDescription("Get the current plan for a task, including any user edits. task_id selects the task: pass your own task ID for your current task, or another task's ID to read that task's plan (allowed only within your reach — same workspace / task tree; a task outside it is rejected, never silently redirected to your own)."),
			mcp.WithString("task_id", mcp.Description("The task ID to get the plan for. Defaults to your current task when omitted; pass another task's ID to read it directly.")),
		),
		s.wrapHandler("get_task_plan_kandev", s.getTaskPlanHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("update_task_plan_kandev",
			mcp.WithDescription("Update an existing task plan. task_id selects the task whose plan to modify: your own task by default, or another task's ID to update that task's plan (allowed only within your reach — same workspace / task tree; a task outside it is rejected, never silently redirected to your own)."),
			mcp.WithString("task_id", mcp.Description("The task ID to update the plan for. Defaults to your current task when omitted; pass another task's ID to target it directly.")),
			mcp.WithString("content", mcp.Required(), mcp.Description("The updated plan content in markdown format")),
			mcp.WithString("title", mcp.Description("Optional new title for the plan")),
		),
		s.wrapHandler("update_task_plan_kandev", s.updateTaskPlanHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("delete_task_plan_kandev",
			mcp.WithDescription("Delete a task plan. task_id selects the task whose plan to delete: your own task by default, or another task's ID to delete that task's plan (allowed only within your reach — same workspace / task tree; a task outside it is rejected, never silently redirected to your own)."),
			mcp.WithString("task_id", mcp.Description("The task ID to delete the plan for. Defaults to your current task when omitted; pass another task's ID to target it directly.")),
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
			mcp.WithDescription("Store an ordered, file-anchored code walkthrough that replaces any prior walkthrough on the task. Use it to narrate a completed diff or explain a code path; order steps from the entry point through the call chain. Reference only files in the local worktree or current review diff, and use line_end for a range. task_id defaults to the current task and may target another task within your reach."),
			mcp.WithString("task_id", mcp.Description("The task ID to attach the walkthrough to. Defaults to your current task when omitted; pass another task's ID (within your reach — same workspace / task tree) to target it directly.")),
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
			mcp.WithDescription("Get the current code walkthrough for a task, including any steps. task_id defaults to your current task; pass another task's ID (within your reach — same workspace / task tree) to read it directly."),
			mcp.WithString("task_id", mcp.Description("The task ID to get the walkthrough for. Defaults to your current task when omitted; pass another task's ID (within your reach — same workspace / task tree) to read it directly.")),
		),
		s.wrapHandler("get_walkthrough_kandev", s.getWalkthroughHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("delete_walkthrough_kandev",
			mcp.WithDescription("Delete the code walkthrough for a task. task_id defaults to your current task; pass another task's ID (within your reach — same workspace / task tree) to target it directly."),
			mcp.WithString("task_id", mcp.Description("The task ID to delete the walkthrough for. Defaults to your current task when omitted; pass another task's ID (within your reach — same workspace / task tree) to target it directly.")),
		),
		s.wrapHandler("delete_walkthrough_kandev", s.deleteWalkthroughHandler()),
	)
}

// registerReviewTools registers the native code-review publishing tool. An
// agent uses it to turn its own reading of the diff into anchored findings that
// render as inline comments in the user's Changes/Review panel, in the same
// place the built-in review pass writes to.
func (s *Server) registerReviewTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("publish_review_findings_kandev",
			mcp.WithDescription("Publish actionable code-review findings as comments anchored to the task's current changes. Use changed files and line numbers from the new version. Report correctness, security, concurrency, error handling, resource, contract, or test defects rather than linter-owned style. Findings are advisory and additive; an unresolved finding with the same anchor and title is refreshed. task_id defaults to the current task and may target another task within your reach."),
			mcp.WithString("task_id", mcp.Description("The task ID to attach the findings to. Defaults to your current task when omitted; pass another task's ID (within your reach — same workspace / task tree) to target it directly.")),
			mcp.WithString("summary", mcp.Description("Optional one-paragraph summary of the review")),
			mcp.WithArray("findings", mcp.Required(),
				mcp.Description("Findings to publish, each anchored to a file and line range."),
				mcp.Items(buildReviewFindingSchemaItem()),
			),
		),
		s.wrapHandler("publish_review_findings_kandev", s.publishReviewFindingsHandler()),
	)
}

// buildReviewFindingSchemaItem describes one finding object in the
// publish_review_findings_kandev tool schema.
func buildReviewFindingSchemaItem() map[string]any {
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
			"repo": str("Optional repository name; required only in a multi-repository task."),
			"file": str("Path to a file in the task's current changes, relative to the repo root."),
			"line": num("1-based start line in the new version of the file."),
			"line_end": num(
				"Optional 1-based end line. Use it only when the finding genuinely spans a range.",
			),
			"severity": map[string]any{
				typeKey:        "string",
				"enum":         []string{"blocker", "major", "minor", "nit"},
				descriptionArg: "blocker breaks correctness or security; nit is genuinely optional.",
			},
			"category": str("Short kebab-case slug for the kind of issue, e.g. correctness, security."),
			"title":    str("One line naming the specific defect."),
			"body": str(
				"Markdown explanation: what is wrong, the input or state that triggers it, and the consequence.",
			),
			"suggestion": str(
				"Optional replacement code. Shown to the user but never applied automatically.",
			),
		},
		"required": []string{"file", "line", "severity", "category", titleArg, "body"},
	}
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
