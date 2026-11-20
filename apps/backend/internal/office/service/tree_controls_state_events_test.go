package service_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/office/shared"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type treeStateEventRecorder struct {
	svc             *service.Service
	taskIDs         []string
	changes         []taskmodels.TaskStateChange
	activityHandled []bool
	observedStates  []map[string]string
	order           *[]string
	readErr         error
}

func (r *treeStateEventRecorder) PublishCommittedTaskStateChange(
	ctx context.Context, change taskmodels.TaskStateChange, activityHandled bool,
) {
	r.changes = append(r.changes, change)
	r.activityHandled = append(r.activityHandled, activityHandled)
	states := make(map[string]string, len(r.taskIDs))
	for _, taskID := range r.taskIDs {
		fields, err := r.svc.GetTaskExecutionFieldsForTest(ctx, taskID)
		if err != nil {
			r.readErr = err
			continue
		}
		states[taskID] = fields.State
	}
	r.observedStates = append(r.observedStates, states)
	if change.Task != nil && r.order != nil {
		*r.order = append(*r.order, "state:"+change.Task.ID)
	}
}

type failingRestoreTaskStateWriter struct {
	shared.TaskStateWriter
	failTaskID string
}

type driftedRestoreTaskStateWriter struct {
	shared.TaskStateWriter
	svc         *service.Service
	t           *testing.T
	driftTaskID string
	drifted     bool
}

func (w failingRestoreTaskStateWriter) CommitTaskState(
	ctx context.Context, taskID string, write taskmodels.TaskStateWrite,
) (taskmodels.TaskStateChange, error) {
	if taskID == w.failTaskID {
		return taskmodels.TaskStateChange{}, errors.New("injected restore failure")
	}
	return w.TaskStateWriter.CommitTaskState(ctx, taskID, write)
}

func (w *driftedRestoreTaskStateWriter) CommitTaskState(
	ctx context.Context, taskID string, write taskmodels.TaskStateWrite,
) (taskmodels.TaskStateChange, error) {
	change, err := w.TaskStateWriter.CommitTaskState(ctx, taskID, write)
	if err == nil && taskID == "root" && !w.drifted {
		w.svc.ExecSQL(w.t, `UPDATE tasks SET state = ? WHERE id = ?`, "IN_PROGRESS", w.driftTaskID)
		w.drifted = true
	}
	return change, err
}

func newTreeStateEventFixture(t *testing.T, taskIDs ...string) (*service.Service, *treeStateEventRecorder, *[]string) {
	t.Helper()
	eb := bus.NewMemoryEventBus(logger.Default())
	order := new([]string)
	for subject, marker := range map[string]string{
		events.OfficeTaskTreeHoldCreated:  "hold:created",
		events.OfficeTaskTreeHoldReleased: "hold:released",
	} {
		marker := marker
		if _, err := eb.Subscribe(subject, func(context.Context, *bus.Event) error {
			*order = append(*order, marker)
			return nil
		}); err != nil {
			t.Fatalf("subscribe %s: %v", subject, err)
		}
	}
	svc := newTestService(t, service.ServiceOptions{EventBus: eb})
	setTreeTaskStateServices(t, svc)
	publication := &treeStateEventRecorder{svc: svc, taskIDs: taskIDs, order: order}
	svc.SetTaskStateChangePublisher(publication)
	return svc, publication, order
}

func assertTreeStates(t *testing.T, svc *service.Service, want map[string]string) {
	t.Helper()
	for taskID, wantState := range want {
		fields, err := svc.GetTaskExecutionFieldsForTest(context.Background(), taskID)
		if err != nil {
			t.Fatalf("read task %s: %v", taskID, err)
		}
		if fields.State != wantState {
			t.Fatalf("state %s = %q, want %q", taskID, fields.State, wantState)
		}
	}
}

