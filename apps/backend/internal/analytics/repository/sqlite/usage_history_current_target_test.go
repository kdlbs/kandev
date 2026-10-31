package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func lookupCurrentTargetForTest(t *testing.T, ctx context.Context, repo *Repository, collector, workspace string, query currentUsageTargetSelector) (*retainedTarget, error) {
	t.Helper()
	selector := pluginsdk.UsageCollectionCurrentTargetQueryV1(query)
	page, err := repo.ListUsageCollectionTargets(ctx, collector, pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: workspace, CurrentTarget: &selector})
	if err != nil {
		return nil, err
	}
	require.True(t, page.CurrentTarget)
	require.False(t, page.Preparing)
	require.False(t, page.HasMore)
	require.Empty(t, page.SnapshotID)
	require.Zero(t, page.SnapshotUpperBound)
	require.Empty(t, page.NextCursor)
	require.LessOrEqual(t, len(page.Targets), 1)
	if len(page.Targets) == 0 {
		return nil, nil
	}
	var target retainedTarget
	require.NoError(t, repo.db.GetContext(ctx, &target, repo.db.Rebind(`SELECT * FROM usage_collection_bindings WHERE binding_id = ?`), page.Targets[0].SubjectBinding))
	target.SourceScopeStatus = page.Targets[0].SourceScopeStatus
	target.ActorStatus = page.Targets[0].ActorAttributionStatus
	return &target, nil
}

func currentTargetForTest(t *testing.T, repo *Repository, collector, workspace string, query currentUsageTargetSelector) *retainedTarget {
	t.Helper()
	target, err := lookupCurrentTargetForTest(t, t.Context(), repo, collector, workspace, query)
	require.NoError(t, err)
	return target
}

func TestUsageHistoryCurrentTargetUsesExactLiveSession(t *testing.T) {
	repo, _ := historyContractFixture(t)
	target := currentTargetForTest(t, repo, "collector-a", "ws-1", currentUsageTargetSelector{TaskID: "task-1", SessionID: "session-1", ExternalSessionID: "transcript-1"})
	require.NotNil(t, target, "an exact live target must not require workspace archive preparation")
	require.Equal(t, "eligible", target.SourceScopeStatus)
	require.Equal(t, "session-1", target.SessionID)
	require.Equal(t, "transcript-1", target.ExternalSessionID)
	var snapshots, prepared int
	require.NoError(t, repo.db.Get(&snapshots, `SELECT COUNT(*) FROM usage_collection_snapshots`))
	require.NoError(t, repo.db.Get(&prepared, `SELECT COUNT(*) FROM usage_collection_preparation`))
	require.Zero(t, snapshots)
	require.Zero(t, prepared)
}

func TestUsageHistoryCurrentTargetRevalidatesIssuedBinding(t *testing.T) {
	repo, _ := historyContractFixture(t)
	first := currentTargetForTest(t, repo, "collector-a", "ws-1", currentUsageTargetSelector{SessionID: "session-1"})
	require.NotNil(t, first)
	require.Equal(t, "unknown", first.ActorStatus)
	admitSharedWorkActor(t, repo, "first-actor", time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	query := currentUsageTargetSelector{SubjectBinding: first.BindingID, ExternalSessionID: "transcript-1"}
	current := currentTargetForTest(t, repo, "collector-a", "ws-1", query)
	require.Equal(t, first.BindingID, current.BindingID)
	require.Equal(t, "attributed", current.ActorStatus)
	admitSharedWorkActor(t, repo, "second-actor", time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC))
	current = currentTargetForTest(t, repo, "collector-a", "ws-1", query)
	require.Equal(t, "mixed", current.ActorStatus)
	var stored retainedTarget
	require.NoError(t, repo.db.Get(&stored, `SELECT * FROM usage_collection_bindings WHERE binding_id = ?`, first.BindingID))
	require.Equal(t, "unknown", stored.ActorStatus, "current evidence must not rewrite an immutable issued snapshot")
	_, err := lookupCurrentTargetForTest(t, t.Context(), repo, "collector-b", "ws-1", query)
	require.ErrorIs(t, err, ErrUsageNotVisible)
	_, err = repo.db.Exec(`DELETE FROM usage_collection_grants WHERE source='plugin:collector-a' AND binding_id=?`, first.BindingID)
	require.NoError(t, err)
	_, err = lookupCurrentTargetForTest(t, t.Context(), repo, "collector-a", "ws-1", query)
	require.ErrorIs(t, err, ErrUsageNotVisible)
}

