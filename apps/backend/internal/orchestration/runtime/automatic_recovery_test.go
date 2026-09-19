package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

const cancelledRunStatus = "cancelled"

const refreshContention = `{"code":-32603,"message":"Internal error: Failed to refresh OAuth token: another Claude Code process is refreshing it or exited mid-refresh. This is usually transient; retry in a minute"}`

func TestConversationAutomaticallyRetriesBeforeResult(t *testing.T) {
	s, _, task := newRuntime(t)
	ctx := context.Background()
	require.NoError(t, s.QueueTurn(ctx, "chief", task, "task_comment", "generic-example", nil))
	run, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	require.NoError(t, s.Runs.UpdateRunRuntimeSnapshot(ctx, run.ID, "workspace_coordinator", run.Payload, "session"))
	data := map[string]any{"task_id": task, "session_id": "session", "run_id": run.ID, "agent_id": "claude-acp", "agent_execution_id": "execution", "prompt_generation": 7, "evidence_known": true, "error_message": refreshContention}
	require.NoError(t, s.onEvent(ctx, bus.NewEvent(events.AgentFailed, "test", data)))
	current, err := s.Runs.GetRunByID(ctx, run.ID)
	require.NoError(t, err)
	require.EqualValues(t, "queued", current.Status)
	require.Equal(t, 1, current.RetryCount)
	require.NotNil(t, current.ScheduledRetryAt)
	require.True(t, current.ScheduledRetryAt.After(time.Now()))
	// Duplicate delivery must not spend another attempt.
	require.NoError(t, s.onEvent(ctx, bus.NewEvent(events.AgentFailed, "test", data)))
	current, err = s.Runs.GetRunByID(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, 1, current.RetryCount)
}

func TestConversationRecoveryFencesUnsafeAndExhaustedTurns(t *testing.T) {
	for _, name := range []string{"output", "effect", "unknown", "no_generation", "no_execution", "exhausted", "permanent_auth", cancelledRunStatus, "stale_session", "stale_run"} {
		t.Run(name, func(t *testing.T) {
			s, db, task := newRuntime(t)
			ctx := context.Background()
			require.NoError(t, s.QueueTurn(ctx, "chief", task, "task_comment", "example", nil))
			run, err := s.Runs.ClaimNextEligibleRun(ctx)
			require.NoError(t, err)
			require.NoError(t, s.Runs.UpdateRunRuntimeSnapshot(ctx, run.ID, "workspace_coordinator", run.Payload, "session"))
			data := map[string]any{"task_id": task, "session_id": "session", "run_id": run.ID, "agent_id": "claude-acp", "agent_execution_id": "execution", "prompt_generation": 7, "evidence_known": true, "error_message": refreshContention}
			expected := "failed"
			switch name {
			case "output":
				data["output_observed"] = true
			case "effect":
				data["effect_observed"] = true
			case "unknown":
				data["evidence_known"] = false
			case "no_generation":
				data["prompt_generation"] = 0
			case "no_execution":
				data["agent_execution_id"] = ""
			case "exhausted":
				_, err = db.Exec(`UPDATE runs SET retry_count=5 WHERE id=?`, run.ID)
				require.NoError(t, err)
			case "permanent_auth":
				data["error_message"] = "Invalid API key; please log in"
			case cancelledRunStatus:
				require.NoError(t, s.Runs.FinishRun(ctx, run.ID, cancelledRunStatus, nil))
				expected = cancelledRunStatus
			case "stale_session":
				data["session_id"] = "old"
				expected = "claimed"
			case "stale_run":
				data["run_id"] = "old"
				expected = "claimed"
			}
			require.NoError(t, s.onEvent(ctx, bus.NewEvent(events.AgentFailed, "test", data)))
			current, err := s.Runs.GetRunByID(ctx, run.ID)
			require.NoError(t, err)
			require.EqualValues(t, expected, current.Status)
			require.Nil(t, current.ScheduledRetryAt)
		})
	}
}

