package lifecycle

import (
	"context"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/acpcompat"
	"github.com/stretchr/testify/require"
)

func codexMCPStartupToolCall(submissionID string) agentctl.AgentEvent {
	return agentctl.AgentEvent{
		Type:                 streams.EventTypeToolCall,
		SessionID:            "session-live",
		ToolCallID:           "mcp_startup.server.550e8400-e29b-41d4-a716-446655440006",
		ToolTitle:            "mcp__server__startup",
		ToolName:             "other",
		ToolStatus:           "failed",
		DeliverySubmissionID: submissionID,
	}
}

func TestCodexMCPStartupReportDoesNotAdoptLegacySubmission(t *testing.T) {
	execution := createTestExecution("codex-legacy-execution", "task-live", "session-live")
	execution.AgentID = acpcompat.CodexAgentID
	execution.setDeliverySubmissionID("live-submission")
	event := codexMCPStartupToolCall("other-submission")
	manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	defer manager.Wait()

	prepared, err := manager.prepareAgentEvent(context.Background(), execution, nil, nil, event)
	require.NoError(t, err)
	require.Equal(t, "live-submission", execution.deliverySubmissionIDSnapshot(),
		"a legacy startup report must not replace the live execution submission")
	require.Equal(t, "other-submission", prepared.event.DeliverySubmissionID,
		"legacy admission must preserve the incoming report for lifecycle normalization")
}

func TestCodexMCPStartupReportKeepsDurableReceiptWithoutAdoptingSubmission(t *testing.T) {
	execution := recoveredTerminalLiveExecution()
	execution.AgentID = acpcompat.CodexAgentID
	execution.setDeliverySubmissionID("live-submission")
	event := codexMCPStartupToolCall("other-submission")
	event.DeliveryStreamID = execution.DeliveryStreamID
	event.DeliverySequence = 1
	event.DeliveryIncarnationID = execution.DeliveryIncarnationID
	event.DeliveryHarnessGeneration = execution.DeliveryHarnessGeneration
	repository := newRecoveredTerminalRepository(execution)
	manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	defer manager.Wait()

	prepared, err := manager.prepareAgentEvent(context.Background(), execution, nil, repository, event)
	require.NoError(t, err)
	require.Equal(t, "live-submission", execution.deliverySubmissionIDSnapshot(),
		"a durable startup report must not replace the live execution submission")
	require.NotNil(t, prepared.durableEvent)
	require.Equal(t, "other-submission", prepared.durableEvent.SubmissionID,
		"durable receipt must keep the submission identity from the original report")
	require.Equal(t, streams.EventTypeToolCall, prepared.durableEvent.EventType,
		"durable receipt must preserve the original tool call before lifecycle normalization")
}