func TestUsageHistoryCurrentTargetRejectsUnknownAndAmbiguousSources(t *testing.T) {
	repo, _ := historyContractFixture(t)
	_, err := repo.db.Exec(`INSERT INTO task_sessions (id,task_id,agent_profile_id,started_at,updated_at,metadata)
		VALUES ('legacy-unknown','task-1','agent','2020-01-01T00:00:00Z','2020-01-01T00:00:00Z','{"acp":{"session_id":"unknown-transcript"}}')`)
	require.NoError(t, err)
	require.Nil(t, currentTargetForTest(t, repo, "collector-a", "ws-1", currentUsageTargetSelector{TaskID: "task-1"}))
	unknown := currentTargetForTest(t, repo, "collector-a", "ws-1", currentUsageTargetSelector{SessionID: "legacy-unknown"})
	require.NotNil(t, unknown)
	require.Equal(t, "unknown", unknown.SourceScopeStatus)
	require.Nil(t, unknown.dto().ExternalSessionID)
	known := currentTargetForTest(t, repo, "collector-a", "ws-1", currentUsageTargetSelector{SessionID: "session-1"})
	require.Equal(t, "eligible", known.SourceScopeStatus)
	sharedWorkSession(t, repo)
	require.Nil(t, currentTargetForTest(t, repo, "collector-a", "ws-1", currentUsageTargetSelector{ExternalSessionID: "transcript-1"}))
	require.Nil(t, currentTargetForTest(t, repo, "collector-a", "ws-1", currentUsageTargetSelector{SessionID: "session-1", ExternalSessionID: "wrong-transcript"}))
	_, err = repo.db.Exec(`UPDATE task_sessions SET metadata='{"acp":{"session_id":"transcript-1"},"codex_fork_source_session_id":"foreign-source"}' WHERE id='session-1'`)
	require.NoError(t, err)
	unsafe := currentTargetForTest(t, repo, "collector-a", "ws-1", currentUsageTargetSelector{SubjectBinding: known.BindingID})
	require.Equal(t, "mixed", unsafe.SourceScopeStatus)
	require.Nil(t, unsafe.dto().ExternalSessionID)
	require.Nil(t, unsafe.dto().SessionStartedAt)
	_, err = lookupCurrentTargetForTest(t, t.Context(), repo, "collector-a", "ws-1", currentUsageTargetSelector{})
	require.ErrorIs(t, err, ErrUsageNotVisible)
}

