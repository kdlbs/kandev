package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// @covers AC-EXECUTORS-IDLE-PARKING-001.11
func TestIdleParkingRecoveryIgnoresPassivePersistenceTimes(t *testing.T) {
	svc, agent, row := newIdleParkingFixture(t, true, 1, "claude-acp")
	ctx := context.Background()
	require.NoError(t, svc.repo.SetSessionMetadataKey(ctx, row.SessionID, "passive", "updated"))
	current, err := svc.repo.GetExecutorRunningBySessionID(ctx, row.SessionID)
	require.NoError(t, err)
	current.Metadata = map[string]interface{}{"passive": "updated"}
	require.NoError(t, svc.repo.UpsertExecutorRunning(ctx, current))
	svc.suspendWorkspaceIdleSessionsOnce(ctx)
	require.Len(t, agent.suspensionCalls(), 1, "fresh row/session metadata must not refresh real activity")
}

// @covers AC-EXECUTORS-IDLE-PARKING-001.11
func TestIdleParkingUnknownRecoveryReceivesOneStableFullInterval(t *testing.T) {
	svc, agent, row := newIdleParkingFixture(t, true, 1, "claude-acp")
	agent.currentPromptLastActivityAt = time.Time{}
	agent.currentPromptActivityEpoch.Store(0)
	now := time.Now().UTC()
	scan := func(at time.Time) {
		current, err := svc.repo.GetExecutorRunningBySessionID(context.Background(), row.SessionID)
		require.NoError(t, err)
		summary := idleParkingScanSummary{observed: make(map[idleParkingCandidateKey]struct{}), skipped: make(map[idleParkingSkipReason]int)}
		svc.processWorkspaceIdleRow(context.Background(), current, at, agent, &summary)
		svc.pruneIdleParkingCandidates(summary.observed)
	}
	scan(now)
	require.Empty(t, agent.suspensionCalls())
	scan(now.Add(59 * time.Second))
	require.Empty(t, agent.suspensionCalls())
	scan(now.Add(time.Minute))
	require.Len(t, agent.suspensionCalls(), 1, "rescanning must not restart the unknown-history interval")
}

func TestIdleParkingFutureAndForeignActivityStayProtected(t *testing.T) {
	for _, kind := range []string{"future", "other execution"} {
		t.Run(kind, func(t *testing.T) {
			svc, agent, _ := newIdleParkingFixture(t, true, 1, "claude-acp")
			old := agent.currentPromptLastActivityAt
			if kind == "future" {
				agent.currentPromptLastActivityAt = time.Now().Add(time.Hour)
			} else {
				agent.currentPromptExecutionID = "other"
			}
			svc.suspendWorkspaceIdleSessionsOnce(context.Background())
			require.Empty(t, agent.suspensionCalls())
			agent.currentPromptLastActivityAt = old
			agent.currentPromptExecutionID = "execution-idle-parking"
			svc.suspendWorkspaceIdleSessionsOnce(context.Background())
			require.Len(t, agent.suspensionCalls(), 1, "positive control: same candidate is suspendable")
		})
	}
}

func TestIdleParkingRoutineSkipSummaryUsesDebug(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	svc := &Service{logger: log}
	summary := idleParkingScanSummary{skipped: map[idleParkingSkipReason]int{idleParkingSkipKnownWork: 1}}
	svc.logWorkspaceIdleScan(summary)
	svc.logWorkspaceIdleScan(summary)
	require.Equal(t, 2, logs.FilterLevelExact(zap.DebugLevel).Len())
	require.Zero(t, logs.FilterLevelExact(zap.InfoLevel).Len(), "unchanged skips should not produce Info every 30 seconds")
	svc.logWorkspaceIdleScan(idleParkingScanSummary{suspended: 1})
	require.Equal(t, 1, logs.FilterLevelExact(zap.InfoLevel).Len(), "real suspension outcomes stay visible")
}

