package mcp

import (
	"context"
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// registerQuestionAnsweringTools registers list_pending_questions_kandev and
// answer_question_kandev. External-surface only (spec S1/S2): these give a
// caller outside the task's own agent session visibility into, and authority
// to resolve, any clarification bundle it can see — ask_user_question_kandev
// itself stays off this surface (spec S3), and neither tool needs a
// system-prompt entry (spec S4).
func (s *Server) registerQuestionAnsweringTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("list_pending_questions_kandev",
			mcp.WithDescription(`List clarification bundles that are still waiting on an answer.

A bundle is pending when no resolution has been recorded for it and at least one of its questions is still open. Each bundle groups every question the agent asked in one ask_user_question_kandev call; answer with answer_question_kandev using the returned pending_id.`),
			mcp.WithString("workspace_id", mcp.Description("Optional workspace ID filter. Omit to list across every workspace visible to the caller.")),
			mcp.WithString("created_since", mcp.Description("Optional RFC3339 timestamp; only bundles created at or after this time are returned.")),
			mcp.WithString("cursor", mcp.Description("Optional opaque pagination cursor from a previous call's next_cursor.")),
			mcp.WithNumber("limit", mcp.Description("Optional page size (default 50, max 200).")),
		),
		s.wrapHandler("list_pending_questions_kandev", s.listPendingQuestionsHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("answer_question_kandev",
			mcp.WithDescription(`Answer or reject a pending clarification bundle by its pending_id (from list_pending_questions_kandev).

Provide either "answers" (one entry per question in the bundle) or "rejected": true with an optional "reject_reason" — not both. Resolution is idempotent and first-write-wins: if the bundle was already resolved by someone else, this returns claimed: false along with the winning resolution's status and response, not an error.`),
			mcp.WithString("pending_id", mcp.Required(), mcp.Description("The bundle's pending_id, from list_pending_questions_kandev.")),
			mcp.WithArray("answers",
				mcp.Description(`One entry per question in the bundle, in any order.`),
				mcp.Items(map[string]any{
					typeKey: objType,
					propsKey: map[string]any{
						"question_id": map[string]any{typeKey: stringType, descriptionArg: "The question's question_id, from list_pending_questions_kandev."},
						"selected_options": map[string]any{
							typeKey:        "array",
							descriptionArg: "Zero or more option IDs returned for this question.",
							"items":        map[string]any{typeKey: stringType},
						},
						"custom_text": map[string]any{typeKey: stringType, descriptionArg: "Free-text answer for this question."},
					},
					reqKey: []string{"question_id"},
				}),
			),
			mcp.WithBoolean("rejected", mcp.Description("Reject the whole bundle instead of answering it.")),
			mcp.WithString("reject_reason", mcp.Description("Optional reason shown alongside a rejection.")),
		),
		s.wrapHandler("answer_question_kandev", s.answerQuestionHandler()),
	)
}

func (s *Server) listPendingQuestionsHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		payload := map[string]interface{}{}
		copyOptionalStringArg(payload, req, "workspace_id")
		copyOptionalStringArg(payload, req, "created_since")
		copyOptionalStringArg(payload, req, "cursor")
		copyOptionalLimitArg(payload, req)

		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPListPendingQuestions, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) answerQuestionHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pendingID, err := req.RequireString("pending_id")
		if err != nil {
			return mcp.NewToolResultError("pending_id is required"), nil
		}
		payload := map[string]interface{}{"pending_id": pendingID}
		if answers, ok := req.GetArguments()["answers"]; ok && answers != nil {
			payload["answers"] = answers
		}
		if rejected := req.GetBool("rejected", false); rejected {
			payload["rejected"] = true
		}
		copyOptionalStringArg(payload, req, "reject_reason")

		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPAnswerQuestion, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}