func TestOfficeTreeStateEvents(t *testing.T) {
	ctx := context.Background()
	svc, publication, order := newTreeStateEventFixture(t, "root", "child", "already-cancelled")
	insertTreeControlTask(t, svc, "root", "", "IN_PROGRESS")
	insertTreeControlTask(t, svc, "child", "root", "TODO")
	insertTreeControlTask(t, svc, "already-cancelled", "root", "CANCELLED")
	for _, taskID := range []string{"root", "child", "already-cancelled"} {
		svc.ExecSQL(t, `UPDATE tasks SET checkout_agent_id = ?, checkout_run_id = ?, checkout_at = CURRENT_TIMESTAMP WHERE id = ?`,
			"agent-"+taskID, "run-"+taskID, taskID)
	}

	if _, err := svc.CancelTaskTree(ctx, "root", "user:test"); err != nil {
		t.Fatalf("CancelTaskTree: %v", err)
	}
	if publication.readErr != nil {
		t.Fatalf("read state during publication: %v", publication.readErr)
	}
	if len(publication.changes) != 2 {
		t.Fatalf("cancel receipts = %+v, want root and child only", publication.changes)
	}
	for i, change := range publication.changes {
		if change.Task == nil || change.Task.State != v1.TaskStateCancelled || change.OldState == v1.TaskStateCancelled {
			t.Fatalf("cancel receipt %d = %+v, want a real transition to CANCELLED", i, change)
		}
		if publication.activityHandled[i] {
			t.Fatalf("activityHandled[%d] = true, want task service activity hook", i)
		}
		states := publication.observedStates[i]
		for _, taskID := range []string{"root", "child", "already-cancelled"} {
			if states[taskID] != "CANCELLED" {
				t.Fatalf("observer for %s saw %s=%q, want all members cancelled: %v", change.Task.ID, taskID, states[taskID], states)
			}
		}
	}
	if len(*order) < 3 {
		t.Fatalf("cancel notification order = %v, want three notifications", *order)
	}
	if got, want := (*order)[:3], []string{"state:root", "state:child", "hold:created"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("cancel notification order = %v, want %v", got, want)
	}
	assertTreeStates(t, svc, map[string]string{
		"root": "CANCELLED", "child": "CANCELLED", "already-cancelled": "CANCELLED",
	})
	for _, taskID := range []string{"root", "child", "already-cancelled"} {
		var agent, run sql.NullString
		var checkoutAt sql.NullTime
		if err := svc.RepoForTest().ReaderDB().QueryRowxContext(ctx,
			`SELECT checkout_agent_id, checkout_run_id, checkout_at FROM tasks WHERE id = ?`, taskID,
		).Scan(&agent, &run, &checkoutAt); err != nil {
			t.Fatalf("read checkout for %s: %v", taskID, err)
		}
		if (agent.Valid && agent.String != "") || (run.Valid && run.String != "") || checkoutAt.Valid {
			t.Fatalf("checkout %s = agent %v run %v at %v, want released", taskID, agent, run, checkoutAt)
		}
	}

	publication.changes = nil
	publication.activityHandled = nil
	publication.observedStates = nil
	if _, err := svc.RestoreTaskTree(ctx, "root", "user:test"); err != nil {
		t.Fatalf("RestoreTaskTree: %v", err)
	}
	if len(publication.changes) != 2 {
		t.Fatalf("restore receipts = %+v, want root and child only", publication.changes)
	}
	if publication.changes[0].OldState != v1.TaskStateCancelled || publication.changes[0].Task.State != v1.TaskStateInProgress {
		t.Fatalf("root restore receipt = %+v, want CANCELLED -> IN_PROGRESS", publication.changes[0])
	}
	if publication.changes[1].OldState != v1.TaskStateCancelled || publication.changes[1].Task.State != v1.TaskStateTODO {
		t.Fatalf("child restore receipt = %+v, want CANCELLED -> TODO", publication.changes[1])
	}
	assertTreeStates(t, svc, map[string]string{
		"root": "IN_PROGRESS", "child": "TODO", "already-cancelled": "CANCELLED",
	})
	if got, want := (*order)[3:], []string{"state:root", "state:child", "hold:released"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("restore notification order = %v, want %v", got, want)
	}
}

func TestOfficeTreeStateEventsBatchFailurePublishesNothing(t *testing.T) {
	ctx := context.Background()
	svc, publication, _ := newTreeStateEventFixture(t, "root", "child")
	insertTreeControlTask(t, svc, "root", "", "IN_PROGRESS")
	insertTreeControlTask(t, svc, "child", "root", "TODO")
	svc.ExecSQL(t, `CREATE TRIGGER fail_tree_checkout_release BEFORE UPDATE OF checkout_agent_id ON tasks
		BEGIN SELECT RAISE(ABORT, 'injected checkout failure'); END`)

	if _, err := svc.CancelTaskTree(ctx, "root", "user:test"); err == nil {
		t.Fatal("CancelTaskTree succeeded despite a checkout cleanup failure")
	}
	assertTreeStates(t, svc, map[string]string{"root": "IN_PROGRESS", "child": "TODO"})
	if len(publication.changes) != 0 {
		t.Fatalf("published changes = %+v, want none after batch rollback", publication.changes)
	}
}

