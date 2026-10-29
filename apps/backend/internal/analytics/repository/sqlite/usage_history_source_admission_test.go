package sqlite

import (
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestUsageHistorySessionCreationCannotReopenErasingScope(t *testing.T) {
	runSessionCreationErasureFence(t, false)
}

func TestUsageHistoryPostgresSessionCreationCannotReopenErasingScope(t *testing.T) {
	runSessionCreationErasureFence(t, true)
}

func runSessionCreationErasureFence(t *testing.T, postgres bool) {
	repo, tasks := currentTargetDatabaseFixture(t, postgres)
	require.NoError(t, repo.BeginOrganizationUsageErasure(t.Context(), "org-a"))
	value := &taskmodels.TaskSession{ID: "late-session", TaskID: "task-1", Metadata: map[string]any{"acp": map[string]any{"session_id": "late-source"}}}
	err := tasks.CreateTaskSession(t.Context(), value)
	require.ErrorIs(t, err, errUsageScopeErasing, "creation capture must serialize with the captured organization fence")
	var count int
	require.NoError(t, repo.db.Get(&count, `SELECT COUNT(*) FROM task_sessions WHERE id='late-session'`))
	require.Zero(t, count)
	require.NoError(t, repo.EraseOrganizationUsage(t.Context(), "org-a"))
	value = &taskmodels.TaskSession{ID: "after-erasure-session", TaskID: "task-1"}
	err = tasks.CreateTaskSession(t.Context(), value)
	require.ErrorIs(t, err, errUsageScopeErasing)
	require.NoError(t, repo.db.Get(&count, `SELECT COUNT(*) FROM usage_scopes WHERE organization_id='org-a'`))
	require.Zero(t, count, "late creation cannot recreate an erased retained scope")
}
