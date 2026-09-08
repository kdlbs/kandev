package mcp

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// registerRecordStepDecisionTool registers record_step_decision_kandev, the
// Office quorum decision-recording tool. Per the tool contract in
// docs/specs/tasks/requirements/workflow-quorum-decision-recording-agent-surface.md, it takes no
// task_id — task/step/role are resolved server-side from the calling
// session, matching every other Office tool.
func (s *Server) registerRecordStepDecisionTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("record_step_decision_kandev",
			mcp.WithDescription(`Record an approve/reject decision for the current workflow step, as a reviewer or approver. Call this once you have finished evaluating the step's output. The task, step, and your role are resolved from your session — you cannot record a decision on another task. Not idempotent: a second call supersedes the first.`),
			mcp.WithString("decision", mcp.Required(), mcp.Enum("approved", "rejected"), mcp.Description("The verdict: \"approved\" or \"rejected\".")),
			mcp.WithString("reason", mcp.Required(), mcp.Description("Non-empty explanation for the verdict. Recorded in the task timeline.")),
		),
		s.wrapHandler("record_step_decision_kandev", s.recordStepDecisionHandler()),
	)
}

func (s *Server) recordStepDecisionHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if s.taskID == "" || s.sessionID == "" {
			return mcp.NewToolResultError("record_step_decision_kandev requires a bound task and session"), nil
		}
		decision := strings.TrimSpace(req.GetString("decision", ""))
		reason := strings.TrimSpace(req.GetString("reason", ""))
		if reason == "" {
			return mcp.NewToolResultError("reason is required"), nil
		}
		payload := map[string]interface{}{
			"task_id":    s.taskID,
			"session_id": s.sessionID,
			"decision":   decision,
			"reason":     reason,
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPRecordStepDecision, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}
