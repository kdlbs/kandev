package backendapp

import (
	"context"
	"database/sql"
	"errors"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"testing"
	"time"

	internaldb "github.com/kandev/kandev/internal/db"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/testutil"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
	"github.com/stretchr/testify/require"
)

func TestOfficeMutationScopeConcurrentParticipantIDCreation(t *testing.T) {
	_ = testutil.PostgresDSNFromEnv(t)
	f := newOfficeAtomicFixture(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	require.NoError(t, f.workflow.CreateStep(ctx, &wfmodels.WorkflowStep{ID: "foreign-step", WorkflowID: "wf-other-workspace", Name: "Foreign", Position: 0}))
	holder, err := f.h.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = holder.Rollback() }()
	require.NoError(t, internaldb.LockTaskHierarchy(ctx, holder, "pgx", f.h.db.Rebind, []string{"ws-1"}, nil))
	var holderPID int
	require.NoError(t, holder.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&holderPID))
	done := make(chan error, 1)
	go func() {
		done <- f.workflow.UpsertStepParticipant(ctx, &wfmodels.WorkflowStepParticipant{ID: "racing-id", StepID: "step-home", Role: wfmodels.ParticipantRoleApprover, AgentProfileID: "agent-a"})
	}()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	blocked := false
	for !blocked {
		select {
		case err := <-done:
			t.Fatalf("source writer passed held workspace before scope revalidation: %v", err)
		case <-tick.C:
			require.NoError(t, f.h.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))`, holderPID).Scan(&blocked))
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	require.NoError(t, f.workflow.UpsertStepParticipant(ctx, &wfmodels.WorkflowStepParticipant{ID: "racing-id", StepID: "foreign-step", Role: wfmodels.ParticipantRoleApprover, AgentProfileID: "agent-b"}))
	require.NoError(t, holder.Rollback())
	require.ErrorIs(t, <-done, repoerrors.ErrWorkflowChangeConflict)
	var step, agent string
	require.NoError(t, f.h.db.QueryRowContext(ctx, `SELECT step_id,agent_profile_id FROM workflow_step_participants WHERE id='racing-id'`).Scan(&step, &agent))
	require.Equal(t, "foreign-step", step)
	require.Equal(t, "agent-b", agent)
}

func TestOfficeMutationScopeOppositeParticipantMoves(t *testing.T) {
	_ = testutil.PostgresDSNFromEnv(t)
	f := newOfficeAtomicFixture(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	require.NoError(t, f.workflow.CreateStep(ctx, &wfmodels.WorkflowStep{ID: "foreign-step", WorkflowID: "wf-other-workspace", Name: "Foreign", Position: 0}))
	seats := []*wfmodels.WorkflowStepParticipant{{ID: "seat-a", StepID: "step-home", Role: wfmodels.ParticipantRoleApprover, AgentProfileID: "agent-a"}, {ID: "seat-b", StepID: "foreign-step", Role: wfmodels.ParticipantRoleApprover, AgentProfileID: "agent-b"}}
	for _, seat := range seats {
		require.NoError(t, f.workflow.UpsertStepParticipant(ctx, seat))
	}
	seats[0].StepID, seats[1].StepID = seats[1].StepID, seats[0].StepID
	start := make(chan struct{})
	done := make(chan error, 2)
	for _, seat := range seats {
		go func() { <-start; done <- f.workflow.UpsertStepParticipant(ctx, seat) }()
	}
	close(start)
	for range seats {
		require.NoError(t, <-done)
	}
	for _, seat := range seats {
		var step string
		require.NoError(t, f.h.db.QueryRowContext(ctx, `SELECT step_id FROM workflow_step_participants WHERE id=$1`, seat.ID).Scan(&step))
		require.Equal(t, seat.StepID, step)
	}
}

func TestTransferWorkspaceFirstOppositeTransfers(t *testing.T) {
	_ = testutil.PostgresDSNFromEnv(t)
	f := newOfficeAtomicFixture(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := f.h.db.Exec(`UPDATE workspaces SET office_workflow_id='',owner_id='owner' WHERE id IN ('ws-1','ws-2')`)
	require.NoError(t, err)
	require.NoError(t, f.workflow.CreateStep(ctx, &wfmodels.WorkflowStep{ID: "foreign-home", WorkflowID: "wf-other-workspace", Name: "Home", Position: 0}))
	require.NoError(t, f.h.repo.CreateTask(ctx, &models.Task{ID: "other-task", WorkspaceID: "ws-2", WorkflowID: "wf-other-workspace", WorkflowStepID: "foreign-home", Title: "Other"}))
	var commands []models.TaskTransferCommand
	for _, id := range []string{"atomic-task", "other-task"} {
		task, err := f.h.repo.GetTask(ctx, id)
		require.NoError(t, err)
		workspace, workflow, step := "ws-2", "wf-other-workspace", "foreign-home"
		if id == "other-task" {
			workspace, workflow, step = "ws-1", "wf-home", "step-home"
		}
		commands = append(commands, models.TaskTransferCommand{TaskID: id, ExpectedSourceWorkspaceID: task.WorkspaceID, ExpectedSourceWorkflowID: task.WorkflowID, ExpectedSourceStepID: task.WorkflowStepID, ExpectedTaskUpdatedAt: task.UpdatedAt, DestinationWorkspaceID: workspace, DestinationWorkflowID: workflow, DestinationStepID: step, IdempotencyKey: "opposite-" + id, PreservationPolicy: models.TaskTransferPreservationPolicyV1, Actor: models.TaskTransferActor{Kind: models.TaskTransferActorHuman, ID: "owner"}, OwnerPredicateSet: true, AuthorizedOwnerID: "owner"})
	}
	start := make(chan struct{})
	done := make(chan error, 2)
	for _, command := range commands {
		go func() { <-start; _, err := f.h.repo.TransferTask(ctx, command); done <- err }()
	}
	close(start)
	for range commands {
		err := <-done
		if err != nil {
			require.True(t, errors.Is(err, repoerrors.ErrTaskTransferConflict), "opposite transfers must terminate with success or the ratified contention conflict: %v", err)
		}
	}
	// A caller may submit each exact command again after contention; no server
	// retry loop or source-following policy is introduced by that manual retry.
	for _, command := range commands {
		receipt, err := f.h.repo.TransferTask(ctx, command)
		require.NoError(t, err)
		require.Equal(t, command.DestinationWorkspaceID, receipt.DestinationWorkspaceID)
		task, err := f.h.repo.GetTask(ctx, command.TaskID)
		require.NoError(t, err)
		require.Equal(t, command.DestinationWorkspaceID, task.WorkspaceID)
	}
	var operations, failed int
	require.NoError(t, f.h.db.QueryRowContext(ctx, `SELECT count(*) FROM task_transfer_operations`).Scan(&operations))
	require.Equal(t, 2, operations)
	require.NoError(t, f.h.db.QueryRowContext(ctx, `SELECT count(*) FROM task_transfer_audit WHERE result='failed'`).Scan(&failed))
	require.Zero(t, failed)
}

func TestOfficeMutationScopeUsesReadCommitted(t *testing.T) {
	_ = testutil.PostgresDSNFromEnv(t)
	f := newOfficeAtomicFixture(t, nil)
	writer, writerDB := officeAtomicSecondWriterWithDB(t, f.h)
	_, err := writerDB.Exec(`SET default_transaction_isolation='repeatable read'`)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	reader := officeHumanCompletionPendingReader(f.office, f.workflow)
	reached := make(chan int, 1)
	resume := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(resume)
		}
	}()
	f.h.repo.SetOfficeHumanCompletionPendingReader(func(ctx context.Context, tx *sql.Tx, taskID, stepID string) ([]string, error) {
		ids, err := reader(ctx, tx, taskID, stepID)
		if err != nil {
			return nil, err
		}
		var pid int
		if err := tx.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
			return nil, err
		}
		reached <- pid
		select {
		case <-resume:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return ids, nil
	})
	completed := make(chan error, 1)
	go func() {
		_, err := f.svc.MoveTaskWithOptions(ctx, "atomic-task", "wf-home", "atomic-done", 0, taskservice.MoveTaskOptions{StepHistoryActor: wfmodels.StepTransitionActorHuman})
		completed <- err
	}()
	var owner int
	select {
	case owner = <-reached:
	case err := <-completed:
		t.Fatalf("admission failed before canonical read: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	done := make(chan error, 1)
	go func() {
		_, err := writer.UpsertTaskParticipant(ctx, "step-home", "atomic-task", "approver", "after-admission")
		done <- err
	}()
	blocked := false
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for !blocked {
		select {
		case err := <-done:
			t.Fatalf("writer passed held admission: %v", err)
		case <-tick.C:
			require.NoError(t, f.h.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))`, owner).Scan(&blocked))
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	close(resume)
	released = true
	require.NoError(t, <-completed)
	require.ErrorIs(t, <-done, repoerrors.ErrWorkflowChangeConflict, "source revalidation must observe the completion even when the server default is repeatable read")
}