func TestUsageHistoryCurrentTargetCannotReuseTransferredSnapshot(t *testing.T) {
	repo, _ := historyContractFixtureInOrg(t, "org-a")
	first := currentTargetForTest(t, repo, "collector-a", "ws-1", currentUsageTargetSelector{SessionID: "session-1"})
	_, err := repo.db.Exec(`UPDATE workspaces SET org_id='org-b' WHERE id='ws-1'`)
	require.NoError(t, err)
	_, err = lookupCurrentTargetForTest(t, t.Context(), repo, "collector-a", "ws-1", currentUsageTargetSelector{SubjectBinding: first.BindingID})
	require.ErrorIs(t, err, ErrUsageNotVisible)
	current := currentTargetForTest(t, repo, "collector-b", "ws-1", currentUsageTargetSelector{SessionID: "session-1"})
	require.Equal(t, "mixed", current.SourceScopeStatus)
	_, err = repo.db.Exec(`UPDATE workspaces SET org_id='org-a' WHERE id='ws-1'`)
	require.NoError(t, err)
	current = currentTargetForTest(t, repo, "collector-a", "ws-1", currentUsageTargetSelector{SubjectBinding: first.BindingID})
	require.Equal(t, "mixed", current.SourceScopeStatus)
	require.Nil(t, current.dto().ExternalSessionID)
	var snapshotStatus string
	require.NoError(t, repo.db.Get(&snapshotStatus, `SELECT source_scope_status FROM usage_collection_bindings WHERE binding_id=?`, first.BindingID))
	require.Equal(t, "eligible", snapshotStatus)
	for _, ctx := range []context.Context{
		authn.WithIdentity(t.Context(), authn.Identity{UserID: "foreign", OrgID: "org-b"}),
		usagehistory.WithWorkspaceAccess(t.Context(), []string{"ws-2"}),
	} {
		_, err = repo.ListUsageCollectionTargets(ctx, "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
		require.ErrorIs(t, err, ErrUsageNotVisible)
		_, err = lookupCurrentTargetForTest(t, ctx, repo, "collector-a", "ws-1", currentUsageTargetSelector{SubjectBinding: first.BindingID})
		require.ErrorIs(t, err, ErrUsageNotVisible)
	}
}

func TestUsageHistoryCurrentTargetDistinguishesActorKinds(t *testing.T) {
	repo, _ := historyContractFixture(t)
	tx, err := repo.db.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	for _, kind := range []string{"user", "service"} {
		require.NoError(t, repo.AdmitBindingTx(t.Context(), tx, usagehistory.Binding{
			ID: "actor-" + kind, Kind: usagehistory.BindingKindTurn, WorkspaceID: "ws-1", TaskID: "task-1",
			TaskIncarnationID: "2026-01-01T00:00:00Z", SessionID: "session-1", SessionIncarnationID: "session-1",
			ActorID: "same-id", ActorKind: kind, ActorStatus: "attributed", ActorSource: "authenticated-turn",
			CapturedAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
		}))
	}
	require.NoError(t, tx.Commit())
	target := currentTargetForTest(t, repo, "collector-a", "ws-1", currentUsageTargetSelector{SessionID: "session-1"})
	require.Equal(t, "mixed", target.ActorStatus, "actor identity includes its kind")
}

func TestUsageHistoryCurrentTargetRejectsEnumerationState(t *testing.T) {
	repo, _ := historyContractFixture(t)
	selector := &pluginsdk.UsageCollectionCurrentTargetQueryV1{SessionID: "session-1"}
	for _, query := range []pluginsdk.UsageCollectionTargetQueryV1{
		{WorkspaceID: "ws-1", CurrentTarget: selector, SnapshotID: "old-snapshot"},
		{WorkspaceID: "ws-1", CurrentTarget: selector, Cursor: "old-cursor"},
		{WorkspaceID: "ws-1", CurrentTarget: &pluginsdk.UsageCollectionCurrentTargetQueryV1{}},
	} {
		_, err := repo.ListUsageCollectionTargets(t.Context(), "collector-a", query)
		require.ErrorIs(t, err, ErrUsageNotVisible)
	}
	require.Nil(t, currentTargetForTest(t, repo, "collector-a", "ws-1", currentUsageTargetSelector{SessionID: "missing-session"}))
}

func TestUsageHistoryCurrentTargetFindsUpdatedTranscriptDirectly(t *testing.T) {
	repo, tasks := currentTargetDatabaseFixture(t, false)
	require.NoError(t, tasks.UpdateSessionMetadata(t.Context(), "session-1", map[string]any{"acp": map[string]any{"session_id": "updated-transcript"}}))
	target := currentTargetForTest(t, repo, "collector-a", "ws-1", currentUsageTargetSelector{ExternalSessionID: "updated-transcript"})
	require.NotNil(t, target, "a current transcript point lookup must not depend on an old captured pointer")
	require.Equal(t, "eligible", target.SourceScopeStatus)
	require.Equal(t, "session-1", target.SessionID)
	require.Nil(t, currentTargetForTest(t, repo, "collector-a", "ws-1", currentUsageTargetSelector{ExternalSessionID: "transcript-1"}))
}
