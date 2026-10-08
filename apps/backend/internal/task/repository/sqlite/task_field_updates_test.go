package sqlite

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/jmoiron/sqlx"
	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/hierarchy"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/stepentry"
)

// @covers AC-TASKS-FIELD-UPDATES-001.2, AC-TASKS-FIELD-UPDATES-001.3, AC-TASKS-FIELD-UPDATES-001.6
func TestTaskFieldUpdatesSQLiteCurrentRow(t *testing.T) {
	a := newRepoForEntityTests(t)
	var path string
	rows, err := a.DB().Query(`PRAGMA database_list`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var seq int
		var name, file string
		if err := rows.Scan(&seq, &name, &file); err != nil {
			t.Fatal(err)
		}
		if name == "main" {
			path = file
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if path == "" {
		t.Fatal("fixture must be file backed")
	}
	raw, err := internaldb.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	db := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	b := NewWithInitializedDB(db, db, nil)
	ctx := context.Background()
	if err := a.CreateWorkspace(ctx, &models.Workspace{ID: "fields-ws", Name: "Fields"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"subject", "parent"} {
		if err := a.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: "fields-ws", Title: id}); err != nil {
			t.Fatal(err)
		}
	}
	title := "Current title"
	if _, err := a.UpdateTaskFieldsWithParentAdmission(ctx, "subject", models.TaskFieldUpdate{Title: &title}, hierarchy.ValidateParent); err != nil {
		t.Fatal(err)
	}
	if err := a.UpdateTaskPriority(ctx, "subject", "high"); err != nil {
		t.Fatal(err)
	}
	description, parent, zero := "Requested description", "parent", 0
	result, err := b.UpdateTaskFieldsWithParentAdmission(ctx, "subject", models.TaskFieldUpdate{Description: &description, ParentID: &parent, Position: &zero}, hierarchy.ValidateParent)
	if err != nil || !result.ParentChanged {
		t.Fatalf("patch: %+v %v", result, err)
	}
	current, err := a.GetTask(ctx, "subject")
	if err != nil || current.Title != "Current title" || current.Description != description || current.Priority != "high" || current.Position != 0 || current.ParentID != parent {
		t.Fatalf("current row: %+v %v", current, err)
	}
	title = "Should roll back"
	_, err = b.UpdateTaskFieldsWithParentAdmission(ctx, "subject", models.TaskFieldUpdate{Title: &title, Metadata: map[string]interface{}{"invalid": func() {}}}, hierarchy.ValidateParent)
	if err == nil {
		t.Fatal("unencodable metadata accepted")
	}
	after, err := a.GetTask(ctx, "subject")
	if err != nil || after.Title != current.Title || !after.UpdatedAt.Equal(current.UpdatedAt) {
		t.Fatalf("rollback: %+v %v", after, err)
	}
	t.Run("entry_and_runner", func(t *testing.T) { testFieldEntryAndRunner(t, a, b) })
	t.Run("dispatch_legacy_context", func(t *testing.T) { testFieldDispatchLegacyContext(t, a, b) })
	_, err = b.UpdateTaskFieldsWithParentAdmission(ctx, "missing", models.TaskFieldUpdate{}, hierarchy.ValidateParent)
	if !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("missing cause: %v", err)
	}
}

