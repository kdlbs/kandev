package mcp

import (
	"context"
	"encoding/json"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerHandoffTools registers the office task-handoff MCP tools:
// list_related_tasks_kandev, list_task_documents_kandev,
// get_task_document_kandev, write_task_document_kandev. These are
// available in ModeOffice and ModeTask so both office coordinators and
// task assignees can read parent / sibling documents and write
// child→parent coordination docs.
func (s *Server) registerHandoffTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("list_related_tasks_kandev",
			mcp.WithDescription(
				`List parent, children, siblings, blockers, and blocked tasks for the current task.
Use this to discover task IDs you can read documents from. Document keys are included so you can pick
the document to fetch with get_task_document_kandev. Pass task_id to inspect a different task in the same workspace.`,
			),
			mcp.WithString("task_id", mcp.Description("Defaults to the current task.")),
		),
		s.wrapHandler("list_related_tasks_kandev", s.listRelatedTasksHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("list_task_documents_kandev",
			mcp.WithDescription(
				`List documents for a task (key + title + author + size; no content).
Allowed for the current task itself, the current task's ancestors/descendants in the same workspace,
and siblings sharing a non-empty parent. Returns access_denied for unrelated tasks.`,
			),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("Target task to list documents for.")),
		),
		s.wrapHandler("list_task_documents_kandev", s.listTaskDocumentsHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("get_task_document_kandev",
			mcp.WithDescription(
				`Fetch a single task document (with content). Same access rules as list_task_documents_kandev:
self, ancestors, descendants in the same workspace, or siblings with a shared non-empty parent.`,
			),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("Target task that owns the document.")),
			mcp.WithString("document_key", mcp.Required(), mcp.Description("Document key (e.g. 'spec', 'plan', 'notes').")),
		),
		s.wrapHandler("get_task_document_kandev", s.getTaskDocumentHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("write_task_document_kandev",
			mcp.WithDescription(
				`Create or update a document on a target task. Allowed for the current task itself or any
ancestor (child→parent coordination writes). Sibling and descendant writes are denied — publish
coordination docs to the shared parent.`,
			),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("Target task to write to. Must be self or an ancestor.")),
			mcp.WithString("document_key", mcp.Required(), mcp.Description("Document key.")),
			mcp.WithString("title", mcp.Description("Optional title; defaults to the document key.")),
			mcp.WithString("content", mcp.Required(), mcp.Description("Full document content.")),
			mcp.WithString("type", mcp.Description("Optional document type; defaults to 'custom'.")),
		),
		s.wrapHandler("write_task_document_kandev", s.writeTaskDocumentHandler()),
	)
}

func (s *Server) listRelatedTasksHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID := req.GetString("task_id", "")
		if taskID == "" || taskID == "self" {
			taskID = s.taskID
		}
		if taskID == "" {
			return mcp.NewToolResultError("task_id is required (no current task context)"), nil
		}
		payload := map[string]string{
			"task_id":        taskID,
			"caller_task_id": s.taskID,
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPListRelatedTasks, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) listTaskDocumentsHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		payload := map[string]string{
			"task_id":        taskID,
			"caller_task_id": s.taskID,
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPListTaskDocuments, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) getTaskDocumentHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		key, err := req.RequireString("document_key")
		if err != nil {
			return mcp.NewToolResultError("document_key is required"), nil
		}
		payload := map[string]string{
			"task_id":        taskID,
			"document_key":   key,
			"caller_task_id": s.taskID,
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPGetTaskDocument, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		// If a 'content' field is present, return it directly so the agent
		// reads markdown — same affordance get_task_plan_kandev offers.
		if content, ok := result["content"].(string); ok && content != "" {
			return mcp.NewToolResultText(content), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) writeTaskDocumentHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		key, err := req.RequireString("document_key")
		if err != nil {
			return mcp.NewToolResultError("document_key is required"), nil
		}
		content, err := req.RequireString("content")
		if err != nil {
			return mcp.NewToolResultError("content is required"), nil
		}
		payload := map[string]interface{}{
			"task_id":        taskID,
			"document_key":   key,
			"title":          req.GetString("title", ""),
			"type":           req.GetString("type", ""),
			"content":        content,
			"caller_task_id": s.taskID,
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPWriteTaskDocument, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}
