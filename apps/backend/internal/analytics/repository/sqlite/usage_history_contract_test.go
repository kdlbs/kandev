package sqlite

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kandev/kandev/internal/db"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

type retainedUsageAPI interface {
	ListUsageCollectionTargets(context.Context, string, pluginsdk.UsageCollectionTargetQueryV1) (pluginsdk.UsageCollectionTargetPageV1, error)
	ReportUsageObservations(context.Context, string, pluginsdk.UsageObservationBatchV1) (pluginsdk.UsageObservationBatchResultV1, error)
}

func historyContractFixture(t *testing.T) (*Repository, retainedUsageAPI) {
	return historyContractFixtureInOrg(t, "")
}

func historyContractFixtureInOrg(t *testing.T, org string) (*Repository, retainedUsageAPI) {
	t.Helper()
	database := createTestDB(t)
	seedLegacyUsageFixtures(t, database)
	_, err := database.Exec(`UPDATE workspaces SET org_id = ? WHERE id = 'ws-1'`, org)
	require.NoError(t, err)
	repo, err := NewWithDB(database, database)
	require.NoError(t, err)
	captureFixtureSessionBirth(t, repo, "task-1", "session-1")
	completeUsageSourceAdoptionForTest(t, repo)
	api, ok := any(repo).(retainedUsageAPI)
	require.True(t, ok, "repository must implement the retained observation contract")
	return repo, api
}

func captureFixtureSessionBirth(t *testing.T, repo *Repository, task, session string) {
	t.Helper()
	tx, err := repo.db.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	require.NoError(t, repo.CaptureUsageCollectionSessionTx(t.Context(), tx, task, session))
	require.NoError(t, tx.Commit())
}

func historyRetainedContractFixture(t *testing.T) (*Repository, retainedUsageAPI) {
	t.Helper()
	raw, err := db.OpenSQLite(filepath.Join(t.TempDir(), "history.db"))
	require.NoError(t, err)
	database := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	tasks, err := tasksqlite.NewWithDB(database, database, nil)
	require.NoError(t, err)
	repo, err := NewWithDB(database, database)
	require.NoError(t, err)
	tasks.SetUsageHistoryRetainer(repo)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, tasks.CreateWorkspace(t.Context(), &taskmodels.Workspace{ID: "ws-1", Name: "Usage"}))
	require.NoError(t, tasks.CreateTask(t.Context(), &taskmodels.Task{ID: "task-1", WorkspaceID: "ws-1", Title: "Task", CreatedAt: created, UpdatedAt: created}))
	require.NoError(t, tasks.CreateTaskSession(t.Context(), &taskmodels.TaskSession{ID: "session-1", TaskID: "task-1", StartedAt: created}))
	completeUsageSourceAdoptionForTest(t, repo)
	return repo, repo
}

func contractObservation(t *testing.T, binding, id string, revision int64, model string, amount int64) pluginsdk.UsageObservationV1 {
	t.Helper()
	item := pluginsdk.UsageObservationV1{
		SubjectBinding: binding, ItemID: id, SourceRecordID: id, SourceVersion: "replacement-fixture-v1", Revision: revision,
		ModelID: &model, Tokens: &pluginsdk.UsageTokenAmountsV1{InputTokens: &amount, TotalTokens: &amount},
		Quality: &pluginsdk.UsageObservationQualityV1{TokenCoverageComplete: true},
	}
	sealContractObservation(t, &item)
	return item
}

func sealContractObservation(t *testing.T, item *pluginsdk.UsageObservationV1) {
	t.Helper()
	digest, err := item.ComputePayloadDigestV1()
	require.NoError(t, err)
	item.PayloadDigest = digest
}

func reportContractObservation(t *testing.T, api retainedUsageAPI, collector string, item pluginsdk.UsageObservationV1) pluginsdk.UsageObservationWriteResultV1 {
	t.Helper()
	result, err := api.ReportUsageObservations(context.Background(), collector, pluginsdk.UsageObservationBatchV1{SchemaVersion: 1, Observations: []pluginsdk.UsageObservationV1{item}})
	require.NoError(t, err)
	require.Len(t, result.Results, 1)
	return result.Results[0]
}

