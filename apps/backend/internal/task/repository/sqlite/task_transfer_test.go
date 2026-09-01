package sqlite

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestTaskTransferSchemaAndRepositorySurface(t *testing.T) {
	repo := newRepoForWorkflowSourceTests(t)
	for range 2 {
		if err := repo.initTaskTransferSchema(); err != nil {
			t.Fatalf("replay task transfer schema: %v", err)
		}
	}

	for _, table := range []string{"task_transfer_operations", "task_transfer_audit"} {
		var count int
		if err := repo.db.Get(&count,
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table); err != nil {
			t.Fatalf("inspect %s: %v", table, err)
		}
		if count != 1 {
			t.Errorf("table %s count = %d, want 1", table, count)
		}
	}

	if !reflect.ValueOf(repo).MethodByName("TransferTask").IsValid() {
		t.Error("Repository.TransferTask method is missing")
	}
}

func TestTransferTaskPreservesIdentityAndIsIdempotent(t *testing.T) {
	repo, task := seedTaskTransferFixture(t)
	ctx := context.Background()
	command := taskTransferCommand(task)

	receipt, err := repo.TransferTask(ctx, command)
	if err != nil {
		t.Fatalf("TransferTask: %v", err)
	}
	if receipt.TaskID != task.ID || receipt.DestinationWorkspaceID != "ws-destination" ||
		receipt.DestinationStepID != "step-destination-work" || receipt.StepTransitionID == 0 {
		t.Fatalf("receipt placement = %+v", receipt)
	}
	if len(receipt.Sessions) != 1 || receipt.Sessions[0].ID != "session-running" ||
		receipt.Sessions[0].State != models.TaskSessionStateRunning || receipt.Sessions[0].TurnID != "turn-active" {
		t.Fatalf("session census = %+v", receipt.Sessions)
	}
	if receipt.PreservationDigest == "" || receipt.PreservationCounts["task_plans"] != 1 ||
		receipt.PreservationCounts["task_repositories"] != 1 {
		t.Fatalf("preservation receipt = %+v", receipt)
	}

	stored, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.ID != task.ID || stored.WorkspaceID != "ws-destination" ||
		stored.WorkflowID != "wf-destination" || stored.WorkflowStepID != "step-destination-work" {
		t.Fatalf("stored placement = %+v", stored)
	}
	if stored.State != v1.TaskStateInProgress || stored.Description != "preserve description" ||
		stored.ParentID != "parent-task" || stored.QueuedForStepID != "step-destination-blocked" {
		t.Fatalf("stored durable identity changed = %+v", stored)
	}

	for _, table := range []string{"task_status_summaries", "task_message_attachments", "github_task_prs"} {
		var workspaceID string
		if err := repo.db.Get(&workspaceID, `SELECT workspace_id FROM `+table+` WHERE task_id = ?`, task.ID); err != nil {
			t.Fatalf("read %s workspace: %v", table, err)
		}
		if workspaceID != "ws-destination" {
			t.Errorf("%s workspace_id = %q, want destination", table, workspaceID)
		}
	}
	var transition struct {
		Trigger   string `db:"trigger"`
		ActorKind string `db:"actor_kind"`
		ActorID   string `db:"actor_id"`
	}
	if err := repo.db.Get(&transition, `SELECT trigger, actor_kind, COALESCE(actor_id, '') AS actor_id
		FROM task_step_transitions WHERE task_id = ? ORDER BY id DESC LIMIT 1`, task.ID); err != nil {
		t.Fatalf("read transfer transition: %v", err)
	}
	if transition.Trigger != "task_transfer" || transition.ActorKind != "human" || transition.ActorID != "owner-1" {
		t.Fatalf("transfer transition = %+v", transition)
	}

	retry, err := repo.TransferTask(ctx, command)
	if err != nil {
		t.Fatalf("TransferTask retry: %v", err)
	}
	if retry.OperationID != receipt.OperationID || retry.PreservationDigest != receipt.PreservationDigest ||
		!retry.TransferredAt.Equal(receipt.TransferredAt) {
		t.Fatalf("retry receipt changed: first=%+v retry=%+v", receipt, retry)
	}
	replayActor, found, err := repo.ResolveTaskTransferReplayActor(ctx, command)
	if err != nil || !found || replayActor != command.Actor {
		t.Fatalf("replay actor = %+v, found=%v, err=%v", replayActor, found, err)
	}

	changed := command
	changed.DestinationStepID = "step-destination-blocked"
	if _, err := repo.TransferTask(ctx, changed); !errors.Is(err, repoerrors.ErrTaskTransferConflict) {
		t.Fatalf("changed retry error = %v, want conflict", err)
	}
	var auditResults []string
	if err := repo.db.Select(&auditResults,
		`SELECT result FROM task_transfer_audit WHERE task_id = ? ORDER BY created_at, id`, task.ID); err != nil {
		t.Fatalf("read transfer audit: %v", err)
	}
	if len(auditResults) != 3 || !containsAuditResult(auditResults, "transferred") ||
		!containsAuditResult(auditResults, "idempotent_replay") || !containsAuditResult(auditResults, "conflict") {
		t.Fatalf("audit results = %v", auditResults)
	}
	var redactedAudit string
	if err := repo.db.Get(&redactedAudit, `SELECT session_census_json FROM task_transfer_audit
		WHERE task_id = ? AND result = 'transferred'`, task.ID); err != nil {
		t.Fatalf("read audit census: %v", err)
	}
	for _, sensitive := range []string{"synthetic plan content", "/synthetic/worktree", "preserve description"} {
		if strings.Contains(redactedAudit, sensitive) {
			t.Fatalf("audit census contains sensitive content %q: %s", sensitive, redactedAudit)
		}
	}
}

