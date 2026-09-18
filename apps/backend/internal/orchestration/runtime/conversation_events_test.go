package runtime

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/stretchr/testify/require"
)

// @covers AC-ORCHESTRATION-ASSISTANT-001.4, AC-ORCHESTRATION-ASSISTANT-005.1
func TestAssistantAttentionEventsDoNotFinishConversationRun(t *testing.T) {
	for _, subject := range []string{
		events.TaskSessionStateChanged, events.TaskSessionErrorChanged,
		events.TaskStatusSummaryUpdated, events.MessageAdded, events.MessageUpdated,
		events.ClarificationAnswered, events.ClarificationPrimaryAnswered,
		events.ClarificationCancelled, events.ClarificationStaleDismissed,
		events.BuildPermissionRequestWildcardSubject(),
	} {
		t.Run(subject, func(t *testing.T) {
			s, _, task := newRuntime(t)
			ctx := context.Background()
			require.NoError(t, s.QueueTurn(ctx, "chief", task, "task_comment", "example-request", nil))
			run, err := s.Runs.ClaimNextEligibleRun(ctx)
			require.NoError(t, err)
			require.NoError(t, s.Runs.UpdateRunRuntimeSnapshot(ctx, run.ID, "workspace_coordinator", run.Payload, "session"))
			payload := map[string]string{"task_id": task, "session_id": "session", "run_id": run.ID,
				"error_message": "Example provider startup rejected"}
			require.NoError(t, s.onEvent(ctx, bus.NewEvent(subject, "test", payload)))
			current, err := s.Runs.GetRunByID(ctx, run.ID)
			require.NoError(t, err)
			require.EqualValues(t, "claimed", current.Status, "attention-only event ended a live conversation run")
			require.NoError(t, s.onEvent(ctx, bus.NewEvent(events.AgentFailed, "test", payload)))
			current, err = s.Runs.GetRunByID(ctx, run.ID)
			require.NoError(t, err)
			require.EqualValues(t, "failed", current.Status)
			require.Equal(t, "Example provider startup rejected", current.ErrorMessage)
		})
	}
}