func TestOfficeParticipantInitiallyMissingTaskCannotAppearUnderRoleLock(t *testing.T) {
	_ = testutil.PostgresDSNFromEnv(t)
	f := newOfficeAtomicFixture(t, nil)
	_, cleanup, err := settingsstore.Provide(f.h.db, f.h.db, f.log)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cleanup() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	holder, err := f.h.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = holder.Rollback() }()
	_, err = holder.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, workflowrepo.ParticipantRoleSeatLockKey("appearing-task", "approver"))
	require.NoError(t, err)
	var pid int
	require.NoError(t, holder.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid))
	type outcome struct {
		result officesqlite.ParticipantWriteResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := f.office.AddTaskParticipant(ctx, "appearing-task", "appearing-approver", "approver")
		done <- outcome{result, err}
	}()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	var early *outcome
	blocked := false
	for !blocked && early == nil {
		select {
		case result := <-done:
			early = &result
		case <-tick.C:
			require.NoError(t, f.h.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))`, pid).Scan(&blocked))
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	// Production repository creation accepts a caller-supplied ID. It does not
	// take the approver-role key held above and can commit in this window.
	f.h.createTask(t, "appearing-task", false)
	require.NoError(t, holder.Rollback())
	var result outcome
	if early != nil {
		result = *early
	} else {
		select {
		case result = <-done:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	require.NoError(t, result.err)
	require.Equal(t, officesqlite.ParticipantWriteOutcomeUnchanged, result.result.Outcome)
	var seats int
	require.NoError(t, f.h.db.QueryRowContext(ctx, `SELECT count(*) FROM workflow_step_participants WHERE task_id='appearing-task'`).Scan(&seats))
	require.Zero(t, seats, "an initially absent task cannot acquire an unfenced approver")
	// The normal subsequent call locates the now-existing workspace and works.
	result.result, result.err = f.office.AddTaskParticipant(ctx, "appearing-task", "appearing-approver", "approver")
	require.NoError(t, result.err)
	require.Equal(t, officesqlite.ParticipantWriteOutcomeInserted, result.result.Outcome)
}
