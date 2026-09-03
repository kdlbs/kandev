package sqlite

// PostgreSQL proof for the dialect-sensitive row lock, receipt JSON, schema,
// and audit statements used by TransferTask. It skips unless
// KANDEV_TEST_POSTGRES_DSN is set; CI runs these tests in postgres-boot.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresTransferTaskRoundTrip(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	db.SetMaxOpenConns(2)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	for _, workspace := range []*models.Workspace{
		{ID: "pg-transfer-source", Name: "Source", OwnerID: "owner-1"},
		{ID: "pg-transfer-destination", Name: "Destination", OwnerID: "owner-1"},
	} {
		if err := repo.CreateWorkspace(ctx, workspace); err != nil {
			t.Fatalf("CreateWorkspace: %v", err)
		}
	}
	for _, workflow := range []*models.Workflow{
		{ID: "pg-transfer-wf-source", WorkspaceID: "pg-transfer-source", Name: "Source"},
		{ID: "pg-transfer-wf-destination", WorkspaceID: "pg-transfer-destination", Name: "Destination"},
	} {
		if err := repo.CreateWorkflow(ctx, workflow); err != nil {
			t.Fatalf("CreateWorkflow: %v", err)
		}
	}
	for _, step := range []struct{ id, workflow string }{
		{"pg-transfer-step-source", "pg-transfer-wf-source"},
		{"pg-transfer-step-destination", "pg-transfer-wf-destination"},
	} {
		if _, err := db.Exec(db.Rebind(
			`INSERT INTO workflow_steps (id, workflow_id, name, position) VALUES (?, ?, 'Work', 0)`),
			step.id, step.workflow); err != nil {
			t.Fatalf("insert workflow step: %v", err)
		}
	}
	task := &models.Task{ID: "pg-transfer-task", WorkspaceID: "pg-transfer-source",
		WorkflowID: "pg-transfer-wf-source", WorkflowStepID: "pg-transfer-step-source", Title: "Synthetic"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	task, err = repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	command := models.TaskTransferCommand{
		TaskID: task.ID, ExpectedSourceWorkspaceID: task.WorkspaceID, ExpectedSourceWorkflowID: task.WorkflowID,
		ExpectedSourceStepID: task.WorkflowStepID, ExpectedTaskUpdatedAt: task.UpdatedAt,
		DestinationWorkspaceID: "pg-transfer-destination", DestinationWorkflowID: "pg-transfer-wf-destination",
		DestinationStepID: "pg-transfer-step-destination", IdempotencyKey: "pg-transfer-key",
		PreservationPolicy: models.TaskTransferPreservationPolicyV1,
		Actor:              models.TaskTransferActor{Kind: models.TaskTransferActorHuman, ID: "owner-1"},
		AuthorizedOwnerID:  "owner-1", OwnerPredicateSet: true,
	}
	receipts := make([]*models.TaskTransferReceipt, 2)
	errs := make([]error, 2)
	start := make(chan struct{})
	var writers sync.WaitGroup
	for index := range receipts {
		writers.Add(1)
		go func(index int) {
			defer writers.Done()
			<-start
			receipts[index], errs[index] = repo.TransferTask(ctx, command)
		}(index)
	}
	close(start)
	writers.Wait()
	for index, transferErr := range errs {
		if transferErr != nil {
			t.Fatalf("TransferTask writer %d: %v", index, transferErr)
		}
	}
	receipt := receipts[0]
	if receipt.TaskID != task.ID || receipt.DestinationWorkspaceID != "pg-transfer-destination" {
		t.Fatalf("receipt = %+v", receipt)
	}
	if receipts[0].OperationID != receipts[1].OperationID {
		t.Fatalf("concurrent operation IDs differ: %q != %q", receipts[0].OperationID, receipts[1].OperationID)
	}
}