func TestAutomaticRecoveryRelaunchesWithFreshAuthority(t *testing.T) {
	s, db, task := newRuntime(t)
	ctx := context.Background()
	s.FailureHandlerInstalled = true
	var tokens []string
	var cleared []string
	s.RecoveryStarting = func(_ context.Context, session string) { cleared = append(cleared, session) }
	s.Start = func(ctx context.Context, l Launch) error {
		require.NoError(t, l.OnSessionPrepared(ctx, fmt.Sprintf("session-%d", len(tokens))))
		tokens = append(tokens, l.Env["KANDEV_RUN_TOKEN"])
		return nil
	}
	require.NoError(t, s.QueueTurn(ctx, "chief", task, "task_comment", "example", nil))
	run, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	_, err = s.Process(ctx, run)
	require.NoError(t, err)
	failure := watcher.AgentEventData{RunID: run.ID, TaskID: task, SessionID: "session-0", AgentID: "claude-acp", AgentExecutionID: "execution", PromptGeneration: 7, EvidenceKnown: true, ErrorMessage: refreshContention}
	// Raw event delivery cannot finish/revoke the managed run before recovery.
	require.NoError(t, s.onEvent(ctx, bus.NewEvent(events.AgentFailed, "test", failure)))
	current, err := s.Runs.GetRunByID(ctx, run.ID)
	require.NoError(t, err)
	require.EqualValues(t, "claimed", current.Status)
	attempt, _, err := s.HandleFailure(ctx, failure)
	require.NoError(t, err)
	require.Equal(t, 1, attempt)
	_, err = db.Exec(`UPDATE runs SET scheduled_retry_at=? WHERE id=?`, time.Now().Add(-time.Minute), run.ID)
	require.NoError(t, err)
	retried, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	require.Equal(t, run.ID, retried.ID)
	_, err = s.Process(ctx, retried)
	require.NoError(t, err)
	require.Equal(t, []string{"session-0"}, cleared)
	require.Len(t, tokens, 2)
	require.NotEmpty(t, tokens[0])
	require.NotEqual(t, tokens[0], tokens[1])
	// The previous execution cannot finish the relaunched request.
	require.NoError(t, s.onEvent(ctx, bus.NewEvent(events.AgentStopped, "test", failure)))
	current, err = s.Runs.GetRunByID(ctx, run.ID)
	require.NoError(t, err)
	require.EqualValues(t, "claimed", current.Status)
	require.NoError(t, s.onEvent(ctx, bus.NewEvent(events.AgentCompleted, "test", map[string]string{"task_id": task, "session_id": "session-1", "run_id": run.ID})))
	current, err = s.Runs.GetRunByID(ctx, run.ID)
	require.NoError(t, err)
	require.EqualValues(t, "finished", current.Status)
}

func TestQueuedConversationRecoveryCanBeCancelled(t *testing.T) {
	s, _, task := newRuntime(t)
	ctx := context.Background()
	require.NoError(t, s.QueueTurn(ctx, "chief", task, "task_comment", "example", nil))
	run, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	require.NoError(t, s.Runs.UpdateRunRuntimeSnapshot(ctx, run.ID, "workspace_coordinator", run.Payload, "session"))
	ok, err := s.Runs.RetryClaimedSession(ctx, run.ID, "session", 0, time.Now().Add(time.Minute))
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, s.CancelRecovery(ctx, task, "session"))
	ok, err = s.Runs.RetryClaimedSession(ctx, run.ID, "session", 1, time.Now().Add(time.Minute))
	require.NoError(t, err)
	require.False(t, ok)
	current, err := s.Runs.GetRunByID(ctx, run.ID)
	require.NoError(t, err)
	require.EqualValues(t, cancelledRunStatus, current.Status)
}

func TestConversationLaterTurnCannotOvertakeRecovery(t *testing.T) {
	s, _, task := newRuntime(t)
	ctx := context.Background()
	require.NoError(t, s.QueueTurn(ctx, "chief", task, "task_comment", "first", nil))
	run, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	require.NoError(t, s.Runs.UpdateRunRuntimeSnapshot(ctx, run.ID, "workspace_coordinator", run.Payload, "session"))
	ok, err := s.Runs.RetryClaimedSession(ctx, run.ID, "session", 0, time.Now().Add(time.Minute))
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, s.QueueTurn(ctx, "chief", task, "task_comment", "second", nil))
	_, err = s.Runs.ClaimNextEligibleRun(ctx)
	require.ErrorIs(t, err, sql.ErrNoRows)
}
