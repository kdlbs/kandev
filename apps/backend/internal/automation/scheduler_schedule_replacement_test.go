package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// scheduleReplacementCursor returns a pointer to a fixed evaluation instant.
func scheduleReplacementCursor(value time.Time) *time.Time {
	return &value
}

// requireAutomationRunCount asserts the recorded run count and trigger status.
func requireAutomationRunCount(
	t *testing.T, svc *Service, automationID string, wantRuns int,
) []*AutomationRun {
	t.Helper()
	runs, err := svc.store.ListRuns(context.Background(), automationID, 20)
	require.NoError(t, err)
	require.Len(t, runs, wantRuns)
	if wantRuns > 0 {
		require.Equal(t, RunStatusTriggered, runs[0].Status)
	}
	return runs
}

// subscribeTriggeredEvents records published automation-triggered events.
func subscribeTriggeredEvents(t *testing.T, svc *Service) *[]*AutomationTriggeredEvent {
	t.Helper()
	var observed []*AutomationTriggeredEvent
	_, err := svc.eventBus.Subscribe(events.AutomationTriggered, func(_ context.Context, event *bus.Event) error {
		triggered, ok := event.Data.(*AutomationTriggeredEvent)
		require.True(t, ok)
		observed = append(observed, triggered)
		return nil
	})
	require.NoError(t, err)
	t.Cleanup(svc.eventBus.Close)
	return &observed
}

// advanceFakeClock advances a synctest bubble to the target instant.
func advanceFakeClock(t *testing.T, target time.Time) {
	t.Helper()
	if delta := time.Until(target); delta > 0 {
		time.Sleep(delta)
	}
}

type replacementCalendarCase struct {
	name     string
	oldExpr  string
	timezone string
	newExpr  string
	cursor   time.Time
	edit     time.Time
	notYet   time.Time
	first    time.Time
	after    time.Time
}

