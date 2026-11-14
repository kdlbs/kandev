package backendapp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/questions"
)

type pendingActionWaitRecorder struct {
	taskID    string
	sessionID string
	calls     int
}

func (r *pendingActionWaitRecorder) AsyncQuestionWaitChanged(_ context.Context, taskID, sessionID string) error {
	r.taskID = taskID
	r.sessionID = sessionID
	r.calls++
	return nil
}

func TestAsyncQuestionNotifierRefreshesPendingActionAfterWaitChange(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	require.NoError(t, err)
	publisher := &pendingActionWaitRecorder{}
	notifier := asyncQuestionNotifier{bus: bus.NewMemoryEventBus(log), pendingActions: publisher}
	notifier.AsyncQuestionChanged(context.Background(), questions.QuestionChange{
		Kind: "wait_changed", TaskID: "task-wait-change", SessionID: "session-wait-change",
	})
	require.Equal(t, 1, publisher.calls)
	require.Equal(t, "task-wait-change", publisher.taskID)
	require.Equal(t, "session-wait-change", publisher.sessionID)

	notifier.AsyncQuestionChanged(context.Background(), questions.QuestionChange{
		Kind: "updated", TaskID: "task-wait-change", SessionID: "session-wait-change",
	})
	require.Equal(t, 1, publisher.calls, "question updates must not trigger a gate projection")
}
