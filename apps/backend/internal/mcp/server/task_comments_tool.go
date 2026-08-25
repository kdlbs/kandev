package mcp

import (
	"context"
	"encoding/json"
	"math"
	"strings"

	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	commentsDefaultLimit = 20
	commentsMaxLimit     = 100
)

// registerTaskCommentsTool registers list_task_comments_kandev, the office-only
// read side of the comment channel every role instruction already writes to
// (REQ-OFFICE-AGENT-COMMENT-READS-001). task_id and limit are declared with
// mcp.WithAny (no JSON Schema type) so a null or wrong-typed value reaches the
// handler instead of being rejected by argument validation before
// resolveCommentsTaskID/resolveCommentsLimit can apply AC-003.4/003.9/005.8/005.9's
// default-rather-than-error and validate-rather-than-silently-fall-back rules.
func (s *Server) registerTaskCommentsTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("list_task_comments_kandev",
			mcp.WithDescription(`Read a task's comments (all authors: agent and human). task_id defaults to the current task; pass another task's id (self/ancestor/descendant/sibling-with-shared-parent/blocker, same workspace) to read its comments. limit defaults to 20, max 100. Each comment carries id, task_id, author_type, author_id, source, body, created_at, and (only when the body was shortened) body_truncated + body_bytes. The response also carries total, returned, and has_more.`),
			mcp.WithAny(mcpKeyTaskID, mcp.Description("Target task. Omitted, null, empty, or 'self' means the current task.")),
			mcp.WithAny("limit", mcp.Description("Max comments to return (default 20, max 100). Any non-positive or non-integer value defaults to 20.")),
		),
		s.wrapHandler("list_task_comments_kandev", s.listTaskCommentsHandler()),
	)
}

// resolveCommentsTaskID inspects the raw "task_id" argument and reports the
// task_id to forward to the backend, plus whether the value was well-formed.
// A well-formed absent/nil/empty/whitespace/"self" value forwards "" so the
// service layer's self-fallback (AC-005.4/005.6/005.8) resolves it against
// the caller task id. A non-empty string forwards trimmed. Any other JSON
// type is ill-formed (AC-005.9) and must not fall back to the caller.
func resolveCommentsTaskID(args map[string]any) (taskID string, ok bool) {
	raw, present := args[mcpKeyTaskID]
	if !present || raw == nil {
		return "", true
	}
	str, isString := raw.(string)
	if !isString {
		return "", false
	}
	trimmed := strings.TrimSpace(str)
	if trimmed == selfTaskSentinel {
		return "", true
	}
	return trimmed, true
}

// resolveCommentsLimit applies AC-003.4/003.5/003.9: omitted, null, zero,
// negative, or non-integer values default to 20; values above 100 clamp to
// 100; no value ever produces an error.
func resolveCommentsLimit(args map[string]any) int {
	raw, present := args["limit"]
	if !present || raw == nil {
		return commentsDefaultLimit
	}
	n, isNumber := raw.(float64)
	if !isNumber || n != math.Trunc(n) || n <= 0 {
		return commentsDefaultLimit
	}
	if n > commentsMaxLimit {
		return commentsMaxLimit
	}
	return int(n)
}

func (s *Server) listTaskCommentsHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		targetTaskID, ok := resolveCommentsTaskID(args)
		if !ok {
			return mcp.NewToolResultError(service.ErrDocumentTaskRequired.Error()), nil
		}
		payload := map[string]interface{}{
			mcpKeyTaskID:       targetTaskID,
			mcpKeyCallerTaskID: s.taskID,
			"limit":            resolveCommentsLimit(args),
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPListTaskComments, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}
