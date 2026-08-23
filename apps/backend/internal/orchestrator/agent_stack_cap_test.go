package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func TestAgentStackReaping_IdleTTLStopsStackIdlePastTTL(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-ttl-reap", "session-ttl-reap", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, "session-ttl-reap", "task-ttl-reap", "exec-ttl-reap")
	setSessionUpdatedAt(t, repo, "session-ttl-reap", time.Now().UTC().Add(-2*time.Minute))
	setExecutorUpdatedAt(t, repo, "session-ttl-reap", time.Now().UTC().Add(-2*time.Minute))

	stopCalls := make(chan stopAgentCall, 1)
	svc := newReapingTestService(t, repo, newMockTaskRepo(), stopCalls)
	svc.idleReaper = newIdleSessionReaper()
	svc.idleReaper.minIdle = 0
	svc.idleReaper.stackIdleTTL = time.Minute
	svc.idleReaper.stackLiveCap = 0

	svc.reclaimIdleSessionsOnce(ctx)

	call := waitForAgentStackStop(t, stopCalls)
	require.Equal(t, "exec-ttl-reap", call.ExecutionID)
	require.Equal(t, stopReasonAgentStackIdleTTL, call.Reason)
}

// executors_running.updated_at is refreshed by execution persistence and
// status writes, so it is not an idle clock. A stack launched long ago whose
// session finished a turn seconds ago must survive the TTL pass.
func TestAgentStackReaping_IdleTTLUsesSessionActivityNotExecutorRow(t *testing.T) {
	tests := []struct {
		name             string
		sessionUpdatedAt time.Time
		turnService      TurnService
		featureOn        bool
	}{
		{
			name:             "recent session on an old executor row",
			sessionUpdatedAt: time.Now().UTC().Add(-5 * time.Second),
			turnService:      &inactiveTurnService{},
			featureOn:        true,
		},
		{
			name:             "active turn",
			sessionUpdatedAt: time.Now().UTC().Add(-2 * time.Minute),
			turnService:      &alwaysActiveTurnService{},
			featureOn:        true,
		},
		{
			name:             "feature disabled",
			sessionUpdatedAt: time.Now().UTC().Add(-2 * time.Minute),
			turnService:      &inactiveTurnService{},
			featureOn:        false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task-ttl-safe", "session-ttl-safe", models.TaskSessionStateWaitingForInput)
			seedExecutorRunning(t, repo, "session-ttl-safe", "task-ttl-safe", "exec-ttl-safe")
			// The executor row is always ancient; only the session clock differs.
			setExecutorUpdatedAt(t, repo, "session-ttl-safe", time.Now().UTC().Add(-3*time.Hour))
			setSessionUpdatedAt(t, repo, "session-ttl-safe", tt.sessionUpdatedAt)

			stopCalls := make(chan stopAgentCall, 1)
			svc := newReapingTestService(t, repo, newMockTaskRepo(), stopCalls)
			svc.config.AgentStackReaping = tt.featureOn
			svc.turnService = tt.turnService
			svc.idleReaper = newIdleSessionReaper()
			svc.idleReaper.minIdle = 0
			svc.idleReaper.stackIdleTTL = time.Minute
			svc.idleReaper.stackLiveCap = 0

			svc.reclaimIdleSessionsOnce(ctx)
			assertNoAgentStackStop(t, stopCalls)
		})
	}
}

// The incident was concurrency, not age: eleven stacks alive at once. The cap
// evicts down to the ceiling, oldest-idle first, and counts every live row.
func TestAgentStackReaping_LiveStackCapEvictsOldestIdleFirst(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	ages := map[string]time.Duration{
		"session-cap-old":    30 * time.Minute,
		"session-cap-mid":    20 * time.Minute,
		"session-cap-recent": 5 * time.Second,
	}
	for sessionID, age := range ages {
		seedTaskAndSession(t, repo, "task-"+sessionID, sessionID, models.TaskSessionStateWaitingForInput)
		seedExecutorRunning(t, repo, sessionID, "task-"+sessionID, "exec-"+sessionID)
		setSessionUpdatedAt(t, repo, sessionID, now.Add(-age))
	}

	stopCalls := make(chan stopAgentCall, 4)
	svc := newReapingTestService(t, repo, newMockTaskRepo(), stopCalls)
	svc.idleReaper = newIdleSessionReaper()
	svc.idleReaper.stackLiveCap = 2

	rows, err := repo.ListExecutorsRunning(ctx)
	require.NoError(t, err)
	svc.enforceAgentStackCap(ctx, rows, now)

	call := waitForAgentStackStop(t, stopCalls)
	require.Equal(t, "exec-session-cap-old", call.ExecutionID)
	require.Equal(t, stopReasonAgentStackOverCap, call.Reason)
	assertNoAgentStackStop(t, stopCalls)
}

