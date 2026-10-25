package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (s *Server) listTaskPlanRevisionsHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := s.resolveTaskID(req)
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		payload := map[string]interface{}{
			"task_id": taskID,
		}
		if args := req.GetArguments(); args["before_revision_number"] != nil {
			payload["before_revision_number"] = req.GetInt("before_revision_number", 0)
		}
		if args := req.GetArguments(); args["limit"] != nil {
			payload["limit"] = req.GetInt("limit", 0)
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPListTaskPlanRevisions, payload, &result); err != nil {
			return mcp.NewToolResultError(planToolError(err)), nil
		}
		data, marshalErr := json.MarshalIndent(result, "", "  ")
		if marshalErr != nil {
			return mcp.NewToolResultError("failed to format task plan revision metadata"), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) getTaskPlanRevisionHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := s.resolveTaskID(req)
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		revisionID, err := req.RequireString("revision_id")
		if err != nil {
			return mcp.NewToolResultError("revision_id is required"), nil
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPGetTaskPlanRevision, map[string]string{
			"task_id": taskID, "revision_id": revisionID,
		}, &result); err != nil {
			return mcp.NewToolResultError(planToolError(err)), nil
		}
		content, ok := result["content"].(string)
		if !ok {
			data, marshalErr := json.MarshalIndent(result, "", "  ")
			if marshalErr != nil {
				return mcp.NewToolResultError("failed to format task plan revision"), nil
			}
			return mcp.NewToolResultText(string(data)), nil
		}
		metadata := make(map[string]interface{}, len(result))
		for key, value := range result {
			if key != "content" {
				metadata[key] = value
			}
		}
		metadata["content_bytes"] = len(content)
		data, marshalErr := json.MarshalIndent(metadata, "", "  ")
		if marshalErr != nil {
			return mcp.NewToolResultError("failed to format task plan revision metadata"), nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{
			mcp.NewTextContent("Revision metadata:\n" + string(data)),
			mcp.NewTextContent(content),
		}}, nil
	}
}

func (s *Server) restoreTaskPlanRevisionHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := s.resolveTaskID(req)
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		revisionID, err := req.RequireString("revision_id")
		if err != nil {
			return mcp.NewToolResultError("revision_id is required"), nil
		}
		expectedVersion, err := req.RequireString("expected_version")
		if err != nil {
			return mcp.NewToolResultError("expected_version is required"), nil
		}
		expectedRevisionVersion, err := req.RequireString("expected_revision_version")
		if err != nil {
			return mcp.NewToolResultError("expected_revision_version is required"), nil
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPRestoreTaskPlanRevision, map[string]string{
			"task_id": taskID, "revision_id": revisionID,
			"expected_version": expectedVersion, "expected_revision_version": expectedRevisionVersion,
		}, &result); err != nil {
			return mcp.NewToolResultError(planToolError(err)), nil
		}
		status := stringField(result, "status")
		if status == "" {
			status = "restored"
		}
		ack := fmt.Sprintf(
			"Plan %s successfully: task_id=%s, revision_id=%s, revision_number=%v, version=%s, %v bytes.",
			status, stringField(result, "task_id"), stringField(result, "revision_id"),
			result["revision_number"], stringField(result, "version"), result["content_bytes"],
		)
		ack += " Plan content is omitted from this response; read it back with get_task_plan_kandev if needed."
		return mcp.NewToolResultText(ack), nil
	}
}
