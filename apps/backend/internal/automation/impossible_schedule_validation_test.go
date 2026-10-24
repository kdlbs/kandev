package automation

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kdlbs/kandev/pluginsdk"
)

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-001.16
func TestScheduledConfig_NoOccurrenceRejectedWithoutMutation(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		for _, route := range []string{"create", "add", "update"} {
			t.Run(route+"/enabled="+strconv.FormatBool(enabled), func(t *testing.T) {
				svc := newTestService(t)
				t.Cleanup(svc.eventBus.Close)
				cfg := json.RawMessage(`{"cron_expression":"0 0 31 2 *","timezone":"UTC"}`)
				switch route {
				case "create":
					assertImpossibleScheduleCreateRejected(t, svc, cfg, enabled)
				case "add":
					assertImpossibleScheduleAddRejected(t, svc, cfg, enabled)
				case "update":
					assertImpossibleScheduleUpdateRejected(t, svc, cfg, enabled)
				}
			})
		}
	}
}

func assertImpossibleScheduleCreateRejected(t *testing.T, svc *Service, cfg json.RawMessage, enabled bool) {
	t.Helper()
	ctx := context.Background()
	_, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		WorkspaceID: "calendar-create", Name: "Calendar report",
		Triggers: []CreateTriggerSpec{
			{Type: TriggerTypeScheduled, Config: json.RawMessage(`{"cron_expression":"@daily"}`), Enabled: true},
			{Type: TriggerTypeScheduled, Config: cfg, Enabled: enabled},
		},
	})
	require.Error(t, err, "an impossible initial schedule must reject the whole request")
	automations, err := svc.store.ListAutomations(ctx, "calendar-create")
	require.NoError(t, err)
	require.Empty(t, automations, "rejected creation must not persist an automation")
	var count int
	require.NoError(t, svc.store.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM automation_triggers`))
	require.Zero(t, count, "rejected creation must not persist any initial trigger")
}

func assertImpossibleScheduleAddRejected(t *testing.T, svc *Service, cfg json.RawMessage, enabled bool) {
	t.Helper()
	ctx := context.Background()
	a, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{WorkspaceID: "calendar-add", Name: "Calendar report"})
	require.NoError(t, err)
	_, err = svc.AddTrigger(ctx, &AddTriggerRequest{
		AutomationID: a.ID, Type: TriggerTypeScheduled, Config: cfg, Enabled: enabled,
	})
	require.Error(t, err, "an impossible schedule must not be added")
	reloaded, err := svc.store.GetAutomation(ctx, a.ID)
	require.NoError(t, err)
	require.Equal(t, a, reloaded, "rejected addition must leave the stored automation unchanged")
}

func assertImpossibleScheduleUpdateRejected(t *testing.T, svc *Service, cfg json.RawMessage, enabled bool) {
	t.Helper()
	ctx := context.Background()
	a, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{WorkspaceID: "calendar-update", Name: "Calendar report"})
	require.NoError(t, err)
	trig, err := svc.AddTrigger(ctx, &AddTriggerRequest{
		AutomationID: a.ID, Type: TriggerTypeScheduled,
		Config: json.RawMessage(`{"cron_expression":"@daily"}`), Enabled: !enabled,
	})
	require.NoError(t, err)
	require.NoError(t, svc.store.UpdateTriggerEvaluatedAt(ctx, trig.ID, time.Date(2026, time.October, 13, 8, 0, 0, 0, time.UTC)))
	before, err := svc.store.GetTrigger(ctx, trig.ID)
	require.NoError(t, err)
	err = svc.UpdateTrigger(ctx, trig.ID, &UpdateTriggerRequest{Config: &cfg, Enabled: &enabled})
	require.Error(t, err, "an impossible config must reject all requested changes")
	after, err := svc.store.GetTrigger(ctx, trig.ID)
	require.NoError(t, err)
	require.Equal(t, before, after, "config, enabled, revision, cursor, and timestamps must survive rejection")
}

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-001.16, AC-OFFICE-AUTOMATIONS-SETTINGS-001.18
func TestScheduledConfig_CompatibleSchedulesAndEmptyConfig(t *testing.T) {
	cases := []struct {
		name  string
		cfg   json.RawMessage
		empty bool
	}{
		{"leap day", json.RawMessage(`{"cron_expression":"0 0 29 2 *","timezone":"UTC"}`), false},
		{"DOM DOW OR", json.RawMessage(`{"cron_expression":"0 0 31 2 MON","timezone":"UTC"}`), false},
		{"descriptor", json.RawMessage(`{"cron_expression":"@daily"}`), false},
		{"interval", json.RawMessage(`{"cron_expression":"@every 2h30m"}`), false},
		{"timezone", json.RawMessage(`{"cron_expression":"0 9 * * *","timezone":"America/New_York"}`), false},
		{"empty expression", json.RawMessage(`{"cron_expression":""}`), true},
		{"whitespace expression", json.RawMessage(`{"cron_expression":"   "}`), true},
		{"object", json.RawMessage(`{}`), true},
		{"null", json.RawMessage(`null`), true},
		{"absent", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t)
			t.Cleanup(svc.eventBus.Close)
			ctx := context.Background()
			a, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
				WorkspaceID: "calendar-controls", Name: "Calendar report",
				Triggers: []CreateTriggerSpec{{Type: TriggerTypeScheduled, Config: tc.cfg, Enabled: true}},
			})
			require.NoError(t, err)
			trig, err := svc.AddTrigger(ctx, &AddTriggerRequest{
				AutomationID: a.ID, Type: TriggerTypeScheduled, Config: tc.cfg, Enabled: true,
			})
			require.NoError(t, err)
			require.NoError(t, svc.UpdateTrigger(ctx, trig.ID, &UpdateTriggerRequest{Config: &tc.cfg}))
			if tc.empty {
				triggers, readErr := svc.store.ListTriggers(ctx, a.ID)
				require.NoError(t, readErr)
				cs := NewCronScheduler(svc, svc.logger)
				for i := range triggers {
					require.False(t, cs.shouldFire(&triggers[i], triggers[i].CreatedAt.Add(24*time.Hour)))
				}
			}
		})
	}
}

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-001.16
func TestManagedScheduleTriggerSpecs_RejectsNoOccurrence(t *testing.T) {
	_, err := managedScheduleTriggerSpecs(pluginsdk.ManagedConversationSchedule{
		Triggers: []pluginsdk.ManagedConversationScheduleTrigger{{
			Type: string(TriggerTypeScheduled), Enabled: true,
			Config: json.RawMessage(`{"cron_expression":"0 0 31 2 *","timezone":"UTC"}`),
		}},
	})
	require.ErrorIs(t, err, ErrManagedScheduleInvalid)
}

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-001.17
func TestScheduledConfig_PersistedImpossibleScheduleCanBeDisabledAndRepaired(t *testing.T) {
	svc := newTestService(t)
	t.Cleanup(svc.eventBus.Close)
	ctx := context.Background()
	a, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{WorkspaceID: "calendar-repair", Name: "Calendar report"})
	require.NoError(t, err)
	trig := &AutomationTrigger{
		AutomationID: a.ID, Type: TriggerTypeScheduled, Enabled: true,
		Config: json.RawMessage(`{"cron_expression":"0 0 31 2 *"}`),
	}
	require.NoError(t, svc.store.CreateTrigger(ctx, trig))
	disabled := false
	require.NoError(t, svc.UpdateTrigger(ctx, trig.ID, &UpdateTriggerRequest{Enabled: &disabled}))
	stored, err := svc.store.GetTrigger(ctx, trig.ID)
	require.NoError(t, err)
	require.False(t, stored.Enabled)
	require.JSONEq(t, string(trig.Config), string(stored.Config))
	valid := json.RawMessage(`{"cron_expression":"@daily"}`)
	enabled := true
	require.NoError(t, svc.UpdateTrigger(ctx, trig.ID, &UpdateTriggerRequest{Config: &valid, Enabled: &enabled}))
	stored, err = svc.store.GetTrigger(ctx, trig.ID)
	require.NoError(t, err)
	require.True(t, stored.Enabled)
	require.JSONEq(t, string(valid), string(stored.Config))
}
