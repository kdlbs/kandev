package statussummary

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/stretchr/testify/require"
)

func TestRunningSessionSummaryMixedSessions(t *testing.T) {
	// @covers AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.14
	for _, testCase := range []struct {
		name     string
		sessions []RebuildSession
		want     bool
	}{
		{name: "known empty", sessions: []RebuildSession{}, want: false},
		{name: "secondary without primary or activity", sessions: []RebuildSession{{ID: "secondary", State: sessionStateRunning}}, want: true},
		{name: "running secondary with waiting primary", sessions: []RebuildSession{
			{ID: "primary", State: sessionStateWaitingForInput, IsPrimary: true},
			{ID: "secondary", State: sessionStateRunning},
		}, want: true},
		{name: "failed and cancelled siblings", sessions: []RebuildSession{
			{ID: "failed", State: "FAILED"},
			{ID: "cancelled", State: "CANCELLED"},
		}, want: false},
		{name: "starting alone", sessions: []RebuildSession{{ID: "starting", State: sessionStateStarting}}, want: false},
		{name: "multiple running sessions", sessions: []RebuildSession{
			{ID: "running-one", State: sessionStateRunning},
			{ID: "running-two", State: sessionStateRunning},
			{ID: "settled", State: "COMPLETED"},
		}, want: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			summary := BuildFromAuthoritative(RebuildInput{
				Sessions:         testCase.sessions,
				SessionsObserved: true,
			})
			assertRunningSummaryJSON(t, summary, &testCase.want)
		})
	}
}

func TestProjectorRunningSessionSummaryLifecycleAndRemoval(t *testing.T) {
	// @covers AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.15
	store := newProjectorTestStore()
	eventBus := bus.NewMemoryEventBus(logger.Default())
	projector := NewProjector(ProjectorConfig{
		Store:            store,
		EventBus:         eventBus,
		ResolveWorkspace: func(context.Context, string) (string, error) { return "workspace-1", nil },
		LoadSessionObservations: func(context.Context, string) (SessionObservationSnapshot, error) {
			return SessionObservationSnapshot{Sessions: []RebuildSession{}}, nil
		},
		Now: func() time.Time { return time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC) },
	})
	require.NoError(t, projector.Start(context.Background()))
	t.Cleanup(projector.Close)
	t.Cleanup(eventBus.Close)

	publishProjectorEvent(t, eventBus, events.TaskSessionStateChanged, events.TaskSessionStateChanged, map[string]interface{}{
		"task_id": "task-running", "workspace_id": "workspace-1", "session_id": "primary",
		"new_state": sessionStateWaitingForInput, "is_primary": true,
	})
	publishProjectorEvent(t, eventBus, events.TaskSessionStateChanged, events.TaskSessionStateChanged, map[string]interface{}{
		"task_id": "task-running", "workspace_id": "workspace-1", "session_id": "secondary",
		"new_state": sessionStateRunning, "is_primary": false,
	})
	assertRunningSummaryJSON(t, *store.summary("task-running"), boolPointer(true))

	publishProjectorEvent(t, eventBus, events.TaskSessionStateChanged, events.TaskSessionStateChanged, map[string]interface{}{
		"task_id": "task-running", "workspace_id": "workspace-1", "session_id": "sibling",
		"new_state": sessionStateRunning, "is_primary": false,
	})
	publishProjectorEvent(t, eventBus, events.SessionRemoved, events.SessionRemoved, map[string]interface{}{
		"task_id": "task-running", "workspace_id": "workspace-1", "session_id": "secondary",
	})
	assertRunningSummaryJSON(t, *store.summary("task-running"), boolPointer(true))

	publishProjectorEvent(t, eventBus, events.SessionRemoved, events.SessionRemoved, map[string]interface{}{
		"task_id": "task-running", "workspace_id": "workspace-1", "session_id": "sibling",
	})
	settled := store.summary("task-running")
	assertRunningSummaryJSON(t, *settled, boolPointer(false))
	revision := settled.Revision
	publishProjectorEvent(t, eventBus, events.SessionRemoved, events.SessionRemoved, map[string]interface{}{
		"task_id": "task-running", "workspace_id": "workspace-1", "session_id": "sibling",
	})
	require.Equal(t, revision, store.summary("task-running").Revision, "duplicate removal must be idempotent")
}

func TestProjectorRehydratesRunningSessionAfterRestart(t *testing.T) {
	// @covers AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.15
	store := newProjectorTestStore()
	store.rows["task-restart-running"] = &StoredTaskStatusSummary{
		TaskID: "task-restart-running", WorkspaceID: "workspace-1",
		Summary: TaskStatusSummary{Revision: 7},
	}
	projector := NewProjector(ProjectorConfig{
		Store:            store,
		ResolveWorkspace: func(context.Context, string) (string, error) { return "workspace-1", nil },
		LoadSessionObservations: func(context.Context, string) (SessionObservationSnapshot, error) {
			return SessionObservationSnapshot{Sessions: []RebuildSession{{
				ID: "secondary", State: sessionStateRunning,
			}}}, nil
		},
		Now: func() time.Time { return time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC) },
	})
	err := projector.HandleEvent(context.Background(), bus.NewEvent(events.TaskUpdated, "test", map[string]interface{}{
		"task_id": "task-restart-running", "workspace_id": "workspace-1",
		"updated_at": "2026-10-01T12:01:00Z",
	}))
	require.NoError(t, err)
	got := store.summary("task-restart-running")
	require.NotNil(t, got)
	assertRunningSummaryJSON(t, *got, boolPointer(true))
}

func assertRunningSummaryJSON(t *testing.T, summary TaskStatusSummary, want *bool) {
	t.Helper()
	encoded, err := summary.SemanticJSON()
	require.NoError(t, err)
	var payload map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &payload))
	value, present := payload["has_running_session"]
	if want == nil {
		require.False(t, present, "unknown session observations must remain absent")
		return
	}
	require.True(t, present, "a complete session observation must publish an explicit boolean")
	var got bool
	require.NoError(t, json.Unmarshal(value, &got))
	require.Equal(t, *want, got)
}

func boolPointer(value bool) *bool { return &value }
