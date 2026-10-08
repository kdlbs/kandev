package statussummary

import (
	"context"
	"encoding/json"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func TestExecutorFailureSummarySurvivesSerializationIndependently(t *testing.T) {
	raw := []byte(`{"executor_failure":{"id":"episode","task_id":"task","revision":2,"state":"active","first_observed_at":"2026-09-30T00:00:00Z","last_observed_at":"2026-09-30T00:00:00Z","observation":{"outcome":"terminated","runtime":"k8s","observed_at":"2026-09-30T00:00:00Z","reason":"Evicted","workspace":"retained"}},"task_error":{"stamp":"launch","occurred_at":"2026-09-30T00:00:00Z","preview":"launch problem"}}`)
	var summary TaskStatusSummary
	require.NoError(t, json.Unmarshal(raw, &summary))
	encoded, err := summary.SemanticJSON()
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"executor_failure"`)
	require.Contains(t, string(encoded), `"Evicted"`)
	require.Contains(t, string(encoded), `"task_error"`)
	var other TaskStatusSummary
	require.NoError(t, json.Unmarshal([]byte(`{"task_error":{"stamp":"launch","occurred_at":"2026-09-30T00:00:00Z","preview":"launch problem"}}`), &other))
	require.False(t, summary.SemanticEqual(other))
}

func TestExecutorFailureRebuildAndProjectorResolution(t *testing.T) {
	now := time.Now().UTC()
	episode := &models.ExecutorFailureEpisode{ID: "episode", TaskID: "task", Revision: 1, State: "active", FirstObservedAt: now, LastObservedAt: now, Observation: &models.ExecutorObservation{Outcome: "terminated", ObservedAt: now, Reason: "Evicted", Workspace: "retained"}}
	rebuilt := BuildFromAuthoritative(RebuildInput{ExecutorFailure: episode, Now: now})
	require.NotNil(t, rebuilt.ExecutorFailure)
	store := newProjectorTestStore()
	projector := NewProjector(ProjectorConfig{Store: store, ResolveWorkspace: func(context.Context, string) (string, error) { return "workspace", nil }, LoadExecutorFailure: func(context.Context, string) (*models.ExecutorFailureEpisode, error) { return episode, nil }})
	event := &bus.Event{Type: events.TaskUpdated, Data: map[string]any{"task_id": "task", "workspace_id": "workspace"}}
	require.NoError(t, projector.HandleEvent(t.Context(), event))
	got := store.summary("task")
	require.NotNil(t, got)
	require.NotNil(t, got.ExecutorFailure)
	require.Equal(t, "active", got.ExecutorFailure.State)
	episode.State = "resolved"
	episode.Revision = 2
	episode.ResolvedAt = &now
	require.NoError(t, projector.HandleEvent(t.Context(), event))
	got = store.summary("task")
	require.Equal(t, "resolved", got.ExecutorFailure.State)
	require.EqualValues(t, 2, got.ExecutorFailure.Revision)
}

func TestExecutorFailureSummaryRejectsUnboundedEvidence(t *testing.T) {
	now := time.Now().UTC()
	episode := &models.ExecutorFailureEpisode{ID: "episode", TaskID: "task", Revision: 1, State: "active", FirstObservedAt: now, LastObservedAt: now, Observation: &models.ExecutorObservation{Outcome: "terminated", ObservedAt: now, Reason: "Evicted", Message: strings.Repeat("x", 769)}}
	summary := TaskStatusSummary{ExecutorFailure: episode}
	require.Error(t, summary.Validate(), "failure diagnostics must respect the task-row transport budget")
}

func TestExecutorFailureProjectionAvoidsUnrelatedEventReads(t *testing.T) {
	reads := 0
	projector := NewProjector(ProjectorConfig{Store: newProjectorTestStore(), ResolveWorkspace: func(context.Context, string) (string, error) { return "workspace", nil }, LoadExecutorFailure: func(context.Context, string) (*models.ExecutorFailureEpisode, error) {
		reads++
		now := time.Now().UTC()
		return &models.ExecutorFailureEpisode{ID: "episode", TaskID: "task", Revision: 1, State: "active", FirstObservedAt: now, LastObservedAt: now, Observation: &models.ExecutorObservation{Outcome: "terminated", ObservedAt: now, Workspace: "unknown"}}, nil
	}})
	data := map[string]any{"task_id": "task", "workspace_id": "workspace"}
	require.NoError(t, projector.HandleEvent(t.Context(), &bus.Event{Type: events.TaskUpdated, Data: data}))
	require.Equal(t, 1, reads)
	require.NoError(t, projector.HandleEvent(t.Context(), &bus.Event{Type: events.TaskStateChanged, Data: data}))
	require.Equal(t, 1, reads, "task state events do not change executor episodes")
	require.NoError(t, projector.HandleEvent(t.Context(), &bus.Event{Type: events.TaskUpdated, Data: data}))
	require.Equal(t, 2, reads, "episode mutation notifications must reload durable evidence")
}
