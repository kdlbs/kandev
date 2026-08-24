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
	svc.idleReaper.stackIdleTTL = time.Minute

	svc.reclaimIdleSessionsOnce(ctx)

	call := waitForAgentStackStop(t, stopCalls)
	require.Equal(t, "exec-ttl-reap", call.ExecutionID)
	require.Equal(t, stopReasonAgentStackIdleTTL, call.Reason)
}

func TestAgentStackReaping_IdleTTLDoesNotDependOnExecutorRowAge(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-ttl-session-clock", "session-ttl-session-clock", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, "session-ttl-session-clock", "task-ttl-session-clock", "exec-ttl-session-clock")
	now := time.Now().UTC()
	setSessionUpdatedAt(t, repo, "session-ttl-session-clock", now.Add(-2*time.Minute))
	setExecutorUpdatedAt(t, repo, "session-ttl-session-clock", now)

	stopCalls := make(chan stopAgentCall, 1)
	svc := newReapingTestService(t, repo, newMockTaskRepo(), stopCalls)
	svc.idleReaper.stackIdleTTL = time.Minute

	svc.reclaimIdleSessionsOnce(ctx)

	call := waitForAgentStackStop(t, stopCalls)
	require.Equal(t, "exec-ttl-session-clock", call.ExecutionID)
	require.Equal(t, stopReasonAgentStackIdleTTL, call.Reason)
}

func TestAgentStackReaping_IdleTTLSkipsUnsafeSessions(t *testing.T) {
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
			setExecutorUpdatedAt(t, repo, "session-ttl-safe", time.Now().UTC().Add(-3*time.Hour))
			setSessionUpdatedAt(t, repo, "session-ttl-safe", tt.sessionUpdatedAt)

			stopCalls := make(chan stopAgentCall, 1)
			svc := newReapingTestService(t, repo, newMockTaskRepo(), stopCalls)
			svc.config.AgentStackReaping = tt.featureOn
			svc.turnService = tt.turnService
			svc.idleReaper.stackIdleTTL = time.Minute

			svc.reclaimIdleSessionsOnce(ctx)
			assertNoAgentStackStop(t, stopCalls)
		})
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
