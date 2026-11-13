package automation

import (
	"context"
	"encoding/json"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

const scheduleReplacementHistoricalCursor = "2026-11-07T09:00:00Z"

// scheduleReplacementTime parses a fixed RFC3339 instant used by schedule tests.
func scheduleReplacementTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	require.NoError(t, err)
	return parsed.UTC()
}

// seedScheduleReplacementAutomation creates an enabled automation for schedule tests.
func seedScheduleReplacementAutomation(t *testing.T, svc *Service) *Automation {
	t.Helper()
	a := &Automation{
		WorkspaceID:       "ws-1",
		Name:              "Schedule replacement",
		WorkflowID:        "wf-1",
		WorkflowStepID:    "step-1",
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "exec-1",
		Enabled:           true,
		MaxConcurrentRuns: 1,
	}
	require.NoError(t, svc.store.CreateAutomation(context.Background(), a))
	return a
}

// seedScheduleReplacementTrigger persists a scheduled trigger with the supplied config and cursor.
func seedScheduleReplacementTrigger(
	t *testing.T, svc *Service, automationID, raw string, cursor *time.Time,
) *AutomationTrigger {
	t.Helper()
	trig := &AutomationTrigger{
		AutomationID:    automationID,
		Type:            TriggerTypeScheduled,
		Config:          json.RawMessage(raw),
		Enabled:         true,
		LastEvaluatedAt: cursor,
	}
	require.NoError(t, svc.store.CreateTrigger(context.Background(), trig))
	return trig
}

// rawScheduleConfig marshals a scheduled config for an update request.
func rawScheduleConfig(t *testing.T, cfg ScheduledTriggerConfig) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	return raw
}