func TestOfficeTreeStateEventsCleanupFailureKeepsCommittedReceipts(t *testing.T) {
	ctx := context.Background()
	svc, publication, _ := newTreeStateEventFixture(t, "root", "child")
	insertTreeControlTask(t, svc, "root", "", "IN_PROGRESS")
	insertTreeControlTask(t, svc, "child", "root", "TODO")
	svc.ExecSQL(t, `DROP TABLE runs`)

	if _, err := svc.CancelTaskTree(ctx, "root", "user:test"); err != nil {
		t.Fatalf("CancelTaskTree: %v", err)
	}
	assertTreeStates(t, svc, map[string]string{"root": "CANCELLED", "child": "CANCELLED"})
	if len(publication.changes) != 2 {
		t.Fatalf("published changes = %+v, want both committed receipts despite cleanup failure", publication.changes)
	}
}

func TestOfficeTreeStateEventsPartialRestoreKeepsHoldAndPriorReceipt(t *testing.T) {
	ctx := context.Background()
	svc, publication, _ := newTreeStateEventFixture(t, "root", "child")
	insertTreeControlTask(t, svc, "root", "", "IN_PROGRESS")
	insertTreeControlTask(t, svc, "child", "root", "TODO")
	if _, err := svc.CancelTaskTree(ctx, "root", "user:test"); err != nil {
		t.Fatalf("CancelTaskTree: %v", err)
	}
	publication.changes = nil
	publication.activityHandled = nil
	publication.observedStates = nil
	taskWriter := treeControlTaskStateWriter{repo: svc.RepoForTest()}
	svc.SetTaskStateWriter(failingRestoreTaskStateWriter{TaskStateWriter: taskWriter, failTaskID: "child"})

	if _, err := svc.RestoreTaskTree(ctx, "root", "user:test"); err == nil {
		t.Fatal("RestoreTaskTree succeeded despite an injected later-member failure")
	}
	assertTreeStates(t, svc, map[string]string{"root": "IN_PROGRESS", "child": "CANCELLED"})
	if len(publication.changes) != 1 || publication.changes[0].Task.ID != "root" {
		t.Fatalf("published restore receipts = %+v, want the successful root restore", publication.changes)
	}
	preview, err := svc.PreviewTaskTree(ctx, "root")
	if err != nil {
		t.Fatalf("PreviewTaskTree: %v", err)
	}
	if preview.ActiveHold == nil || preview.ActiveHold.Mode != "cancel" {
		t.Fatalf("active hold = %+v, want cancel hold retained", preview.ActiveHold)
	}

	svc.SetTaskStateWriter(taskWriter)
	if _, err := svc.RestoreTaskTree(ctx, "root", "user:test"); err != nil {
		t.Fatalf("retry RestoreTaskTree: %v", err)
	}
	assertTreeStates(t, svc, map[string]string{"root": "IN_PROGRESS", "child": "TODO"})
	if len(publication.changes) != 2 || publication.changes[0].Task.ID != "root" || publication.changes[1].Task.ID != "child" {
		t.Fatalf("restore receipts after retry = %+v, want one receipt for each actual transition", publication.changes)
	}
	preview, err = svc.PreviewTaskTree(ctx, "root")
	if err != nil {
		t.Fatalf("PreviewTaskTree after retry: %v", err)
	}
	if preview.ActiveHold != nil {
		t.Fatalf("active hold after retry = %+v, want released", preview.ActiveHold)
	}
}

