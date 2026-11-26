package sqlite

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	analyticssqlite "github.com/kandev/kandev/internal/analytics/repository/sqlite"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestTransferTaskPreservesCapturedUsageOwnership(t *testing.T) {
	runTransferTaskCapturedUsage(t, false)
}

func TestPostgresTransferTaskPreservesCapturedUsageOwnership(t *testing.T) {
	runTransferTaskCapturedUsage(t, true)
}

func runTransferTaskCapturedUsage(t *testing.T, postgres bool) {
	t.Helper()
	repo, analytics, task := newTransferCapturedUsageFixture(t, postgres)
	before := snapshotTransferCapturedUsage(t, repo)
	assertTransferCapturedUsageAmounts(t, repo)
	command := taskTransferCommand(task)
	receipt, err := repo.TransferTask(t.Context(), command)
	require.NoError(t, err)
	require.Equal(t, "ws-destination", receipt.DestinationWorkspaceID)
	require.Equal(t, 1, receipt.PreservationCounts["session_usage_measurements"])
	require.Equal(t, 1, receipt.PreservationCounts["session_usage_buckets"])
	require.Equal(t, before, snapshotTransferCapturedUsage(t, repo), "captured accounting is immutable during a task transfer")
	assertTransferCapturedUsageAmounts(t, repo)
	stored, err := repo.GetTask(t.Context(), task.ID)
	require.NoError(t, err)
	require.Equal(t, "ws-destination", stored.WorkspaceID)
	var workspace string
	require.NoError(t, repo.db.Get(&workspace, repo.db.Rebind(`SELECT workspace_id FROM task_status_summaries WHERE task_id = ?`), task.ID))
	require.Equal(t, "ws-destination", workspace, "live workspace routing still follows the task")
	replay, err := repo.TransferTask(t.Context(), command)
	require.NoError(t, err)
	require.True(t, replay.IdempotentReplay)
	require.Equal(t, receipt.OperationID, replay.OperationID)
	require.Equal(t, before, snapshotTransferCapturedUsage(t, repo))
	page, err := analytics.ListUsageCollectionTargets(t.Context(), "captured-transfer", pluginsdk.UsageCollectionTargetQueryV1{
		WorkspaceID: "ws-source", CurrentTarget: &pluginsdk.UsageCollectionCurrentTargetQueryV1{TaskID: task.ID},
	})
	require.NoError(t, err)
	require.True(t, page.CurrentTarget)
	require.Empty(t, page.Targets, "retained ownership must not authorize a current target in the old workspace")
}

func newTransferCapturedUsageFixture(t *testing.T, postgres bool) (*Repository, *analyticssqlite.Repository, *models.Task) {
	t.Helper()
	var repo *Repository
	if postgres {
		db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
		var err error
		repo, err = NewWithDB(db, db, nil)
		require.NoError(t, err)
	} else {
		repo = newUsageEventsTestRepo(t)
	}
	analytics, err := analyticssqlite.NewWithDB(repo.db, repo.db)
	require.NoError(t, err)
	repo.SetUsageHistoryRetainer(analytics)
	ctx := t.Context()
	for _, side := range []string{"source", "destination"} {
		require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
			ID: "ws-" + side, Name: side, OwnerID: "owner-1", OrgID: "org-" + side,
		}))
		require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-" + side, WorkspaceID: "ws-" + side, Name: side}))
		mustExecTransferTest(t, repo, `INSERT INTO workflow_steps (id, workflow_id, name, position) VALUES (?, ?, 'Work', 0)`,
			"step-"+side+"-work", "wf-"+side)
	}
	task := &models.Task{ID: "task-transfer", WorkspaceID: "ws-source", WorkflowID: "wf-source",
		WorkflowStepID: "step-source-work", Title: "Captured usage transfer"}
	require.NoError(t, repo.CreateTask(ctx, task))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "captured-session", TaskID: task.ID, StartedAt: time.Now().UTC(),
		Metadata: map[string]any{"acp": map[string]any{"session_id": "captured-transcript"}},
	}))
	seedTransferCapturedNative(t, repo, task.ID)
	seedTransferCapturedExternal(t, analytics)
	mustExecTransferTest(t, repo, `INSERT INTO task_status_summaries (task_id, workspace_id, revision, summary, updated_at)
		VALUES (?, 'ws-source', 1, '{}', CURRENT_TIMESTAMP)`, task.ID)
	task, err = repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	return repo, analytics, task
}

func seedTransferCapturedNative(t *testing.T, repo *Repository, taskID string) {
	t.Helper()
	now := time.Now().UTC()
	turn := &models.Turn{ID: "captured-turn", TaskID: taskID, TaskSessionID: "captured-session",
		StartedAt: now, CreatedAt: now, UpdatedAt: now}
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "captured-user", Role: authn.RoleMember})
	require.NoError(t, repo.CreateTurn(ctx, turn))
	require.NotEmpty(t, turn.UsageBindingID)
	event := newTestUsageEvent("captured-native", taskID, "captured-session")
	event.TurnID, event.UsageBindingID = turn.ID, turn.UsageBindingID
	event.CostSource = "provider_reported"
	require.NoError(t, repo.CreateTaskUsageEvent(ctx, event))
}

