package backendapp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

type recordingReadiness struct {
	calls int
}

func (r *recordingReadiness) CheckQueueAdmissionReadiness(context.Context, messagequeue.QueueSessionIdentity) {
	r.calls++
}

func TestAsyncQuestionQueueAdapterWakesAdmittedDestination(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	require.NoError(t, err)
	queue := messagequeue.NewServiceMemory(log)
	readiness := &recordingReadiness{}
	adapter := &asyncQuestionQueueAdapter{queue: queue, readiness: readiness}

	delivery := &models.AsyncQuestionDelivery{
		AdmissionID:        "aq-adapter-wake",
		QuestionID:         "question-1",
		Kind:               models.AsyncQuestionDeliveryOwner,
		DestinationTaskID:  "task-1",
		DestinationSession: "session-1",
		Content:            "correlated answer",
		Metadata:           map[string]interface{}{"async_question_id": "question-1"},
	}
	ctx := context.Background()
	require.NoError(t, adapter.AdmitAsyncQuestionDelivery(ctx, delivery))
	require.Equal(t, 1, readiness.calls, "admission must wake the destination")
	require.Equal(t, 1, queue.GetStatus(ctx, "session-1").Count)

	// Replaying the exact admission identity produces no second queue row but
	// still rechecks readiness.
	require.NoError(t, adapter.AdmitAsyncQuestionDelivery(ctx, delivery))
	require.Equal(t, 2, readiness.calls)
	require.Equal(t, 1, queue.GetStatus(ctx, "session-1").Count)
}
