package backendapp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

func TestConstructOfficeServices_RecordsUsageWhenOfficeDisabled(t *testing.T) {
	ctx := t.Context()
	harness := newBootStateTestHarness(t)
	task := &models.Task{ID: "task-usage", Title: "Record usage"}
	require.NoError(t, harness.taskRepo.CreateTask(ctx, task))
	session := &models.TaskSession{ID: "session-usage", TaskID: task.ID}
	require.NoError(t, harness.taskRepo.CreateTaskSession(ctx, session))

	log := newTestLogger()
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)
	cfg := &config.Config{HomeDir: t.TempDir(), Features: config.FeaturesConfig{Office: false}}
	_, ok := constructOfficeServices(
		ctx, cfg, log, &Repositories{Task: harness.taskRepo}, &Services{Task: harness.taskSvc},
		nil, eventBus, "", func(cleanup func() error) {
			t.Cleanup(func() { assert.NoError(t, cleanup()) })
		}, nil, nil,
	)
	require.True(t, ok)

	const eventID = "evt-wiring"
	require.NoError(t, eventBus.Publish(ctx, events.BuildSessionPromptUsageSubject(session.ID), &bus.Event{
		Data: map[string]any{
			"task_id":        task.ID,
			"session_id":     session.ID,
			"usage_event_id": eventID,
			"usage":          map[string]any{"input_tokens": int64(7)},
		},
	}))

	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		var tokensIn int64
		require.NoError(collect, harness.db.GetContext(ctx, &tokensIn, `
			SELECT tokens_in FROM task_usage_events WHERE usage_event_id = ? AND task_id = ? AND session_id = ?
		`, eventID, task.ID, session.ID))
		assert.Equal(collect, int64(7), tokensIn)
	}, 3*time.Second, 10*time.Millisecond, "usage event %q was not recorded", eventID)
}