func testFieldDispatchLegacyContext(t *testing.T, a, b *Repository) {
	t.Helper()
	ctx := context.Background()
	for _, task := range []*models.Task{
		{ID: "field-dispatch", WorkspaceID: "fields-ws", WorkflowID: "dispatch-wf", WorkflowStepID: "dispatch-source", Title: "Dispatch"},
		{ID: "legacy-dispatch", WorkspaceID: "fields-ws", Title: "Legacy", Metadata: map[string]interface{}{models.MetaKeyAgentTitlePending: true, "nullable": nil}},
	} {
		if err := a.CreateTask(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	a.SetStepEntryDispatcher(fieldEntryProbe(func(ctx context.Context, taskID, _, stepID, _ string, _ int64) {
		current, err := b.GetTask(ctx, taskID)
		if err != nil || current.WorkflowStepID != stepID {
			t.Fatalf("dispatch before committed step: %+v %v", current, err)
		}
		legacy, err := b.GetTask(ctx, "legacy-dispatch")
		if err != nil {
			t.Fatal(err)
		}
		if err := b.UpdateTask(ctx, legacy); err != nil {
			t.Fatal(err)
		}
	}))
	t.Cleanup(func() { a.SetStepEntryDispatcher(nil) })
	target := "dispatch-target"
	if _, err := a.UpdateTaskFieldsWithParentAdmission(ctx, "field-dispatch", models.TaskFieldUpdate{WorkflowStepID: &target}, hierarchy.ValidateParent); err != nil {
		t.Fatal(err)
	}
	legacy, err := b.GetTask(ctx, "legacy-dispatch")
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := legacy.Metadata["nullable"]; exists {
		t.Fatal("postcommit callback changed the legacy pending-metadata merge")
	}
}

type fieldEntryProbe func(context.Context, string, string, string, string, int64)

func (f fieldEntryProbe) DispatchStepEntry(ctx context.Context, taskID, workflowID, stepID, entryID string, markerID int64) {
	f(ctx, taskID, workflowID, stepID, entryID, markerID)
}

func testFieldEntryAndRunner(t *testing.T, a, b *Repository) {
	t.Helper()
	ctx := context.Background()
	task := &models.Task{ID: "field-runner", WorkspaceID: "fields-ws", WorkflowID: "fields-wf", WorkflowStepID: "fields-source", Title: "Runner", AssigneeAgentProfileID: "profile-fields"}
	if err := a.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	observed := false
	a.SetStepEntryDispatcher(fieldEntryProbe(func(ctx context.Context, taskID, workflowID, stepID, entryID string, markerID int64) {
		current, err := b.GetTask(ctx, taskID)
		if err != nil || current.WorkflowStepID != stepID || current.WorkflowID != workflowID || current.AssigneeAgentProfileID != "profile-fields" {
			t.Fatalf("dispatch before committed current row: %+v %v", current, err)
		}
		ledgerID, err := strconv.ParseInt(entryID, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		var ledgerStep, markerStep, digest string
		if err := b.DB().QueryRowContext(ctx, `SELECT to_workflow_step_id FROM task_step_transitions WHERE id=? AND task_id=?`, ledgerID, taskID).Scan(&ledgerStep); err != nil {
			t.Fatal(err)
		}
		if err := b.DB().QueryRowContext(ctx, `SELECT step_id,digest FROM workflow_step_entries WHERE id=? AND task_id=?`, markerID, taskID).Scan(&markerStep, &digest); err != nil {
			t.Fatal(err)
		}
		if ledgerStep != stepID || markerStep != stepID || digest == "" {
			t.Fatal("dispatch identities did not name committed records")
		}
		observed = true
	}))
	t.Cleanup(func() { a.SetStepEntryDispatcher(nil) })
	target := "fields-target"
	pending, ok := stepentry.BuildPendingAllocation(target, []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterClearDecisions}})
	if !ok {
		t.Fatal("real marker declaration did not resolve")
	}
	updateCtx := stepentry.WithPendingAllocation(ctx, pending)
	holder := &stepentry.AllocationResult{}
	updateCtx = stepentry.WithResultHolder(updateCtx, holder)
	if _, err := a.UpdateTaskFieldsWithParentAdmission(updateCtx, task.ID, models.TaskFieldUpdate{WorkflowStepID: &target}, hierarchy.ValidateParent); err != nil {
		t.Fatal(err)
	}
	if !observed || holder.TransitionID == 0 || holder.EntryID == 0 {
		t.Fatal("successful transition lacked committed entry evidence")
	}
	var runner string
	if err := b.DB().QueryRow(`SELECT agent_profile_id FROM workflow_step_participants WHERE task_id=? AND step_id=? AND role='runner'`, task.ID, target).Scan(&runner); err != nil || runner != "profile-fields" {
		t.Fatalf("runner projection lost: %q %v", runner, err)
	}
	before, err := b.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	var beforeLedger, beforeEntries int
	if err := b.DB().QueryRow(`SELECT count(*) FROM task_step_transitions WHERE task_id=?`, task.ID).Scan(&beforeLedger); err != nil {
		t.Fatal(err)
	}
	if err := b.DB().QueryRow(`SELECT count(*) FROM workflow_step_entries WHERE task_id=?`, task.ID).Scan(&beforeEntries); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB().Exec(`CREATE TRIGGER reject_field_runner BEFORE INSERT ON workflow_step_participants WHEN NEW.step_id='fields-failed' BEGIN SELECT RAISE(ABORT,'runner refused after allocation'); END`); err != nil {
		t.Fatal(err)
	}
	target = "fields-failed"
	pending.StepID = target
	observed = false
	_, err = a.UpdateTaskFieldsWithParentAdmission(stepentry.WithPendingAllocation(ctx, pending), task.ID, models.TaskFieldUpdate{WorkflowStepID: &target}, hierarchy.ValidateParent)
	if err == nil {
		t.Fatal("late runner failure accepted")
	}
	current, err := b.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	var ledger, entries int
	if err := b.DB().QueryRow(`SELECT count(*) FROM task_step_transitions WHERE task_id=?`, task.ID).Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if err := b.DB().QueryRow(`SELECT count(*) FROM workflow_step_entries WHERE task_id=?`, task.ID).Scan(&entries); err != nil {
		t.Fatal(err)
	}
	if current.WorkflowStepID != before.WorkflowStepID || !current.UpdatedAt.Equal(before.UpdatedAt) || current.AssigneeAgentProfileID != before.AssigneeAgentProfileID || ledger != beforeLedger || entries != beforeEntries || observed {
		t.Fatal("late failure leaked row, runner, ledger, entry or dispatch")
	}
}
