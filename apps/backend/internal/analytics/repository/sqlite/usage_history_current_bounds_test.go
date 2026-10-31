package sqlite

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/stretchr/testify/require"
)

func seedUnrelatedCurrentTargets(t *testing.T, repo *Repository, count int) {
	t.Helper()
	at := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	tx, err := repo.db.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(t.Context(), repo.db.Rebind(`INSERT INTO workspaces (id,name,org_id,created_at,updated_at) VALUES ('ws-other','Other','org-other',?,?)`), at, at)
	require.NoError(t, err)
	for _, workspace := range []string{"ws-1", "ws-other"} {
		var organization string
		require.NoError(t, tx.GetContext(t.Context(), &organization, repo.db.Rebind(`SELECT COALESCE(org_id,'') FROM workspaces WHERE id = ?`), workspace))
		task := "archive-" + workspace
		_, err = tx.ExecContext(t.Context(), repo.db.Rebind(`INSERT INTO tasks (id,workspace_id,title,created_at,updated_at) VALUES (?,?,'Historical',?,?)`), task, workspace, at, at)
		require.NoError(t, err)
		for index := range count {
			session := fmt.Sprintf("%s-session-%04d", task, index)
			metadata, err := json.Marshal(map[string]any{"acp": map[string]any{"session_id": "transcript-" + session}})
			require.NoError(t, err)
			_, err = tx.ExecContext(t.Context(), repo.db.Rebind(`INSERT INTO task_sessions
				(id,task_id,queue_incarnation_id,metadata,started_at,updated_at) VALUES (?,?,?,?,?,?)`), session, task, session, string(metadata), at, at)
			require.NoError(t, err)
			if index%2 != 0 {
				continue
			}
			require.NoError(t, repo.CaptureUsageCollectionSessionTx(t.Context(), tx, task, session))
			require.NoError(t, repo.AdmitBindingTx(t.Context(), tx, usagehistory.Binding{
				ID: "binding-" + session, Kind: usagehistory.BindingKindExecution, WorkspaceID: workspace,
				OrganizationID: organization,
				TaskID:         task, TaskIncarnationID: at.Format(time.RFC3339Nano), SessionID: session, SessionIncarnationID: session,
				ActorKind: "unknown", ActorStatus: "unknown", ActorSource: "legacy", CapturedAt: at,
			}))
		}
	}
	require.NoError(t, tx.Commit())
}

func TestUsageHistoryCurrentTargetExcludesHistoricalPages(t *testing.T) {
	runCurrentTargetExcludesHistoricalPages(t, false)
}

func TestUsageHistoryPostgresCurrentTargetExcludesHistoricalPages(t *testing.T) {
	runCurrentTargetExcludesHistoricalPages(t, true)
}

// @covers AC-COSTS-USAGE-HISTORY-005.2 AC-COSTS-USAGE-HISTORY-002.5
func runCurrentTargetExcludesHistoricalPages(t *testing.T, postgres bool) {
	repo, _ := currentTargetDatabaseFixture(t, postgres)
	seedUnrelatedCurrentTargets(t, repo, 451)
	first := currentTargetForTest(t, repo, "collector-a", "ws-1", currentUsageTargetSelector{SessionID: "session-1"})
	require.NotNil(t, first)
	for _, query := range []currentUsageTargetSelector{
		{TaskID: "task-1"},
		{SessionID: "session-1"},
		{ExternalSessionID: "transcript-1"},
		{TaskID: "task-1", SessionID: "session-1", ExternalSessionID: "transcript-1"},
		{SubjectBinding: first.BindingID},
	} {
		target := currentTargetForTest(t, repo, "collector-a", "ws-1", query)
		require.NotNil(t, target)
		require.Equal(t, first.SubjectID, target.SubjectID)
		require.Equal(t, "eligible", target.SourceScopeStatus)
	}
	require.Nil(t, currentTargetForTest(t, repo, "collector-a", "ws-1", currentUsageTargetSelector{TaskID: "archive-ws-other"}))
	require.Nil(t, currentTargetForTest(t, repo, "collector-a", "ws-1", currentUsageTargetSelector{SessionID: "archive-ws-other-session-0000"}))
	_, err := lookupCurrentTargetForTest(t, t.Context(), repo, "collector-a", "ws-other", currentUsageTargetSelector{SubjectBinding: first.BindingID})
	require.ErrorIs(t, err, ErrUsageNotVisible)
	for _, table := range []string{"usage_collection_snapshots", "usage_collection_preparation"} {
		var count int
		require.NoError(t, repo.db.Get(&count, `SELECT COUNT(*) FROM `+table))
		require.Zero(t, count, "point refresh must not enumerate or prepare historical pages")
	}
}
