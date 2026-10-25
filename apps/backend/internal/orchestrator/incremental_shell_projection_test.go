package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestIncrementalShellProjectionDoesNotPersistAgain(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task", "session", "")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	creator := &mockMessageCreator{}
	svc.messageCreator = creator
	payload := &lifecycle.AgentStreamEventPayload{TaskID: "task", SessionID: "session", Data: &lifecycle.AgentStreamEventData{Type: "tool_update", ToolCallID: "tool", ToolStatus: "in_progress"}}
	svc.persistToolUpdateMessage(context.Background(), payload)
	require.Equal(t, 1, creator.toolUpdateWrites)
	payload.Data.CanonicalProjection = true
	payload.Data.MessageID = "committed-message"
	svc.persistToolUpdateMessage(context.Background(), payload)
	require.Equal(t, 1, creator.toolUpdateWrites)
}

func TestIncrementalShellRetainedIdleJournal(t *testing.T) {
	inputs := &silentRestoreInputs{session: &models.TaskSession{ID: "session"}, recovery: models.AgentDeliveryRecovery{IncarnationID: "incarnation", HarnessGeneration: 1}}
	for _, version := range []uint32{1, 2} {
		descriptor := journal.RecoveryDescriptor{StorageCapability: journal.StorageCapability{Version: version, Durable: true}, SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1}
		require.True(t, legacyNativeDescriptorIdle(descriptor))
		require.True(t, restartJournalOwnerMatches(inputs, descriptor))
	}
	require.False(t, legacyNativeDescriptorIdle(journal.RecoveryDescriptor{StorageCapability: journal.StorageCapability{Version: 3, Durable: true}}))
}

func TestIncrementalShellToolStartDoesNotPersistAgain(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task", "session", "")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	creator := &mockMessageCreator{}
	svc.messageCreator = creator
	payload := &lifecycle.AgentStreamEventPayload{TaskID: "task", SessionID: "session", Data: &lifecycle.AgentStreamEventData{
		Type: streams.EventTypeToolCall, ToolCallID: "tool", ToolStatus: "pending", ToolTitle: "cmd",
		CanonicalProjection: true, MessageID: "committed-message", Normalized: streams.NewShellExec("cmd", "", "", 0, false),
	}}
	svc.handleToolCallEvent(context.Background(), payload)
	require.Zero(t, creator.toolCallWrites, "the committed card must only be notified")
}