// @covers AC-EXECUTORS-IDLE-PARKING-001.6
// @covers AC-EXECUTORS-IDLE-PARKING-001.11
func TestIdleParkingExplicitFocusFloorSurvivesRecovery(t *testing.T) {
	ctx := context.Background()
	svc, agent, row := newIdleParkingFixture(t, true, 1, "claude-acp")
	agent.isAgentReadyFn = func(context.Context, string) bool { return true }
	svc.refreshFocusedLiveSession(ctx, row.TaskID, row.SessionID)
	require.False(t, svc.idleParkingFocusAt[row.SessionID].IsZero(), "positive control: the real explicit-focus path was reached")
	svc.suspendWorkspaceIdleSessionsOnce(ctx)
	require.Empty(t, agent.suspensionCalls(), "positive control: live explicit focus protects the same idle candidate")
	svc.idleParkingFocusAt, svc.idleParkingCandidates = nil, nil
	svc.suspendWorkspaceIdleSessionsOnce(ctx)
	require.Empty(t, agent.suspensionCalls(), "restart must not discard the fresh explicit-focus interval")
	session, err := svc.repo.GetTaskSession(ctx, row.SessionID)
	require.NoError(t, err)
	require.Contains(t, session.Metadata, "idle_parking_focus", "explicit focus must have a durable positive control")
	focus := session.Metadata["idle_parking_focus"].(map[string]interface{})
	focusedAt, err := time.Parse(time.RFC3339Nano, focus["focused_at"].(string))
	require.NoError(t, err)
	// Startup recreates these maps; the tracked activity still describes the
	// old settled turn, as it does after lifecycle restores its saved clock.
	svc.idleParkingFocusAt, svc.idleParkingCandidates = nil, nil
	svc.suspendWorkspaceIdleSessionsOnce(ctx)
	require.Empty(t, agent.suspensionCalls(), "restart must not discard the fresh explicit-focus interval")
	row, err = svc.repo.GetExecutorRunningBySessionID(ctx, row.SessionID)
	require.NoError(t, err)
	now := focusedAt.Add(10 * time.Second)
	candidate, _, ok := svc.workspaceIdleCandidate(ctx, row, now)
	require.True(t, ok)
	require.False(t, svc.idleParkingCandidateDue(candidate.key, focusedAt.Add(59*time.Second), time.Minute))
	require.True(t, svc.idleParkingCandidateDue(candidate.key, focusedAt.Add(time.Minute), time.Minute))
}

func TestIdleParkingFocusFloorCannotTransferToAnotherExecution(t *testing.T) {
	ctx := context.Background()
	svc, agent, row := newIdleParkingFixture(t, true, 1, "claude-acp")
	agent.isAgentReadyFn = func(context.Context, string) bool { return true }
	svc.refreshFocusedLiveSession(ctx, row.TaskID, row.SessionID)
	svc.idleParkingFocusAt, svc.idleParkingCandidates = nil, nil
	row.AgentExecutionID = "replacement"
	agent.currentPromptExecutionID = row.AgentExecutionID
	require.NoError(t, svc.repo.UpsertExecutorRunning(ctx, row))
	svc.suspendWorkspaceIdleSessionsOnce(ctx)
	require.Len(t, agent.suspensionCalls(), 1, "a predecessor focus must not change a successor's idle interval")
}

func TestIdleParkingRecoveryKeepsActiveLSPLeaseFence(t *testing.T) {
	svc, agent, row := newIdleParkingFixture(t, true, 1, "claude-acp")
	lease := &activeLSPLeaseForTest{sessionID: row.SessionID}
	svc.SetLSPLeaseLifecycle(lease)
	svc.suspendWorkspaceIdleSessionsOnce(t.Context())
	require.Empty(t, agent.suspensionCalls())
	require.Greater(t, lease.checks, 0, "the lease owner must actually be consulted")
	svc.lspLeases = nil
	svc.suspendWorkspaceIdleSessionsOnce(t.Context())
	require.Len(t, agent.suspensionCalls(), 1, "positive control: releasing the lease permits suspension")
}

func TestIdleParkingRecoveryRejectsUntrustedFocusFloors(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value interface{}
	}{
		{"wrong execution", map[string]interface{}{"execution_id": "other", "focused_at": time.Now().UTC().Format(time.RFC3339Nano)}},
		{"malformed snapshot", "invalid"},
		{"malformed time", map[string]interface{}{"execution_id": "execution-idle-parking", "focused_at": "invalid"}},
		{"future", map[string]interface{}{"execution_id": "execution-idle-parking", "focused_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, agent, row := newIdleParkingFixture(t, true, 1, "claude-acp")
			require.NoError(t, svc.repo.SetSessionMetadataKey(t.Context(), row.SessionID, "idle_parking_focus", tc.value))
			svc.suspendWorkspaceIdleSessionsOnce(t.Context())
			require.Len(t, agent.suspensionCalls(), 1, "invalid focus history cannot replace an execution's idle interval")
		})
	}
}

func TestIdleParkingUnknownActivityKeepsFullIntervalWithOldFocusFloor(t *testing.T) {
	svc, agent, row := newIdleParkingFixture(t, true, 1, "claude-acp")
	agent.currentPromptLastActivityAt = time.Time{}
	require.NoError(t, svc.repo.SetSessionMetadataKey(t.Context(), row.SessionID, "idle_parking_focus", map[string]interface{}{
		"execution_id": row.AgentExecutionID, "focused_at": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano),
	}))
	now := time.Now().UTC()
	current, err := svc.repo.GetExecutorRunningBySessionID(t.Context(), row.SessionID)
	require.NoError(t, err)
	for _, at := range []time.Time{now, now.Add(59 * time.Second)} {
		summary := idleParkingScanSummary{observed: make(map[idleParkingCandidateKey]struct{}), skipped: make(map[idleParkingSkipReason]int)}
		svc.processWorkspaceIdleRow(t.Context(), current, at, agent, &summary)
		require.Empty(t, agent.suspensionCalls())
	}
	candidate, _, ok := svc.workspaceIdleCandidate(t.Context(), current, now.Add(time.Minute))
	require.True(t, ok)
	require.True(t, svc.idleParkingCandidateDue(candidate.key, now.Add(time.Minute), time.Minute))
}