// requireReplacementBoundary asserts the stored boundary matches one edit instant.
func requireReplacementBoundary(
	t *testing.T, stored *AutomationTrigger, before, after time.Time, wantRevision int64,
) {
	t.Helper()
	require.NotNil(t, stored.LastEvaluatedAt)
	require.False(t, stored.LastEvaluatedAt.Before(before), "boundary predates the edit")
	require.False(t, stored.LastEvaluatedAt.After(after), "boundary postdates the edit")
	require.True(t, stored.LastEvaluatedAt.Equal(stored.UpdatedAt),
		"updated_at and last_evaluated_at must share one edit timestamp")
	require.Equal(t, wantRevision, stored.TriggerRevision)
}

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-001.18, AC-OFFICE-AUTOMATIONS-SETTINGS-001.20
// TestScheduledTriggerUpdate_ReplacementBoundary verifies changed definitions start after the edit.
func TestScheduledTriggerUpdate_ReplacementBoundary(t *testing.T) {
	historical := scheduleReplacementTime(t, scheduleReplacementHistoricalCursor)
	weekly := `{"cron_expression":"0 9 * * 6"}`
	cases := []struct {
		name   string
		old    string
		new    string
		cursor *time.Time
	}{
		{name: "weekly expression", old: weekly, new: `{"cron_expression":"9 9 * * 6"}`, cursor: &historical},
		{name: "daily expression", old: `{"cron_expression":"0 9 * * *"}`, new: `{"cron_expression":"15 10 * * *"}`},
		{name: "hourly expression", old: `{"cron_expression":"0 * * * *"}`, new: `{"cron_expression":"30 * * * *"}`, cursor: &historical},
		{name: "descriptor", old: `{"cron_expression":"@daily"}`, new: `{"cron_expression":"@hourly"}`, cursor: &historical},
		{name: "interval", old: `{"cron_expression":"@every 1h"}`, new: `{"cron_expression":"@every 2h"}`, cursor: &historical},
		{
			name:   "timezone field",
			old:    `{"cron_expression":"0 9 * * *","timezone":"UTC"}`,
			new:    `{"cron_expression":"0 9 * * *","timezone":"America/New_York"}`,
			cursor: &historical,
		},
		{
			name:   "inline timezone prefix",
			old:    `{"cron_expression":"TZ=UTC 0 9 * * *"}`,
			new:    `{"cron_expression":"TZ=America/New_York 0 9 * * *"}`,
			cursor: &historical,
		},
		{name: "unreadable saved definition", old: `{"cron_expression":`, new: `{"cron_expression":"0 9 * * *"}`, cursor: &historical},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t)
			ctx := context.Background()
			a := seedScheduleReplacementAutomation(t, svc)
			trig := seedScheduleReplacementTrigger(t, svc, a.ID, tc.old, tc.cursor)

			before := time.Now().UTC()
			require.NoError(t, svc.UpdateTrigger(ctx, trig.ID, &UpdateTriggerRequest{
				Config: ptrRawMessage(json.RawMessage(tc.new)),
			}))
			after := time.Now().UTC()

			stored, err := svc.store.GetTrigger(ctx, trig.ID)
			require.NoError(t, err)
			require.Equal(t, tc.new, string(stored.Config))
			require.True(t, stored.Enabled)
			requireReplacementBoundary(t, stored, before, after, trig.TriggerRevision+1)
		})
	}
}

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-001.21, AC-OFFICE-AUTOMATIONS-SETTINGS-001.26
// TestScheduledTriggerUpdate_UnchangedDefinition verifies equivalent saves keep the pending occurrence.
func TestScheduledTriggerUpdate_UnchangedDefinition(t *testing.T) {
	historical := scheduleReplacementTime(t, scheduleReplacementHistoricalCursor)
	disabled := false
	cases := []struct {
		name        string
		old         string
		request     UpdateTriggerRequest
		wantEnabled bool
	}{
		{
			name:        "identical JSON",
			old:         `{"cron_expression":"0 9 * * *","timezone":"UTC"}`,
			request:     UpdateTriggerRequest{Config: ptrRawMessage(json.RawMessage(`{"cron_expression":"0 9 * * *","timezone":"UTC"}`))},
			wantEnabled: true,
		},
		{
			name: "member order formatting and unknown members",
			old:  `{"cron_expression":"0 9 * * *","timezone":"UTC"}`,
			request: UpdateTriggerRequest{Config: ptrRawMessage(json.RawMessage(
				`{"timezone":"UTC","unknown_member":true,"cron_expression":"0 9 * * *"}`))},
			wantEnabled: true,
		},
		{
			name:        "surrounding expression whitespace",
			old:         `{"cron_expression":"0 9 * * *"}`,
			request:     UpdateTriggerRequest{Config: ptrRawMessage(json.RawMessage(`{"cron_expression":"  0 9 * * *  "}`))},
			wantEnabled: true,
		},
		{
			name:        "empty timezone versus UTC",
			old:         `{"cron_expression":"0 9 * * *"}`,
			request:     UpdateTriggerRequest{Config: ptrRawMessage(rawScheduleConfig(t, ScheduledTriggerConfig{CronExpression: "0 9 * * *", Timezone: "UTC"}))},
			wantEnabled: true,
		},
		{
			name:        "interval ignores timezone field",
			old:         `{"cron_expression":"@every 1h","timezone":"UTC"}`,
			request:     UpdateTriggerRequest{Config: ptrRawMessage(json.RawMessage(`{"cron_expression":"@every 1h","timezone":"America/New_York"}`))},
			wantEnabled: true,
		},
		{
			name:        "inline timezone expression ignores timezone field",
			old:         `{"cron_expression":"TZ=UTC 0 9 * * *"}`,
			request:     UpdateTriggerRequest{Config: ptrRawMessage(json.RawMessage(`{"cron_expression":"TZ=UTC 0 9 * * *","timezone":"Asia/Tokyo"}`))},
			wantEnabled: true,
		},
		{name: "omitted config", old: `{"cron_expression":"0 9 * * *"}`, request: UpdateTriggerRequest{}, wantEnabled: true},
		{
			name:        "enabled only",
			old:         `{"cron_expression":"0 9 * * *"}`,
			request:     UpdateTriggerRequest{Enabled: &disabled},
			wantEnabled: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t)
			ctx := context.Background()
			a := seedScheduleReplacementAutomation(t, svc)
			trig := seedScheduleReplacementTrigger(t, svc, a.ID, tc.old, &historical)

			require.NoError(t, svc.UpdateTrigger(ctx, trig.ID, &tc.request))

			stored, err := svc.store.GetTrigger(ctx, trig.ID)
			require.NoError(t, err)
			require.NotNil(t, stored.LastEvaluatedAt)
			require.True(t, stored.LastEvaluatedAt.Equal(historical),
				"unchanged schedule must keep its pending occurrence")
			require.Equal(t, tc.wantEnabled, stored.Enabled)
			require.Equal(t, trig.TriggerRevision+1, stored.TriggerRevision)
		})
	}
}

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-001.22, AC-OFFICE-AUTOMATIONS-SETTINGS-001.23
// TestScheduledTriggerUpdate_DisabledAndEmpty verifies disabled and empty schedule transitions.
func TestScheduledTriggerUpdate_DisabledAndEmpty(t *testing.T) {
	t.Run("disabled replacement keeps the boundary until re-enabled", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			edit := time.Date(2026, time.November, 9, 9, 19, 3, 0, time.UTC)
			advanceFakeClock(t, edit)
			svc := newTestService(t)
			ctx := context.Background()
			historical := scheduleReplacementTime(t, scheduleReplacementHistoricalCursor)
			a := seedScheduleReplacementAutomation(t, svc)
			trig := seedScheduleReplacementTrigger(t, svc, a.ID, `{"cron_expression":"0 9 * * 6"}`, &historical)

			disabled := false
			replacement := json.RawMessage(`{"cron_expression":"9 9 * * 6"}`)
			before := time.Now().UTC()
			require.NoError(t, svc.UpdateTrigger(ctx, trig.ID, &UpdateTriggerRequest{
				Config: &replacement, Enabled: &disabled,
			}))
			after := time.Now().UTC()

			stored, err := svc.store.GetTrigger(ctx, trig.ID)
			require.NoError(t, err)
			require.False(t, stored.Enabled)
			requireReplacementBoundary(t, stored, before, after, trig.TriggerRevision+1)
			boundary := *stored.LastEvaluatedAt
			require.True(t, boundary.Equal(stored.UpdatedAt))

			enabled := true
			require.NoError(t, svc.UpdateTrigger(ctx, trig.ID, &UpdateTriggerRequest{Enabled: &enabled}))
			stored, err = svc.store.GetTrigger(ctx, trig.ID)
			require.NoError(t, err)
			require.True(t, stored.Enabled)
			require.NotNil(t, stored.LastEvaluatedAt)
			require.True(t, stored.LastEvaluatedAt.Equal(boundary),
				"re-enabling must preserve the replacement boundary")

			next, err := nextCronFire("9 9 * * 6", "", boundary)
			require.NoError(t, err)
			require.Equal(t, "2026-11-14T09:09:00Z", next.UTC().Format(time.RFC3339))
		})
	})

	t.Run("cleared schedule then non-empty replacement", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			firstEdit := time.Date(2026, time.November, 9, 9, 0, 0, 0, time.UTC)
			advanceFakeClock(t, firstEdit)
			svc := newTestService(t)
			ctx := context.Background()
			historical := scheduleReplacementTime(t, scheduleReplacementHistoricalCursor)
			a := seedScheduleReplacementAutomation(t, svc)
			trig := seedScheduleReplacementTrigger(t, svc, a.ID, `{"cron_expression":"0 9 * * *"}`, &historical)

			cleared := json.RawMessage(`{"cron_expression":"","timezone":"America/New_York"}`)
			require.NoError(t, svc.UpdateTrigger(ctx, trig.ID, &UpdateTriggerRequest{Config: &cleared}))
			stored, err := svc.store.GetTrigger(ctx, trig.ID)
			require.NoError(t, err)
			require.NotNil(t, stored.LastEvaluatedAt)
			identity, err := normalizeScheduledConfig(stored.Config)
			require.NoError(t, err)
			require.False(t, identity.scheduled)
			firstBoundary := *stored.LastEvaluatedAt
			require.True(t, firstBoundary.Equal(firstEdit))

			secondEdit := firstEdit.Add(time.Hour)
			advanceFakeClock(t, secondEdit)
			replacement := json.RawMessage(`{"cron_expression":"30 8 * * 1"}`)
			require.NoError(t, svc.UpdateTrigger(ctx, trig.ID, &UpdateTriggerRequest{Config: &replacement}))
			stored, err = svc.store.GetTrigger(ctx, trig.ID)
			require.NoError(t, err)
			require.NotNil(t, stored.LastEvaluatedAt)
			require.True(t, stored.LastEvaluatedAt.After(firstBoundary),
				"a later non-empty replacement records its own boundary")
			require.True(t, stored.LastEvaluatedAt.Equal(secondEdit))
			next, err := nextCronFire("30 8 * * 1", "", *stored.LastEvaluatedAt)
			require.NoError(t, err)
			require.Equal(t, "2026-11-16T08:30:00Z", next.UTC().Format(time.RFC3339))
		})
	})
}

