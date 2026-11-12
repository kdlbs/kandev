package backendapp

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

// @covers AC-OFFICE-HUMAN-DONE-002.5
func TestOfficeHumanCompletionSQLiteAdmissionBeforeWriter(t *testing.T) {
	t.Setenv("KANDEV_TEST_POSTGRES_DSN", "")
	for _, template := range []bool{false, true} {
		name := "task seat"
		if template {
			name = "template seat"
		}
		t.Run(name, func(t *testing.T) {
			f := newOfficeAtomicFixture(t, nil)
			writer, database := officeAtomicSecondWriterWithDB(t, f.h)
			database.SetMaxOpenConns(1)
			database.SetMaxIdleConns(1)
			// Fail immediately at SQLite's real writer reservation, rather than
			// using a delay as evidence that a competing goroutine ran.
			_, err := database.Exec(`PRAGMA busy_timeout=0`)
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			read, resume := make(chan struct{}), make(chan struct{})
			released := false
			defer func() {
				if !released {
					close(resume)
				}
			}()
			canonical := officeHumanCompletionPendingReader(f.office, f.workflow)
			f.h.repo.SetOfficeHumanCompletionPendingReader(func(ctx context.Context, tx *sql.Tx, taskID, stepID string) ([]string, error) {
				pending, err := canonical(ctx, tx, taskID, stepID)
				if err != nil {
					return nil, err
				}
				close(read)
				select {
				case <-resume:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				return pending, nil
			})
			done := make(chan error, 1)
			go func() {
				_, err := f.svc.MoveTaskWithOptions(ctx, "atomic-task", "wf-home", "atomic-done", 0,
					service.MoveTaskOptions{StepHistoryActor: wfmodels.StepTransitionActorHuman})
				done <- err
			}()
			select {
			case <-read:
			case err := <-done:
				t.Fatalf("completion did not reach canonical reader: %v", err)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			mutate := func() error {
				if template {
					return writer.UpsertStepParticipant(ctx, &wfmodels.WorkflowStepParticipant{
						ID: "late-template", StepID: "atomic-done", Role: wfmodels.ParticipantRoleApprover,
						AgentProfileID: "late-agent", DecisionRequired: true,
					})
				}
				_, err := writer.UpsertTaskParticipant(ctx, "step-home", "atomic-task", "approver", "late-agent")
				return err
			}
			err = mutate()
			var busy sqlite3.Error
			require.True(t, errors.As(err, &busy), "writer error=%v", err)
			require.Equal(t, sqlite3.ErrBusy, busy.Code)
			var seats int
			require.NoError(t, database.QueryRow(`SELECT count(*) FROM workflow_step_participants WHERE agent_profile_id='late-agent'`).Scan(&seats))
			require.Zero(t, seats, "refused writer has no partial seat")
			close(resume)
			released = true
			require.NoError(t, <-done)
			task, err := f.h.repo.GetTask(ctx, "atomic-task")
			require.NoError(t, err)
			require.Equal(t, v1.TaskStateCompleted, task.State)
			require.NoError(t, mutate(), "same writer succeeds after completion releases its transaction")
			pending, err := f.office.ListTaskParticipantsAtStep(ctx, "atomic-task", "atomic-done", "approver")
			require.NoError(t, err)
			require.Len(t, pending, 1)
		})
	}
}
