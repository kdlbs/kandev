package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/service"
	runsmodels "github.com/kandev/kandev/internal/runs/models"
)

func parkCeilingDeferredRun(t *testing.T, svc *service.Service) *runsmodels.Run {
	t.Helper()
	run := &runsmodels.Run{
		AgentProfileID: "missing-agent",
		Reason:         "task_assigned",
		Payload:        `{"task_id":"t-cap"}`,
		Status:         "queued",
		CoalescedCount: 1,
	}
	require.NoError(t, svc.RepoForTest().CreateRun(context.Background(), run))
	require.NoError(t, svc.RepoForTest().ParkRunForProviderCapacity(
		context.Background(), run.ID, string(runsmodels.RoutingBlockedActionRequired), time.Time{},
	))
	svc.AppendRunEvent(context.Background(), run.ID, "adapter.invoke", "info", map[string]interface{}{
		"phase":  "deferred",
		"reason": "session_ceiling",
	})
	return run
}

func TestCeilingDeferredLaunchReplayedSettlesParkedRun(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	run := parkCeilingDeferredRun(t, svc)

	require.NoError(t, svc.CeilingDeferredLaunchReplayed(ctx, "t-cap", run.ID, "session-1", "turn-1"))

	got, err := svc.RepoForTest().GetRunByID(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, service.RunStatusClaimed, string(got.Status))
	require.Equal(t, "session-1", got.SessionID)
	require.Nil(t, got.RoutingBlockedStatus)
	require.NotNil(t, got.ClaimedAt)

	// A duplicate replay leaves the first binding intact.
	require.NoError(t, svc.CeilingDeferredLaunchReplayed(ctx, "t-cap", run.ID, "session-2", "turn-2"))
	got, err = svc.RepoForTest().GetRunByID(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, "session-1", got.SessionID)
}

func TestAgentReadyRecoversCeilingDeferredRunSettlement(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	run := parkCeilingDeferredRun(t, svc)

	require.NoError(t, eb.Publish(ctx, events.AgentReady, bus.NewEvent(events.AgentReady, "test", lifecycle.AgentEventPayload{
		TaskID:         "t-cap",
		SessionID:      "session-1",
		AgentProfileID: "missing-agent",
		CausingRunID:   run.ID,
	})))

	got, err := svc.RepoForTest().GetRunByID(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, service.RunStatusClaimed, string(got.Status))
	require.Equal(t, "session-1", got.SessionID)
	require.Nil(t, got.RoutingBlockedStatus)
}

func TestAgentReadyMismatchedTaskDoesNotSettleCeilingDeferredRun(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	run := parkCeilingDeferredRun(t, svc)

	require.NoError(t, eb.Publish(ctx, events.AgentReady, bus.NewEvent(events.AgentReady, "test", lifecycle.AgentEventPayload{
		TaskID:         "other-task",
		SessionID:      "session-1",
		AgentProfileID: "missing-agent",
		CausingRunID:   run.ID,
	})))

	got, err := svc.RepoForTest().GetRunByID(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, service.RunStatusQueued, string(got.Status))
	require.NotNil(t, got.RoutingBlockedStatus)
	require.Empty(t, got.SessionID)
}

func TestAgentReadyMismatchedAgentDoesNotSettleCeilingDeferredRun(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	run := parkCeilingDeferredRun(t, svc)

	require.NoError(t, eb.Publish(ctx, events.AgentReady, bus.NewEvent(events.AgentReady, "test", lifecycle.AgentEventPayload{
		TaskID:         "t-cap",
		SessionID:      "session-1",
		AgentProfileID: "other-agent",
		CausingRunID:   run.ID,
	})))

	got, err := svc.RepoForTest().GetRunByID(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, service.RunStatusQueued, string(got.Status))
	require.NotNil(t, got.RoutingBlockedStatus)
	require.Empty(t, got.SessionID)
}

