package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestExecutorOwnershipLaunchFailsBeforeRuntimeAdmission(t *testing.T) {
	manager := newTestManager(t)
	denied := errors.New("private executor denied")
	manager.SetExecutorAdmission(func(ctx context.Context, sessionID, executorID, profileID string) (context.Context, error) {
		require.Equal(t, "session-private", sessionID)
		require.Equal(t, "private-parent", executorID)
		require.Equal(t, "private-profile", profileID)
		return ctx, denied
	})
	execution, err := manager.Launch(t.Context(), &LaunchRequest{
		SessionID: "session-private", TaskID: "task-private",
		Metadata: map[string]any{"executor_id": "private-parent", MetadataKeyExecutorProfileID: "private-profile"},
	})
	require.ErrorIs(t, err, denied)
	require.Nil(t, execution)
}

func TestExecutorOwnershipTerminalCredentialsFailClosed(t *testing.T) {
	manager := newTestManager(t)
	manager.SetExecutorProfileReader(&fakeExecutorProfileReader{
		session:  &models.TaskSession{ID: "session-private", ExecutorProfileID: "private-profile"},
		profiles: map[string]*models.ExecutorProfile{"private-profile": {ID: "private-profile", EnvVars: []models.ProfileEnvVar{{Key: "TOKEN", Value: "private-value"}}}},
	})
	denied := errors.New("private executor denied")
	manager.SetExecutorAdmission(func(ctx context.Context, _, _, _ string) (context.Context, error) { return ctx, denied })
	env, err := manager.ExecutorProfileEnvForSession(t.Context(), "session-private", "")
	require.ErrorIs(t, err, denied)
	require.Nil(t, env)
	manager.SetExecutorAdmission(func(ctx context.Context, _, _, _ string) (context.Context, error) { return ctx, nil })
	env, err = manager.ExecutorProfileEnvForSession(t.Context(), "session-private", "")
	require.NoError(t, err)
	require.Equal(t, "private-value", env["TOKEN"], "same fixture proves credential resolution is reachable when allowed")
}

func TestExecutorOwnershipPromptAdmissionPrecedesDispatch(t *testing.T) {
	denied := errors.New("private executor unavailable")
	sm := NewSessionManager(newSessionTestLogger(), newTestStopCh(t))
	sm.executorAdmission = func(ctx context.Context, sessionID, _, _ string) (context.Context, error) {
		require.Equal(t, "private-session", sessionID)
		return ctx, denied
	}
	_, err := sm.SendPrompt(t.Context(), &AgentExecution{SessionID: "private-session"}, "run", false, nil, false)
	require.ErrorIs(t, err, denied)
}
