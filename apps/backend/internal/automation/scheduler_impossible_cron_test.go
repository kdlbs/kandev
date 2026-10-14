package automation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-001.17
func TestCronScheduler_PersistedImpossibleScheduleProducesNoRuns(t *testing.T) {
	anchor := time.Date(2026, time.October, 13, 8, 0, 0, 0, time.UTC)
	for _, evaluated := range []*time.Time{nil, &anchor} {
		name := "creation anchor"
		if evaluated != nil {
			name = "evaluation anchor"
		}
		t.Run(name, func(t *testing.T) {
			svc := newTestService(t)
			t.Cleanup(svc.eventBus.Close)
			ctx := context.Background()
			a, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
				WorkspaceID: "calendar-history", Name: "Calendar report", MaxConcurrentRuns: 1,
			})
			require.NoError(t, err)
			trig := persistCalendarTrigger(t, svc, a.ID, "0 0 31 2 *", anchor, evaluated)
			active := &AutomationRun{
				AutomationID: a.ID, TriggerID: trig.ID, TriggerType: TriggerTypeScheduled,
				Status: RunStatusTaskCreated, DedupKey: "calendar-active", TriggerData: json.RawMessage(`{}`),
			}
			require.NoError(t, svc.store.CreateRun(ctx, active))
			eventsObserved := 0
			_, err = svc.eventBus.Subscribe(events.AutomationTriggered, func(context.Context, *bus.Event) error {
				eventsObserved++
				return nil
			})
			require.NoError(t, err)
			cs := NewCronScheduler(svc, svc.logger)
			for minute := 1; minute <= 3; minute++ {
				stored, readErr := svc.store.GetTrigger(ctx, trig.ID)
				require.NoError(t, readErr)
				now := anchor.Add(time.Duration(minute) * time.Minute)
				due := cs.shouldFire(stored, now)
				if due {
					cs.fire(ctx, stored, now)
				}
				assert.False(t, due, "impossible schedule was due at minute %d", minute)
			}
			runs, err := svc.store.ListRuns(ctx, a.ID, 10)
			require.NoError(t, err)
			require.Len(t, runs, 1, "scheduler checks must not create skipped run history")
			require.Equal(t, active.ID, runs[0].ID)
			after, err := svc.store.GetTrigger(ctx, trig.ID)
			require.NoError(t, err)
			require.Equal(t, trig, after, "scheduler checks must not mutate an impossible trigger")
			require.Zero(t, eventsObserved)
		})
	}
}

func persistCalendarTrigger(t *testing.T, svc *Service, automationID, expr string, anchor time.Time, evaluated *time.Time) *AutomationTrigger {
	t.Helper()
	ctx := context.Background()
	cfg, err := json.Marshal(ScheduledTriggerConfig{CronExpression: expr, Timezone: "UTC"})
	require.NoError(t, err)
	trig := &AutomationTrigger{
		AutomationID: automationID, Type: TriggerTypeScheduled, Config: cfg, Enabled: true,
		LastEvaluatedAt: evaluated,
	}
	require.NoError(t, svc.store.CreateTrigger(ctx, trig))
	_, err = svc.store.db.ExecContext(ctx, `UPDATE automation_triggers SET created_at = ? WHERE id = ?`, anchor, trig.ID)
	require.NoError(t, err)
	stored, err := svc.store.GetTrigger(ctx, trig.ID)
	require.NoError(t, err)
	return stored
}

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-001.17, AC-OFFICE-AUTOMATIONS-SETTINGS-001.18
func TestCronScheduler_ImpossibleTriggerDoesNotBlockValidSibling(t *testing.T) {
	svc := newTestService(t)
	t.Cleanup(svc.eventBus.Close)
	ctx := context.Background()
	a, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{WorkspaceID: "calendar-siblings", Name: "Calendar report"})
	require.NoError(t, err)
	anchor := time.Date(2026, time.October, 13, 8, 0, 0, 0, time.UTC)
	impossible := persistCalendarTrigger(t, svc, a.ID, "0 0 31 2 *", anchor, nil)
	valid := persistCalendarTrigger(t, svc, a.ID, "0 9 * * *", anchor, nil)
	var published []string
	_, err = svc.eventBus.Subscribe(events.AutomationTriggered, func(_ context.Context, event *bus.Event) error {
		published = append(published, event.Data.(*AutomationTriggeredEvent).TriggerID)
		return nil
	})
	require.NoError(t, err)
	triggers, err := svc.store.ListEnabledTriggersByType(ctx, TriggerTypeScheduled)
	require.NoError(t, err)
	require.Len(t, triggers, 2)
	cs := NewCronScheduler(svc, svc.logger)
	now := anchor.Add(time.Hour)
	for i := range triggers {
		if cs.shouldFire(&triggers[i], now) {
			cs.fire(ctx, &triggers[i], now)
		}
	}
	runs, err := svc.store.ListRuns(ctx, a.ID, 10)
	require.NoError(t, err)
	require.Len(t, runs, 1, "only the valid sibling must create a run")
	require.Equal(t, valid.ID, runs[0].TriggerID)
	require.Equal(t, RunStatusTriggered, runs[0].Status)
	require.Equal(t, []string{valid.ID}, published, "the valid sibling must reach actual event publication")
	stored, err := svc.store.GetTrigger(ctx, impossible.ID)
	require.NoError(t, err)
	require.Equal(t, impossible, stored)
}

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-001.18
func TestCronScheduler_ScheduleOccurrenceControls(t *testing.T) {
	anchor := time.Date(2026, time.October, 13, 8, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		expr string
		due  time.Time
	}{
		{"daily", "0 9 * * *", time.Date(2026, time.October, 13, 9, 0, 0, 0, time.UTC)},
		{"leap day", "0 0 29 2 *", time.Date(2028, time.February, 29, 0, 0, 0, 0, time.UTC)},
		{"DOM DOW OR", "0 0 31 2 MON", time.Date(2027, time.February, 1, 0, 0, 0, 0, time.UTC)},
		{"interval", "@every 5m", anchor.Add(5 * time.Minute)},
	}
	svc := newTestService(t)
	t.Cleanup(svc.eventBus.Close)
	cs := NewCronScheduler(svc, svc.logger)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := json.Marshal(ScheduledTriggerConfig{CronExpression: tc.expr, Timezone: "UTC"})
			require.NoError(t, err)
			trig := &AutomationTrigger{Config: cfg, CreatedAt: anchor}
			require.False(t, cs.shouldFire(trig, tc.due.Add(-time.Nanosecond)))
			require.True(t, cs.shouldFire(trig, tc.due))
			trig.LastEvaluatedAt = &tc.due
			require.False(t, cs.shouldFire(trig, tc.due), "evaluated occurrence must not immediately refire")
		})
	}
}