func finishFixtureRetention(t *testing.T, repo *Repository) {
	t.Helper()
	for range 100 {
		done, err := repo.BackfillUsageHistory(t.Context(), 200)
		require.NoError(t, err)
		if done {
			return
		}
	}
	t.Fatal("all retention watermarks must finish before saved canonical reads")
}

// @covers AC-COSTS-USAGE-HISTORY-004.3 AC-COSTS-USAGE-HISTORY-003.2
func TestUsageHistoryContractImmutableItemCorrectionAndCanonicalRead(t *testing.T) {
	repo, api := historyRetainedContractFixture(t)
	ctx := context.Background()
	page, err := api.ListUsageCollectionTargets(ctx, "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1", Limit: 10})
	require.NoError(t, err)
	require.NotEmpty(t, page.SnapshotID)
	require.Len(t, page.Targets, 1)
	item := contractObservation(t, page.Targets[0].SubjectBinding, "stable-item", 1, "unknown-label", 100)
	first := reportContractObservation(t, api, "collector-a", item)
	require.Equal(t, "accepted", first.Status)
	require.Equal(t, item.Revision, first.CurrentRevision)
	require.Equal(t, item.PayloadDigest, first.CurrentDigest)
	require.Equal(t, "unknown", first.ActorAttributionStatus)
	var liveLinks int
	require.NoError(t, repo.db.Get(&liveLinks, `SELECT COUNT(*) FROM usage_live_links WHERE live_task_id = 'task-1' AND live_session_id = 'session-1'`))
	require.Equal(t, 1, liveLinks)

	item.ModelID = stringPtr("exact-model")
	item.ModelProvider = stringPtr("provider")
	item.Revision = 2
	item.Tokens.InputTokens, item.Tokens.TotalTokens = models.Int64(80), models.Int64(80)
	sealContractObservation(t, &item)
	corrected := reportContractObservation(t, api, "collector-a", item)
	require.Equal(t, "accepted", corrected.Status)
	require.Equal(t, item, *corrected.CurrentObservation)
	replay := reportContractObservation(t, api, "collector-a", item)
	require.Equal(t, "unchanged", replay.Status)
	completeFixtureProjection(t, repo)
	preparingRows, state, err := repo.ListRetainedCanonicalUsagePage(ctx, models.SessionUsageFilter{WorkspaceID: "ws-1", SessionIDs: []string{"session-1"}})
	require.NoError(t, err)
	require.Empty(t, preparingRows)
	require.Equal(t, "preparing", state.Status)
	require.Empty(t, state.Generation)
	finishFixtureRetention(t, repo)
	completeFixtureProjection(t, repo)
	rows, err := repo.ListCanonicalSessionUsageMeasurements(ctx, models.SessionUsageFilter{WorkspaceID: "ws-1", SessionIDs: []string{"session-1"}})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 80, *rows[0].TotalTokens)
	require.Equal(t, "exact-model", rows[0].Model)
}

