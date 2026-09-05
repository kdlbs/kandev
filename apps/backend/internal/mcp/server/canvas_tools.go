package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	canvasIDArg               = "canvas_id"
	canvasTitleArg            = "title"
	canvasSummaryArg          = "summary"
	canvasSourcePathArg       = "source_path"
	canvasSkillPathArg        = "path"
	canvasStateKeyArg         = "key"
	canvasStateValueArg       = "value"
	canvasExpectedRevisionArg = "expected_revision"
)

// registerCanvasTools registers the complete task-local canvas surface. The
// profile registry calls this only for a backend-issued Kanban profile carrying
// CapabilityCanvas. The handlers intentionally do not send task, session, or
// workspace IDs from this MCP argument map; the backend derives those from the
// trusted execution context on the stream.
func (s *Server) registerCanvasTools() {
	s.registerCanvasDiscoveryTools()
	s.registerCanvasCreateTool()
	s.registerCanvasGetTool()
	s.registerCanvasPublishTool()
	s.registerCanvasStateTools()
}

func (s *Server) registerCanvasDiscoveryTools() {
	s.mcpServer.AddTool(
		mcp.NewToolWithRawSchema(
			"list_canvases_kandev",
			"List canvases belonging to the current task.",
			json.RawMessage(`{"type":"object","properties":{}}`),
		),
		s.wrapHandler("list_canvases_kandev", s.listCanvasesHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool(
			"read_canvas_authoring_skill_kandev",
			mcp.WithDescription("Read the complete versioned Kandev canvas-authoring core bundle without path, or one allowlisted supporting file with a relative path. The skill is read from Kandev's system inventory, not the task workspace."),
			mcp.WithString(canvasSkillPathArg, mcp.Description("Optional relative inventory path. Omit it to receive the complete core bundle.")),
		),
		s.wrapHandler("read_canvas_authoring_skill_kandev", s.readCanvasAuthoringSkillHandler()),
	)
}

func (s *Server) registerCanvasCreateTool() {
	s.mcpServer.AddTool(
		mcp.NewTool(
			"create_canvas_kandev",
			mcp.WithDescription("Create an inactive draft canvas for the current task and return its assigned source directory, generated scaffold files, manifest scaffold, skill identity, and permission ceiling."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString(canvasTitleArg, mcp.Required(), mcp.Description("A concise name for the canvas.")),
			mcp.WithString(canvasSummaryArg, mcp.Required(), mcp.Description("A short summary of what the canvas shows or does.")),
		),
		s.wrapHandler("create_canvas_kandev", s.createCanvasHandler()),
	)
}

func (s *Server) registerCanvasGetTool() {
	s.mcpServer.AddTool(
		mcp.NewTool(
			"get_canvas_kandev",
			mcp.WithDescription("Inspect one canvas assigned to the current task, including its active release and authoring metadata."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString(canvasIDArg, mcp.Required(), mcp.Description("Canvas ID returned by create_canvas_kandev or list_canvases_kandev.")),
		),
		s.wrapHandler("get_canvas_kandev", s.getCanvasHandler()),
	)
}

func (s *Server) registerCanvasPublishTool() {
	s.mcpServer.AddTool(
		mcp.NewTool(
			"publish_canvas_kandev",
			mcp.WithDescription("Validate and publish source from the assigned canvas source directory. A rejected publish leaves the current active release unchanged."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString(canvasIDArg, mcp.Required(), mcp.Description("Canvas ID to publish.")),
			mcp.WithString(canvasSourcePathArg, mcp.Required(), mcp.Description("Workspace-relative source directory returned by create_canvas_kandev.")),
		),
		s.wrapHandler("publish_canvas_kandev", s.publishCanvasHandler()),
	)
}

func (s *Server) registerCanvasStateTools() {
	s.mcpServer.AddTool(
		mcp.NewTool(
			"get_canvas_state_kandev",
			mcp.WithDescription("Read shared application state for a canvas. Omit key to return the current state object."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString(canvasIDArg, mcp.Required(), mcp.Description("Canvas ID.")),
			mcp.WithString(canvasStateKeyArg, mcp.Description("Optional state key.")),
		),
		s.wrapHandler("get_canvas_state_kandev", s.getCanvasStateHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool(
			"set_canvas_state_kandev",
			mcp.WithDescription("Set one shared application-state value for a canvas. Use expected_revision to reject stale writes."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString(canvasIDArg, mcp.Required(), mcp.Description("Canvas ID.")),
			mcp.WithString(canvasStateKeyArg, mcp.Required(), mcp.Description("State key.")),
			mcp.WithAny(canvasStateValueArg, mcp.Required(), mcp.Description("JSON value to store.")),
			mcp.WithNumber(canvasExpectedRevisionArg, mcp.Description("Optional current revision required for this write.")),
		),
		s.wrapHandler("set_canvas_state_kandev", s.setCanvasStateHandler()),
	)
}

func (s *Server) listCanvasesHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return s.requestCanvasTool(ctx, ws.ActionMCPListCanvases, nil)
	}
}

func (s *Server) readCanvasAuthoringSkillHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		payload := map[string]interface{}{}
		if path := req.GetString(canvasSkillPathArg, ""); path != "" {
			payload[canvasSkillPathArg] = path
		}
		return s.requestCanvasTool(ctx, ws.ActionMCPReadCanvasAuthoringSkill, payload)
	}
}

func (s *Server) createCanvasHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		title, err := req.RequireString(canvasTitleArg)
		if err != nil || title == "" {
			return mcp.NewToolResultError("title is required"), nil
		}
		summary, err := req.RequireString(canvasSummaryArg)
		if err != nil || summary == "" {
			return mcp.NewToolResultError("summary is required"), nil
		}
		return s.requestCanvasTool(ctx, ws.ActionMCPCreateCanvas, map[string]interface{}{
			canvasTitleArg: title, canvasSummaryArg: summary,
		})
	}
}

func (s *Server) getCanvasHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		canvasID, err := req.RequireString(canvasIDArg)
		if err != nil || canvasID == "" {
			return mcp.NewToolResultError("canvas_id is required"), nil
		}
		return s.requestCanvasTool(ctx, ws.ActionMCPGetCanvas, map[string]interface{}{canvasIDArg: canvasID})
	}
}