// replacementCalendarCases returns fixed calendar replacement scenarios.
func replacementCalendarCases(t *testing.T) []replacementCalendarCase {
	t.Helper()
	newYork, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	return []replacementCalendarCase{
		{
			name:    "weekly Saturday replacement on Monday",
			oldExpr: "0 9 * * 6", newExpr: "9 9 * * 6",
			cursor: time.Date(2026, time.November, 7, 9, 0, 0, 0, time.UTC),
			edit:   time.Date(2026, time.November, 9, 9, 19, 3, 0, time.UTC),
			notYet: time.Date(2026, time.November, 9, 9, 19, 25, 0, time.UTC),
			first:  time.Date(2026, time.November, 14, 9, 9, 0, 0, time.UTC),
			after:  time.Date(2026, time.November, 14, 9, 9, 30, 0, time.UTC),
		},
		{
			name:    "Monday New York replacement",
			oldExpr: "0 9 * * 1", timezone: "America/New_York", newExpr: "9 9 * * 1",
			cursor: time.Date(2026, time.November, 2, 9, 0, 0, 0, newYork),
			edit:   time.Date(2026, time.November, 9, 9, 19, 3, 0, newYork),
			notYet: time.Date(2026, time.November, 9, 9, 19, 25, 0, newYork),
			first:  time.Date(2026, time.November, 16, 9, 9, 0, 0, newYork),
			after:  time.Date(2026, time.November, 16, 9, 9, 30, 0, newYork),
		},
		{
			name:    "spring forward replacement",
			oldExpr: "0 2 * * *", timezone: "America/New_York", newExpr: "0 3 * * *",
			cursor: time.Date(2027, time.March, 13, 3, 0, 0, 0, newYork),
			edit:   time.Date(2027, time.March, 14, 1, 30, 0, 0, newYork),
			notYet: time.Date(2027, time.March, 14, 6, 59, 59, 0, time.UTC),
			first:  time.Date(2027, time.March, 14, 7, 0, 0, 0, time.UTC),
			after:  time.Date(2027, time.March, 14, 7, 0, 30, 0, time.UTC),
		},
		{
			name:    "fall back replacement",
			oldExpr: "0 0 * * *", timezone: "America/New_York", newExpr: "0 1 * * *",
			cursor: time.Date(2026, time.October, 31, 0, 0, 0, 0, newYork),
			edit:   time.Date(2026, time.November, 1, 0, 30, 0, 0, newYork),
			notYet: time.Date(2026, time.November, 1, 4, 59, 59, 0, time.UTC),
			first:  time.Date(2026, time.November, 1, 5, 0, 0, 0, time.UTC),
			after:  time.Date(2026, time.November, 1, 6, 0, 30, 0, time.UTC),
		},
		{
			name:    "edit exactly on a scheduled instant",
			oldExpr: "30 8 * * *", newExpr: "0 9 * * *",
			cursor: time.Date(2026, time.November, 8, 8, 30, 0, 0, time.UTC),
			edit:   time.Date(2026, time.November, 9, 9, 0, 0, 0, time.UTC),
			notYet: time.Date(2026, time.November, 9, 9, 0, 0, 0, time.UTC),
			first:  time.Date(2026, time.November, 10, 9, 0, 0, 0, time.UTC),
			after:  time.Date(2026, time.November, 10, 9, 0, 30, 0, time.UTC),
		},
	}
}

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-001.18, AC-OFFICE-AUTOMATIONS-SETTINGS-001.20, AC-OFFICE-AUTOMATIONS-SETTINGS-001.26
// TestCronScheduler_ReplacementWaitsForNextOccurrence verifies a replacement waits for its first occurrence.
func TestCronScheduler_ReplacementWaitsForNextOccurrence(t *testing.T) {
	for _, tc := range replacementCalendarCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				advanceFakeClock(t, tc.edit)
				svc := newTestService(t)
				ctx := context.Background()
				eventsObserved := subscribeTriggeredEvents(t, svc)
				cs := NewCronScheduler(svc, svc.logger)
				a := seedScheduleReplacementAutomation(t, svc)
				trig := seedScheduleReplacementTrigger(t, svc, a.ID, scheduleReplacementConfig(t, tc.oldExpr, tc.timezone), scheduleReplacementCursor(tc.cursor))

				replacement := json.RawMessage(scheduleReplacementConfig(t, tc.newExpr, tc.timezone))
				require.NoError(t, svc.UpdateTrigger(ctx, trig.ID, &UpdateTriggerRequest{Config: &replacement}))

				stored, err := svc.store.GetTrigger(ctx, trig.ID)
				require.NoError(t, err)
				require.NotNil(t, stored.LastEvaluatedAt)
				require.True(t, stored.LastEvaluatedAt.Equal(tc.edit),
					"replacement boundary must be the successful edit")
				require.False(t, cs.shouldFire(stored, tc.notYet),
					"the replacement must not fire before its first occurrence")
				require.True(t, cs.shouldFire(stored, tc.first),
					"the replacement must fire at its first occurrence")

				advanceFakeClock(t, tc.first)
				cs.fire(ctx, stored, tc.first)
				requireAutomationRunCount(t, svc, a.ID, 1)
				require.Len(t, *eventsObserved, 1)

				after, err := svc.store.GetTrigger(ctx, trig.ID)
				require.NoError(t, err)
				require.NotNil(t, after.LastEvaluatedAt)
				require.True(t, after.LastEvaluatedAt.Equal(tc.first))
				require.False(t, cs.shouldFire(after, tc.after),
					"an admitted occurrence must not repeat immediately")
				requireAutomationRunCount(t, svc, a.ID, 1)
			})
		})
	}
}