// requireScheduleRowUnchanged asserts a rejected edit left the stored trigger intact.
func requireScheduleRowUnchanged(t *testing.T, svc *Service, want *AutomationTrigger) {
	t.Helper()
	stored, err := svc.store.GetTrigger(context.Background(), want.ID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.Equal(t, want.ConfigJSON, stored.ConfigJSON)
	require.Equal(t, want.Enabled, stored.Enabled)
	require.Equal(t, want.TriggerRevision, stored.TriggerRevision)
	require.True(t, want.UpdatedAt.Equal(stored.UpdatedAt))
	if want.LastEvaluatedAt == nil {
		require.Nil(t, stored.LastEvaluatedAt)
		return
	}
	require.NotNil(t, stored.LastEvaluatedAt)
	require.True(t, want.LastEvaluatedAt.Equal(*stored.LastEvaluatedAt))
}

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-001.24
// TestScheduledTriggerUpdate_RejectedEditPreservesState verifies validation and persistence failures.
func TestScheduledTriggerUpdate_RejectedEditPreservesState(t *testing.T) {
	invalidEdits := []struct {
		name string
		raw  string
	}{
		{name: "invalid expression", raw: `{"cron_expression":"not a cron"}`},
		{name: "impossible expression", raw: `{"cron_expression":"0 0 31 2 *"}`},
		{name: "invalid effective timezone", raw: `{"cron_expression":"0 9 * * *","timezone":"Not/AZone"}`},
	}
	for _, tc := range invalidEdits {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t)
			ctx := context.Background()
			historical := scheduleReplacementTime(t, scheduleReplacementHistoricalCursor)
			a := seedScheduleReplacementAutomation(t, svc)
			trig := seedScheduleReplacementTrigger(t, svc, a.ID, `{"cron_expression":"0 9 * * 6"}`, &historical)
			original, err := svc.store.GetTrigger(ctx, trig.ID)
			require.NoError(t, err)

			raw := json.RawMessage(tc.raw)
			require.Error(t, svc.UpdateTrigger(ctx, trig.ID, &UpdateTriggerRequest{Config: &raw}))
			requireScheduleRowUnchanged(t, svc, original)
		})
	}

	t.Run("failed persistence statement", func(t *testing.T) {
		svc := newTestService(t)
		ctx := context.Background()
		historical := scheduleReplacementTime(t, scheduleReplacementHistoricalCursor)
		a := seedScheduleReplacementAutomation(t, svc)
		trig := seedScheduleReplacementTrigger(t, svc, a.ID, `{"cron_expression":"0 9 * * 6"}`, &historical)
		original, err := svc.store.GetTrigger(ctx, trig.ID)
		require.NoError(t, err)

		_, err = svc.store.db.ExecContext(ctx, `CREATE TRIGGER reject_schedule_update
			BEFORE UPDATE ON automation_triggers
			BEGIN SELECT RAISE(ABORT, 'forced update failure'); END`)
		require.NoError(t, err)

		replacement := json.RawMessage(`{"cron_expression":"9 9 * * 6"}`)
		require.Error(t, svc.UpdateTrigger(ctx, trig.ID, &UpdateTriggerRequest{Config: &replacement}))

		_, err = svc.store.db.ExecContext(ctx, `DROP TRIGGER reject_schedule_update`)
		require.NoError(t, err)
		requireScheduleRowUnchanged(t, svc, original)
	})
}

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-001.21
// TestScheduledTriggerUpdate_NonScheduledCursor verifies non-scheduled updates keep the cursor.
func TestScheduledTriggerUpdate_NonScheduledCursor(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	historical := scheduleReplacementTime(t, scheduleReplacementHistoricalCursor)
	a := seedScheduleReplacementAutomation(t, svc)
	trig := &AutomationTrigger{
		AutomationID:    a.ID,
		Type:            TriggerTypeWebhook,
		Config:          json.RawMessage(`{"filters":[]}`),
		Enabled:         true,
		LastEvaluatedAt: &historical,
	}
	require.NoError(t, svc.store.CreateTrigger(ctx, trig))

	replacement := json.RawMessage(`{"filters":[{"path":"action","op":"exists"}]}`)
	require.NoError(t, svc.UpdateTrigger(ctx, trig.ID, &UpdateTriggerRequest{Config: &replacement}))

	stored, err := svc.store.GetTrigger(ctx, trig.ID)
	require.NoError(t, err)
	require.JSONEq(t, string(replacement), string(stored.Config))
	require.NotNil(t, stored.LastEvaluatedAt)
	require.True(t, stored.LastEvaluatedAt.Equal(historical),
		"non-scheduled updates must not move the evaluation cursor")
	require.Equal(t, trig.TriggerRevision+1, stored.TriggerRevision)
}

// ptrRawMessage returns a pointer to a raw JSON message for update requests.
func ptrRawMessage(raw json.RawMessage) *json.RawMessage {
	return &raw
}
