package mcp

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	ws "github.com/kandev/kandev/pkg/websocket"
)

const (
	scopeOwnArg          = "scope"
	scopeOwnValue        = "own"
	scopeChildren        = "children"
	statusArg            = "status"
	questionIDsArg       = "question_ids"
	questionIDArgName    = "question_id"
	reasonArg            = "reason"
	limitArg             = "limit"
	cursorArg            = "cursor"
	maxQuestionReadLimit = 200
	maxQuestionIDs       = 50
)

// asyncQuestionsSupported keeps the async lifecycle on persistent kanban task
// surfaces. Office, configuration, external, quick-chat, and managed
// conversation profiles keep their existing question behavior.
func (s *Server) asyncQuestionsSupported() bool {
	return s.profile.Surface == mcpprofile.SurfaceKanbanTask
}

// readAsyncMode parses the optional async/request_id arguments strictly. A
// non-boolean async value or a request_id without async mode fails before any
// backend write.
func readAsyncMode(req mcp.CallToolRequest) (bool, string, *mcp.CallToolResult) {
	arguments := req.GetArguments()
	async := false
	if raw, present := arguments[asyncArg]; present && raw != nil {
		value, ok := raw.(bool)
		if !ok {
			return false, "", mcp.NewToolResultError("async must be a boolean")
		}
		async = value
	}
	requestID := ""
	if raw, present := arguments[requestIDArg]; present && raw != nil {
		value, ok := raw.(string)
		if !ok {
			return false, "", mcp.NewToolResultError("request_id must be a string")
		}
		requestID = strings.TrimSpace(value)
	}
	if requestID != "" && !async {
		return false, "", mcp.NewToolResultError("request_id is valid only with async=true")
	}
	return async, requestID, nil
}

func questionReceiptResult(receipt map[string]interface{}) *mcp.CallToolResult {
	data, _ := json.MarshalIndent(receipt, "", "  ")
	return mcp.NewToolResultStructured(receipt, string(data))
}

func (s *Server) registerAsyncOperatorQuestionTools() {
	s.addWithdrawQuestionTool()
	s.addForwardQuestionTool()
	s.addWaitForQuestionAnswersTool()
}

func (s *Server) registerAsyncParentQuestionTools() {
	s.addWithdrawQuestionTool()
	s.addWaitForQuestionAnswersTool()
}

const listQuestionAnswersDescription = `List your durable async questions and their answers. scope=own (default) reads requests you authored; scope=children reads pending requests from your direct child tasks. Reads are bounded and repeatable: reading never consumes or closes a request. Use question_ids to read a known checkpoint set.`

func (s *Server) addListQuestionAnswersTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("list_question_answers_kandev",
			mcp.WithDescription(listQuestionAnswersDescription),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithString(scopeOwnArg, mcp.Description("own (default) or children.")),
			mcp.WithString(statusArg, mcp.Description("pending, answered, rejected, withdrawn, stale, or all (default).")),
			mcp.WithArray(questionIDsArg, mcp.Description("Optional question ids to read, at most 50."),
				mcp.Items(map[string]any{"type": "string"})),
			mcp.WithNumber(limitArg, mcp.Description("Page size, 1-200. Defaults to 50."), mcp.Min(1), mcp.Max(maxQuestionReadLimit)),
			mcp.WithString(cursorArg, mcp.Description("Opaque next_cursor from a previous page.")),
		),
		s.wrapHandler("list_question_answers_kandev", s.listQuestionAnswersHandler()),
	)
}

func (s *Server) addWithdrawQuestionTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("withdraw_question_kandev",
			mcp.WithDescription("Withdraw a pending async question you authored. Every item closes as withdrawn; history keeps the reason and no answer is recorded. Repeating the withdrawal is idempotent."),
			mcp.WithString(questionIDArgName, mcp.Required(), mcp.Description("Question id returned by the async ask tool.")),
			mcp.WithString(reasonArg, mcp.Required(), mcp.Description("Why the question is no longer needed.")),
		),
		s.wrapHandler("withdraw_question_kandev", s.withdrawQuestionHandler()),
	)
}

func (s *Server) addForwardQuestionTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("forward_question_kandev",
			mcp.WithDescription("Forward a pending async question from your direct child task to the operator. The child request keeps its identity; an operator answer resolves it and your child receives the same canonical outcome. Only a direct parent with user-question capability may forward."),
			mcp.WithString(questionIDArgName, mcp.Required(), mcp.Description("The child question id.")),
			mcp.WithString(reasonArg, mcp.Required(), mcp.Description("Why the operator must decide.")),
		),
		s.wrapHandler("forward_question_kandev", s.forwardQuestionHandler()),
	)
}

func (s *Server) addWaitForQuestionAnswersTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("wait_for_question_answers_kandev",
			mcp.WithDescription("Pause only when all remaining work depends on these pending async questions. If any named dependency already resolved, the tool returns ready snapshots without pausing. Otherwise it declares an explicit dependency gate and this turn parks until any named dependency reaches a terminal outcome. Open questions alone never block completion."),
			mcp.WithArray(questionIDsArg, mcp.Required(), mcp.Description("Pending question ids you authored, at most 50."),
				mcp.Items(map[string]any{"type": "string"}),
				mcp.MinItems(1), mcp.MaxItems(maxQuestionIDs)),
			mcp.WithString(reasonArg, mcp.Description("Short explanation of the dependency.")),
		),
		s.wrapHandler("wait_for_question_answers_kandev", s.waitForQuestionAnswersHandler()),
	)
}

func (s *Server) listQuestionAnswersHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		payload := map[string]interface{}{
			"task_id":    s.taskID,
			"session_id": s.sessionID,
			scopeOwnArg:  req.GetString(scopeOwnArg, scopeOwnValue),
			statusArg:    req.GetString(statusArg, "all"),
			limitArg:     req.GetInt(limitArg, 50),
			cursorArg:    req.GetString(cursorArg, ""),
		}
		if ids := req.GetStringSlice(questionIDsArg, nil); len(ids) > 0 {
			payload[questionIDsArg] = ids
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPListQuestionAnswers, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultStructured(result, string(data)), nil
	}
}

func (s *Server) withdrawQuestionHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		questionID := strings.TrimSpace(req.GetString(questionIDArgName, ""))
		if questionID == "" {
			return mcp.NewToolResultError("question_id is required"), nil
		}
		payload := map[string]interface{}{
			"task_id":         s.taskID,
			"session_id":      s.sessionID,
			questionIDArgName: questionID,
			reasonArg:         req.GetString(reasonArg, ""),
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPWithdrawQuestion, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultStructured(result, string(data)), nil
	}
}

func (s *Server) forwardQuestionHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		questionID := strings.TrimSpace(req.GetString(questionIDArgName, ""))
		if questionID == "" {
			return mcp.NewToolResultError("question_id is required"), nil
		}
		payload := map[string]interface{}{
			"task_id":         s.taskID,
			"session_id":      s.sessionID,
			questionIDArgName: questionID,
			reasonArg:         req.GetString(reasonArg, ""),
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPForwardQuestion, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultStructured(result, string(data)), nil
	}
}

func (s *Server) waitForQuestionAnswersHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ids := req.GetStringSlice(questionIDsArg, nil)
		if len(ids) == 0 {
			return mcp.NewToolResultError("question_ids is required"), nil
		}
		payload := map[string]interface{}{
			"task_id":      s.taskID,
			"session_id":   s.sessionID,
			questionIDsArg: ids,
			reasonArg:      req.GetString(reasonArg, ""),
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPWaitForQuestionAnswers, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultStructured(result, string(data)), nil
	}
}
