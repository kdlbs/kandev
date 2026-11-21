package sqlite

import (
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestUsageHistorySourceGuardConcurrentFirstOwners(t *testing.T) {
	runSourceGuardConcurrentFirstOwners(t, false)
}

func TestUsageHistoryPostgresSourceGuardConcurrentFirstOwners(t *testing.T) {
	runSourceGuardConcurrentFirstOwners(t, true)
}

func runSourceGuardConcurrentFirstOwners(t *testing.T, postgres bool) {
	repo, tasks := currentTargetDatabaseFixture(t, postgres)
	if postgres {
		repo.db.SetMaxOpenConns(3)
		repo.db.SetMaxIdleConns(3)
	}
	require.NoError(t, tasks.CreateWorkspace(t.Context(), &taskmodels.Workspace{ID: "ws-b", Name: "B", OrgID: "org-b"}))
	createCurrentTargetSessionInWorkspace(t, tasks, "ws-b", "b-task", "b-session", "clean-b")
	start := make(chan struct{})
	points := make(chan struct{})
	results := make(chan error, 2)
	for _, pair := range [][2]string{{"ws-1", "session-1"}, {"ws-b", "b-session"}} {
		go func() {
			<-start
			results <- tasks.UpdateSessionMetadata(t.Context(), pair[1], map[string]any{"acp": map[string]any{"session_id": "shared-race-transcript"}})
			<-points
			_, err := repo.ListUsageCollectionTargets(t.Context(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{
				WorkspaceID: pair[0], CurrentTarget: &pluginsdk.UsageCollectionCurrentTargetQueryV1{SessionID: pair[1], ExternalSessionID: "shared-race-transcript"},
			})
			results <- err
		}()
	}
	close(start)
	updates := []error{<-results, <-results}
	close(points)
	writes := []error{<-results, <-results}
	for _, err := range append(updates, writes...) {
		require.NoError(t, err)
	}
	for _, pair := range [][2]string{{"ws-1", "session-1"}, {"ws-b", "b-session"}} {
		target := currentTargetForTest(t, repo, "collector-a", pair[0], currentUsageTargetSelector{SessionID: pair[1], ExternalSessionID: "shared-race-transcript"})
		require.NotNil(t, target)
		require.Equal(t, "mixed", target.SourceScopeStatus, "neither owner may aggregate the shared foreign transcript")
		require.Nil(t, target.dto().ExternalSessionID)
	}
	var count, mixed int
	require.NoError(t, repo.db.QueryRowx(repo.db.Rebind(`SELECT COUNT(*),MAX(mixed) FROM usage_collection_source_guards WHERE source_hash = ?`),
		usageSourceGuardHash("usage-transcript", "shared-race-transcript")).Scan(&count, &mixed))
	require.Equal(t, 1, count)
	require.Equal(t, 1, mixed)
	require.NoError(t, repo.db.Get(&count, repo.db.Rebind(`SELECT COUNT(*) FROM usage_collection_guard_workspaces WHERE source_hash = ?`),
		usageSourceGuardHash("usage-transcript", "shared-race-transcript")))
	require.Equal(t, 2, count, "both concurrent workspaces retain their guard membership")
	require.NoError(t, repo.ResetWorkspaceUsageHistory(t.Context(), "ws-1"))
	require.NoError(t, repo.db.Get(&count, repo.db.Rebind(`SELECT COUNT(*) FROM usage_collection_source_guards WHERE source_hash = ?`),
		usageSourceGuardHash("usage-transcript", "shared-race-transcript")))
	require.Equal(t, 1, count, "fixture reset cannot remove a guard still held by another workspace")
	createCurrentTargetSessionInWorkspace(t, tasks, "ws-b", "b-fresh-task", "b-fresh-session", "shared-race-transcript")
	other := currentTargetForTest(t, repo, "collector-a", "ws-b", currentUsageTargetSelector{SessionID: "b-fresh-session"})
	require.Equal(t, "mixed", other.SourceScopeStatus)
}