func TestOfficeTreeStateEventsRestoreRetryAfterHoldReleaseFailure(t *testing.T) {
	ctx := context.Background()
	svc, publication, _ := newTreeStateEventFixture(t, "root", "child")
	insertTreeControlTask(t, svc, "root", "", "IN_PROGRESS")
	insertTreeControlTask(t, svc, "child", "root", "TODO")
	if _, err := svc.CancelTaskTree(ctx, "root", "user:test"); err != nil {
		t.Fatalf("CancelTaskTree: %v", err)
	}
	publication.changes = nil
	publication.activityHandled = nil
	publication.observedStates = nil
	svc.ExecSQL(t, `CREATE TRIGGER fail_tree_hold_release BEFORE UPDATE OF released_at ON office_task_tree_holds
		WHEN NEW.released_at IS NOT NULL BEGIN SELECT RAISE(ABORT, 'injected hold release failure'); END`)

	if _, err := svc.RestoreTaskTree(ctx, "root", "user:test"); err == nil {
		t.Fatal("RestoreTaskTree succeeded despite an injected hold release failure")
	}
	assertTreeStates(t, svc, map[string]string{"root": "IN_PROGRESS", "child": "TODO"})
	if len(publication.changes) != 2 {
		t.Fatalf("restore receipts before hold retry = %+v, want both committed transitions", publication.changes)
	}
	preview, err := svc.PreviewTaskTree(ctx, "root")
	if err != nil {
		t.Fatalf("PreviewTaskTree: %v", err)
	}
	if preview.ActiveHold == nil || preview.ActiveHold.Mode != "cancel" {
		t.Fatalf("active hold = %+v, want cancel hold retained", preview.ActiveHold)
	}

	svc.ExecSQL(t, `DROP TRIGGER fail_tree_hold_release`)
	publication.changes = nil
	publication.activityHandled = nil
	publication.observedStates = nil
	if _, err := svc.RestoreTaskTree(ctx, "root", "user:test"); err != nil {
		t.Fatalf("retry RestoreTaskTree after hold release failure: %v", err)
	}
	if len(publication.changes) != 0 {
		t.Fatalf("duplicate restore receipts after release retry = %+v, want none", publication.changes)
	}
	preview, err = svc.PreviewTaskTree(ctx, "root")
	if err != nil {
		t.Fatalf("PreviewTaskTree after release retry: %v", err)
	}
	if preview.ActiveHold != nil {
		t.Fatalf("active hold after release retry = %+v, want released", preview.ActiveHold)
	}
}

func TestOfficeTreeStateEventsRestoreSkipsDriftedMemberAndReleasesHold(t *testing.T) {
	ctx := context.Background()
	svc, publication, _ := newTreeStateEventFixture(t, "root", "a-drifted", "z-later")
	insertTreeControlTask(t, svc, "root", "", "IN_PROGRESS")
	insertTreeControlTask(t, svc, "a-drifted", "root", "TODO")
	insertTreeControlTask(t, svc, "z-later", "root", "TODO")
	if _, err := svc.CancelTaskTree(ctx, "root", "user:test"); err != nil {
		t.Fatalf("CancelTaskTree: %v", err)
	}
	publication.changes = nil
	publication.activityHandled = nil
	publication.observedStates = nil

	taskWriter := treeControlTaskStateWriter{repo: svc.RepoForTest()}
	svc.SetTaskStateWriter(&driftedRestoreTaskStateWriter{
		TaskStateWriter: taskWriter,
		svc:             svc,
		t:               t,
		driftTaskID:     "a-drifted",
	})

	if _, err := svc.RestoreTaskTree(ctx, "root", "user:test"); err != nil {
		t.Fatalf("RestoreTaskTree: %v", err)
	}
	assertTreeStates(t, svc, map[string]string{
		"root": "IN_PROGRESS", "a-drifted": "IN_PROGRESS", "z-later": "TODO",
	})
	if len(publication.changes) != 2 || publication.changes[0].Task.ID != "root" ||
		publication.changes[1].Task.ID != "z-later" {
		t.Fatalf("restore receipts = %+v, want root and later-member transitions", publication.changes)
	}
	preview, err := svc.PreviewTaskTree(ctx, "root")
	if err != nil {
		t.Fatalf("PreviewTaskTree: %v", err)
	}
	if preview.ActiveHold != nil {
		t.Fatalf("active hold after restore = %+v, want released", preview.ActiveHold)
	}
}

func TestOfficeTreeStateEventsNoopControlsDoNotPublish(t *testing.T) {
	ctx := context.Background()
	svc, publication, _ := newTreeStateEventFixture(t, "root")
	insertTreeControlTask(t, svc, "root", "", "IN_PROGRESS")
	if _, err := svc.PauseTaskTree(ctx, "root"); err != nil {
		t.Fatalf("PauseTaskTree: %v", err)
	}
	if _, err := svc.ResumeTaskTree(ctx, "root", "user:test"); err != nil {
		t.Fatalf("ResumeTaskTree: %v", err)
	}
	if _, err := svc.CancelTaskTree(ctx, "root", "user:test"); err != nil {
		t.Fatalf("CancelTaskTree: %v", err)
	}
	if _, err := svc.CancelTaskTree(ctx, "root", "user:test"); err != nil {
		t.Fatalf("repeated CancelTaskTree: %v", err)
	}
	if len(publication.changes) != 1 {
		t.Fatalf("published changes = %+v, want one event for the initial cancellation only", publication.changes)
	}
}