func TestPostgresTaskTransferRelationLockBlocksWriter(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	db.SetMaxOpenConns(2)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE pending_moves (
		id TEXT PRIMARY KEY, task_id TEXT NOT NULL, workflow_id TEXT NOT NULL, workflow_step_id TEXT NOT NULL)`); err != nil {
		t.Fatalf("create pending_moves: %v", err)
	}
	tx, err := db.BeginTxx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	relations := []transferWorkspaceProjection{{table: "pending_moves", taskColumn: "task_id", identityColumn: "id"}}
	if err := repo.lockTaskTransferRelations(context.Background(), tx, nil, relations, transferRelationInventory{}); err != nil {
		t.Fatalf("lockTaskTransferRelations: %v", err)
	}
	writerCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, writeErr := db.ExecContext(writerCtx, `INSERT INTO pending_moves (id, task_id, workflow_id, workflow_step_id)
		VALUES ('pending-race', 'task-race', 'workflow-source', 'step-source')`)
	if !errors.Is(writeErr, context.DeadlineExceeded) {
		t.Fatalf("writer error = %v, want deadline while relation lock held", writeErr)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO pending_moves (id, task_id, workflow_id, workflow_step_id)
		VALUES ('pending-after', 'task-race', 'workflow-source', 'step-source')`); err != nil {
		t.Fatalf("writer after lock release: %v", err)
	}
}

func TestPostgresTransferTaskConcurrentCapacityAndChangedKey(t *testing.T) {
	tests := []struct {
		name      string
		wipLimit  int
		sharedKey bool
	}{
		{name: "destination capacity", wipLimit: 1},
		{name: "changed request same key", sharedKey: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, tasks := seedPostgresConcurrentTransferTasks(t, tt.wipLimit)
			commands := make([]models.TaskTransferCommand, len(tasks))
			for index, task := range tasks {
				key := "pg-concurrent-key-" + task.ID
				if tt.sharedKey {
					key = "pg-concurrent-shared-key"
				}
				commands[index] = models.TaskTransferCommand{
					TaskID: task.ID, ExpectedSourceWorkspaceID: task.WorkspaceID,
					ExpectedSourceWorkflowID: task.WorkflowID, ExpectedSourceStepID: task.WorkflowStepID,
					ExpectedTaskUpdatedAt: task.UpdatedAt, DestinationWorkspaceID: "pg-race-destination",
					DestinationWorkflowID: "pg-race-wf-destination", DestinationStepID: "pg-race-step-destination",
					IdempotencyKey: key, PreservationPolicy: models.TaskTransferPreservationPolicyV1,
					Actor:             models.TaskTransferActor{Kind: models.TaskTransferActorHuman, ID: "owner-1"},
					AuthorizedOwnerID: "owner-1", OwnerPredicateSet: true,
				}
			}
			errs := runConcurrentPostgresTransfers(repo, commands)
			var committed, conflicted int
			for _, err := range errs {
				switch {
				case err == nil:
					committed++
				case errors.Is(err, repoerrors.ErrTaskTransferConflict):
					conflicted++
				default:
					t.Fatalf("unexpected transfer error: %v", err)
				}
			}
			if committed != 1 || conflicted != 1 {
				t.Fatalf("committed=%d conflicted=%d errors=%v", committed, conflicted, errs)
			}
		})
	}
}

