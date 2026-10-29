package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/stretchr/testify/require"
)

type approvalAttemptSource struct{ value *sandbox.Attempt }

func (s approvalAttemptSource) ReadAttempt(_ context.Context, org, attempt string) (*sandbox.Attempt, error) {
	if org != "org" || attempt != "attempt" {
		return nil, errors.New("private storage error")
	}
	return s.value, nil
}

func TestSandboxApprovalNotificationBindsCurrentSession(t *testing.T) {
	tasks, _, _ := observationTestScope(t)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	events := bus.NewMemoryEventBus(log)
	t.Cleanup(events.Close)
	var published []*bus.Event
	_, err = events.Subscribe(">", func(_ context.Context, event *bus.Event) error { published = append(published, event); return nil })
	require.NoError(t, err)
	attempt := &sandbox.Attempt{ID: "attempt", Snapshot: sandbox.AttemptSnapshot{TaskID: "task", SessionID: "session"}}
	notifier := sandboxApprovalNotifier{tasks: tasks, policies: approvalAttemptSource{attempt}, eventBus: events}
	require.NoError(t, notifier.publish(t.Context(), "org", "attempt"))
	require.Len(t, published, 1)
	require.Equal(t, "task_session.sandbox_approvals_changed", published[0].Type)
	session, err := tasks.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	require.Equal(t, map[string]any{"task_id": "task", "session_id": "session", "workspace_id": "workspace", "queue_incarnation_id": session.QueueIncarnationID}, published[0].Data)
	for _, invalid := range []struct{ org, task, session string }{
		{"foreign", "task", "session"}, {"org", "foreign", "session"}, {"org", "task", "missing"},
	} {
		attempt.Snapshot.TaskID, attempt.Snapshot.SessionID = invalid.task, invalid.session
		require.Error(t, notifier.publish(t.Context(), invalid.org, "attempt"))
	}
	require.Len(t, published, 1, "foreign or deleted session events must not publish")
	attempt.Snapshot.SessionID = ""
	require.NoError(t, notifier.publish(t.Context(), "org", "attempt"))
	require.Len(t, published, 1, "sessionless discovery has no browser approval destination")
}
