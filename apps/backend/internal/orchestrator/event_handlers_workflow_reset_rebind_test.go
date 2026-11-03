package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

const (
	rebindExecutionID = "exec-rebind"
	rebindOldNative   = "native-before-reset"
	rebindFreshNative = "native-after-reset"
)

type nativeSwitchingResetManager struct {
	*mockAgentManager
	reset               atomic.Bool
	previousUnavailable bool
	resetErr            error
	onReset             func()
}

func (m *nativeSwitchingResetManager) ResetAgentContext(context.Context, string) error {
	m.reset.Store(true)
	if m.onReset != nil {
		m.onReset()
	}
	return m.resetErr
}

func newNativeRebindResetService(t *testing.T) (*Service, *sqliterepo.Repository, *models.TaskSession) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", rebindExecutionID)
	require.NoError(t, repo.UpdateResumeToken(ctx, "s1", rebindExecutionID, rebindOldNative, ""))
	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: "s1", IncarnationID: rebindIncarnationID(session), Generation: 1,
		NativeSessionID: rebindOldNative, CreationReason: "native_started",
	}))
	manager := &nativeSwitchingResetManager{mockAgentManager: &mockAgentManager{repoForExecutionLookup: repo}}
	manager.getACPSessionIDForSessionFunc = func(string) (string, bool) {
		if manager.reset.Load() {
			return rebindFreshNative, true
		}
		if manager.previousUnavailable {
			return "", false
		}
		return rebindOldNative, true
	}
	session.AgentExecutionID = rebindExecutionID
	return createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager), repo, session
}

func rebindIncarnationID(session *models.TaskSession) string {
	if session.QueueIncarnationID != "" {
		return session.QueueIncarnationID
	}
	return session.ID
}

func requireResetNativeIdentity(t *testing.T, repo *sqliterepo.Repository, session *models.TaskSession, native string, history int) {
	t.Helper()
	ctx := context.Background()
	current, err := repo.GetCurrentHarnessSessionGeneration(ctx, session.ID, rebindIncarnationID(session))
	require.NoError(t, err)
	require.Equal(t, native, current.NativeSessionID, "the generation must project the live native conversation")
	running, err := repo.GetExecutorRunningBySessionID(ctx, session.ID)
	require.NoError(t, err)
	require.Equal(t, native, running.ResumeToken)
	var count int
	require.NoError(t, repo.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM harness_native_session_rebinds WHERE session_id = ?`, session.ID).Scan(&count))
	require.Equal(t, history, count)
}

func TestWorkflowContextResetRebindsCommittedGeneration(t *testing.T) {
	svc, repo, session := newNativeRebindResetService(t)
	require.True(t, svc.resetAgentContext(context.Background(), "t1", session, "review"))
	requireResetNativeIdentity(t, repo, session, rebindFreshNative, 1)
}

func TestWorkflowContextResetRefusedRebindFailsReset(t *testing.T) {
	svc, repo, session := newNativeRebindResetService(t)
	manager := svc.agentManager.(*nativeSwitchingResetManager)
	manager.onReset = func() {
		svc.handleSessionStatusEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
			TaskID: "t1", SessionID: session.ID, ExecutionID: rebindExecutionID,
			Data: &lifecycle.AgentStreamEventData{ACPSessionID: rebindFreshNative, SessionStatus: "new"},
		})
	}
	_, err := repo.DB().ExecContext(context.Background(), `INSERT INTO session_restore_attempts
		(id, session_id, incarnation_id, expected_generation, action, outcome, created_at)
		VALUES ('attempt-rebind', 's1', ?, 1, 'silent_restart_restore', 'pending', ?)`,
		rebindIncarnationID(session), time.Now().UTC())
	require.NoError(t, err)
	require.False(t, svc.resetAgentContext(context.Background(), "t1", session, "review"),
		"a refused rebind must fail the reset instead of drifting the token")
	require.Equal(t, rebindFreshNative, svc.currentACPSessionID(session.ID),
		"the provider reset succeeded before the durable rebind was refused")
	requireResetNativeIdentity(t, repo, session, rebindOldNative, 0)
	require.ErrorIs(t, svc.validatePromptTaskStart(session.ID), ErrSessionResetInProgress,
		"an uncommitted native identity must keep prompt admission blocked")
}

func TestWorkflowContextResetRebindsAfterPartialProviderFailure(t *testing.T) {
	svc, repo, session := newNativeRebindResetService(t)
	svc.agentManager.(*nativeSwitchingResetManager).resetErr = errors.New("runtime config restore failed")

	require.False(t, svc.resetAgentContext(context.Background(), "t1", session, "review"),
		"the provider error remains visible even when persistence reconciliation succeeds")
	requireResetNativeIdentity(t, repo, session, rebindFreshNative, 1)
	require.NoError(t, svc.validatePromptTaskStart(session.ID),
		"a reconciled partial reset must release prompt admission")
}

func TestWorkflowContextResetUsesCommittedGenerationWhenPreviousSessionIDIsUnavailable(t *testing.T) {
	svc, repo, session := newNativeRebindResetService(t)
	svc.agentManager.(*nativeSwitchingResetManager).previousUnavailable = true

	require.True(t, svc.resetAgentContext(context.Background(), "t1", session, "review"))
	requireResetNativeIdentity(t, repo, session, rebindFreshNative, 1)
}