// @covers AC-COSTS-USAGE-HISTORY-004.5 AC-COSTS-USAGE-HISTORY-002.1
func TestUsageHistoryContractRetainedCorrectionRequiresIssuedBinding(t *testing.T) {
	repo, api := historyContractFixture(t)
	ctx := context.Background()
	page, err := api.ListUsageCollectionTargets(ctx, "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	item := contractObservation(t, page.Targets[0].SubjectBinding, "retained-item", 1, "model", 20)
	require.Equal(t, "accepted", reportContractObservation(t, api, "collector-a", item).Status)
	tx, err := repo.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	_, err = tx.Exec(`DELETE FROM task_sessions WHERE task_id = 'task-1'`)
	require.NoError(t, err)
	_, err = tx.Exec(`DELETE FROM tasks WHERE id = 'task-1'`)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	item.Revision = 2
	sealContractObservation(t, &item)
	require.Equal(t, "accepted", reportContractObservation(t, api, "collector-a", item).Status)
	require.Equal(t, "rejected", reportContractObservation(t, api, "unissued-collector", item).Status)
	item.SubjectBinding = "forged-orphan"
	sealContractObservation(t, &item)
	require.Equal(t, "rejected", reportContractObservation(t, api, "collector-a", item).Status)
}

func stringPtr(v string) *string { return &v }

// @covers AC-COSTS-USAGE-HISTORY-004.6 AC-COSTS-USAGE-HISTORY-002.5
func TestUsageHistoryContractSnapshotSurvivesAdditionDeletionAndResume(t *testing.T) {
	repo, api := historyContractFixture(t)
	_, err := repo.db.Exec(`UPDATE task_sessions SET metadata = '{"acp":{"session_id":"original-context"}}' WHERE id = 'session-1'`)
	require.NoError(t, err)
	for _, id := range []string{"session-2", "session-3"} {
		_, err := repo.db.Exec(`INSERT INTO task_sessions (id, task_id, agent_profile_id, started_at, updated_at) VALUES (?, 'task-1', 'agent', ?, ?)`, id, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z")
		require.NoError(t, err)
		captureFixtureSessionBirth(t, repo, "task-1", id)
	}
	first, err := api.ListUsageCollectionTargets(context.Background(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1", Limit: 1})
	require.NoError(t, err)
	for range maxSessionUsageBatch {
		if !first.Preparing {
			break
		}
		first, err = api.ListUsageCollectionTargets(t.Context(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1", Limit: 1})
		require.NoError(t, err)
	}
	require.False(t, first.Preparing, "bounded target preparation must finish for the three-session fixture")
	require.True(t, first.HasMore)
	require.NotNil(t, first.Targets[0].SessionStartedAt)
	require.Nil(t, first.Targets[0].CoverageStart)
	_, err = repo.db.Exec(`DELETE FROM task_sessions WHERE id = 'session-3'`)
	require.NoError(t, err)
	_, err = repo.db.Exec(`UPDATE task_sessions SET metadata = '{"acp":{"session_id":"resumed-context"}}', completed_at = ? WHERE id = 'session-2'`, "2026-01-04T00:00:00Z")
	require.NoError(t, err)
	_, err = repo.db.Exec(`INSERT INTO task_sessions (id, task_id, agent_profile_id, started_at, updated_at) VALUES ('session-0', 'task-1', 'agent', ?, ?)`, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z")
	require.NoError(t, err)
	seen := map[string]bool{first.Targets[0].TaskID + ":" + *first.Targets[0].SessionID: true}
	cursor := first.NextCursor
	for cursor != "" {
		page, err := api.ListUsageCollectionTargets(context.Background(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1", Limit: 1, SnapshotID: first.SnapshotID, Cursor: cursor})
		require.NoError(t, err)
		require.Equal(t, first.SnapshotUpperBound, page.SnapshotUpperBound)
		for _, target := range page.Targets {
			key := target.TaskID + ":" + *target.SessionID
			require.False(t, seen[key])
			seen[key] = true
			require.NotEqual(t, "session-0", *target.SessionID)
			if *target.SessionID == "session-2" {
				require.Nil(t, target.SessionEndedAt)
				require.Nil(t, target.ExternalSessionID)
			}
		}
		if page.HasMore {
			require.NotEqual(t, cursor, page.NextCursor)
		}
		cursor = page.NextCursor
	}
	require.Len(t, seen, 3)
	newSnapshot, err := api.ListUsageCollectionTargets(context.Background(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Greater(t, newSnapshot.SnapshotUpperBound, first.SnapshotUpperBound)
	_, err = api.ListUsageCollectionTargets(context.Background(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1", SnapshotID: first.SnapshotID})
	require.ErrorIs(t, err, ErrUsageNotVisible, "a newer same-source snapshot replaces the older retained snapshot")
	_, err = api.ListUsageCollectionTargets(context.Background(), "other-collector", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1", SnapshotID: newSnapshot.SnapshotID})
	require.ErrorIs(t, err, ErrUsageNotVisible)
}

// @covers AC-COSTS-USAGE-HISTORY-004.3 AC-COSTS-USAGE-HISTORY-003.7
func TestUsageHistoryContractExplicitRetirementAndUnknownSemantics(t *testing.T) {
	repo, api := historyContractFixture(t)
	page, err := api.ListUsageCollectionTargets(context.Background(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	a := contractObservation(t, page.Targets[0].SubjectBinding, "set-a", 1, "model", 100)
	b := contractObservation(t, a.SubjectBinding, "set-b", 1, "other-model", 100)
	setID := "provider-set"
	setRev := int64(1)
	for _, item := range []*pluginsdk.UsageObservationV1{&a, &b} {
		item.SourceSetID = &setID
		item.SourceSetRevision = &setRev
		sealContractObservation(t, item)
		require.Equal(t, "accepted", reportContractObservation(t, api, "collector-a", *item).Status)
	}
	a.Revision = 2
	sealContractObservation(t, &a)
	require.Equal(t, "accepted", reportContractObservation(t, api, "collector-a", a).Status)
	var active int
	require.NoError(t, repo.db.Get(&active, `SELECT COUNT(*) FROM usage_observation_items WHERE source = 'plugin:collector-a' AND retired = 0`))
	require.Equal(t, 2, active, "partial omission preserves the previous member")
	setRev = 2
	a.Revision = 3
	sealContractObservation(t, &a)
	set := pluginsdk.UsageCompleteSetRevisionV1{SubjectBinding: a.SubjectBinding, SetID: setID, Revision: setRev}
	set.PayloadDigest, err = set.ComputePayloadDigestV1([]string{a.ItemID})
	require.NoError(t, err)
	result, err := api.ReportUsageObservations(context.Background(), "collector-a", pluginsdk.UsageObservationBatchV1{SchemaVersion: 1, Observations: []pluginsdk.UsageObservationV1{a}, CompleteSetRevision: &set})
	require.NoError(t, err)
	require.Equal(t, "accepted", result.Status)
	require.NoError(t, repo.db.Get(&active, `SELECT COUNT(*) FROM usage_observation_items WHERE source = 'plugin:collector-a' AND retired = 0`))
	require.Equal(t, 1, active)
	c := contractObservation(t, a.SubjectBinding, "replacement-item", 1, "corrected-model", 90)
	c.SupersedesItemID = &a.ItemID
	sealContractObservation(t, &c)
	require.Equal(t, "accepted", reportContractObservation(t, api, "collector-a", c).Status)
	a.Revision = 4
	sealContractObservation(t, &a)
	require.Equal(t, "conflict", reportContractObservation(t, api, "collector-a", a).Status)
	max := int64(math.MaxInt64)
	c.Revision = 2
	c.Tokens = &pluginsdk.UsageTokenAmountsV1{InputTokens: &max, CacheReadTokens: &max, TotalTokens: &max}
	c.SupersedesItemID = nil
	sealContractObservation(t, &c)
	accepted := reportContractObservation(t, api, "collector-a", c)
	require.Equal(t, "accepted", accepted.Status)
	require.Nil(t, accepted.CurrentObservation.TokenSemantics)
	require.Equal(t, max, *accepted.CurrentObservation.Tokens.TotalTokens)
	c.Revision = 3
	c.Tokens = &pluginsdk.UsageTokenAmountsV1{InputTokens: models.Int64(7), CacheReadTokens: models.Int64(8), TotalTokens: models.Int64(1)}
	c.Quality.TokenCoverageComplete = false
	sealContractObservation(t, &c)
	unknown := reportContractObservation(t, api, "collector-a", c)
	require.Equal(t, "accepted", unknown.Status, "unknown semantics cannot invent source-total/category relationships")
	require.EqualValues(t, 1, *unknown.CurrentObservation.Tokens.TotalTokens)
	c.Revision = 4
	c.TokenSemantics = &pluginsdk.UsageTokenSemanticsV1{Version: "unsupported-9"}
	sealContractObservation(t, &c)
	require.Equal(t, "unsupported_version", reportContractObservation(t, api, "collector-a", c).Status)
	tomb := pluginsdk.UsageObservationTombstoneV1{SubjectBinding: c.SubjectBinding, ItemID: c.ItemID, Revision: 4}
	tomb.PayloadDigest, err = tomb.ComputePayloadDigestV1()
	require.NoError(t, err)
	result, err = api.ReportUsageObservations(context.Background(), "collector-a", pluginsdk.UsageObservationBatchV1{SchemaVersion: 1, Tombstones: []pluginsdk.UsageObservationTombstoneV1{tomb}})
	require.NoError(t, err)
	require.Equal(t, "accepted", result.Results[0].Status)
}

// @covers AC-COSTS-USAGE-HISTORY-002.5 AC-COSTS-USAGE-HISTORY-002.1
func TestUsageHistoryContractScopeTransferAndErasureFailClosed(t *testing.T) {
	repo, api := historyContractFixtureInOrg(t, "org-a")
	_, err := repo.db.Exec(`CREATE TABLE task_usage_events (id INTEGER PRIMARY KEY, usage_event_id TEXT, usage_binding_id TEXT, task_id TEXT)`)
	require.NoError(t, err)
	page, err := api.ListUsageCollectionTargets(context.Background(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	item := contractObservation(t, page.Targets[0].SubjectBinding, "owned-item", 1, "model", 3)
	require.Equal(t, "accepted", reportContractObservation(t, api, "collector-a", item).Status)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "foreign", OrgID: "org-b"})
	_, err = api.ListUsageCollectionTargets(ctx, "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.ErrorIs(t, err, ErrUsageNotVisible)
	_, err = repo.db.Exec(`UPDATE workspaces SET org_id = 'org-b' WHERE id = 'ws-1'`)
	require.NoError(t, err)
	item.Revision = 2
	sealContractObservation(t, &item)
	require.Equal(t, "rejected", reportContractObservation(t, api, "collector-a", item).Status)
	require.NoError(t, repo.EraseOrganizationUsage(context.Background(), "org-a"))
	for _, table := range []string{"usage_collection_bindings", "usage_collection_grants", "usage_collection_snapshots", "usage_observation_items", "usage_observation_sets"} {
		var count int
		require.NoError(t, repo.db.Get(&count, `SELECT COUNT(*) FROM `+table))
		require.Zero(t, count, table)
	}
}

// @covers AC-COSTS-USAGE-HISTORY-002.3 AC-COSTS-USAGE-HISTORY-003.4
func TestUsageHistorySelectionIndependentTokenCostSourcesAndActors(t *testing.T) {
	repo, api := historyContractFixture(t)
	ctx := context.Background()
	point := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	tx, err := repo.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	for _, actor := range []string{"actor-a", "actor-b"} {
		require.NoError(t, repo.AdmitBindingTx(ctx, tx, usagehistory.Binding{ID: actor, Kind: usagehistory.BindingKindExecution, WorkspaceID: "ws-1", TaskID: "task-1", TaskIncarnationID: "2026-01-01T00:00:00Z", SessionID: "session-1", SessionIncarnationID: "session-1", ActorID: actor, ActorStatus: "attributed", ActorKind: "user", ActorSource: "authenticated-execution", CapturedAt: point}))
	}
	require.NoError(t, repo.RetainNativeUsageTx(ctx, tx, usagehistory.NativeUsage{BindingID: "actor-a", UsageEventID: "native-actor-a", TaskID: "task-1", SessionID: "session-1", Model: "model", Provider: "provider", InputTokens: 100, TotalTokens: 100, UsageCompleteness: "complete", OccurredAt: point, CollectedAt: point}))
	require.NoError(t, tx.Commit())
	page, err := api.ListUsageCollectionTargets(ctx, "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Equal(t, "mixed", page.Targets[0].ActorAttributionStatus)
	cost := contractObservation(t, page.Targets[0].SubjectBinding, "reported-cost", 1, "model", 150)
	cost.ModelProvider = stringPtr("provider")
	cost.WorkIdentity = &pluginsdk.UsageWorkIdentityV1{Scheme: "provider_event", ID: "native-actor-a"}
	cost.Quality.TokenCoverageComplete = false
	cost.Quality.CostCoverageComplete = true
	cost.Cost = &pluginsdk.UsageCostV1{Amount: models.Int64(500), Currency: stringPtr("USD"), Basis: pluginsdk.UsageCostBasisReported}
	cost.Coverage = &pluginsdk.UsageCoverageV1{OccurrenceAt: stringPtr(point.Format(time.RFC3339Nano))}
	sealContractObservation(t, &cost)
	require.Equal(t, "accepted", reportContractObservation(t, api, "collector-a", cost).Status)
	selected := selectContractPartition(t, repo, cost.ItemID)
	require.Len(t, selected.Facts, 1)
	fact := selected.Facts[0]
	require.EqualValues(t, 100, *fact.Measurement.TotalTokens)
	require.NotNil(t, fact.Measurement.CostSubcents)
	require.EqualValues(t, 500, *fact.Measurement.CostSubcents)
	require.Equal(t, "native", fact.TokenSource.Source)
	require.Equal(t, "actor-a", fact.TokenSource.ActorID)
	require.Equal(t, "plugin:collector-a", fact.CostSource.Source)
	require.Equal(t, "attributed", fact.CostSource.ActorStatus, "the exact provider event has native actor evidence")
	require.Equal(t, "actor-a", fact.CostSource.ActorID)
	assertUnallocatedAggregateCost(t, repo, cost)
	tx, err = repo.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, repo.RetainNativeUsageTx(ctx, tx, usagehistory.NativeUsage{BindingID: "actor-b", UsageEventID: "native-actor-b", TaskID: "task-1", SessionID: "session-1", Model: "model", Provider: "provider", InputTokens: 50, TotalTokens: 50, UsageCompleteness: "complete", OccurredAt: point, CollectedAt: point}))
	require.NoError(t, tx.Commit())
	require.Len(t, selectContractPartition(t, repo, cost.ItemID).Facts, 2, "distinct core events remain additive even at one occurrence point")
	spoofPage, err := api.ListUsageCollectionTargets(ctx, "native", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	spoof := contractObservation(t, spoofPage.Targets[0].SubjectBinding, "claimed-native", 999, "model", 999)
	spoof.ModelProvider = stringPtr("provider")
	spoof.WorkIdentity = cost.WorkIdentity
	spoof.Coverage = cost.Coverage
	spoof.Tokens.OutputTokens = models.Int64(0)
	spoof.Tokens.CacheReadTokens = models.Int64(0)
	spoof.Tokens.CacheWriteTokens = models.Int64(0)
	spoof.Tokens.ReasoningTokens = models.Int64(0)
	sealContractObservation(t, &spoof)
	require.Equal(t, "accepted", reportContractObservation(t, api, "native", spoof).Status)
	for _, selectedFact := range selectContractPartition(t, repo, cost.ItemID).Facts {
		if selectedFact.Key == fact.Key {
			require.EqualValues(t, 100, *selectedFact.Measurement.TotalTokens)
			require.Equal(t, "native", selectedFact.TokenSource.Source)
		}
	}
}

func TestUsageHistoryContractLegacyDTOCanBeExplicitlySuperseded(t *testing.T) {
	repo, api := historyContractFixture(t)
	page, err := api.ListUsageCollectionTargets(context.Background(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	legacy := usageFixtureMeasurement(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	legacy.Source = "plugin:collector-a"
	old, err := repo.upsertSessionUsageFixture(context.Background(), []models.SessionUsageMeasurement{legacy})
	require.NoError(t, err)
	item := contractObservation(t, page.Targets[0].SubjectBinding, "new-contract-item", 1, "corrected-model", 23)
	item.SupersedesItemID = &old[0].Measurement.ID
	sealContractObservation(t, &item)
	require.Equal(t, "accepted", reportContractObservation(t, api, "collector-a", item).Status)
	var retired int
	require.NoError(t, repo.db.Get(&retired, `SELECT retired FROM session_usage_measurements WHERE id = ?`, old[0].Measurement.ID))
	require.Equal(t, 1, retired)
	legacy.Revision++
	legacy.PayloadDigest = "later-legacy-poll"
	again, err := repo.upsertSessionUsageFixture(context.Background(), []models.SessionUsageMeasurement{legacy})
	require.NoError(t, err)
	require.Equal(t, models.UsageWriteConflict, again[0].Status)
}