func TestAgentReadyWithoutCeilingDeferralMarkerDoesNotSettle(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	run := &runsmodels.Run{
		AgentProfileID: "missing-agent",
		Reason:         "task_assigned",
		Payload:        `{"task_id":"t-cap"}`,
		Status:         "queued",
		CoalescedCount: 1,
	}
	require.NoError(t, svc.RepoForTest().CreateRun(ctx, run))
	require.NoError(t, svc.RepoForTest().ParkRunForProviderCapacity(
		ctx, run.ID, string(runsmodels.RoutingBlockedActionRequired), time.Time{},
	))

	// No ceiling-deferral marker was recorded: a normal task-owned ready event
	// must not release this generic action-required park.
	require.NoError(t, eb.Publish(ctx, events.AgentReady, bus.NewEvent(events.AgentReady, "test", lifecycle.AgentEventPayload{
		TaskID:         "t-cap",
		SessionID:      "session-1",
		AgentProfileID: "missing-agent",
		CausingRunID:   run.ID,
	})))

	got, err := svc.RepoForTest().GetRunByID(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, service.RunStatusQueued, string(got.Status))
	require.NotNil(t, got.RoutingBlockedStatus)
	require.Empty(t, got.SessionID)
}

func TestAgentReadyWithoutCausingRunLeavesCeilingDeferredRunParked(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	run := parkCeilingDeferredRun(t, svc)

	require.NoError(t, eb.Publish(ctx, events.AgentReady, bus.NewEvent(events.AgentReady, "test", lifecycle.AgentEventPayload{
		TaskID:         "t-cap",
		SessionID:      "session-1",
		AgentProfileID: "missing-agent",
	})))

	got, err := svc.RepoForTest().GetRunByID(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, service.RunStatusQueued, string(got.Status))
	require.NotNil(t, got.RoutingBlockedStatus)
	require.Empty(t, got.SessionID)
}

func TestAgentStoppedBeforeReadyRecoveryFinishesCeilingDeferredRun(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	run := parkCeilingDeferredRun(t, svc)

	// Completion arrives before the ready recovery: the run is still parked.
	require.NoError(t, eb.Publish(ctx, events.AgentStopped, bus.NewEvent(events.AgentStopped, "test", lifecycle.AgentEventPayload{
		TaskID:               "t-cap",
		SessionID:            "session-1",
		AgentProfileID:       "missing-agent",
		TerminalCausingRunID: run.ID,
	})))

	got, err := svc.RepoForTest().GetRunByID(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, service.RunStatusFinished, string(got.Status))
	require.Equal(t, "session-1", got.SessionID)
	require.Nil(t, got.RoutingBlockedStatus)
	require.NotNil(t, got.FinishedAt)

	// A later ready event must not re-claim the finished run.
	require.NoError(t, eb.Publish(ctx, events.AgentReady, bus.NewEvent(events.AgentReady, "test", lifecycle.AgentEventPayload{
		TaskID:         "t-cap",
		SessionID:      "session-1",
		AgentProfileID: "missing-agent",
		CausingRunID:   run.ID,
	})))
	got, err = svc.RepoForTest().GetRunByID(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, service.RunStatusFinished, string(got.Status))
}

func TestAgentStoppedWithoutCausingRunDoesNotFinishCeilingDeferredRun(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	run := parkCeilingDeferredRun(t, svc)

	require.NoError(t, eb.Publish(ctx, events.AgentStopped, bus.NewEvent(events.AgentStopped, "test", lifecycle.AgentEventPayload{
		TaskID:         "t-cap",
		SessionID:      "old-session",
		AgentProfileID: "missing-agent",
	})))

	got, err := svc.RepoForTest().GetRunByID(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, service.RunStatusQueued, string(got.Status))
	require.NotNil(t, got.RoutingBlockedStatus)
	require.Empty(t, got.SessionID)
}

func TestAgentFailedBeforeReadyFailsCeilingDeferredRun(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	run := parkCeilingDeferredRun(t, svc)

	require.NoError(t, eb.Publish(ctx, events.AgentFailed, bus.NewEvent(events.AgentFailed, "test", lifecycle.AgentEventPayload{
		TaskID:               "t-cap",
		SessionID:            "session-1",
		AgentProfileID:       "missing-agent",
		TerminalCausingRunID: run.ID,
		ErrorMessage:         "agent failed before ready",
	})))

	got, err := svc.RepoForTest().GetRunByID(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, service.RunStatusFailed, string(got.Status))
	require.Equal(t, "agent failed before ready", got.ErrorMessage)

	require.NoError(t, eb.Publish(ctx, events.AgentStopped, bus.NewEvent(events.AgentStopped, "test", lifecycle.AgentEventPayload{
		TaskID:               "t-cap",
		SessionID:            "session-1",
		AgentProfileID:       "missing-agent",
		TerminalCausingRunID: run.ID,
		ErrorMessage:         "agent failed before ready",
	})))
	got, err = svc.RepoForTest().GetRunByID(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, service.RunStatusFailed, string(got.Status), "failure cleanup must not convert the run to success")
}

