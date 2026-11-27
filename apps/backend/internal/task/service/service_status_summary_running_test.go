package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/statussummary"
	"github.com/stretchr/testify/require"
)

func TestReconcileTaskStatusSummariesRunningSessions(t *testing.T) {
	// @covers AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.7, AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.15
	svc, _, repo := createTestService(t)
	svc.statusSummaries = repo
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)
	activityAt := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	legacy := statussummary.TaskStatusSummary{
		Revision:       1,
		LastActivityAt: &activityAt,
		PrimarySession: &statussummary.PrimarySessionSummary{ID: "primary", State: "WAITING_FOR_INPUT"},
	}
	_, err := repo.CompareAndUpdateTaskStatusSummary(ctx, &statussummary.StoredTaskStatusSummary{
		TaskID: "task-1", WorkspaceID: "ws-1", Summary: legacy,
	})
	require.NoError(t, err)
	stored, err := svc.GetTaskStatusSummaries(ctx, []string{"task-1"})
	require.NoError(t, err)

	sessions := map[string][]*models.TaskSession{"task-1": {
		{ID: "primary", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput, IsPrimary: true},
		{ID: "secondary", TaskID: "task-1", State: models.TaskSessionStateRunning},
	}}
	got, err := svc.ReconcileTaskStatusSummaries(ctx, []*models.Task{{ID: "task-1", WorkspaceID: "ws-1"}}, sessions, nil, stored)
	require.NoError(t, err)
	require.NotNil(t, got["task-1"])
	require.Equal(t, uint64(2), got["task-1"].Revision)
	require.Equal(t, activityAt, *got["task-1"].LastActivityAt,
		"projection repair must not advance semantic task activity")
	assertServiceRunningSummaryJSON(t, *got["task-1"], true)
}

func assertServiceRunningSummaryJSON(t *testing.T, summary statussummary.TaskStatusSummary, want bool) {
	t.Helper()
	encoded, err := summary.SemanticJSON()
	require.NoError(t, err)
	var payload map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &payload))
	var got bool
	require.NotEmpty(t, payload["has_running_session"], "summary must include the task-wide running value")
	require.NoError(t, json.Unmarshal(payload["has_running_session"], &got))
	require.Equal(t, want, got)
}
