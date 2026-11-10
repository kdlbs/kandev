package automation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/testutil"
)

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-001.18, AC-OFFICE-AUTOMATIONS-SETTINGS-001.20, AC-OFFICE-AUTOMATIONS-SETTINGS-001.21
func TestScheduledTriggerUpdate_PostgresParity(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	store, err := NewStore(database, database)
	require.NoError(t, err)
	ctx := context.Background()

	a := &Automation{
		ID:                "pg-schedule-replacement",
		WorkspaceID:       "ws-pg",
		Name:              "PostgreSQL schedule boundary",
		WorkflowID:        "wf-pg",
		WorkflowStepID:    "step-pg",
		AgentProfileID:    "agent-pg",
		ExecutorProfileID: "executor-pg",
		Enabled:           true,
	}
	require.NoError(t, store.CreateAutomation(ctx, a))

	historical := time.Date(2026, time.November, 7, 9, 0, 0, 0, time.UTC)

	t.Run("changed definition with nil cursor", func(t *testing.T) {
		trig := &AutomationTrigger{
			ID:           "pg-trigger-nil-cursor",
			AutomationID: a.ID,
			Type:         TriggerTypeScheduled,
			Config:       json.RawMessage(`{"cron_expression":"0 9 * * 6"}`),
			Enabled:      true,
		}
		require.NoError(t, store.CreateTrigger(ctx, trig))
		replacement := json.RawMessage(`{"cron_expression":"9 9 * * 6"}`)

		before := time.Now().UTC()
		require.NoError(t, store.UpdateTrigger(ctx, trig.ID, &UpdateTriggerRequest{Config: &replacement}))
		after := time.Now().UTC()

		stored, err := store.GetTrigger(ctx, trig.ID)
		require.NoError(t, err)
		require.NotNil(t, stored.LastEvaluatedAt)
		require.False(t, stored.LastEvaluatedAt.Before(before))
		require.False(t, stored.LastEvaluatedAt.After(after))
		require.True(t, stored.LastEvaluatedAt.Equal(stored.UpdatedAt))
		require.Equal(t, int64(2), stored.TriggerRevision)
	})

	t.Run("changed definition with historical cursor", func(t *testing.T) {
		trig := &AutomationTrigger{
			ID:              "pg-trigger-historical-cursor",
			AutomationID:    a.ID,
			Type:            TriggerTypeScheduled,
			Config:          json.RawMessage(`{"cron_expression":"0 9 * * 6"}`),
			Enabled:         true,
			LastEvaluatedAt: &historical,
		}
		require.NoError(t, store.CreateTrigger(ctx, trig))
		replacement := json.RawMessage(`{"cron_expression":"15 10 * * 1"}`)

		before := time.Now().UTC()
		require.NoError(t, store.UpdateTrigger(ctx, trig.ID, &UpdateTriggerRequest{Config: &replacement}))
		after := time.Now().UTC()

		stored, err := store.GetTrigger(ctx, trig.ID)
		require.NoError(t, err)
		require.NotNil(t, stored.LastEvaluatedAt)
		require.True(t, stored.LastEvaluatedAt.After(historical))
		require.False(t, stored.LastEvaluatedAt.Before(before))
		require.False(t, stored.LastEvaluatedAt.After(after))
		require.True(t, stored.LastEvaluatedAt.Equal(stored.UpdatedAt))
	})

	t.Run("unchanged definition keeps cursor", func(t *testing.T) {
		trig := &AutomationTrigger{
			ID:              "pg-trigger-unchanged",
			AutomationID:    a.ID,
			Type:            TriggerTypeScheduled,
			Config:          json.RawMessage(`{"cron_expression":"0 9 * * *","timezone":"UTC"}`),
			Enabled:         true,
			LastEvaluatedAt: &historical,
		}
		require.NoError(t, store.CreateTrigger(ctx, trig))
		sameDefinition := json.RawMessage(`{"timezone":"UTC","cron_expression":"  0 9 * * *  "}`)

		require.NoError(t, store.UpdateTrigger(ctx, trig.ID, &UpdateTriggerRequest{Config: &sameDefinition}))

		stored, err := store.GetTrigger(ctx, trig.ID)
		require.NoError(t, err)
		require.NotNil(t, stored.LastEvaluatedAt)
		require.True(t, stored.LastEvaluatedAt.Equal(historical))
		require.Equal(t, int64(2), stored.TriggerRevision)
	})
}
