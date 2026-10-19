package lifecycle

import (
	"context"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

type canonicalShellMessageReader interface {
	GetMessageByToolCallID(context.Context, string, string) (*models.Message, error)
	RehydrateMessagePayload(context.Context, *models.Message) error
}

func canonicalShellStartEvent(event agentctl.AgentEvent) bool {
	if event.Type != streams.EventTypeToolCall || event.Role == string(models.MessageAuthorUser) || event.ToolCallID == "" || event.NormalizedPayload == nil || event.NormalizedPayload.BackgroundWork() != nil {
		return false
	}
	shell := event.NormalizedPayload.ShellExec()
	return shell != nil && !shell.Background
}

func committedShellProjectionEvent(ctx context.Context, delivery AgentDeliveryRepository, event agentctl.AgentEvent, sessionID string) (agentctl.AgentEvent, error) {
	reader, ok := delivery.(canonicalShellMessageReader)
	if !ok {
		return event, ErrDeliveryRecoveryBlocked
	}
	message, err := reader.GetMessageByToolCallID(ctx, sessionID, event.ToolCallID)
	if err != nil {
		return event, err
	}
	if message == nil {
		return event, ErrDeliveryRecoveryBlocked
	}
	event.CanonicalMessageID = message.ID
	if event.Type == streams.EventTypeToolCall {
		return event, nil
	}
	event.Type = streams.EventTypeToolUpdate
	event.CanonicalMessageAppend = true
	if !isTerminalToolUpdate(event) || models.ToolPayloadRemoved(message.Metadata) {
		return event, nil
	}
	if err := reader.RehydrateMessagePayload(ctx, message); err != nil {
		return event, err
	}
	output, valid := models.ExtractShellExecOutput(message.Metadata)
	if !valid || event.NormalizedPayload == nil || event.NormalizedPayload.ShellExec() == nil {
		return event, ErrDeliveryRecoveryBlocked
	}
	event.NormalizedPayload = event.NormalizedPayload.Snapshot()
	event.NormalizedPayload.ShellExec().Output = &streams.ShellExecOutput{Stdout: output.Stdout, Stderr: output.Stderr, ExitCode: output.ExitCode, Truncated: output.Truncated}
	return event, nil
}
