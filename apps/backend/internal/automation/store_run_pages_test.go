package automation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestListRunPageReturnsOlderStandaloneRunsByStableCursor(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	automation := &Automation{ID: "run-page-automation", WorkspaceID: "run-page-workspace", Name: "run pages", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, automation))
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for index, id := range []string{"run-old", "run-middle", "run-new"} {
		run := &AutomationRun{
			ID: id, AutomationID: automation.ID, TriggerType: TriggerTypeManual,
			Status: RunStatusSucceeded, CreatedAt: base.Add(time.Duration(index) * time.Minute),
		}
		require.NoError(t, store.CreateRun(ctx, run))
	}

	first, err := store.ListRunPage(ctx, automation.ID, "", 2)
	require.NoError(t, err)
	require.Equal(t, []string{"run-new", "run-middle"}, runIDs(first.Items))
	require.NotEmpty(t, first.NextCursor)

	second, err := store.ListRunPage(ctx, automation.ID, first.NextCursor, 2)
	require.NoError(t, err)
	require.Equal(t, []string{"run-old"}, runIDs(second.Items))
	require.Empty(t, second.NextCursor)
}

func runIDs(runs []*AutomationRun) []string {
	ids := make([]string, len(runs))
	for index, run := range runs {
		ids[index] = run.ID
	}
	return ids
}