func (s *Server) publishCanvasHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		canvasID, err := req.RequireString(canvasIDArg)
		if err != nil || canvasID == "" {
			return mcp.NewToolResultError("canvas_id is required"), nil
		}
		sourcePath, err := req.RequireString(canvasSourcePathArg)
		if err != nil || sourcePath == "" {
			return mcp.NewToolResultError("source_path is required"), nil
		}
		return s.requestCanvasTool(ctx, ws.ActionMCPPublishCanvas, map[string]interface{}{
			canvasIDArg: canvasID, canvasSourcePathArg: sourcePath,
		})
	}
}

func (s *Server) getCanvasStateHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		canvasID, err := req.RequireString(canvasIDArg)
		if err != nil || canvasID == "" {
			return mcp.NewToolResultError("canvas_id is required"), nil
		}
		payload := map[string]interface{}{canvasIDArg: canvasID}
		if key := req.GetString(canvasStateKeyArg, ""); key != "" {
			payload[canvasStateKeyArg] = key
		}
		return s.requestCanvasTool(ctx, ws.ActionMCPGetCanvasState, payload)
	}
}

func (s *Server) setCanvasStateHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		canvasID, err := req.RequireString(canvasIDArg)
		if err != nil || canvasID == "" {
			return mcp.NewToolResultError("canvas_id is required"), nil
		}
		key, err := req.RequireString(canvasStateKeyArg)
		if err != nil || key == "" {
			return mcp.NewToolResultError("key is required"), nil
		}
		value, ok := req.GetArguments()[canvasStateValueArg]
		if !ok {
			return mcp.NewToolResultError("value is required"), nil
		}
		payload := map[string]interface{}{
			canvasIDArg: canvasID, canvasStateKeyArg: key, canvasStateValueArg: value,
		}
		if revision, present, err := canvasExpectedRevision(req.GetArguments()); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		} else if present {
			payload[canvasExpectedRevisionArg] = revision
		}
		return s.requestCanvasTool(ctx, ws.ActionMCPSetCanvasState, payload)
	}
}

func (s *Server) requestCanvasTool(ctx context.Context, action string, payload interface{}) (*mcp.CallToolResult, error) {
	var result interface{}
	if err := s.backend.RequestPayload(ctx, action, payload, &result); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode canvas response: %v", err)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func canvasExpectedRevision(arguments map[string]interface{}) (int64, bool, error) {
	value, ok := arguments[canvasExpectedRevisionArg]
	if !ok || value == nil {
		return 0, false, nil
	}
	var revision float64
	switch typed := value.(type) {
	case float64:
		revision = typed
	case float32:
		revision = float64(typed)
	case int:
		revision = float64(typed)
	case int64:
		revision = float64(typed)
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return 0, false, errorsForCanvasRevision(value)
		}
		revision = parsed
	default:
		return 0, false, errorsForCanvasRevision(value)
	}
	// A float64 cannot represent every int64 exactly. Reject values at the
	// upper boundary instead of allowing a platform-dependent conversion to
	// wrap into a negative revision.
	if revision < 0 || revision >= float64(math.MaxInt64) || math.Trunc(revision) != revision || math.IsInf(revision, 0) || math.IsNaN(revision) {
		return 0, false, errorsForCanvasRevision(value)
	}
	return int64(revision), true, nil
}

func errorsForCanvasRevision(value interface{}) error {
	return fmt.Errorf("%s must be a non-negative integer, got %T", canvasExpectedRevisionArg, value)
}
