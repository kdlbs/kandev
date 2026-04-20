package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestHandlePrepareCompleted_PersistsViaJsonSet(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "sess-1", "step1")

	svc := &Service{logger: testLogger(), repo: repo}

	event := &bus.Event{
		Type: "executor.prepare.completed",
		Data: &lifecycle.PrepareCompletedEventPayload{
			SessionID: "sess-1",
			Success:   true,
			Steps:     []lifecycle.PrepareStep{{Name: "step1", Status: lifecycle.PrepareStepCompleted}},
		},
		Timestamp: time.Now(),
	}
	err := svc.handlePrepareCompleted(context.Background(), event)
	require.NoError(t, err)

	session, err := repo.GetTaskSession(context.Background(), "sess-1")
	require.NoError(t, err)
	require.NotNil(t, session.Metadata["prepare_result"], "prepare_result should be persisted")
}

func TestHandlePrepareCompleted_PersistsFailure(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "sess-1", "step1")

	svc := &Service{logger: testLogger(), repo: repo}

	event := &bus.Event{
		Type: "executor.prepare.completed",
		Data: &lifecycle.PrepareCompletedEventPayload{
			SessionID:    "sess-1",
			Success:      false,
			ErrorMessage: "setup script exited with code 1",
			Steps: []lifecycle.PrepareStep{
				{Name: "step1", Status: lifecycle.PrepareStepCompleted},
				{Name: "step2", Status: lifecycle.PrepareStepFailed, Output: "npm ERR! missing"},
			},
		},
		Timestamp: time.Now(),
	}
	err := svc.handlePrepareCompleted(context.Background(), event)
	require.NoError(t, err)

	session, err := repo.GetTaskSession(context.Background(), "sess-1")
	require.NoError(t, err)

	pr, ok := session.Metadata["prepare_result"].(map[string]interface{})
	require.True(t, ok, "prepare_result should be a map")
	require.Equal(t, "failed", pr["status"])
	require.Equal(t, "setup script exited with code 1", pr["error_message"])

	steps, ok := pr["steps"].([]interface{})
	require.True(t, ok)
	require.Len(t, steps, 2)
}

func TestHandlePrepareCompleted_HandlesWrongType(t *testing.T) {
	svc := &Service{logger: testLogger()}

	event := &bus.Event{
		Type:      "executor.prepare.completed",
		Data:      "wrong type",
		Timestamp: time.Now(),
	}
	err := svc.handlePrepareCompleted(context.Background(), event)
	require.NoError(t, err)
}