func TestAgentStackReaping_LiveStackCapSkipsWorkingAndUnderCap(t *testing.T) {
	tests := []struct {
		name         string
		sessionState models.TaskSessionState
		liveCap      int
		featureOn    bool
	}{
		{name: "under cap", sessionState: models.TaskSessionStateWaitingForInput, liveCap: 5, featureOn: true},
		{name: "cap disabled", sessionState: models.TaskSessionStateWaitingForInput, liveCap: 0, featureOn: true},
		{name: "feature disabled", sessionState: models.TaskSessionStateWaitingForInput, liveCap: 1, featureOn: false},
		{name: "working session over cap", sessionState: models.TaskSessionStateRunning, liveCap: 1, featureOn: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			now := time.Now().UTC()
			for _, sessionID := range []string{"session-cap-a", "session-cap-b"} {
				seedTaskAndSession(t, repo, "task-"+sessionID, sessionID, tt.sessionState)
				seedExecutorRunning(t, repo, sessionID, "task-"+sessionID, "exec-"+sessionID)
				setSessionUpdatedAt(t, repo, sessionID, now.Add(-time.Hour))
			}

			stopCalls := make(chan stopAgentCall, 4)
			svc := newReapingTestService(t, repo, newMockTaskRepo(), stopCalls)
			svc.config.AgentStackReaping = tt.featureOn
			svc.idleReaper = newIdleSessionReaper()
			svc.idleReaper.stackLiveCap = tt.liveCap

			rows, err := repo.ListExecutorsRunning(ctx)
			require.NoError(t, err)
			svc.enforceAgentStackCap(ctx, rows, now)

			assertNoAgentStackStop(t, stopCalls)
		})
	}
}

// Service.Stop must join the detached task sweeps rather than letting a
// blocked StopAgentWithReason outlive shutdown, and a stopped sweeper must
// refuse new work instead of leaking an untracked goroutine.
func TestAgentStackSweeper_JoinsWorkersAndRefusesAfterStop(t *testing.T) {
	sweeper := newAgentStackSweeper()
	require.False(t, sweeper.spawn(func(context.Context) {}), "spawn before start must be refused")

	sweeper.start(context.Background())
	release := make(chan struct{})
	require.True(t, sweeper.spawn(func(context.Context) {
		<-release
	}))

	stopped := make(chan struct{})
	go func() {
		sweeper.stop()
		close(stopped)
	}()

	select {
	case <-stopped:
		t.Fatal("sweeper.stop returned while its worker was still running")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("sweeper.stop did not join its worker")
	}

	require.False(t, sweeper.spawn(func(context.Context) {}), "spawn after stop must be refused")
}

// The sweep context is derived from the service context, so shutdown unblocks
// a sweep that is waiting on it.
func TestAgentStackSweeper_CancelsSweepContextOnStop(t *testing.T) {
	sweeper := newAgentStackSweeper()
	sweeper.start(context.Background())

	observed := make(chan error, 1)
	require.True(t, sweeper.spawn(func(ctx context.Context) {
		<-ctx.Done()
		observed <- ctx.Err()
	}))

	sweeper.stop()

	select {
	case err := <-observed:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("sweep context was not cancelled by stop")
	}
}

func setSessionUpdatedAt(t *testing.T, repo *sqliterepo.Repository, sessionID string, updatedAt time.Time) {
	t.Helper()
	_, err := repo.DB().ExecContext(context.Background(),
		`UPDATE task_sessions SET updated_at = ? WHERE id = ?`, updatedAt, sessionID)
	require.NoError(t, err)
}

func setExecutorUpdatedAt(t *testing.T, repo *sqliterepo.Repository, sessionID string, updatedAt time.Time) {
	t.Helper()
	_, err := repo.DB().ExecContext(context.Background(),
		`UPDATE executors_running SET updated_at = ? WHERE session_id = ?`, updatedAt, sessionID)
	require.NoError(t, err)
}