func TestPostgresTransferTaskWaitsForDestinationLaneCapacityDecision(t *testing.T) {
	repo, tasks := seedPostgresConcurrentTransferTasks(t, 1)
	db := repo.db
	holder, err := db.BeginTxx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	defer func() { _ = holder.Rollback() }()
	var stepID string
	if err := holder.Get(&stepID, `SELECT id FROM workflow_steps
		WHERE id = 'pg-race-step-destination' FOR UPDATE`); err != nil {
		t.Fatalf("lock destination lane: %v", err)
	}
	if _, err := holder.Exec(`UPDATE tasks SET workspace_id = 'pg-race-destination',
		workflow_id = 'pg-race-wf-destination', workflow_step_id = 'pg-race-step-destination',
		wip_admitted = 1, queued_for_step_id = '' WHERE id = $1`, tasks[1].ID); err != nil {
		t.Fatalf("stage destination occupant: %v", err)
	}
	command := models.TaskTransferCommand{
		TaskID: tasks[0].ID, ExpectedSourceWorkspaceID: tasks[0].WorkspaceID,
		ExpectedSourceWorkflowID: tasks[0].WorkflowID, ExpectedSourceStepID: tasks[0].WorkflowStepID,
		ExpectedTaskUpdatedAt: tasks[0].UpdatedAt, DestinationWorkspaceID: "pg-race-destination",
		DestinationWorkflowID: "pg-race-wf-destination", DestinationStepID: "pg-race-step-destination",
		IdempotencyKey: "pg-capacity-lock-key", PreservationPolicy: models.TaskTransferPreservationPolicyV1,
		Actor:             models.TaskTransferActor{Kind: models.TaskTransferActorHuman, ID: "owner-1"},
		AuthorizedOwnerID: "owner-1", OwnerPredicateSet: true,
	}
	done := make(chan error, 1)
	go func() {
		_, transferErr := repo.TransferTask(context.Background(), command)
		done <- transferErr
	}()
	select {
	case transferErr := <-done:
		t.Fatalf("transfer bypassed destination lane lock: %v", transferErr)
	case <-time.After(100 * time.Millisecond):
	}
	if err := holder.Commit(); err != nil {
		t.Fatalf("commit destination occupant: %v", err)
	}
	select {
	case transferErr := <-done:
		if !errors.Is(transferErr, repoerrors.ErrTaskTransferConflict) {
			t.Fatalf("TransferTask error = %v, want capacity conflict", transferErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("transfer remained blocked after destination lane commit")
	}
}

func TestPostgresTransferTaskSerializesTreeHoldCreation(t *testing.T) {
	repo, tasks := seedPostgresConcurrentTransferTasks(t, 0)
	db := repo.db
	if _, err := db.Exec(`CREATE TABLE office_task_tree_holds (
		id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, root_task_id TEXT NOT NULL)`); err != nil {
		t.Fatalf("create tree holds: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE office_task_tree_hold_members (
		hold_id TEXT NOT NULL, task_id TEXT NOT NULL)`); err != nil {
		t.Fatalf("create tree hold members: %v", err)
	}
	holder, err := db.BeginTxx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	defer func() { _ = holder.Rollback() }()
	if _, err := holder.Exec(`INSERT INTO office_task_tree_holds
		(id, workspace_id, root_task_id) VALUES ('hold-race', 'pg-race-source', $1)`, tasks[0].ID); err != nil {
		t.Fatalf("stage tree hold: %v", err)
	}
	command := models.TaskTransferCommand{
		TaskID: tasks[0].ID, ExpectedSourceWorkspaceID: tasks[0].WorkspaceID,
		ExpectedSourceWorkflowID: tasks[0].WorkflowID, ExpectedSourceStepID: tasks[0].WorkflowStepID,
		ExpectedTaskUpdatedAt: tasks[0].UpdatedAt, DestinationWorkspaceID: "pg-race-destination",
		DestinationWorkflowID: "pg-race-wf-destination", DestinationStepID: "pg-race-step-destination",
		IdempotencyKey: "pg-tree-hold-lock-key", PreservationPolicy: models.TaskTransferPreservationPolicyV1,
		Actor:             models.TaskTransferActor{Kind: models.TaskTransferActorHuman, ID: "owner-1"},
		AuthorizedOwnerID: "owner-1", OwnerPredicateSet: true,
	}
	done := make(chan error, 1)
	go func() {
		_, transferErr := repo.TransferTask(context.Background(), command)
		done <- transferErr
	}()
	select {
	case transferErr := <-done:
		t.Fatalf("transfer bypassed tree-hold serialization: %v", transferErr)
	case <-time.After(100 * time.Millisecond):
	}
	if err := holder.Commit(); err != nil {
		t.Fatalf("commit tree hold: %v", err)
	}
	select {
	case transferErr := <-done:
		if !errors.Is(transferErr, repoerrors.ErrTaskTransferConflict) {
			t.Fatalf("TransferTask error = %v, want tree-hold conflict", transferErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("transfer remained blocked after tree-hold commit")
	}
}

func TestPostgresTransferTaskBindsAuthorizedOwnerAtSerializationPoint(t *testing.T) {
	repo, tasks := seedPostgresConcurrentTransferTasks(t, 0)
	holder, err := repo.db.BeginTxx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	defer func() { _ = holder.Rollback() }()
	if _, err := holder.Exec(`UPDATE workspaces SET owner_id = 'owner-2'
		WHERE id IN ('pg-race-source', 'pg-race-destination')`); err != nil {
		t.Fatalf("stage ownership revocation: %v", err)
	}
	command := postgresRaceTransferCommand(tasks[0], "pg-owner-lock-key")
	done := make(chan error, 1)
	go func() {
		_, transferErr := repo.TransferTask(context.Background(), command)
		done <- transferErr
	}()
	assertPostgresTransferBlocked(t, done, "ownership serialization")
	if err := holder.Commit(); err != nil {
		t.Fatalf("commit ownership revocation: %v", err)
	}
	assertPostgresTransferConflict(t, done, "ownership revocation")
}

func TestPostgresTransferTaskSerializesCoordinatorSessionRevocation(t *testing.T) {
	repo, tasks := seedPostgresConcurrentTransferTasks(t, 0)
	if _, err := repo.db.Exec(`UPDATE workspaces SET office_workflow_id = 'pg-race-wf-source'
		WHERE id = 'pg-race-source'`); err != nil {
		t.Fatalf("mark source workflow as Office: %v", err)
	}
	if _, err := repo.db.Exec(`CREATE TABLE agent_profiles (
		id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '', role TEXT NOT NULL DEFAULT '',
		deleted_at TIMESTAMP, enabled INTEGER NOT NULL DEFAULT 1, status TEXT NOT NULL DEFAULT 'idle')`); err != nil {
		t.Fatalf("create agent profiles: %v", err)
	}
	if _, err := repo.db.Exec(`INSERT INTO agent_profiles (id, workspace_id, role, status) VALUES
		('pg-ceo-source', 'pg-race-source', 'ceo', 'working'),
		('pg-ceo-destination', 'pg-race-destination', 'ceo', 'idle')`); err != nil {
		t.Fatalf("insert agent profiles: %v", err)
	}
	if _, err := repo.db.Exec(`INSERT INTO workflow_step_participants
		(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES ('pg-ceo-runner', 'pg-race-step-source', $1, 'runner', 'pg-ceo-source', 0, 0)`, tasks[0].ID); err != nil {
		t.Fatalf("insert CEO runner: %v", err)
	}
	if err := repo.CreateTaskSession(context.Background(), &models.TaskSession{
		ID: "pg-ceo-session", TaskID: tasks[0].ID, AgentProfileID: "pg-ceo-source",
		State: models.TaskSessionStateRunning, IsPrimary: true,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	holder, err := repo.db.BeginTxx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	defer func() { _ = holder.Rollback() }()
	if _, err := holder.Exec(`UPDATE task_sessions SET state = 'COMPLETED' WHERE id = 'pg-ceo-session'`); err != nil {
		t.Fatalf("stage session revocation: %v", err)
	}
	command := postgresRaceTransferCommand(tasks[0], "pg-session-lock-key")
	command.Actor = models.TaskTransferActor{
		Kind: models.TaskTransferActorCoordinator, ID: "pg-ceo-source", SessionID: "pg-ceo-session",
		CallerTaskID: tasks[0].ID,
	}
	done := make(chan error, 1)
	go func() {
		_, transferErr := repo.TransferTask(context.Background(), command)
		done <- transferErr
	}()
	assertPostgresTransferBlocked(t, done, "coordinator session serialization")
	if err := holder.Commit(); err != nil {
		t.Fatalf("commit session revocation: %v", err)
	}
	assertPostgresTransferConflict(t, done, "session revocation")
}

func postgresRaceTransferCommand(task *models.Task, key string) models.TaskTransferCommand {
	return models.TaskTransferCommand{
		TaskID: task.ID, ExpectedSourceWorkspaceID: task.WorkspaceID,
		ExpectedSourceWorkflowID: task.WorkflowID, ExpectedSourceStepID: task.WorkflowStepID,
		ExpectedTaskUpdatedAt: task.UpdatedAt, DestinationWorkspaceID: "pg-race-destination",
		DestinationWorkflowID: "pg-race-wf-destination", DestinationStepID: "pg-race-step-destination",
		IdempotencyKey: key, PreservationPolicy: models.TaskTransferPreservationPolicyV1,
		Actor:             models.TaskTransferActor{Kind: models.TaskTransferActorHuman, ID: "owner-1"},
		AuthorizedOwnerID: "owner-1", OwnerPredicateSet: true,
	}
}

func assertPostgresTransferBlocked(t *testing.T, done <-chan error, reason string) {
	t.Helper()
	select {
	case transferErr := <-done:
		t.Fatalf("transfer bypassed %s: %v", reason, transferErr)
	case <-time.After(100 * time.Millisecond):
	}
}

func assertPostgresTransferConflict(t *testing.T, done <-chan error, reason string) {
	t.Helper()
	select {
	case transferErr := <-done:
		if !errors.Is(transferErr, repoerrors.ErrTaskTransferConflict) {
			t.Fatalf("TransferTask error after %s = %v, want conflict", reason, transferErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("transfer remained blocked after %s", reason)
	}
}

func seedPostgresConcurrentTransferTasks(t *testing.T, wipLimit int) (*Repository, []*models.Task) {
	t.Helper()
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	db.SetMaxOpenConns(2)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	for _, workspace := range []*models.Workspace{
		{ID: "pg-race-source", Name: "Source", OwnerID: "owner-1"},
		{ID: "pg-race-destination", Name: "Destination", OwnerID: "owner-1"},
	} {
		if err := repo.CreateWorkspace(ctx, workspace); err != nil {
			t.Fatalf("CreateWorkspace: %v", err)
		}
	}
	for _, workflow := range []*models.Workflow{
		{ID: "pg-race-wf-source", WorkspaceID: "pg-race-source", Name: "Source"},
		{ID: "pg-race-wf-destination", WorkspaceID: "pg-race-destination", Name: "Destination"},
	} {
		if err := repo.CreateWorkflow(ctx, workflow); err != nil {
			t.Fatalf("CreateWorkflow: %v", err)
		}
	}
	for _, step := range []struct{ id, workflow string }{
		{"pg-race-step-source", "pg-race-wf-source"},
		{"pg-race-step-destination", "pg-race-wf-destination"},
	} {
		if _, err := db.Exec(db.Rebind(`INSERT INTO workflow_steps
			(id, workflow_id, name, position, wip_limit) VALUES (?, ?, 'Work', 0, ?)`),
			step.id, step.workflow, wipLimit); err != nil {
			t.Fatalf("insert workflow step: %v", err)
		}
	}
	tasks := make([]*models.Task, 2)
	for index := range tasks {
		task := &models.Task{ID: fmt.Sprintf("pg-race-task-%d", index), WorkspaceID: "pg-race-source",
			WorkflowID: "pg-race-wf-source", WorkflowStepID: "pg-race-step-source", Title: "Synthetic"}
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		tasks[index], err = repo.GetTask(ctx, task.ID)
		if err != nil {
			t.Fatalf("GetTask: %v", err)
		}
	}
	return repo, tasks
}

func runConcurrentPostgresTransfers(repo *Repository, commands []models.TaskTransferCommand) []error {
	errs := make([]error, len(commands))
	start := make(chan struct{})
	var writers sync.WaitGroup
	for index := range commands {
		writers.Add(1)
		go func(index int) {
			defer writers.Done()
			<-start
			_, errs[index] = repo.TransferTask(context.Background(), commands[index])
		}(index)
	}
	close(start)
	writers.Wait()
	return errs
}
