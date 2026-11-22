package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
)

func stampedEvent() watcher.AgentEventData {
	at := time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)
	return watcher.AgentEventData{
		TaskID: "task-1", SessionID: "sess-1", TurnID: "turn-1",
		DeliveryStreamID: "stream-1", DeliverySequence: 7,
		DetachedAtCommit: true, CommittedAt: &at,
	}
}

func TestTurnStatusRecordDetachedEndWritesOneIdempotentRecord(t *testing.T) {
	svc, _ := newServiceWithRealRepo(t)
	messages := &mockMessageCreator{}
	svc.messageCreator = messages

	svc.recordTurnStatusNotice(context.Background(), stampedEvent())
	svc.recordTurnStatusNotice(context.Background(), stampedEvent())

	require.Len(t, messages.sessionMessages, 1)
	note := messages.sessionMessages[0]
	require.Equal(t, "sess-1", note.sessionID)
	require.Equal(t, "status", note.messageType)
	require.Equal(t, turnStatusKindDetached, note.metadata["kind"])
	require.Equal(t, "stream-1:7", note.metadata["episode"])
	require.Equal(t, "2026-10-26T10:00:00Z", note.metadata["occurred_at"])
	require.NotContains(t, note.content, "—")
}

func TestTurnStatusRecordBudgetCancelWritesOnlyThePausedRecord(t *testing.T) {
	svc, _ := newServiceWithRealRepo(t)
	messages := &mockMessageCreator{}
	svc.messageCreator = messages
	event := stampedEvent()
	decided := time.Date(2026, 10, 26, 9, 45, 0, 0, time.UTC)
	event.CancelCause = "offline_budget"
	event.CancelDecidedAt = &decided

	svc.recordTurnStatusNotice(context.Background(), event)

	require.Len(t, messages.sessionMessages, 1)
	require.Equal(t, turnStatusKindPaused, messages.sessionMessages[0].metadata["kind"])
	require.Equal(t, "2026-10-26T09:45:00Z", messages.sessionMessages[0].metadata["occurred_at"])
}

func TestTurnStatusRecordUnstampedTerminalWritesNothing(t *testing.T) {
	svc, _ := newServiceWithRealRepo(t)
	messages := &mockMessageCreator{}
	svc.messageCreator = messages
	event := stampedEvent()
	event.DetachedAtCommit = false
	event.CommittedAt = nil

	svc.recordTurnStatusNotice(context.Background(), event)

	require.Empty(t, messages.sessionMessages)
}

func TestTurnStatusRecordMissingCommitTimeFallsBackToNow(t *testing.T) {
	svc, _ := newServiceWithRealRepo(t)
	messages := &mockMessageCreator{}
	svc.messageCreator = messages
	event := stampedEvent()
	event.CommittedAt = nil

	svc.recordTurnStatusNotice(context.Background(), event)

	require.Len(t, messages.sessionMessages, 1)
	_, err := time.Parse(time.RFC3339, messages.sessionMessages[0].metadata["occurred_at"].(string))
	require.NoError(t, err)
}

func TestTurnStatusRecordNeedsATerminalIdentity(t *testing.T) {
	svc, _ := newServiceWithRealRepo(t)
	messages := &mockMessageCreator{}
	svc.messageCreator = messages
	event := stampedEvent()
	event.DeliveryStreamID = ""

	svc.recordTurnStatusNotice(context.Background(), event)

	require.Empty(t, messages.sessionMessages)
}