func TestTransferTaskNameSelectionCanonicalizesExactReplay(t *testing.T) {
	repo, task := seedTaskTransferFixture(t)
	command := taskTransferCommand(task)
	command.DestinationStepID = ""
	command.DestinationStepName = " Work "
	receipt, err := repo.TransferTask(context.Background(), command)
	if err != nil {
		t.Fatalf("TransferTask: %v", err)
	}
	replay, err := repo.TransferTask(context.Background(), command)
	if err != nil || replay.OperationID != receipt.OperationID || !replay.IdempotentReplay {
		t.Fatalf("TransferTask replay = %+v, err=%v", replay, err)
	}
	actor, found, err := repo.ResolveTaskTransferReplayActor(context.Background(), command)
	if err != nil || !found || actor != command.Actor {
		t.Fatalf("ResolveTaskTransferReplayActor = %+v, found=%v, err=%v", actor, found, err)
	}
}

func TestTransferTaskAuditsValidationAndTransactionFailuresAfterCancellation(t *testing.T) {
	tests := []struct {
		name    string
		command func(*models.Task) models.TaskTransferCommand
		result  string
	}{
		{name: "validation conflict", command: func(task *models.Task) models.TaskTransferCommand {
			command := taskTransferCommand(task)
			command.DestinationWorkflowID = ""
			return command
		}, result: taskTransferResultConflict},
		{name: "transaction failure", command: taskTransferCommand, result: taskTransferResultFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, task := seedTaskTransferFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err := repo.TransferTask(ctx, tt.command(task)); err == nil {
				t.Fatal("TransferTask unexpectedly succeeded")
			}
			var audits int
			if err := repo.db.Get(&audits, `SELECT COUNT(*) FROM task_transfer_audit
				WHERE task_id = ? AND result = ?`, task.ID, tt.result); err != nil {
				t.Fatalf("read cancellation audit: %v", err)
			}
			if audits != 1 {
				t.Fatalf("audit rows = %d, want 1", audits)
			}
		})
	}
}

func TestHandleTaskTransferOperationViolationDeterministicallyRecoversChangedKeyConflict(t *testing.T) {
	repo, task := seedTaskTransferFixture(t)
	command := taskTransferCommand(task)
	if _, err := repo.TransferTask(context.Background(), command); err != nil {
		t.Fatalf("TransferTask: %v", err)
	}
	changed := command
	changed.TaskID = "different-task"
	digest, err := taskTransferRequestDigest(changed)
	if err != nil {
		t.Fatalf("taskTransferRequestDigest: %v", err)
	}
	uniqueErr := errors.New(sqliteTaskTransferOperationViolation)
	if _, err := repo.handleTaskTransferApplyError(context.Background(), changed, digest, uniqueErr); !errors.Is(err, repoerrors.ErrTaskTransferConflict) {
		t.Fatalf("handleTaskTransferApplyError = %v, want conflict", err)
	}
}

func containsAuditResult(results []string, want string) bool {
	for _, result := range results {
		if result == want {
			return true
		}
	}
	return false
}

