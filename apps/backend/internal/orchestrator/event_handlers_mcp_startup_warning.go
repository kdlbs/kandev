package orchestrator

import (
	"context"
	"strings"

	"github.com/google/uuid"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

func codexMCPStartupWarningMessageID(sessionID, executionID, reportID string) string {
	identity := strings.Join([]string{"codex-mcp-startup-warning-v1", sessionID, executionID, reportID}, "\x00")
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(identity)).String()
}

func (s *Service) handleCodexMCPStartupWarning(
	ctx context.Context,
	payload *agentruntime.AgentStreamEventPayload,
	executionID string,
) {
	if s.messageCreator == nil || payload.TaskID == "" || payload.SessionID == "" ||
		executionID == "" || payload.Data.ToolCallID == "" {
		return
	}
	creator, ok := s.messageCreator.(lifecycleRecoveryNoticeMessageCreator)
	if !ok {
		s.logger.Warn("message creator cannot persist lifecycle-only MCP startup warnings",
			zap.String("task_id", payload.TaskID),
			zap.String("session_id", payload.SessionID))
		return
	}
	content := streams.SanitizeProviderMessage(payload.Data.Text)
	if content == "" {
		return
	}
	metadata := map[string]interface{}{
		"kind":      streams.EventTypeMCPStartupWarning,
		"variant":   "warning",
		"report_id": payload.Data.ToolCallID,
	}
	messageID := codexMCPStartupWarningMessageID(payload.SessionID, executionID, payload.Data.ToolCallID)
	if err := creator.CreateSessionLifecycleMessageIdempotent(
		ctx,
		messageID,
		payload.TaskID,
		content,
		payload.SessionID,
		string(v1.MessageTypeStatus),
		metadata,
	); err != nil {
		s.logger.Warn("failed to persist Codex MCP startup warning",
			zap.String("task_id", payload.TaskID),
			zap.String("session_id", payload.SessionID),
			zap.String("report_id", payload.Data.ToolCallID),
			zap.Error(err))
	}
}
