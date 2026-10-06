package sqlite_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/hierarchy"
)

// @covers AC-TASKS-FIELD-UPDATES-001.1, AC-TASKS-FIELD-UPDATES-001.2, AC-TASKS-FIELD-UPDATES-001.5, AC-TASKS-FIELD-UPDATES-001.6
func TestTaskFieldUpdatesPostgresPhysicalWait(t *testing.T) {
	for _, variant := range []string{"workspace_title_first", "workspace_description_first", "row_title_first", "row_description_first", "row_metadata", "row_pending", "row_cancel", "workspace_parent_rejected"} {
		t.Run(variant, func(t *testing.T) {
			a, b, observer := newHierarchyPostgresRepoPair(t)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if err := a.CreateWorkspace(ctx, &models.Workspace{ID: "fields-ws", Name: "Fields"}); err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"subject", "parent", "ancestor"} {
				if err := a.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: "fields-ws", Title: id, Metadata: map[string]interface{}{"ordinary": "old"}}); err != nil {
					t.Fatal(err)
				}
			}
			before, err := b.GetTask(ctx, "subject")
			if err != nil {
				t.Fatal(err)
			}
			pid := hierarchyBackendPID(t, b.DB())
			holder, err := a.DB().BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = holder.Rollback() }()
			lock := `SELECT id FROM tasks WHERE id='subject' FOR UPDATE`
			if variant == "workspace_title_first" || variant == "workspace_description_first" || variant == "workspace_parent_rejected" {
				lock = `SELECT id FROM workspaces WHERE id='fields-ws' FOR UPDATE`
			}
			if _, err := holder.ExecContext(ctx, lock); err != nil {
				t.Fatal(err)
			}
			title, description, parent := "Current title", "Current description", "parent"
			update := models.TaskFieldUpdate{Description: &description}
			mutation := `UPDATE tasks SET title='Current title' WHERE id='subject'`
			switch variant {
			case "workspace_description_first", "row_description_first":
				update = models.TaskFieldUpdate{Title: &title}
				mutation = `UPDATE tasks SET description='Current description' WHERE id='subject'`
			case "row_metadata":
				update = models.TaskFieldUpdate{Title: &title, Metadata: map[string]interface{}{"ordinary": "new", models.MetaKeyDeferredLaunch: "forged"}}
				mutation = `UPDATE tasks SET metadata='{"deferred_launch":{"prompt":"current"},"office_carrier_causation_depth":3}' WHERE id='subject'`
			case "row_pending":
				mutation = `UPDATE tasks SET title='Pending title', metadata='{"agent_title_pending":true,"agent_title_owner_session_id":"pending-owner","nullable":null}' WHERE id='subject'`
			case "workspace_parent_rejected":
				update.ParentID = &parent
				mutation = `UPDATE tasks SET parent_id='ancestor' WHERE id='parent'`
			}
			waiterCtx, cancelWaiter := context.WithCancel(ctx)
			defer cancelWaiter()
			result := make(chan error, 1)
			go func() {
				_, err := b.UpdateTaskFieldsWithParentAdmission(waiterCtx, "subject", update, hierarchy.ValidateParent)
				result <- err
			}()
			joined := false
			defer func() {
				cancelWaiter()
				_ = holder.Rollback()
				if !joined {
					<-result
				}
			}()
			waitHierarchyPostgresLocks(t, holder, pid, 1, variant)
			var waitEvent, waitType string
			var blockers int
			if err := observer.QueryRowContext(ctx, `SELECT wait_event_type,wait_event,cardinality(pg_blocking_pids(pid)) FROM pg_stat_activity WHERE pid=$1`, pid).Scan(&waitType, &waitEvent, &blockers); err != nil {
				t.Fatal(err)
			}
			if waitType != "Lock" || blockers < 1 {
				t.Fatalf("no physical wait: %s/%s blockers=%d", waitType, waitEvent, blockers)
			}
			t.Logf("%s: independent backend PID %d physically waiting %s/%s blockers=%d", variant, pid, waitType, waitEvent, blockers)
			if variant == "row_cancel" {
				cancelWaiter()
				err = <-result
				joined = true
				if err := holder.Commit(); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := holder.ExecContext(ctx, mutation); err != nil {
					t.Fatal(err)
				}
				if err := holder.Commit(); err != nil {
					t.Fatal(err)
				}
				err = <-result
				joined = true
			}
			switch variant {
			case "row_cancel":
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancel cause: %v", err)
				}
			case "workspace_parent_rejected":
				if !errors.Is(err, hierarchy.ErrInvalidParent) {
					t.Fatalf("parent cause: %v", err)
				}
			default:
				if err != nil {
					t.Fatal(err)
				}
			}
			current, err := b.GetTask(ctx, "subject")
			if err != nil {
				t.Fatal(err)
			}
			switch variant {
			case "row_cancel", "workspace_parent_rejected":
				if current.Title != before.Title || current.Description != before.Description || current.ParentID != "" || !current.UpdatedAt.Equal(before.UpdatedAt) {
					t.Fatalf("rejected row mutated: %+v", current)
				}
			case "row_pending":
				value, present := current.Metadata["nullable"]
				if current.Title != "Pending title" || current.Description != description || !models.IsAgentTitlePending(current.Metadata) || !models.IsAgentTitleOwner(current.Metadata, "pending-owner") || !present || value != nil {
					t.Fatalf("omitted pending metadata lost: %+v", current)
				}
			case "row_metadata":
				deferred, _ := current.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
				if current.Title != title || deferred["prompt"] != "current" || current.Metadata[models.MetaKeyOfficeCarrierCausationDepth] != float64(3) || current.Metadata["ordinary"] != "new" {
					t.Fatalf("mixed current owners: %+v", current)
				}
			default:
				if current.Title != title || current.Description != description {
					t.Fatalf("disjoint field lost: %+v", current)
				}
			}
			if variant != "workspace_parent_rejected" {
				if _, err := b.UpdateTaskFieldsWithParentAdmission(ctx, "subject", models.TaskFieldUpdate{ParentID: &parent}, hierarchy.ValidateParent); err != nil {
					t.Fatalf("valid parent control: %v", err)
				}
			}
		})
	}
}
