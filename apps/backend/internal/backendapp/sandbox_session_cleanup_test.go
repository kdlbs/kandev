package backendapp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/stretchr/testify/require"
)

func TestSandboxSessionCleanupWithoutRuntimePreservesCapturedOwnership(t *testing.T) {
	h := newBootStateTestHarness(t)
	policies, err := newSandboxPolicyService(&config.Config{HomeDir: t.TempDir()}, h.db)
	require.NoError(t, err)
	cleanup := sandboxSessionCleanup{policies: policies}
	require.NoError(t, cleanup.cleanup(t.Context(), "task", "ordinary-session"))
	_, err = h.db.Exec(`INSERT INTO sandbox_attempts (org_id,attempt_id,execution_id,session_id,generation,version,state,snapshot_json,owner_json,updated_at) VALUES ('','stored-attempt','execution','session',1,1,?,'{}','null',?)`, sandbox.AttemptStopped, time.Now().UTC().Format(time.RFC3339Nano))
	require.NoError(t, err)
	require.ErrorIs(t, cleanup.cleanup(t.Context(), "task", "session"), errSandboxRuntimeUnavailable)
	var count int
	require.NoError(t, h.db.QueryRow(`SELECT COUNT(*) FROM sandbox_attempts WHERE session_id='session'`).Scan(&count))
	require.Equal(t, 1, count)
	blocked := errors.New("cleanup refused")
	calls := 0
	cleanup.remove = func(_ context.Context, taskID, sessionID string) error {
		calls++
		require.Equal(t, "task", taskID)
		require.Equal(t, "session", sessionID)
		return blocked
	}
	require.ErrorIs(t, cleanup.cleanup(t.Context(), "task", "session"), blocked)
	require.Equal(t, 1, calls)
}
