package handlers

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestSessionWakeProjectionUsesPublicSnakeCaseShape(t *testing.T) {
	now := time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC)
	wake := &models.TaskSessionWake{
		ID:                 "wake-1",
		TaskID:             "task-secret",
		SessionID:          "session-secret",
		Marker:             "standup",
		Prompt:             "WAKE:STANDUP",
		CronExpression:     "55 7 * * *",
		Timezone:           "America/Montreal",
		NextRunAt:          now.Add(time.Hour),
		ExpiresAt:          now.Add(24 * time.Hour),
		LastDeliveryStatus: models.SessionWakeDeliveryPending,
		CreatedAt:          now.Add(-time.Hour),
		UpdatedAt:          now,
	}

	body, err := json.Marshal(map[string]any{"wake": projectSessionWake(wake, now)})
	require.NoError(t, err)

	var payload map[string]map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	projected := payload["wake"]
	require.Equal(t, "wake-1", projected["id"])
	require.Equal(t, "standup", projected["marker"])
	require.Equal(t, "WAKE:STANDUP", projected["prompt"])
	require.Equal(t, "55 7 * * *", projected["cron_expression"])
	require.Equal(t, "America/Montreal", projected["timezone"])
	require.Equal(t, "active", projected["state"])
	require.Contains(t, projected, "last_attempt_at")
	require.NotContains(t, projected, "ID")
	require.NotContains(t, projected, "TaskID")
	require.NotContains(t, projected, "SessionID")
	require.NotContains(t, projected, "task_id")
	require.NotContains(t, projected, "session_id")
}

func TestSessionWakeProjectionMarksExpiredRows(t *testing.T) {
	now := time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC)
	wake := &models.TaskSessionWake{
		ID:             "wake-expired",
		Marker:         "expired",
		Prompt:         "WAKE:OLD",
		CronExpression: "@hourly",
		Timezone:       "UTC",
		NextRunAt:      now.Add(-time.Hour),
		ExpiresAt:      now.Add(-time.Second),
	}

	projected := projectSessionWake(wake, now)
	require.Equal(t, "expired", projected.State)
}

func TestMessageTaskSelfSessionErrorDocumentsPersistedFallback(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPMessageTask, map[string]any{
		"task_id":           "task-1",
		"prompt":            "note",
		"sender_task_id":    "task-1",
		"sender_session_id": "session-1",
	})
	resp, err := h.handleMessageTask(t.Context(), msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	body, err := json.Marshal(resp)
	require.NoError(t, err)
	require.Contains(t, string(body), "current assistant response")
	require.Contains(t, string(body), "task plan")
	require.Contains(t, string(body), "sibling session")
}
