package backendapp

import (
	"context"
	"database/sql"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/dashboard"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/testutil"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
	"github.com/stretchr/testify/require"
)

// @covers AC-OFFICE-HUMAN-DONE-002.5
func TestOfficeHumanCompletionPostgresAdmissionBeforeWriter(t *testing.T) {
	for _, kind := range []string{"task seat", "template seat", "record decision", "supersede decisions", "clear decisions", "create step", "update step", "delete step", "delete participant", "delete task participant", "ensure role seat", "office add", "office remove", "marker clear", "clear start", "reorder steps", "delete workflow", "delete workspace workflows"} {
		t.Run(kind, func(t *testing.T) {
			database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
			database.SetMaxOpenConns(4)
			database.SetMaxIdleConns(4)
			h := newPluginMoveErrorBindingHarness(t, database)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
			require.NoError(t, err)
			_, cleanup, err := settingsstore.Provide(h.db, h.db, log)
			require.NoError(t, err)
			t.Cleanup(func() { _ = cleanup() })
			office, err := officesqlite.NewWithDB(h.db, h.db, log)
			require.NoError(t, err)
			workflow, err := workflowrepo.NewWithDB(h.db, h.db, log)
			require.NoError(t, err)
			dash := dashboard.NewDashboardService(office, log, nil, nil, nil)
			dash.SetDecisionStore(workflow)
			svc := h.adapter.svc.(*taskservice.Service)
			svc.SetOfficeApprovalGuard(officeApprovalGuardAdapter{reader: dash})
			require.NoError(t, h.workflowSvc.CreateStep(ctx, &wfmodels.WorkflowStep{ID: "atomic-done", WorkflowID: "wf-home", Name: "Done", Position: 10, CompleteTaskOnEnter: true}))
			h.createTask(t, "atomic-task", false)
			_, err = h.db.Exec(`UPDATE workspaces SET office_workflow_id='wf-home' WHERE id='ws-1'`)
			require.NoError(t, err)
			seatID, err := workflow.UpsertTaskParticipant(ctx, "step-home", "atomic-task", "approver", "approved-agent")
			require.NoError(t, err)
			require.NoError(t, workflow.RecordStepDecision(ctx, &wfmodels.WorkflowStepDecision{TaskID: "atomic-task", StepID: "step-home", ParticipantID: seatID, Decision: "approved", DeciderType: "agent", DeciderID: "approved-agent", Role: "approver"}))
			reader := officeHumanCompletionPendingReader(office, workflow)
			read := make(chan int, 1)
			resume := make(chan struct{})
			released := false
			defer func() {
				if !released {
					close(resume)
				}
			}()
			h.repo.SetOfficeHumanCompletionPendingReader(func(ctx context.Context, tx *sql.Tx, taskID, stepID string) ([]string, error) {
				pending, err := reader(ctx, tx, taskID, stepID)
				if err != nil {
					return nil, err
				}
				var pid int
				if err := tx.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
					return nil, err
				}
				read <- pid
				select {
				case <-resume:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				return pending, nil
			})
			completed := make(chan error, 1)
			go func() {
				_, err := svc.MoveTaskWithOptions(ctx, "atomic-task", "wf-home", "atomic-done", 0, taskservice.MoveTaskOptions{AllowActivePrimarySession: true, StepHistoryActor: wfmodels.StepTransitionActorHuman})
				completed <- err
			}()
			var admissionPID int
			select {
			case admissionPID = <-read:
			case err := <-completed:
				t.Fatalf("admission did not reach canonical read: %v", err)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			writer := make(chan error, 1)
			go func() {
				var err error
				switch kind {
				case "task seat":
					_, err = workflow.UpsertTaskParticipant(ctx, "step-home", "atomic-task", "approver", "new-agent")
				case "template seat":
					err = workflow.UpsertStepParticipant(ctx, &wfmodels.WorkflowStepParticipant{StepID: "atomic-done", Role: wfmodels.ParticipantRoleApprover, AgentProfileID: "new-agent", DecisionRequired: true})
				case "record decision":
					err = workflow.RecordStepDecision(ctx, &wfmodels.WorkflowStepDecision{TaskID: "atomic-task", StepID: "step-home", ParticipantID: seatID, Decision: "rejected", DeciderType: "agent", DeciderID: "approved-agent", Role: "approver"})
				case "supersede decisions":
					err = workflow.SupersedeTaskDecisions(ctx, "atomic-task")
				case "clear decisions":
					_, err = workflow.ClearStepDecisions(ctx, "atomic-task", "step-home")
				case "create step":
					err = workflow.CreateStep(ctx, &wfmodels.WorkflowStep{ID: "new-final", WorkflowID: "wf-home", Name: "Next", Position: 20})
				case "update step":
					step, e := workflow.GetStep(ctx, "atomic-done")
					if e != nil {
						err = e
						break
					}
					step.CompleteTaskOnEnter = false
					err = workflow.UpdateStep(ctx, step)
				case "delete participant":
					err = workflow.DeleteStepParticipant(ctx, seatID)
				case "delete task participant":
					err = workflow.DeleteTaskParticipant(ctx, "step-home", "atomic-task", "approver", "approved-agent")
				case "ensure role seat":
					_, _, err = workflow.EnsureRoleSeat(ctx, "wf-home", "atomic-done", "atomic-task", "approver", "new-agent")
				case "office add":
					_, err = office.AddTaskParticipant(ctx, "atomic-task", "new-agent", "approver")
				case "office remove":
					err = office.RemoveTaskParticipant(ctx, "atomic-task", "approved-agent", "approver")
				case "marker clear":
					_, err = h.repo.ClearStepDecisionsAndCompleteMarker(ctx, "atomic-task", "step-home", 0, 0, time.Now().UTC())
				case "clear start":
					err = workflow.ClearStartStepFlag(ctx, "wf-home", "")
				case "reorder steps":
					steps, e := workflow.ListStepsByWorkflow(ctx, "wf-home")
					if e != nil {
						err = e
						break
					}
					var version time.Time
					if e := h.db.QueryRowContext(ctx, `SELECT updated_at FROM workflows WHERE id='wf-home'`).Scan(&version); e != nil {
						err = e
						break
					}
					versions := map[string]time.Time{}
					ids := []string{}
					for i := len(steps) - 1; i >= 0; i-- {
						ids = append(ids, steps[i].ID)
						versions[steps[i].ID] = steps[i].UpdatedAt
					}
					err = workflow.ReorderStepsIfUnchanged(ctx, "wf-home", ids, version, versions)
				case "delete workflow":
					err = h.repo.DeleteWorkflow(ctx, "wf-home")
				case "delete workspace workflows":
					_, err = h.repo.DeleteWorkflowsByWorkspace(ctx, "ws-1", []string{"wf-target"})
				case "delete step":
					err = workflow.DeleteStep(ctx, "atomic-done")
				}
				writer <- err
			}()
			// Observe either the actual writer's premature commit or its real server
			// wait edge. A bounded ticker observes SQL state; it does not create a race.
			tick := time.NewTicker(10 * time.Millisecond)
			defer tick.Stop()
			waiting := false
			for !waiting {
				select {
				case err := <-writer:
					close(resume)
					released = true
					require.NoError(t, <-completed)
					require.NoError(t, err)
					t.Fatal("eligibility/policy writer committed between canonical read and task write")
				case <-tick.C:
					err = h.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE pid <> pg_backend_pid() AND $1 = ANY(pg_blocking_pids(pid)))`, admissionPID).Scan(&waiting)
					require.NoError(t, err)
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			}
			close(resume)
			released = true
			require.NoError(t, <-completed)
			writerErr := <-writer
			switch kind {
			case "task seat", "record decision", "supersede decisions", "clear decisions", "delete participant", "delete task participant", "ensure role seat", "office add", "office remove", "marker clear":
				require.ErrorIs(t, writerErr, repoerrors.ErrWorkflowChangeConflict, "a low-level writer must not silently follow changed placement")
			default:
				require.NoError(t, writerErr)
			}
			task, err := h.repo.GetTask(ctx, "atomic-task")
			require.NoError(t, err)
			require.Equal(t, "COMPLETED", string(task.State), "writer after admission must not retroactively undo completion")
		})
	}
}