func TestAgentReadyThenCompletionFinishesCeilingDeferredRun(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	run := parkCeilingDeferredRun(t, svc)

	require.NoError(t, eb.Publish(ctx, events.AgentReady, bus.NewEvent(events.AgentReady, "test", lifecycle.AgentEventPayload{
		TaskID:         "t-cap",
		SessionID:      "session-1",
		AgentProfileID: "missing-agent",
		CausingRunID:   run.ID,
	})))
	require.NoError(t, eb.Publish(ctx, events.AgentStopped, bus.NewEvent(events.AgentStopped, "test", lifecycle.AgentEventPayload{
		TaskID:               "t-cap",
		SessionID:            "session-1",
		AgentProfileID:       "missing-agent",
		TerminalCausingRunID: run.ID,
	})))

	got, err := svc.RepoForTest().GetRunByID(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, service.RunStatusFinished, string(got.Status))
	require.Equal(t, "session-1", got.SessionID)
	require.Nil(t, got.RoutingBlockedStatus)
	require.NotNil(t, got.FinishedAt)
}

func TestAgentStoppedStaleCauseDoesNotFinishSuccessorCeilingRun(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	successor := parkCeilingDeferredRun(t, svc)

	require.NoError(t, eb.Publish(ctx, events.AgentStopped, bus.NewEvent(events.AgentStopped, "test", lifecycle.AgentEventPayload{
		TaskID:               "t-cap",
		SessionID:            "old-session",
		AgentProfileID:       "missing-agent",
		TerminalCausingRunID: "already-finished-run",
	})))

	got, err := svc.RepoForTest().GetRunByID(ctx, successor.ID)
	require.NoError(t, err)
	require.Equal(t, service.RunStatusQueued, string(got.Status))
	require.NotNil(t, got.RoutingBlockedStatus)
	require.Empty(t, got.SessionID)
}

func TestAgentStoppedWithErrorDoesNotFinishCeilingDeferredRun(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	run := parkCeilingDeferredRun(t, svc)

	require.NoError(t, eb.Publish(ctx, events.AgentStopped, bus.NewEvent(events.AgentStopped, "test", lifecycle.AgentEventPayload{
		TaskID:               "t-cap",
		SessionID:            "session-1",
		AgentProfileID:       "missing-agent",
		TerminalCausingRunID: run.ID,
		ErrorMessage:         "agent failed before ready",
	})))

	got, err := svc.RepoForTest().GetRunByID(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, service.RunStatusQueued, string(got.Status))
	require.NotNil(t, got.RoutingBlockedStatus)
	require.Empty(t, got.SessionID)
}

func TestAgentStoppedWithAmbiguousCeilingRunsDoesNotGuess(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	first := parkCeilingDeferredRun(t, svc)
	second := parkCeilingDeferredRun(t, svc)

	require.NoError(t, eb.Publish(ctx, events.AgentStopped, bus.NewEvent(events.AgentStopped, "test", lifecycle.AgentEventPayload{
		TaskID:         "t-cap",
		SessionID:      "session-1",
		AgentProfileID: "missing-agent",
	})))

	for _, run := range []*runsmodels.Run{first, second} {
		got, err := svc.RepoForTest().GetRunByID(ctx, run.ID)
		require.NoError(t, err)
		require.Equal(t, service.RunStatusQueued, string(got.Status))
		require.NotNil(t, got.RoutingBlockedStatus)
		require.Empty(t, got.SessionID)
	}
}

func TestCeilingDeferredLaunchDroppedFailsParkedRun(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	run := parkCeilingDeferredRun(t, svc)

	require.NoError(t, svc.CeilingDeferredLaunchDropped(ctx, "t-cap", run.ID, "superseded", "destination changed"))

	got, err := svc.RepoForTest().GetRunByID(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, service.RunStatusFailed, string(got.Status))
	require.Contains(t, got.ErrorMessage, "destination changed")
	require.Nil(t, got.RoutingBlockedStatus)
	require.NotNil(t, got.FinishedAt)

	// A stale second drop is a no-op.
	require.NoError(t, svc.CeilingDeferredLaunchDropped(ctx, "t-cap", run.ID, "superseded", "second"))
	got, err = svc.RepoForTest().GetRunByID(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, service.RunStatusFailed, string(got.Status))
	require.Contains(t, got.ErrorMessage, "destination changed")
}
