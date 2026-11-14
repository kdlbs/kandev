package sqlite

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func sharedWorkSession(t *testing.T, repo *Repository) {
	t.Helper()
	_, err := repo.db.Exec(`INSERT INTO tasks (id,workspace_id,board_id,title,created_at,updated_at)
		VALUES ('task-shared','ws-1','board-1','Shared','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`)
	require.NoError(t, err)
	_, err = repo.db.Exec(`INSERT INTO task_sessions (id,task_id,agent_profile_id,state,started_at,updated_at,metadata)
		VALUES ('session-shared','task-shared','agent-1','RUNNING','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z','{"acp":{"session_id":"transcript-1"}}')`)
	require.NoError(t, err)
	captureFixtureSessionBirth(t, repo, "task-shared", "session-shared")
}

// @covers AC-COSTS-USAGE-HISTORY-003.2 AC-COSTS-USAGE-HISTORY-002.3
func TestUsageHistorySharedTranscriptSubjectsContributeOnce(t *testing.T) {
	for _, collector := range []string{"collector-a", "collector-b"} {
		for _, dated := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/dated=%t", collector, dated), func(t *testing.T) {
				repo, api := historyContractFixture(t)
				sharedWorkSession(t, repo)
				page, err := api.ListUsageCollectionTargets(t.Context(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
				require.NoError(t, err)
				require.Len(t, page.Targets, 2)
				_, err = api.ListUsageCollectionTargets(t.Context(), collector, pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
				require.NoError(t, err)
				for i, target := range page.Targets {
					item := contractObservation(t, target.SubjectBinding, fmt.Sprintf("shared-%d", i), 1, "model", 100)
					if dated {
						item.Coverage = &pluginsdk.UsageCoverageV1{CoverageStart: stringPtr("2026-01-02T00:00:00Z"), CoverageEnd: stringPtr("2026-01-03T00:00:00Z"), SourceDate: stringPtr("2026-01-02"), SourceTimezone: stringPtr("UTC")}
					}
					sealContractObservation(t, &item)
					source := "collector-a"
					if i == 1 {
						source = collector
					}
					require.Equal(t, "accepted", reportContractObservation(t, api, source, item).Status)
				}
				completeFixtureProjection(t, repo)
				facts, _, err := repo.ListUsageCanonicalFacts(t.Context(), models.SessionUsageFilter{WorkspaceID: "ws-1"})
				require.NoError(t, err)
				require.Len(t, facts, 1, "the same approved transcript must not multiply across subjects")
				require.EqualValues(t, 100, *facts[0].Measurement.TotalTokens)
				require.Empty(t, facts[0].Measurement.TaskID, "a whole shared transcript has no exclusive task allocation")
				require.Empty(t, facts[0].Measurement.SessionID)
				require.Empty(t, facts[0].TokenSource.ActorID)
			})
		}
	}
}

func admitSharedWorkActor(t *testing.T, repo *Repository, actor string, at time.Time) {
	t.Helper()
	tx, err := repo.db.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	require.NoError(t, repo.AdmitBindingTx(t.Context(), tx, usagehistory.Binding{ID: actor, Kind: usagehistory.BindingKindTurn,
		WorkspaceID: "ws-1", TaskID: "task-1", TaskIncarnationID: "2026-01-01T00:00:00Z", SessionID: "session-1", SessionIncarnationID: "session-1",
		ActorID: actor, ActorStatus: "attributed", ActorKind: "user", ActorSource: "authenticated-turn", CapturedAt: at}))
	require.NoError(t, tx.Commit())
}

func TestUsageHistoryOldDiscoveryBindingCannotFreezeAggregateActor(t *testing.T) {
	repo, api := historyContractFixture(t)
	admitSharedWorkActor(t, repo, "actor-a", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	page, err := api.ListUsageCollectionTargets(t.Context(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Equal(t, "attributed", page.Targets[0].ActorAttributionStatus)
	admitSharedWorkActor(t, repo, "actor-b", time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC))
	for _, dated := range []bool{false, true} {
		item := contractObservation(t, page.Targets[0].SubjectBinding, fmt.Sprintf("after-second-actor-%t", dated), 1, "model", 100)
		if dated {
			item.Coverage = &pluginsdk.UsageCoverageV1{CoverageStart: stringPtr("2026-01-02T00:00:00Z"), CoverageEnd: stringPtr("2026-01-03T00:00:00Z")}
		}
		sealContractObservation(t, &item)
		receipt := reportContractObservation(t, api, "collector-a", item)
		require.Equal(t, "accepted", receipt.Status)
		require.Equal(t, "mixed", receipt.ActorAttributionStatus, "discovery actor evidence is not future aggregate authority")
	}
}

func TestUsageHistoryFreshBindingCannotReassignTransferredTranscript(t *testing.T) {
	repo, api := historyContractFixtureInOrg(t, "org-a")
	first, err := api.ListUsageCollectionTargets(t.Context(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	prior := contractObservation(t, first.Targets[0].SubjectBinding, "prior", 1, "model", 100)
	require.Equal(t, "accepted", reportContractObservation(t, api, "collector-a", prior).Status)
	_, err = repo.db.Exec(`UPDATE workspaces SET org_id='org-b' WHERE id='ws-1'`)
	require.NoError(t, err)
	fresh, err := api.ListUsageCollectionTargets(t.Context(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	for _, target := range fresh.Targets {
		item := contractObservation(t, target.SubjectBinding, "fresh-binding-old-cumulative", 1, "model", 100)
		require.Equal(t, "rejected", reportContractObservation(t, api, "collector-a", item).Status, "fresh bindings cannot authorize another scope's cumulative source")
	}
	var leaked int
	require.NoError(t, repo.db.Get(&leaked, `SELECT COUNT(*) FROM session_usage_measurements m JOIN usage_scopes s ON s.scope_id=m.scope_id WHERE s.organization_id='org-b'`))
	require.Zero(t, leaked)
}

func TestUsageHistoryTargetSmallPageHasBoundedSQLWork(t *testing.T) {
	repo, api := historyContractFixture(t)
	for i := range 205 {
		_, err := repo.db.Exec(`INSERT INTO task_sessions (id,task_id,agent_profile_id,state,started_at,updated_at)
			VALUES (?, 'task-1','agent-1','COMPLETED','2020-01-01T00:00:00Z','2020-01-01T00:00:00Z')`, fmt.Sprintf("archive-%04d", i))
		require.NoError(t, err)
	}
	var selects atomic.Int64
	var before int
	require.NoError(t, repo.db.Get(&before, `SELECT COUNT(DISTINCT session_id) FROM usage_collection_bindings`))
	conn, err := repo.db.Connx(context.Background())
	require.NoError(t, err)
	require.NoError(t, conn.Raw(func(raw any) error {
		raw.(*sqlite3.SQLiteConn).RegisterAuthorizer(func(op int, _, _, _ string) int {
			if op == sqlite3.SQLITE_SELECT {
				selects.Add(1)
			}
			return sqlite3.SQLITE_OK
		})
		return nil
	}))
	require.NoError(t, conn.Close())
	page, err := api.ListUsageCollectionTargets(t.Context(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1", Limit: 1})
	require.NoError(t, err)
	require.True(t, page.Preparing)
	require.Empty(t, page.SnapshotID)
	require.Empty(t, page.Targets)
	require.LessOrEqual(t, selects.Load(), int64(100), "one small page must not issue SQL for the complete session archive")
	var captured int
	require.NoError(t, repo.db.Get(&captured, `SELECT COUNT(DISTINCT session_id) FROM usage_collection_bindings`))
	require.LessOrEqual(t, captured-before, 1)
}
