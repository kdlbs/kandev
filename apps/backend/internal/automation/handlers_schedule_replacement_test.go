package automation

import (
	"context"
	"encoding/json"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-001.20, AC-OFFICE-AUTOMATIONS-SETTINGS-001.21, AC-OFFICE-AUTOMATIONS-SETTINGS-001.25
func TestAutomationTriggerUpdate_ScheduleReplacement(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		edit := time.Date(2026, time.November, 9, 9, 19, 3, 0, time.UTC)
		advanceFakeClock(t, edit)
		svc := newTestService(t)
		ctx := context.Background()
		eventsObserved := subscribeTriggeredEvents(t, svc)
		cs := NewCronScheduler(svc, svc.logger)
		a := seedScheduleReplacementAutomation(t, svc)
		trig := seedScheduleReplacementTrigger(
			t, svc, a.ID, scheduleReplacementConfig(t, "0 9 * * 6", ""),
			scheduleReplacementCursor(time.Date(2026, time.November, 7, 9, 0, 0, 0, time.UTC)),
		)

		request, err := ws.NewRequest("request-5220", ws.ActionAutomationTriggerUpdate, map[string]any{
			"id": trig.ID,
			"config": map[string]string{
				"cron_expression": "9 9 * * 6",
			},
		})
		require.NoError(t, err)
		response, err := wsUpdateTrigger(svc, svc.logger)(ctx, request)
		require.NoError(t, err)
		require.Equal(t, ws.MessageTypeResponse, response.Type)

		stored, err := svc.store.GetTrigger(ctx, trig.ID)
		require.NoError(t, err)
		require.NotNil(t, stored.LastEvaluatedAt)
		require.True(t, stored.LastEvaluatedAt.Equal(edit),
			"the public update action must persist the replacement boundary")
		require.Equal(t, trig.TriggerRevision+1, stored.TriggerRevision)

		require.False(t, cs.shouldFire(stored, edit.Add(22*time.Second)))
		first := time.Date(2026, time.November, 14, 9, 9, 0, 0, time.UTC)
		require.True(t, cs.shouldFire(stored, first))

		advanceFakeClock(t, first)
		cs.fire(ctx, stored, first)
		runs, err := svc.store.ListRuns(ctx, a.ID, 10)
		require.NoError(t, err)
		require.Len(t, runs, 1)
		require.Equal(t, RunStatusTriggered, runs[0].Status)
		require.Len(t, *eventsObserved, 1)

		var payload map[string]any
		require.NoError(t, json.Unmarshal(response.Payload, &payload))
		require.Equal(t, true, payload["updated"])
	})
}
