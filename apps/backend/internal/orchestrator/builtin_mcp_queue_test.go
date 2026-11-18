package orchestrator

import (
	"context"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestBuiltinApplicationDispatchBarrierKeepsInputQueued(t *testing.T) {
	svc, _ := newBuiltinIntentResumeFixture(t, "retired", true)
	repo := svc.repo.(*sqliterepo.Repository)
	ctx := context.Background()
	now := time.Now().UTC()
	_, err := repo.DB().Exec(`INSERT INTO task_builtin_mcp_applications(session_id,task_id,workspace_id,target_revision,policy_revision,selection_hash,requested_by,request_id,stage,created_at,updated_at) VALUES ('session','task','ws1',3,0,'hash','human','request','starting',?,?)`, now, now)
	require.NoError(t, err)
	guard, ok := any(svc).(interface {
		BuiltinMCPDispatchBlocked(context.Context, string) (bool, error)
	})
	require.True(t, ok, "queue reservation needs the durable provider refresh barrier")
	blocked, err := guard.BuiltinMCPDispatchBlocked(ctx, "session")
	require.NoError(t, err)
	require.True(t, blocked)
	_, err = repo.DB().Exec(`UPDATE task_builtin_mcp_applications SET stage='applied' WHERE session_id='session'`)
	require.NoError(t, err)
	blocked, err = guard.BuiltinMCPDispatchBlocked(ctx, "session")
	require.NoError(t, err)
	require.False(t, blocked)
}