// scheduleReplacementConfig marshals a scheduled config string.
func scheduleReplacementConfig(t *testing.T, expression, timezone string) string {
	t.Helper()
	raw, err := json.Marshal(ScheduledTriggerConfig{CronExpression: expression, Timezone: timezone})
	require.NoError(t, err)
	return string(raw)
}

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-001.21, AC-OFFICE-AUTOMATIONS-SETTINGS-001.26
// TestCronScheduler_UnchangedScheduleCatchUp verifies an unchanged save keeps single catch-up behavior.
func TestCronScheduler_UnchangedScheduleCatchUp(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		now := time.Date(2026, time.November, 7, 10, 0, 0, 0, time.UTC)
		advanceFakeClock(t, now)
		svc := newTestService(t)
		ctx := context.Background()
		eventsObserved := subscribeTriggeredEvents(t, svc)
		cs := NewCronScheduler(svc, svc.logger)
		a := seedScheduleReplacementAutomation(t, svc)
		cursor := time.Date(2026, time.November, 6, 9, 0, 0, 0, time.UTC)
		config := scheduleReplacementConfig(t, "0 9 * * *", "")
		trig := seedScheduleReplacementTrigger(t, svc, a.ID, config, scheduleReplacementCursor(cursor))

		same := json.RawMessage(scheduleReplacementConfig(t, "0 9 * * *", "UTC"))
		require.NoError(t, svc.UpdateTrigger(ctx, trig.ID, &UpdateTriggerRequest{Config: &same}))

		stored, err := svc.store.GetTrigger(ctx, trig.ID)
		require.NoError(t, err)
		require.NotNil(t, stored.LastEvaluatedAt)
		require.True(t, stored.LastEvaluatedAt.Equal(cursor),
			"an unchanged save must keep the pending occurrence")
		require.True(t, cs.shouldFire(stored, now))

		cs.fire(ctx, stored, now)
		requireAutomationRunCount(t, svc, a.ID, 1)
		require.Len(t, *eventsObserved, 1)

		after, err := svc.store.GetTrigger(ctx, trig.ID)
		require.NoError(t, err)
		require.NotNil(t, after.LastEvaluatedAt)
		require.True(t, after.LastEvaluatedAt.Equal(now))
		require.False(t, cs.shouldFire(after, now.Add(30*time.Second)),
			"a single catch-up execution must not repeat on the next tick")
	})
}

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-001.25
// TestCronScheduler_StaleSnapshotAfterEdit verifies an edited schedule rejects its old snapshot.
func TestCronScheduler_StaleSnapshotAfterEdit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		now := time.Date(2026, time.November, 9, 9, 19, 3, 0, time.UTC)
		advanceFakeClock(t, now)
		svc := newTestService(t)
		ctx := context.Background()
		eventsObserved := subscribeTriggeredEvents(t, svc)
		cs := NewCronScheduler(svc, svc.logger)
		a := seedScheduleReplacementAutomation(t, svc)
		oldCursor := time.Date(2026, time.November, 9, 9, 0, 0, 0, time.UTC)
		trig := seedScheduleReplacementTrigger(
			t, svc, a.ID, scheduleReplacementConfig(t, "19 9 * * 1", ""),
			scheduleReplacementCursor(oldCursor),
		)
		snapshot, err := svc.store.GetTrigger(ctx, trig.ID)
		require.NoError(t, err)
		require.True(t, cs.shouldFire(snapshot, now), "the old snapshot is due")

		editAt := now.Add(time.Second)
		advanceFakeClock(t, editAt)
		replacement := json.RawMessage(scheduleReplacementConfig(t, "9 9 * * 6", ""))
		require.NoError(t, svc.UpdateTrigger(ctx, trig.ID, &UpdateTriggerRequest{Config: &replacement}))

		cs.fire(ctx, snapshot, now)
		requireAutomationRunCount(t, svc, a.ID, 0)
		require.Empty(t, *eventsObserved)

		edited, err := svc.store.GetTrigger(ctx, trig.ID)
		require.NoError(t, err)
		require.NotNil(t, edited.LastEvaluatedAt)
		require.True(t, edited.LastEvaluatedAt.Equal(editAt),
			"a rejected observation must not advance the replacement boundary")

		first := time.Date(2026, time.November, 14, 9, 9, 0, 0, time.UTC)
		advanceFakeClock(t, first)
		fresh, err := svc.store.GetTrigger(ctx, trig.ID)
		require.NoError(t, err)
		require.True(t, cs.shouldFire(fresh, first))
		cs.fire(ctx, fresh, first)
		requireAutomationRunCount(t, svc, a.ID, 1)
		require.Len(t, *eventsObserved, 1)
	})
}

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-001.25
// TestCronScheduler_DisabledOrDeletedSnapshot verifies disabled and deleted snapshots have no side effects.
func TestCronScheduler_DisabledOrDeletedSnapshot(t *testing.T) {
	due := time.Date(2026, time.November, 9, 9, 19, 3, 0, time.UTC)
	cases := []struct {
		name   string
		mutate func(t *testing.T, svc *Service, automationID, triggerID string)
	}{
		{
			name: "trigger disabled",
			mutate: func(t *testing.T, svc *Service, _, triggerID string) {
				_, err := svc.store.db.ExecContext(context.Background(),
					`UPDATE automation_triggers SET enabled = FALSE WHERE id = ?`, triggerID)
				require.NoError(t, err)
			},
		},
		{
			name: "automation disabled",
			mutate: func(t *testing.T, svc *Service, automationID, _ string) {
				require.NoError(t, svc.DisableAutomation(context.Background(), automationID))
			},
		},
		{
			name: "trigger deleted",
			mutate: func(t *testing.T, svc *Service, _, triggerID string) {
				require.NoError(t, svc.DeleteTrigger(context.Background(), triggerID))
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				advanceFakeClock(t, due)
				svc := newTestService(t)
				ctx := context.Background()
				eventsObserved := subscribeTriggeredEvents(t, svc)
				cs := NewCronScheduler(svc, svc.logger)
				a := seedScheduleReplacementAutomation(t, svc)
				oldCursor := time.Date(2026, time.November, 9, 9, 0, 0, 0, time.UTC)
				trig := seedScheduleReplacementTrigger(
					t, svc, a.ID, scheduleReplacementConfig(t, "19 9 * * 1", ""),
					scheduleReplacementCursor(oldCursor),
				)
				snapshot, err := svc.store.GetTrigger(ctx, trig.ID)
				require.NoError(t, err)
				require.True(t, cs.shouldFire(snapshot, due))

				tc.mutate(t, svc, a.ID, trig.ID)
				cs.fire(ctx, snapshot, due)

				requireAutomationRunCount(t, svc, a.ID, 0)
				require.Empty(t, *eventsObserved)
				if tc.name == "trigger deleted" {
					deleted, err := svc.store.GetTrigger(ctx, trig.ID)
					require.NoError(t, err)
					require.Nil(t, deleted)
					return
				}
				stored, err := svc.store.GetTrigger(ctx, trig.ID)
				require.NoError(t, err)
				require.NotNil(t, stored.LastEvaluatedAt)
				require.True(t, stored.LastEvaluatedAt.Equal(oldCursor),
					"an obsolete observation must not change evaluation history")
			})
		})
	}
}

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-001.25
// TestCronScheduler_StaleCursorSnapshot verifies a stale cursor cannot admit a second run.
func TestCronScheduler_StaleCursorSnapshot(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		firstAdmission := time.Date(2026, time.November, 9, 9, 0, 5, 0, time.UTC)
		staleAdmission := time.Date(2026, time.November, 9, 9, 1, 0, 0, time.UTC)
		advanceFakeClock(t, firstAdmission)
		svc := newTestService(t)
		ctx := context.Background()
		eventsObserved := subscribeTriggeredEvents(t, svc)
		cs := NewCronScheduler(svc, svc.logger)
		a := seedScheduleReplacementAutomation(t, svc)
		oldCursor := time.Date(2026, time.November, 8, 9, 0, 0, 0, time.UTC)
		trig := seedScheduleReplacementTrigger(
			t, svc, a.ID, scheduleReplacementConfig(t, "0 9 * * *", ""),
			scheduleReplacementCursor(oldCursor),
		)
		staleSnapshot, err := svc.store.GetTrigger(ctx, trig.ID)
		require.NoError(t, err)

		cs.fire(ctx, staleSnapshot, firstAdmission)
		requireAutomationRunCount(t, svc, a.ID, 1)

		advanced, err := svc.store.GetTrigger(ctx, trig.ID)
		require.NoError(t, err)
		require.NotNil(t, advanced.LastEvaluatedAt)
		require.True(t, advanced.LastEvaluatedAt.Equal(firstAdmission))
		require.Equal(t, staleSnapshot.TriggerRevision, advanced.TriggerRevision)

		advanceFakeClock(t, staleAdmission)
		cs.fire(ctx, staleSnapshot, staleAdmission)

		requireAutomationRunCount(t, svc, a.ID, 1)
		require.Len(t, *eventsObserved, 1)
		final, err := svc.store.GetTrigger(ctx, trig.ID)
		require.NoError(t, err)
		require.NotNil(t, final.LastEvaluatedAt)
		require.True(t, final.LastEvaluatedAt.Equal(firstAdmission),
			"a stale cursor observation must not move evaluation history")
	})
}

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-001.27
// TestCronScheduler_EditAdmissionOrdering verifies edit and admission lock ordering.
func TestCronScheduler_EditAdmissionOrdering(t *testing.T) {
	due := time.Date(2026, time.November, 9, 9, 19, 3, 0, time.UTC)

	t.Run("admission wins and the later edit keeps its boundary", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			advanceFakeClock(t, due)
			svc := newTestService(t)
			ctx := context.Background()
			eventsObserved := subscribeTriggeredEvents(t, svc)
			cs := NewCronScheduler(svc, svc.logger)
			a := seedScheduleReplacementAutomation(t, svc)
			trig := seedScheduleReplacementTrigger(
				t, svc, a.ID, scheduleReplacementConfig(t, "19 9 * * 1", ""),
				scheduleReplacementCursor(time.Date(2026, time.November, 9, 9, 0, 0, 0, time.UTC)),
			)
			snapshot, err := svc.store.GetTrigger(ctx, trig.ID)
			require.NoError(t, err)

			cs.fire(ctx, snapshot, due)
			requireAutomationRunCount(t, svc, a.ID, 1)
			require.Len(t, *eventsObserved, 1)

			editAt := due.Add(time.Second)
			advanceFakeClock(t, editAt)
			replacement := json.RawMessage(scheduleReplacementConfig(t, "9 9 * * 6", ""))
			require.NoError(t, svc.UpdateTrigger(ctx, trig.ID, &UpdateTriggerRequest{Config: &replacement}))

			stored, err := svc.store.GetTrigger(ctx, trig.ID)
			require.NoError(t, err)
			require.NotNil(t, stored.LastEvaluatedAt)
			require.True(t, stored.LastEvaluatedAt.Equal(editAt),
				"the later edit must own the replacement boundary")
			requireAutomationRunCount(t, svc, a.ID, 1)
		})
	})

	t.Run("edit wins and the obsolete observation is discarded", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			advanceFakeClock(t, due)
			svc := newTestService(t)
			ctx := context.Background()
			eventsObserved := subscribeTriggeredEvents(t, svc)
			cs := NewCronScheduler(svc, svc.logger)
			a := seedScheduleReplacementAutomation(t, svc)
			trig := seedScheduleReplacementTrigger(
				t, svc, a.ID, scheduleReplacementConfig(t, "19 9 * * 1", ""),
				scheduleReplacementCursor(time.Date(2026, time.November, 9, 9, 0, 0, 0, time.UTC)),
			)
			snapshot, err := svc.store.GetTrigger(ctx, trig.ID)
			require.NoError(t, err)

			editAt := due.Add(time.Second)
			advanceFakeClock(t, editAt)
			replacement := json.RawMessage(scheduleReplacementConfig(t, "9 9 * * 6", ""))
			require.NoError(t, svc.UpdateTrigger(ctx, trig.ID, &UpdateTriggerRequest{Config: &replacement}))
			cs.fire(ctx, snapshot, due)

			requireAutomationRunCount(t, svc, a.ID, 0)
			require.Empty(t, *eventsObserved)
			stored, err := svc.store.GetTrigger(ctx, trig.ID)
			require.NoError(t, err)
			require.NotNil(t, stored.LastEvaluatedAt)
			require.True(t, stored.LastEvaluatedAt.Equal(editAt),
				"a discarded observation must not move the edit boundary")
		})
	})

	t.Run("an edit waits for admission to finish", func(t *testing.T) {
		now := time.Now().UTC()
		svc := newTestService(t)
		svc.store.db.SetMaxOpenConns(1)
		svc.store.ro.SetMaxOpenConns(1)
		ctx := context.Background()
		eventsObserved := subscribeTriggeredEvents(t, svc)
		cs := NewCronScheduler(svc, svc.logger)
		a := seedScheduleReplacementAutomation(t, svc)
		trig := seedScheduleReplacementTrigger(
			t, svc, a.ID, scheduleReplacementConfig(t, "@every 1m", ""),
			scheduleReplacementCursor(now.Add(-5*time.Minute)),
		)
		snapshot, err := svc.store.GetTrigger(ctx, trig.ID)
		require.NoError(t, err)
		require.True(t, cs.shouldFire(snapshot, now))

		// Pause admission after its fresh read but before it writes the run and
		// cursor. The actor-context seam runs while the admission lock is held.
		entered := make(chan struct{})
		release := make(chan struct{})
		svc.SetExecutorContext(func(ctx context.Context, _, _ string) (context.Context, error) {
			close(entered)
			<-release
			return ctx, nil
		})

		fireDone := make(chan struct{})
		go func() {
			cs.fire(ctx, snapshot, now)
			close(fireDone)
		}()
		<-entered

		editStarted := time.Now().UTC()
		editDone := make(chan error, 1)
		go func() {
			replacement := json.RawMessage(scheduleReplacementConfig(t, "@every 2m", ""))
			editDone <- svc.UpdateTrigger(ctx, trig.ID, &UpdateTriggerRequest{Config: &replacement})
		}()

		select {
		case editErr := <-editDone:
			close(release)
			<-fireDone
			t.Fatalf("edit completed while admission held the lock: %v", editErr)
		case <-time.After(500 * time.Millisecond):
		}
		close(release)
		<-fireDone
		require.NoError(t, <-editDone)
		editFinished := time.Now().UTC()

		requireAutomationRunCount(t, svc, a.ID, 1)
		require.Len(t, *eventsObserved, 1)
		stored, err := svc.store.GetTrigger(ctx, trig.ID)
		require.NoError(t, err)
		require.NotNil(t, stored.LastEvaluatedAt)
		require.False(t, stored.LastEvaluatedAt.Before(editStarted),
			"a discarded admission cursor must not precede the waiting edit")
		require.False(t, stored.LastEvaluatedAt.After(editFinished))
		require.Equal(t, scheduleReplacementConfig(t, "@every 2m", ""), string(stored.Config))
	})

	t.Run("edit during post-admission publication does not deadlock", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			advanceFakeClock(t, due)
			svc := newTestService(t)
			ctx := context.Background()
			cs := NewCronScheduler(svc, svc.logger)
			a := seedScheduleReplacementAutomation(t, svc)
			trig := seedScheduleReplacementTrigger(
				t, svc, a.ID, scheduleReplacementConfig(t, "19 9 * * 1", ""),
				scheduleReplacementCursor(time.Date(2026, time.November, 9, 9, 0, 0, 0, time.UTC)),
			)
			snapshot, err := svc.store.GetTrigger(ctx, trig.ID)
			require.NoError(t, err)

			edited := false
			editAt := due.Add(2 * time.Minute)
			_, err = svc.eventBus.Subscribe(events.AutomationTriggered, func(subscriberCtx context.Context, _ *bus.Event) error {
				advanceFakeClock(t, editAt)
				replacement := json.RawMessage(scheduleReplacementConfig(t, "9 9 * * 6", ""))
				if err := svc.UpdateTrigger(subscriberCtx, trig.ID, &UpdateTriggerRequest{Config: &replacement}); err != nil {
					return fmt.Errorf("subscriber edit: %w", err)
				}
				edited = true
				return nil
			})
			require.NoError(t, err)
			t.Cleanup(svc.eventBus.Close)

			cs.fire(ctx, snapshot, due)

			require.True(t, edited, "the publication subscriber must complete the edit")
			requireAutomationRunCount(t, svc, a.ID, 1)
			stored, err := svc.store.GetTrigger(ctx, trig.ID)
			require.NoError(t, err)
			require.NotNil(t, stored.LastEvaluatedAt)
			require.True(t, stored.LastEvaluatedAt.Equal(editAt),
				"the admitted run must not move the later replacement boundary backward")
		})
	})
}
