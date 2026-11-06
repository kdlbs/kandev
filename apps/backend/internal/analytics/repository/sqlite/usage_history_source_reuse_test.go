package sqlite

import (
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

// @covers AC-COSTS-USAGE-HISTORY-002.5 AC-COSTS-USAGE-HISTORY-001.6
func TestUsageHistoryFreshSessionCannotReuseErasedTranscript(t *testing.T) {
	runFreshSessionCannotReuseErasedTranscript(t, false)
}

func TestUsageHistoryPostgresFreshSessionCannotReuseErasedTranscript(t *testing.T) {
	runFreshSessionCannotReuseErasedTranscript(t, true)
}

func runFreshSessionCannotReuseErasedTranscript(t *testing.T, postgres bool) {
	for _, legacy := range []bool{false, true} {
		name := "captured_guard"
		if legacy {
			name = "pre_guard_bindings"
		}
		t.Run(name, func(t *testing.T) { checkFreshSessionCannotReuseErasedTranscript(t, postgres, legacy) })
	}
}

func checkFreshSessionCannotReuseErasedTranscript(t *testing.T, postgres, legacy bool) {
	repo, tasks := currentTargetDatabaseFixture(t, postgres)
	page, err := repo.ListUsageCollectionTargets(t.Context(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Len(t, page.Targets, 1)
	require.Equal(t, "eligible", page.Targets[0].SourceScopeStatus)
	if legacy {
		_, err = repo.db.Exec(`DELETE FROM usage_collection_source_guards`)
		require.NoError(t, err)
	}
	_, err = repo.db.Exec(`UPDATE workspaces SET org_id='org-b' WHERE id='ws-1'`)
	require.NoError(t, err)
	require.NoError(t, repo.EraseOrganizationUsage(t.Context(), "org-a"))
	var count int
	require.NoError(t, repo.db.Get(&count, `SELECT COUNT(*) FROM usage_collection_bindings`))
	require.Zero(t, count, "erasure must remove old recoverable binding data")
	createCurrentTargetSession(t, tasks, "fresh-b-task", "fresh-b-session", "transcript-1")
	createCurrentTargetSession(t, tasks, "clean-b-task", "clean-b-session", "clean-transcript-b")
	page, err = repo.ListUsageCollectionTargets(t.Context(), "collector-b", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	var found, clean bool
	for _, target := range page.Targets {
		if target.TaskID == "clean-b-task" {
			clean = true
			require.Equal(t, "eligible", target.SourceScopeStatus, "a distinct new transcript keeps its normal supported path")
			require.Equal(t, "clean-transcript-b", *target.ExternalSessionID)
			observation := contractObservation(t, target.SubjectBinding, "clean-b", 1, "model", 23)
			require.Equal(t, "accepted", reportContractObservation(t, repo, "collector-b", observation).Status)
			continue
		}
		if target.TaskID != "fresh-b-task" {
			continue
		}
		found = true
		require.Equal(t, "mixed", target.SourceScopeStatus, "new session creation cannot prove ownership of an erased foreign transcript")
		require.Nil(t, target.ExternalSessionID)
		point := currentTargetForTest(t, repo, "collector-b", "ws-1", currentUsageTargetSelector{TaskID: "fresh-b-task", SessionID: "fresh-b-session", ExternalSessionID: "transcript-1"})
		require.Equal(t, "mixed", point.SourceScopeStatus)
		require.Nil(t, point.dto().ExternalSessionID)
		observation := contractObservation(t, target.SubjectBinding, "foreign-reuse", 1, "model", 123)
		require.Equal(t, "rejected", reportContractObservation(t, repo, "collector-b", observation).Status)
	}
	require.True(t, found)
	require.True(t, clean)
}

func TestUsageHistoryCurrentTargetE2EResetReusesFixtureTranscript(t *testing.T) {
	runCurrentTargetResetSource(t, false)
}

func TestUsageHistoryPostgresCurrentTargetE2EResetReusesFixtureTranscript(t *testing.T) {
	runCurrentTargetResetSource(t, true)
}

func runCurrentTargetResetSource(t *testing.T, postgres bool) {
	repo, tasks := currentTargetDatabaseFixture(t, postgres)
	old := currentTargetForTest(t, repo, "collector-a", "ws-1", currentUsageTargetSelector{SessionID: "session-1"})
	require.NoError(t, tasks.DeleteTask(t.Context(), "task-1"))
	require.NoError(t, repo.ResetWorkspaceUsageHistory(t.Context(), "ws-1"))
	_, err := lookupCurrentTargetForTest(t, t.Context(), repo, "collector-a", "ws-1", currentUsageTargetSelector{SubjectBinding: old.BindingID})
	require.ErrorIs(t, err, ErrUsageNotVisible, "reset must revoke old in-flight binding authority")
	createCurrentTargetSession(t, tasks, "reset-task", "reset-session", "transcript-1")
	fresh := currentTargetForTest(t, repo, "collector-a", "ws-1", currentUsageTargetSelector{SessionID: "reset-session"})
	require.Equal(t, "eligible", fresh.SourceScopeStatus, "the gated E2E path permits a deterministic fixture transcript to be reused")
	require.NotEqual(t, old.SubjectID, fresh.SubjectID)
	require.NotEqual(t, old.BindingID, fresh.BindingID)
	require.Equal(t, "accepted", reportContractObservation(t, repo, "collector-a", contractObservation(t, fresh.BindingID, "reset-reuse", 1, "model", 1)).Status)
	require.Equal(t, "rejected", reportContractObservation(t, repo, "collector-a", contractObservation(t, old.BindingID, "old-reset-write", 1, "model", 99)).Status)
	createCurrentTargetSession(t, tasks, "reset-clean-task", "reset-clean-session", "reset-new-transcript")
	clean := currentTargetForTest(t, repo, "collector-a", "ws-1", currentUsageTargetSelector{SessionID: "reset-clean-session"})
	require.Equal(t, "eligible", clean.SourceScopeStatus)
}

func TestUsageHistoryProductionWorkspaceEraseKeepsSourceGuards(t *testing.T) {
	runProductionWorkspaceEraseSource(t, false)
}

func TestUsageHistoryPostgresProductionWorkspaceEraseKeepsSourceGuards(t *testing.T) {
	runProductionWorkspaceEraseSource(t, true)
}

func runProductionWorkspaceEraseSource(t *testing.T, postgres bool) {
	repo, tasks := currentTargetDatabaseFixture(t, postgres)
	_, _, err := tasks.DeleteWorkspaceCascade(t.Context(), "ws-1")
	require.NoError(t, err)
	var guards, live int
	require.NoError(t, repo.db.Get(&guards, repo.db.Rebind(`SELECT COUNT(*) FROM usage_collection_source_guards WHERE source_hash = ?`), usageSourceGuardHash("usage-transcript", "transcript-1")))
	require.Equal(t, 1, guards)
	require.NoError(t, repo.db.Get(&live, `SELECT COUNT(*) FROM task_sessions`))
	require.Zero(t, live, "the retained guard must work after live metadata has disappeared")
	require.NoError(t, tasks.CreateWorkspace(t.Context(), &taskmodels.Workspace{ID: "ws-b", Name: "Fresh B", OrgID: "org-b"}))
	createCurrentTargetSessionInWorkspace(t, tasks, "ws-b", "fresh-b-task", "fresh-b-session", "transcript-1")
	unsafe := currentTargetForTest(t, repo, "collector-b", "ws-b", currentUsageTargetSelector{SessionID: "fresh-b-session"})
	require.Equal(t, "mixed", unsafe.SourceScopeStatus, "production erase never reopens old transcript ownership")
	require.Nil(t, unsafe.dto().ExternalSessionID)
}