func seedTransferCapturedExternal(t *testing.T, repo *analyticssqlite.Repository) {
	t.Helper()
	page, err := repo.ListUsageCollectionTargets(t.Context(), "captured-transfer", pluginsdk.UsageCollectionTargetQueryV1{
		WorkspaceID: "ws-source", CurrentTarget: &pluginsdk.UsageCollectionCurrentTargetQueryV1{SessionID: "captured-session"},
	})
	require.NoError(t, err)
	require.True(t, page.CurrentTarget)
	require.Len(t, page.Targets, 1)
	require.Equal(t, "eligible", page.Targets[0].SourceScopeStatus)
	total, cost, currency := int64(900), int64(73), "USD"
	observation := pluginsdk.UsageObservationV1{
		SubjectBinding: page.Targets[0].SubjectBinding, ItemID: "captured-external", SourceRecordID: "captured-external",
		Revision: 1, Tokens: &pluginsdk.UsageTokenAmountsV1{TotalTokens: &total},
		Cost: &pluginsdk.UsageCostV1{Amount: &cost, Currency: &currency, Basis: pluginsdk.UsageCostBasisReported},
	}
	observation.PayloadDigest, err = observation.ComputePayloadDigestV1()
	require.NoError(t, err)
	result, err := repo.ReportUsageObservations(t.Context(), "captured-transfer", pluginsdk.UsageObservationBatchV1{
		SchemaVersion: pluginsdk.UsageObservationSchemaVersionV1, Observations: []pluginsdk.UsageObservationV1{observation},
	})
	require.NoError(t, err)
	require.Len(t, result.Results, 1)
	require.Equal(t, pluginsdk.UsageObservationStatusAccepted, result.Results[0].Status)
}

func snapshotTransferCapturedUsage(t *testing.T, repo *Repository) map[string]json.RawMessage {
	t.Helper()
	tables, err := repo.transferRelationTableNames(context.Background())
	require.NoError(t, err)
	snapshot := make(map[string]json.RawMessage)
	for _, table := range tables {
		if !strings.HasPrefix(table, "usage_") && table != "session_usage_measurements" && table != "session_usage_buckets" {
			continue
		}
		rows, err := repo.db.Queryx(`SELECT * FROM ` + table + ` ORDER BY 1, 2`)
		require.NoError(t, err)
		var data [][]any
		for rows.Next() {
			row, err := rows.SliceScan()
			require.NoError(t, err)
			data = append(data, row)
		}
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())
		snapshot[table], err = json.Marshal(data)
		require.NoError(t, err)
	}
	return snapshot
}

func assertTransferCapturedUsageAmounts(t *testing.T, repo *Repository) {
	t.Helper()
	type retainedAmount struct {
		Workspace string `db:"workspace_id"`
		Org       string `db:"organization_id"`
		Actor     string `db:"actor_id"`
		Tokens    int64  `db:"total_tokens"`
		Cost      int64  `db:"cost_subcents"`
	}
	for _, test := range []struct {
		table string
		want  retainedAmount
	}{
		{"session_usage_buckets", retainedAmount{"ws-source", "org-source", "captured-user", 155, 42}},
		{"session_usage_measurements", retainedAmount{"ws-source", "org-source", "captured-user", 900, 73}},
	} {
		var got retainedAmount
		require.NoError(t, repo.db.Get(&got, `SELECT b.workspace_id, s.organization_id, b.actor_id, b.total_tokens, b.cost_subcents
			FROM `+test.table+` b JOIN usage_scopes s ON s.scope_id = b.scope_id`))
		require.Equal(t, test.want, got, test.table)
	}
}

func TestTransferTaskCapturedUsageStillRejectsUnknownWorkspaceOwner(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		name := "SQLite"
		if postgres {
			name = "Postgres"
		}
		t.Run(name, func(t *testing.T) {
			repo, _, task := newTransferCapturedUsageFixture(t, postgres)
			before := snapshotTransferCapturedUsage(t, repo)
			mustExecTransferTest(t, repo, `CREATE TABLE usage_unapproved_owner (task_id TEXT NOT NULL, workspace_id TEXT NOT NULL)`)
			mustExecTransferTest(t, repo, `INSERT INTO usage_unapproved_owner VALUES (?, 'ws-source')`, task.ID)
			_, err := repo.TransferTask(t.Context(), taskTransferCommand(task))
			require.ErrorIs(t, err, repoerrors.ErrTaskTransferConflict)
			require.ErrorContains(t, err, `"usage_unapproved_owner" has unapproved workspace ownership`)
			mustExecTransferTest(t, repo, `DROP TABLE usage_unapproved_owner`)
			require.Equal(t, before, snapshotTransferCapturedUsage(t, repo))
			stored, err := repo.GetTask(t.Context(), task.ID)
			require.NoError(t, err)
			require.Equal(t, "ws-source", stored.WorkspaceID)
		})
	}
}
