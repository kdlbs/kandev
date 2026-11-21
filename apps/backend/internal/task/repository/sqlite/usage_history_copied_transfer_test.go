package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

func TestUsageHistoryCopiedNativeAllowsTaskWorkspaceTransfer(t *testing.T) {
	runCopiedNativeTaskWorkspaceTransfer(t, false)
}

func TestUsageHistoryPostgresCopiedNativeAllowsTaskWorkspaceTransfer(t *testing.T) {
	runCopiedNativeTaskWorkspaceTransfer(t, true)
}

func TestUsageHistoryCopiedNativeRejectsDifferentTaskIncarnation(t *testing.T) {
	runCopiedNativeDifferentTaskIncarnation(t, false)
}

func TestUsageHistoryPostgresCopiedNativeRejectsDifferentTaskIncarnation(t *testing.T) {
	runCopiedNativeDifferentTaskIncarnation(t, true)
}

func runCopiedNativeDifferentTaskIncarnation(t *testing.T, postgres bool) {
	t.Helper()
	f := newUncopiedNativeFixture(t, postgres, 2)
	old := f.readLedger(t)
	for _, event := range old {
		f.retainOriginalEvent(t, event)
	}
	require.NoError(t, f.cleanup(false))
	ctx := context.Background()
	require.NoError(t, f.tasks.CreateWorkspace(ctx, &models.Workspace{ID: "ws-destination", Name: "Destination", OrgID: "org-b"}))
	f.tasks.SetUsageHistoryRetainer(nil)
	require.NoError(t, f.tasks.CreateTask(ctx, &models.Task{ID: f.proof.TaskID, WorkspaceID: "ws-destination", Title: "New incarnation"}))
	task, err := f.tasks.GetTask(ctx, f.proof.TaskID)
	require.NoError(t, err)
	require.NotEqual(t, f.proof.TaskIncarnationID, task.CreatedAt.UTC().Format(time.RFC3339Nano))
	// A legacy writer can replay an event ID after the original ledger's
	// cascade. Its old retained copy proves only the original task birth.
	replay := *old[len(old)-1]
	replay.ID, replay.SessionID = 0, ""
	require.NoError(t, f.tasks.CreateTaskUsageEvent(ctx, &replay))
	f.tasks.SetUsageHistoryRetainer(f.history)
	before := f.readLedger(t)
	require.Len(t, before, 1)
	require.ErrorContains(t, f.cleanup(false), "no accepted retained copy")
	require.Equal(t, before, f.readLedger(t))
	_, err = f.tasks.GetTask(ctx, task.ID)
	require.NoError(t, err)
	f.assertRetained(t, old)
}

func runCopiedNativeTaskWorkspaceTransfer(t *testing.T, postgres bool) {
	t.Helper()
	for _, prune := range []bool{false, true} {
		name := "delete"
		if prune {
			name = "prune"
		}
		t.Run(name, func(t *testing.T) {
			f := newUncopiedNativeFixture(t, postgres, 2)
			before := f.readLedger(t)
			for _, event := range before {
				f.retainOriginalEvent(t, event)
			}
			f.assertRetained(t, before)
			placement := f.copiedPlacement(t)
			f.moveLiveTaskWorkspace(t)
			require.Equal(t, before, f.readLedger(t))
			require.Equal(t, placement, f.copiedPlacement(t), "task transfer must preserve captured native ownership")
			f.restart(t)
			require.NoError(t, f.cleanup(prune))
			f.assertRetained(t, before)
			require.Equal(t, placement, f.copiedPlacement(t), "cleanup must preserve original native scope and binding")
			var liveSessions int
			require.NoError(t, f.db.Get(&liveSessions, f.db.Rebind(`SELECT COUNT(*) FROM task_sessions WHERE id = ?`), f.session.ID))
			require.Zero(t, liveSessions)
			if prune {
				task, err := f.tasks.GetTask(context.Background(), f.proof.TaskID)
				require.NoError(t, err)
				require.Equal(t, "ws-destination", task.WorkspaceID)
			} else {
				require.Empty(t, f.readLedger(t))
			}
		})
	}
}

type copiedNativePlacement struct {
	EventID     string `db:"source_record_id"`
	ScopeID     string `db:"scope_id"`
	SubjectID   string `db:"subject_id"`
	WorkspaceID string `db:"workspace_id"`
	BindingID   string `db:"usage_binding_id"`
	Digest      string `db:"payload_digest"`
}

func (f *uncopiedNativeFixture) copiedPlacement(t *testing.T) []copiedNativePlacement {
	t.Helper()
	var rows []copiedNativePlacement
	require.NoError(t, f.db.Select(&rows, `SELECT source_record_id, scope_id, subject_id, workspace_id,
		usage_binding_id, payload_digest FROM session_usage_buckets WHERE source = 'native' ORDER BY source_record_id`))
	require.Len(t, rows, 2)
	for _, row := range rows {
		require.Equal(t, f.proof.WorkspaceID, row.WorkspaceID)
	}
	return rows
}

func (f *uncopiedNativeFixture) moveLiveTaskWorkspace(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, f.tasks.CreateWorkspace(ctx, &models.Workspace{ID: "ws-destination", Name: "Destination", OrgID: "org-b", OwnerID: "fixture-owner"}))
	_, err := f.db.Exec(f.db.Rebind(`UPDATE workspaces SET owner_id = ? WHERE id = ?`), "fixture-owner", f.proof.WorkspaceID)
	require.NoError(t, err)
	for _, workflow := range []*models.Workflow{
		{ID: "wf-source", WorkspaceID: f.proof.WorkspaceID, Name: "Source"},
		{ID: "wf-destination", WorkspaceID: "ws-destination", Name: "Destination"},
	} {
		require.NoError(t, f.tasks.CreateWorkflow(ctx, workflow))
		_, err = f.db.Exec(f.db.Rebind(`INSERT INTO workflow_steps (id, workflow_id, name, position) VALUES (?, ?, ?, 0)`),
			workflow.ID+"-step", workflow.ID, "Work")
		require.NoError(t, err)
	}
	_, err = f.db.Exec(f.db.Rebind(`UPDATE tasks SET workflow_id = ?, workflow_step_id = ? WHERE id = ?`),
		"wf-source", "wf-source-step", f.proof.TaskID)
	require.NoError(t, err)
	task, err := f.tasks.GetTask(ctx, f.proof.TaskID)
	require.NoError(t, err)
	receipt, err := f.tasks.TransferTask(ctx, models.TaskTransferCommand{
		TaskID: task.ID, ExpectedSourceWorkspaceID: task.WorkspaceID, ExpectedSourceWorkflowID: task.WorkflowID,
		ExpectedSourceStepID: task.WorkflowStepID, ExpectedTaskUpdatedAt: task.UpdatedAt,
		DestinationWorkspaceID: "ws-destination", DestinationWorkflowID: "wf-destination", DestinationStepID: "wf-destination-step",
		IdempotencyKey: "usage-history-transfer", PreservationPolicy: models.TaskTransferPreservationPolicyV1,
		Actor: models.TaskTransferActor{Kind: models.TaskTransferActorHuman, ID: "fixture-owner"}, OwnerPredicateSet: true,
		AuthorizedOwnerID: "fixture-owner",
	})
	require.NoError(t, err)
	require.Equal(t, "ws-destination", receipt.DestinationWorkspaceID)
	task, err = f.tasks.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, "ws-destination", task.WorkspaceID)
}