func TestTransferTaskConflictsLeavePlacementUntouched(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*models.TaskTransferCommand)
		setup  func(*testing.T, *Repository)
	}{
		{name: "stale generation", mutate: func(c *models.TaskTransferCommand) {
			c.ExpectedTaskUpdatedAt = c.ExpectedTaskUpdatedAt.Add(-time.Second)
		}},
		{name: "stale source lane", mutate: func(c *models.TaskTransferCommand) {
			c.ExpectedSourceStepID = "step-source-blocked"
		}},
		{name: "ambiguous destination name", mutate: func(c *models.TaskTransferCommand) {
			c.DestinationStepID = ""
			c.DestinationStepName = "Work"
		}, setup: func(t *testing.T, repo *Repository) {
			mustExecTransferTest(t, repo, `INSERT INTO workflow_steps (id, workflow_id, name, position) VALUES (?, ?, ?, ?)`,
				"step-destination-work-duplicate", "wf-destination", "Work", 4)
		}},
		{name: "different semantic lane", mutate: func(c *models.TaskTransferCommand) {
			c.DestinationStepID = "step-destination-blocked"
		}},
		{name: "incompatible destination label", setup: func(t *testing.T, repo *Repository) {
			mustExecTransferTest(t, repo, `CREATE TABLE office_labels (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, name TEXT NOT NULL)`)
			mustExecTransferTest(t, repo, `CREATE TABLE office_task_labels (task_id TEXT NOT NULL, label_id TEXT NOT NULL)`)
			mustExecTransferTest(t, repo, `INSERT INTO office_labels (id, workspace_id, name) VALUES (?, ?, ?)`,
				"label-source", "ws-source", "urgent")
			mustExecTransferTest(t, repo, `INSERT INTO office_task_labels (task_id, label_id) VALUES (?, ?)`,
				"task-transfer", "label-source")
		}},
		{name: "cleanup in progress", setup: func(t *testing.T, repo *Repository) {
			now := time.Now().UTC()
			mustExecTransferTest(t, repo, `INSERT INTO task_resource_cleanup_jobs
				(id, operation_id, task_id, trigger, state, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				"cleanup-1", "cleanup-op", "task-transfer", "archive", "running", now, now)
		}},
		{name: "workspace group relation", setup: func(t *testing.T, repo *Repository) {
			mustExecTransferTest(t, repo, `CREATE TABLE task_workspace_groups
				(id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, owner_task_id TEXT NOT NULL)`)
			mustExecTransferTest(t, repo, `CREATE TABLE task_workspace_group_members
				(workspace_group_id TEXT NOT NULL, task_id TEXT NOT NULL)`)
			mustExecTransferTest(t, repo, `INSERT INTO task_workspace_groups (id, workspace_id, owner_task_id)
				VALUES (?, ?, ?)`, "group-1", "ws-source", "task-transfer")
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, task := seedTaskTransferFixture(t)
			command := taskTransferCommand(task)
			if tt.setup != nil {
				tt.setup(t, repo)
			}
			if tt.mutate != nil {
				tt.mutate(&command)
			}
			if _, err := repo.TransferTask(context.Background(), command); !errors.Is(err, repoerrors.ErrTaskTransferConflict) {
				t.Fatalf("TransferTask error = %v, want conflict", err)
			}
			stored, err := repo.GetTask(context.Background(), task.ID)
			if err != nil {
				t.Fatalf("GetTask: %v", err)
			}
			if stored.WorkspaceID != "ws-source" || stored.WorkflowID != "wf-source" ||
				stored.WorkflowStepID != "step-source-work" || !stored.UpdatedAt.Equal(task.UpdatedAt) {
				t.Fatalf("conflict mutated placement: %+v", stored)
			}
		})
	}
}

func TestTransferTaskPreservesNamedLaneSemantics(t *testing.T) {
	for _, lane := range []string{"Work", "Blocked", "Done"} {
		t.Run(lane, func(t *testing.T) {
			repo, task := seedTaskTransferFixture(t)
			suffix := map[string]string{"Work": "work", "Blocked": "blocked", "Done": "done"}[lane]
			if lane != "Work" {
				now := time.Now().UTC().Add(time.Second)
				mustExecTransferTest(t, repo, `UPDATE tasks SET workflow_step_id = ?, updated_at = ? WHERE id = ?`,
					"step-source-"+suffix, now, task.ID)
				task, _ = repo.GetTask(context.Background(), task.ID)
			}
			command := taskTransferCommand(task)
			command.ExpectedSourceStepID = "step-source-" + suffix
			command.DestinationStepID = "step-destination-" + suffix
			command.IdempotencyKey = "lane-" + suffix
			receipt, err := repo.TransferTask(context.Background(), command)
			if err != nil {
				t.Fatalf("TransferTask: %v", err)
			}
			if receipt.DestinationStepName != lane {
				t.Fatalf("destination lane = %q, want %q", receipt.DestinationStepName, lane)
			}
		})
	}
}

func TestTransferTaskAllowsDestinationCompatibleLabels(t *testing.T) {
	repo, task := seedTaskTransferFixture(t)
	mustExecTransferTest(t, repo, `CREATE TABLE office_labels
		(id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, name TEXT NOT NULL)`)
	mustExecTransferTest(t, repo, `CREATE TABLE office_task_labels (task_id TEXT NOT NULL, label_id TEXT NOT NULL)`)
	mustExecTransferTest(t, repo, `INSERT INTO office_labels (id, workspace_id, name) VALUES (?, ?, ?)`,
		"label-destination", "ws-destination", "urgent")
	mustExecTransferTest(t, repo, `INSERT INTO office_task_labels (task_id, label_id) VALUES (?, ?)`,
		task.ID, "label-destination")
	if _, err := repo.TransferTask(context.Background(), taskTransferCommand(task)); err != nil {
		t.Fatalf("TransferTask with compatible label: %v", err)
	}
}

func TestTransferTaskRemapsQueuedAndPersistedPendingMoves(t *testing.T) {
	repo, task := seedTaskTransferFixture(t)
	now := time.Now().UTC()
	mustExecTransferTest(t, repo, `CREATE TABLE pending_moves (
		id TEXT PRIMARY KEY, session_id TEXT NOT NULL UNIQUE, task_id TEXT NOT NULL,
		workflow_id TEXT NOT NULL, workflow_step_id TEXT NOT NULL, step_position INTEGER NOT NULL, queued_at TIMESTAMP NOT NULL)`)
	mustExecTransferTest(t, repo, `INSERT INTO pending_moves
		(id, session_id, task_id, workflow_id, workflow_step_id, step_position, queued_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, "pending-1", "session-running", task.ID,
		"wf-source", "step-source-blocked", 2, now)

	receipt, err := repo.TransferTask(context.Background(), taskTransferCommand(task))
	if err != nil {
		t.Fatalf("TransferTask: %v", err)
	}
	if receipt.PreservationCounts["pending_moves"] != 1 {
		t.Fatalf("pending move census = %+v", receipt.PreservationCounts)
	}
	var pending struct {
		WorkflowID string `db:"workflow_id"`
		StepID     string `db:"workflow_step_id"`
	}
	if err := repo.db.Get(&pending, `SELECT workflow_id, workflow_step_id FROM pending_moves WHERE id = ?`, "pending-1"); err != nil {
		t.Fatalf("read pending move: %v", err)
	}
	if pending.WorkflowID != "wf-destination" || pending.StepID != "step-destination-blocked" {
		t.Fatalf("pending move = %+v", pending)
	}
}

func TestTransferTaskPreservesTaskScopedParticipant(t *testing.T) {
	repo, task := seedTaskTransferFixture(t)
	mustExecTransferTest(t, repo, `CREATE TABLE agent_profiles (
		id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '', role TEXT NOT NULL DEFAULT '',
		deleted_at TIMESTAMP, enabled INTEGER NOT NULL DEFAULT 1, status TEXT NOT NULL DEFAULT 'idle')`)
	mustExecTransferTest(t, repo, `INSERT INTO agent_profiles (id, workspace_id) VALUES (?, ''), (?, '')`,
		"agent-1", "reviewer-1")
	mustExecTransferTest(t, repo, `INSERT INTO workflow_step_participants
		(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES (?, ?, ?, 'runner', ?, 0, 0)`, "participant-runner", task.WorkflowStepID, task.ID, "agent-1")
	mustExecTransferTest(t, repo, `INSERT INTO workflow_step_participants
		(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES (?, ?, ?, 'reviewer', ?, 1, 0)`, "participant-future", "step-source-blocked", task.ID, "reviewer-1")

	if _, err := repo.TransferTask(context.Background(), taskTransferCommand(task)); err != nil {
		t.Fatalf("TransferTask: %v", err)
	}
	var stepID string
	if err := repo.db.Get(&stepID, `SELECT step_id FROM workflow_step_participants WHERE id = ?`, "participant-runner"); err != nil {
		t.Fatalf("read participant: %v", err)
	}
	if stepID != "step-destination-work" {
		t.Fatalf("participant step = %q", stepID)
	}
	if err := repo.db.Get(&stepID, `SELECT step_id FROM workflow_step_participants WHERE id = ?`, "participant-future"); err != nil {
		t.Fatalf("read future participant: %v", err)
	}
	if stepID != "step-destination-blocked" {
		t.Fatalf("future participant step = %q", stepID)
	}
	stored, err := repo.GetTask(context.Background(), task.ID)
	if err != nil || stored.AssigneeAgentProfileID != "agent-1" {
		t.Fatalf("effective runner = %q, err=%v", stored.AssigneeAgentProfileID, err)
	}
}

func TestTransferTaskOfficeCEORunnerMapping(t *testing.T) {
	type destinationCEO struct {
		id      string
		enabled int
		status  string
		deleted bool
	}
	tests := []struct {
		name         string
		actor        models.TaskTransferActor
		destinations []destinationCEO
		wantSuccess  bool
	}{
		{name: "human", actor: models.TaskTransferActor{Kind: models.TaskTransferActorHuman, ID: "owner-1"},
			destinations: []destinationCEO{{id: "ceo-destination", enabled: 1, status: "idle"}}, wantSuccess: true},
		{name: "coordinator own runner", actor: models.TaskTransferActor{
			Kind: models.TaskTransferActorCoordinator, ID: "ceo-source", SessionID: "session-running",
			CallerTaskID: "task-transfer",
		}, destinations: []destinationCEO{{id: "ceo-destination", enabled: 1, status: "idle"}}, wantSuccess: true},
		{name: "coordinator other runner", actor: models.TaskTransferActor{
			Kind: models.TaskTransferActorCoordinator, ID: "ceo-other", SessionID: "session-running",
			CallerTaskID: "task-transfer",
		}, destinations: []destinationCEO{{id: "ceo-destination", enabled: 1, status: "idle"}}},
		{name: "no destination CEO", actor: models.TaskTransferActor{Kind: models.TaskTransferActorHuman, ID: "owner-1"}},
		{name: "multiple destination CEOs", actor: models.TaskTransferActor{Kind: models.TaskTransferActorHuman, ID: "owner-1"},
			destinations: []destinationCEO{{id: "ceo-a", enabled: 1, status: "idle"}, {id: "ceo-b", enabled: 1, status: "idle"}}},
		{name: "disabled destination CEO", actor: models.TaskTransferActor{Kind: models.TaskTransferActorHuman, ID: "owner-1"},
			destinations: []destinationCEO{{id: "ceo-destination", status: "idle"}}},
		{name: "stopped destination CEO", actor: models.TaskTransferActor{Kind: models.TaskTransferActorHuman, ID: "owner-1"},
			destinations: []destinationCEO{{id: "ceo-destination", enabled: 1, status: "stopped"}}},
		{name: "pending destination CEO", actor: models.TaskTransferActor{Kind: models.TaskTransferActorHuman, ID: "owner-1"},
			destinations: []destinationCEO{{id: "ceo-destination", enabled: 1, status: "pending_approval"}}},
		{name: "deleted destination CEO", actor: models.TaskTransferActor{Kind: models.TaskTransferActorHuman, ID: "owner-1"},
			destinations: []destinationCEO{{id: "ceo-destination", enabled: 1, status: "idle", deleted: true}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, task := seedTaskTransferFixture(t)
			mustExecTransferTest(t, repo, `UPDATE workspaces SET office_workflow_id = ? WHERE id = ?`,
				"wf-source", "ws-source")
			mustExecTransferTest(t, repo, `CREATE TABLE agent_profiles (
				id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '', role TEXT NOT NULL DEFAULT '',
				deleted_at TIMESTAMP, enabled INTEGER NOT NULL DEFAULT 1, status TEXT NOT NULL DEFAULT 'idle')`)
			mustExecTransferTest(t, repo, `INSERT INTO agent_profiles
				(id, workspace_id, role, enabled, status) VALUES (?, ?, 'ceo', 1, 'working')`,
				"ceo-source", "ws-source")
			for _, candidate := range tt.destinations {
				var deletedAt interface{}
				if candidate.deleted {
					deletedAt = time.Now().UTC()
				}
				mustExecTransferTest(t, repo, `INSERT INTO agent_profiles
					(id, workspace_id, role, deleted_at, enabled, status) VALUES (?, ?, 'ceo', ?, ?, ?)`,
					candidate.id, "ws-destination", deletedAt, candidate.enabled, candidate.status)
			}
			mustExecTransferTest(t, repo, `INSERT INTO workflow_step_participants
				(id, step_id, task_id, role, agent_profile_id, decision_required, position)
				VALUES (?, ?, ?, 'runner', ?, 0, 0)`, "participant-ceo", task.WorkflowStepID, task.ID, "ceo-source")
			mustExecTransferTest(t, repo, `UPDATE task_sessions SET agent_profile_id = ? WHERE id = ?`,
				"ceo-source", "session-running")
			command := taskTransferCommand(task)
			command.Actor = tt.actor
			receipt, err := repo.TransferTask(context.Background(), command)
			if !tt.wantSuccess {
				if !errors.Is(err, repoerrors.ErrTaskTransferConflict) {
					t.Fatalf("TransferTask error = %v, want conflict", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("TransferTask: %v", err)
			}
			stored, getErr := repo.GetTask(context.Background(), task.ID)
			if getErr != nil || stored.AssigneeAgentProfileID != "ceo-destination" {
				t.Fatalf("destination task = %+v, err=%v", stored, getErr)
			}
			session, getErr := repo.GetTaskSession(context.Background(), "session-running")
			if getErr != nil || session.AgentProfileID != "ceo-source" || session.State != models.TaskSessionStateRunning {
				t.Fatalf("preserved session = %+v, err=%v", session, getErr)
			}
			replay, replayErr := repo.TransferTask(context.Background(), command)
			if replayErr != nil || replay.OperationID != receipt.OperationID || !replay.IdempotentReplay {
				t.Fatalf("destination replay = %+v, err=%v", replay, replayErr)
			}
		})
	}
}

func TestTransferTaskRejectsBehavioralAndOfficeIncompatibilities(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, *Repository, *models.Task)
	}{
		{name: "events", setup: func(t *testing.T, repo *Repository, _ *models.Task) {
			mustExecTransferTest(t, repo, `UPDATE workflow_steps SET events = ? WHERE id = ?`, `[{"type":"start_agent"}]`, "step-destination-work")
		}},
		{name: "agent", setup: func(t *testing.T, repo *Repository, _ *models.Task) {
			mustExecTransferTest(t, repo, `UPDATE workflow_steps SET agent_profile_id = ? WHERE id = ?`, "agent-other", "step-destination-work")
		}},
		{name: "signal policy", setup: func(t *testing.T, repo *Repository, _ *models.Task) {
			mustExecTransferTest(t, repo, `UPDATE workflow_steps SET auto_advance_requires_signal = 1 WHERE id = ?`, "step-destination-work")
		}},
		{name: "workflow prompt", setup: func(t *testing.T, repo *Repository, _ *models.Task) {
			mustExecTransferTest(t, repo, `UPDATE workflows SET prompt = ? WHERE id = ?`, "different instructions", "wf-destination")
		}},
		{name: "workflow agent", setup: func(t *testing.T, repo *Repository, _ *models.Task) {
			mustExecTransferTest(t, repo, `UPDATE workflows SET agent_profile_id = ? WHERE id = ?`, "agent-other", "wf-destination")
		}},
		{name: "lane participant", setup: func(t *testing.T, repo *Repository, _ *models.Task) {
			mustExecTransferTest(t, repo, `INSERT INTO workflow_step_participants
				(id, step_id, task_id, role, agent_profile_id, decision_required, position)
				VALUES (?, ?, '', 'reviewer', ?, 1, 0)`, "default-reviewer", "step-destination-work", "reviewer-other")
		}},
		{name: "workspace scoped task participant", setup: func(t *testing.T, repo *Repository, task *models.Task) {
			mustExecTransferTest(t, repo, `CREATE TABLE agent_profiles (
				id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '', role TEXT NOT NULL DEFAULT '',
				deleted_at TIMESTAMP, enabled INTEGER NOT NULL DEFAULT 1, status TEXT NOT NULL DEFAULT 'idle')`)
			mustExecTransferTest(t, repo, `INSERT INTO agent_profiles (id, workspace_id) VALUES (?, ?)`, "source-runner", "ws-source")
			mustExecTransferTest(t, repo, `INSERT INTO workflow_step_participants
				(id, step_id, task_id, role, agent_profile_id, decision_required, position)
				VALUES (?, ?, ?, 'runner', ?, 0, 0)`, "source-runner-seat", task.WorkflowStepID, task.ID, "source-runner")
		}},
		{name: "full WIP lane", setup: func(t *testing.T, repo *Repository, task *models.Task) {
			mustExecTransferTest(t, repo, `UPDATE tasks SET wip_admitted = 1, queued_for_step_id = '' WHERE id = ?`, task.ID)
			mustExecTransferTest(t, repo, `UPDATE workflow_steps SET wip_limit = 1 WHERE id IN (?, ?)`, "step-source-work", "step-destination-work")
			other := &models.Task{ID: "destination-occupant", WorkspaceID: "ws-destination", WorkflowID: "wf-destination",
				WorkflowStepID: "step-destination-work", Title: "Occupant"}
			if err := repo.CreateTask(context.Background(), other); err != nil {
				t.Fatalf("CreateTask occupant: %v", err)
			}
		}},
		{name: "office project", setup: func(t *testing.T, repo *Repository, task *models.Task) {
			mustExecTransferTest(t, repo, `UPDATE tasks SET project_id = ? WHERE id = ?`, "project-source", task.ID)
		}},
		{name: "office tree hold", setup: func(t *testing.T, repo *Repository, task *models.Task) {
			mustExecTransferTest(t, repo, `CREATE TABLE office_task_tree_holds
				(id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, root_task_id TEXT NOT NULL)`)
			mustExecTransferTest(t, repo, `CREATE TABLE office_task_tree_hold_members
				(hold_id TEXT NOT NULL, task_id TEXT NOT NULL)`)
			mustExecTransferTest(t, repo, `INSERT INTO office_task_tree_holds VALUES (?, ?, ?)`, "hold-1", "ws-source", task.ID)
			mustExecTransferTest(t, repo, `INSERT INTO office_task_tree_hold_members VALUES (?, ?)`, "hold-1", task.ID)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, task := seedTaskTransferFixture(t)
			tt.setup(t, repo, task)
			task, _ = repo.GetTask(context.Background(), task.ID)
			command := taskTransferCommand(task)
			if _, err := repo.TransferTask(context.Background(), command); !errors.Is(err, repoerrors.ErrTaskTransferConflict) {
				t.Fatalf("TransferTask error = %v, want conflict", err)
			}
		})
	}
}

func TestTransferTaskConcurrentExactRetriesReturnSameReceipt(t *testing.T) {
	repo, task := seedTaskTransferFixture(t)
	command := taskTransferCommand(task)
	start := make(chan struct{})
	receipts := make([]*models.TaskTransferReceipt, 2)
	errs := make([]error, 2)
	var writers sync.WaitGroup
	for index := range receipts {
		writers.Add(1)
		go func(index int) {
			defer writers.Done()
			<-start
			receipts[index], errs[index] = repo.TransferTask(context.Background(), command)
		}(index)
	}
	close(start)
	writers.Wait()
	for index, err := range errs {
		if err != nil {
			t.Fatalf("writer %d: %v", index, err)
		}
	}
	if receipts[0].OperationID != receipts[1].OperationID {
		t.Fatalf("operation IDs differ: %q != %q", receipts[0].OperationID, receipts[1].OperationID)
	}
}

func TestTransferTaskConcurrentChangedRequestsShareKeyAsConflict(t *testing.T) {
	repo, first := seedTaskTransferFixture(t)
	second := &models.Task{ID: "task-transfer-second", WorkspaceID: "ws-source", WorkflowID: "wf-source",
		WorkflowStepID: "step-source-work", Title: "Second", QueuedForStepID: "step-source-blocked", WIPAdmitted: false}
	if err := repo.CreateTask(context.Background(), second); err != nil {
		t.Fatalf("CreateTask second: %v", err)
	}
	second, _ = repo.GetTask(context.Background(), second.ID)
	commands := []models.TaskTransferCommand{taskTransferCommand(first), taskTransferCommand(second)}
	commands[1].TaskID = second.ID
	commands[1].ExpectedTaskUpdatedAt = second.UpdatedAt
	start := make(chan struct{})
	errs := make([]error, 2)
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
}

func TestTaskTransferSerializationBlocksConcurrentSQLiteRelationWriter(t *testing.T) {
	repo, task := seedTaskTransferFixture(t)
	mustExecTransferTest(t, repo, `CREATE TABLE pending_moves (
		id TEXT PRIMARY KEY, session_id TEXT NOT NULL UNIQUE, task_id TEXT NOT NULL,
		workflow_id TEXT NOT NULL, workflow_step_id TEXT NOT NULL, step_position INTEGER NOT NULL, queued_at TIMESTAMP NOT NULL)`)
	tx, err := repo.db.BeginTxx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	if err := repo.lockTaskTransferRelations(context.Background(), tx, nil, nil, transferRelationInventory{}); err != nil {
		t.Fatalf("lockTaskTransferRelations: %v", err)
	}
	writerDone := make(chan error, 1)
	go func() {
		_, writeErr := repo.db.Exec(`INSERT INTO pending_moves
			(id, session_id, task_id, workflow_id, workflow_step_id, step_position, queued_at)
			VALUES (?, ?, ?, ?, ?, 0, ?)`, "pending-race", "session-race", task.ID,
			"wf-source", "step-source-blocked", time.Now().UTC())
		writerDone <- writeErr
	}()
	select {
	case writeErr := <-writerDone:
		t.Fatalf("relation writer bypassed transfer serialization: %v", writeErr)
	case <-time.After(50 * time.Millisecond):
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	select {
	case writeErr := <-writerDone:
		if writeErr != nil {
			t.Fatalf("relation writer after release: %v", writeErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("relation writer remained blocked after transfer lock release")
	}
}

func TestTransferTaskCommitFailureRollsBackAndAudits(t *testing.T) {
	repo, task := seedTaskTransferFixture(t)
	mustExecTransferTest(t, repo, `CREATE TABLE synthetic_transfer_workspaces (id TEXT PRIMARY KEY)`)
	mustExecTransferTest(t, repo, `INSERT INTO synthetic_transfer_workspaces (id) VALUES (?)`, "ws-source")
	mustExecTransferTest(t, repo, `CREATE TABLE synthetic_transfer_projection (
		id TEXT PRIMARY KEY, task_id TEXT NOT NULL, workspace_id TEXT NOT NULL,
		FOREIGN KEY (workspace_id) REFERENCES synthetic_transfer_workspaces(id) DEFERRABLE INITIALLY DEFERRED)`)
	mustExecTransferTest(t, repo, `INSERT INTO synthetic_transfer_projection (id, task_id, workspace_id) VALUES (?, ?, ?)`,
		"projection-1", task.ID, "ws-source")

	if _, err := repo.TransferTask(context.Background(), taskTransferCommand(task)); err == nil {
		t.Fatal("TransferTask unexpectedly committed")
	}
	stored, err := repo.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.WorkspaceID != "ws-source" || stored.WorkflowID != "wf-source" {
		t.Fatalf("failed commit changed placement: %+v", stored)
	}
	var failed int
	if err := repo.db.Get(&failed, `SELECT COUNT(*) FROM task_transfer_audit WHERE task_id = ? AND result = 'failed'`, task.ID); err != nil {
		t.Fatalf("read failed audit: %v", err)
	}
	if failed != 1 {
		t.Fatalf("failed audit rows = %d, want 1", failed)
	}
}

func TestTransferTaskConcurrentWritersProduceOneCommit(t *testing.T) {
	repo, task := seedTaskTransferFixture(t)
	commands := []models.TaskTransferCommand{taskTransferCommand(task), taskTransferCommand(task)}
	commands[1].IdempotencyKey = "transfer-key-2"
	start := make(chan struct{})
	errorsByWriter := make([]error, len(commands))
	var writers sync.WaitGroup
	for index := range commands {
		writers.Add(1)
		go func(index int) {
			defer writers.Done()
			<-start
			_, errorsByWriter[index] = repo.TransferTask(context.Background(), commands[index])
		}(index)
	}
	close(start)
	writers.Wait()

	var committed, conflicted int
	for _, err := range errorsByWriter {
		switch {
		case err == nil:
			committed++
		case errors.Is(err, repoerrors.ErrTaskTransferConflict):
			conflicted++
		default:
			t.Fatalf("concurrent transfer error = %v", err)
		}
	}
	if committed != 1 || conflicted != 1 {
		t.Fatalf("concurrent results committed=%d conflicted=%d errors=%v", committed, conflicted, errorsByWriter)
	}
}

func seedTaskTransferFixture(t *testing.T) (*Repository, *models.Task) {
	t.Helper()
	repo := newRepoForWorkflowSourceTests(t)
	ctx := context.Background()
	for _, workspace := range []*models.Workspace{
		{ID: "ws-source", Name: "Source", OwnerID: "owner-1"},
		{ID: "ws-destination", Name: "Destination", OwnerID: "owner-1"},
	} {
		if err := repo.CreateWorkspace(ctx, workspace); err != nil {
			t.Fatalf("CreateWorkspace: %v", err)
		}
	}
	for _, workflow := range []*models.Workflow{
		{ID: "wf-source", WorkspaceID: "ws-source", Name: "Source workflow"},
		{ID: "wf-destination", WorkspaceID: "ws-destination", Name: "Destination workflow"},
	} {
		if err := repo.CreateWorkflow(ctx, workflow); err != nil {
			t.Fatalf("CreateWorkflow: %v", err)
		}
	}
	for _, step := range []struct{ id, workflow, name string }{
		{"step-source-work", "wf-source", "Work"},
		{"step-source-blocked", "wf-source", "Blocked"},
		{"step-destination-work", "wf-destination", "Work"},
		{"step-destination-blocked", "wf-destination", "Blocked"},
		{"step-source-done", "wf-source", "Done"},
		{"step-destination-done", "wf-destination", "Done"},
	} {
		mustExecTransferTest(t, repo, `INSERT INTO workflow_steps (id, workflow_id, name, position) VALUES (?, ?, ?, 0)`,
			step.id, step.workflow, step.name)
	}

	task := &models.Task{
		ID: "task-transfer", WorkspaceID: "ws-source", WorkflowID: "wf-source",
		WorkflowStepID: "step-source-work", Title: "Transfer", Description: "preserve description",
		State: v1.TaskStateInProgress, Priority: "high", ParentID: "parent-task",
		WIPAdmitted: false, QueuedForStepID: "step-source-blocked", Metadata: map[string]interface{}{"pending_move": "keep"},
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	task, _ = repo.GetTask(ctx, task.ID)

	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-running", TaskID: task.ID, State: models.TaskSessionStateRunning,
		IsPrimary: true, TaskEnvironmentID: "environment-task", WorkspacePath: "/synthetic/worktree",
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	now := time.Now().UTC()
	mustExecTransferTest(t, repo, `INSERT INTO task_session_turns
		(id, task_session_id, task_id, started_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"turn-active", "session-running", task.ID, now, now, now)
	mustExecTransferTest(t, repo, `INSERT INTO task_plans
		(id, task_id, title, content, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"plan-1", task.ID, "Plan", "synthetic plan content", "tester", now, now)

	repository := &models.Repository{ID: "repository-source", WorkspaceID: "ws-source", Name: "repo", Provider: "github"}
	if err := repo.CreateRepository(ctx, repository); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repository-1", TaskID: task.ID, RepositoryID: repository.ID,
		BaseBranch: "main", CheckoutBranch: "feature/preserve",
	}); err != nil {
		t.Fatalf("CreateTaskRepository: %v", err)
	}

	mustExecTransferTest(t, repo, `INSERT INTO task_status_summaries
		(task_id, workspace_id, revision, summary, updated_at) VALUES (?, ?, 3, '{}', ?)`, task.ID, "ws-source", now)
	mustExecTransferTest(t, repo, `INSERT INTO task_message_attachments
		(id, workspace_id, task_id, name, size_bytes, storage_key, expires_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, "attachment-1", "ws-source", task.ID, "a.txt", 1,
		"synthetic/attachment", now.Add(time.Hour), now, now)
	mustExecTransferTest(t, repo, `CREATE TABLE github_task_prs
		(id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, task_id TEXT NOT NULL, pr_number INTEGER NOT NULL)`)
	mustExecTransferTest(t, repo, `INSERT INTO github_task_prs (id, workspace_id, task_id, pr_number) VALUES (?, ?, ?, ?)`,
		"pr-link-1", "ws-source", task.ID, 42)
	return repo, task
}

func taskTransferCommand(task *models.Task) models.TaskTransferCommand {
	return models.TaskTransferCommand{
		TaskID: task.ID, ExpectedSourceWorkspaceID: "ws-source", ExpectedSourceWorkflowID: "wf-source",
		ExpectedSourceStepID: "step-source-work", ExpectedTaskUpdatedAt: task.UpdatedAt,
		DestinationWorkspaceID: "ws-destination", DestinationWorkflowID: "wf-destination",
		DestinationStepID: "step-destination-work", IdempotencyKey: "transfer-key-1",
		PreservationPolicy: models.TaskTransferPreservationPolicyV1,
		Actor:              models.TaskTransferActor{Kind: models.TaskTransferActorHuman, ID: "owner-1"},
		AuthorizedOwnerID:  "owner-1", OwnerPredicateSet: true,
	}
}

func mustExecTransferTest(t *testing.T, repo *Repository, query string, args ...interface{}) {
	t.Helper()
	if _, err := repo.db.Exec(repo.db.Rebind(query), args...); err != nil {
		t.Fatalf("exec transfer fixture: %v", err)
	}
}
