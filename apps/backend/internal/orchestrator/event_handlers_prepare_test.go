package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/events/bus"
)

func makeCompletedEvent(payload interface{}) *bus.Event {
	return &bus.Event{
		Type:      "executor.prepare.completed",
		Data:      payload,
		Timestamp: time.Now(),
	}
}

// getPrepareResult retrieves and type-asserts the prepare_result from session metadata
// after a SQLite round-trip. JSON deserialization gives map[string]interface{} for nested
// objects and []interface{} for slices (not []map[string]interface{}).
func getPrepareResult(t *testing.T, metadata map[string]interface{}) map[string]interface{} {
	t.Helper()
	raw, ok := metadata["prepare_result"]
	require.True(t, ok, "prepare_result key must exist in metadata")
	result, ok := raw.(map[string]interface{})
	require.True(t, ok, "prepare_result must be a map, got %T", raw)
	return result
}

func TestHandlePrepareCompleted_Success(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "sess-1", "step1")

	svc := &Service{logger: testLogger(), repo: repo}

	now := time.Now().UTC()
	payload := &lifecycle.PrepareCompletedEventPayload{
		TaskID:     "task-1",
		SessionID:  "sess-1",
		Success:    true,
		DurationMs: 1200,
		Steps: []lifecycle.PrepareStep{
			{
				Name:      "Validate workspace",
				Status:    lifecycle.PrepareStepCompleted,
				Output:    "ok",
				StartedAt: &now,
				EndedAt:   &now,
			},
		},
	}

	err := svc.handlePrepareCompleted(ctx, makeCompletedEvent(payload))
	require.NoError(t, err)

	session, err := repo.GetTaskSession(ctx, "sess-1")
	require.NoError(t, err)
	require.NotNil(t, session.Metadata)

	result := getPrepareResult(t, session.Metadata)
	require.Equal(t, "completed", result["status"])

	// duration_ms round-trips through JSON as float64.
	require.Equal(t, float64(1200), result["duration_ms"])

	// Steps round-trip through JSON as []interface{}.
	stepsRaw, ok := result["steps"].([]interface{})
	require.True(t, ok, "steps must be a slice")
	require.Len(t, stepsRaw, 1)

	step, ok := stepsRaw[0].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "Validate workspace", step["name"])
	require.Equal(t, "completed", step["status"])
	require.Equal(t, "ok", step["output"])
	require.NotEmpty(t, step["started_at"])
	require.NotEmpty(t, step["ended_at"])
}

func TestHandlePrepareCompleted_Failure(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-2", "sess-2", "step1")

	svc := &Service{logger: testLogger(), repo: repo}

	payload := &lifecycle.PrepareCompletedEventPayload{
		TaskID:       "task-2",
		SessionID:    "sess-2",
		Success:      false,
		ErrorMessage: "git checkout failed: branch not found",
		Steps: []lifecycle.PrepareStep{
			{
				Name:   "Checkout branch",
				Status: lifecycle.PrepareStepFailed,
				Error:  "branch not found",
			},
		},
	}

	err := svc.handlePrepareCompleted(ctx, makeCompletedEvent(payload))
	require.NoError(t, err)

	session, err := repo.GetTaskSession(ctx, "sess-2")
	require.NoError(t, err)
	require.NotNil(t, session.Metadata)

	result := getPrepareResult(t, session.Metadata)
	require.Equal(t, agentEventFailed, result["status"])
	require.Equal(t, "git checkout failed: branch not found", result["error_message"])

	stepsRaw, ok := result["steps"].([]interface{})
	require.True(t, ok)
	require.Len(t, stepsRaw, 1)

	step, ok := stepsRaw[0].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "branch not found", step["error"])
}

func TestHandlePrepareCompleted_TruncatesLargeOutput(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-3", "sess-3", "step1")

	svc := &Service{logger: testLogger(), repo: repo}

	largeOutput := strings.Repeat("x", lifecycle.MaxStepOutputBytes+500)
	payload := &lifecycle.PrepareCompletedEventPayload{
		TaskID:    "task-3",
		SessionID: "sess-3",
		Success:   true,
		Steps: []lifecycle.PrepareStep{
			{Name: "Run setup script", Status: lifecycle.PrepareStepCompleted, Output: largeOutput},
		},
	}

	err := svc.handlePrepareCompleted(ctx, makeCompletedEvent(payload))
	require.NoError(t, err)

	session, err := repo.GetTaskSession(ctx, "sess-3")
	require.NoError(t, err)

	result := getPrepareResult(t, session.Metadata)
	stepsRaw := result["steps"].([]interface{})
	step := stepsRaw[0].(map[string]interface{})
	output := step["output"].(string)

	require.True(t, strings.HasSuffix(output, "\n... (truncated)"), "truncated output must end with marker")
	require.Less(t, len(output), len(largeOutput), "output must be shorter than original")
	require.LessOrEqual(t, len(output), lifecycle.MaxStepOutputBytes+len("\n... (truncated)"))
}

func TestHandlePrepareCompleted_MissingSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	// Intentionally no seedSession — "sess-missing" does not exist.

	svc := &Service{logger: testLogger(), repo: repo}

	payload := &lifecycle.PrepareCompletedEventPayload{
		TaskID:    "task-4",
		SessionID: "sess-missing",
		Success:   true,
	}

	// Must return nil (non-retryable) rather than propagating the not-found error.
	err := svc.handlePrepareCompleted(ctx, makeCompletedEvent(payload))
	require.NoError(t, err)
}

func TestHandlePrepareCompleted_JSONFallback(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-5", "sess-5", "step1")

	svc := &Service{logger: testLogger(), repo: repo}

	// Pass a non-*lifecycle.PrepareCompletedEventPayload to trigger the JSON
	// fallback path. The raw struct uses the same JSON field names so it
	// round-trips cleanly into PrepareCompletedEventPayload.
	type rawPayload struct {
		TaskID    string `json:"task_id"`
		SessionID string `json:"session_id"`
		Success   bool   `json:"success"`
	}

	err := svc.handlePrepareCompleted(ctx, makeCompletedEvent(&rawPayload{
		TaskID:    "task-5",
		SessionID: "sess-5",
		Success:   true,
	}))
	require.NoError(t, err)

	session, err := repo.GetTaskSession(ctx, "sess-5")
	require.NoError(t, err)
	require.NotNil(t, session.Metadata)

	result := getPrepareResult(t, session.Metadata)
	require.Equal(t, "completed", result["status"])
}
