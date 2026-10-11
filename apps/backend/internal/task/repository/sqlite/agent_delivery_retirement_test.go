package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	managed "github.com/kandev/kandev/internal/task/repository/managedconversation"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestDeliveryRetentionDeletionRetirement(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			t.Run("session", func(t *testing.T) {
				repo := newDeliveryRetentionDeletionRepo(t, engine)
				ctx := context.Background()
				const taskID, sessionID = "retention-delete-session-task", "retention-delete-session"
				session := createRetentionSession(t, repo, ctx, taskID, sessionID, false, "")
				seedRetentionDeliveryEvidence(t, repo, ctx, session)

				if err := repo.DeleteTaskSession(ctx, session); err != nil {
					t.Fatalf("delete task session: %v", err)
				}
				assertDeliveryRetirementJob(t, repo, session.ID, session.QueueIncarnationID)
				assertRetentionDeliveryEvidence(t, repo, session.ID)
				purgeDeliveryRetirementJobs(t, repo)
				assertDeliveryRetirementEvidencePurged(t, repo, session.ID)
			})

			t.Run("session_delete_rolls_back_retirement", func(t *testing.T) {
				repo := newDeliveryRetentionDeletionRepo(t, engine)
				ctx := context.Background()
				const taskID, sessionID = "retention-delete-rollback-task", "retention-delete-rollback-session"
				session := createRetentionSession(t, repo, ctx, taskID, sessionID, false, "")
				seedRetentionDeliveryEvidence(t, repo, ctx, session)
				removeFailure := installRetentionDeleteFailure(t, repo, "task_sessions")

				if err := repo.DeleteTaskSession(ctx, session); err == nil {
					t.Fatal("session delete succeeded with a rejecting trigger")
				}
				removeFailure()
				assertRetentionRowCount(t, repo, "agent_delivery_cleanup_jobs", "session_id = ?", session.ID, 0)
				if _, err := repo.GetTaskSession(ctx, session.ID); err != nil {
					t.Fatalf("session disappeared after rolled-back delete: %v", err)
				}
				assertRetentionDeliveryEvidence(t, repo, session.ID)
			})

			t.Run("archive_and_stop_preserve_delivery", func(t *testing.T) {
				repo := newDeliveryRetentionDeletionRepo(t, engine)
				ctx := context.Background()
				const taskID, sessionID = "retention-preserve-task", "retention-preserve-session"
				session := createRetentionSession(t, repo, ctx, taskID, sessionID, false, "")
				seedRetentionDeliveryEvidence(t, repo, ctx, session)
				if err := repo.UpdateTaskSessionState(ctx, session.ID, models.TaskSessionStateCancelled, "stopped"); err != nil {
					t.Fatalf("stop session: %v", err)
				}
				if err := repo.ArchiveTask(ctx, taskID); err != nil {
					t.Fatalf("archive task: %v", err)
				}
				assertRetentionRowCount(t, repo, "agent_delivery_cleanup_jobs", "session_id = ?", sessionID, 0)
				assertRetentionDeliveryEvidence(t, repo, sessionID)
			})

			t.Run("task_cascade", func(t *testing.T) {
				repo := newDeliveryRetentionDeletionRepo(t, engine)
				ctx := context.Background()
				const taskID = "retention-delete-task-cascade"
				if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Cascade"}); err != nil {
					t.Fatalf("create task: %v", err)
				}
				sessions := []*models.TaskSession{
					createRetentionSessionForExistingTask(t, repo, ctx, taskID, taskID+"-session-a", false, ""),
					createRetentionSessionForExistingTask(t, repo, ctx, taskID, taskID+"-session-b", false, ""),
				}
				for _, session := range sessions {
					seedRetentionDeliveryEvidence(t, repo, ctx, session)
				}

				if err := repo.DeleteTask(ctx, taskID); err != nil {
					t.Fatalf("delete task: %v", err)
				}
				for _, session := range sessions {
					assertDeliveryRetirementJob(t, repo, session.ID, session.QueueIncarnationID)
					assertRetentionDeliveryEvidence(t, repo, session.ID)
				}
				purgeDeliveryRetirementJobs(t, repo)
				for _, session := range sessions {
					assertDeliveryRetirementEvidencePurged(t, repo, session.ID)
				}
			})

			t.Run("workspace_cascade", func(t *testing.T) {
				repo := newDeliveryRetentionDeletionRepo(t, engine)
				ctx := context.Background()
				const workspaceID, taskID, sessionID = "retention-delete-workspace", "retention-delete-workspace-task", "retention-delete-workspace-session"
				if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Retention"}); err != nil {
					t.Fatalf("create workspace: %v", err)
				}
				if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: "Workspace cascade"}); err != nil {
					t.Fatalf("create task: %v", err)
				}
				session := createRetentionSessionForExistingTask(t, repo, ctx, taskID, sessionID, false, "")
				seedRetentionDeliveryEvidence(t, repo, ctx, session)

				if _, _, err := repo.DeleteWorkspaceCascade(ctx, workspaceID); err != nil {
					t.Fatalf("delete workspace cascade: %v", err)
				}
				assertDeliveryRetirementJob(t, repo, session.ID, session.QueueIncarnationID)
				assertRetentionDeliveryEvidence(t, repo, session.ID)
				purgeDeliveryRetirementJobs(t, repo)
				assertDeliveryRetirementEvidencePurged(t, repo, session.ID)
			})

			t.Run("ephemeral_profile_bulk_delete", func(t *testing.T) {
				repo := newDeliveryRetentionDeletionRepo(t, engine)
				ctx := context.Background()
				const taskID, sessionID, agentProfileID = "retention-delete-ephemeral", "retention-delete-ephemeral-session", "retention-delete-profile"
				session := createRetentionSession(t, repo, ctx, taskID, sessionID, true, agentProfileID)
				seedRetentionDeliveryEvidence(t, repo, ctx, session)

				deleted, err := repo.DeleteEphemeralTasksByAgentProfile(ctx, agentProfileID)
				if err != nil || deleted != 1 {
					t.Fatalf("delete ephemeral tasks = %d, err %v; want 1", deleted, err)
				}
				assertDeliveryRetirementJob(t, repo, session.ID, session.QueueIncarnationID)
				assertRetentionDeliveryEvidence(t, repo, session.ID)
				purgeDeliveryRetirementJobs(t, repo)
				assertDeliveryRetirementEvidencePurged(t, repo, session.ID)
			})

			t.Run("expired_quick_chat", func(t *testing.T) {
				repo := newDeliveryRetentionDeletionRepo(t, engine)
				ctx := context.Background()
				const taskID, sessionID = "retention-delete-quick-chat", "retention-delete-quick-chat-session"
				old := time.Now().UTC().Add(-48 * time.Hour)
				if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Quick chat", IsEphemeral: true, CreatedAt: old, UpdatedAt: old}); err != nil {
					t.Fatalf("create quick chat: %v", err)
				}
				session := createRetentionSessionForExistingTask(t, repo, ctx, taskID, sessionID, false, "")
				session.UpdatedAt = old
				if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE task_sessions SET updated_at = ? WHERE id = ?`), old, sessionID); err != nil {
					t.Fatalf("age quick-chat session: %v", err)
				}
				seedRetentionDeliveryEvidence(t, repo, ctx, session)

				deleted, err := repo.DeleteExpiredQuickChatTask(ctx, taskID, time.Now().UTC())
				if err != nil || !deleted {
					t.Fatalf("delete expired quick chat = %v, err %v; want true", deleted, err)
				}
				assertDeliveryRetirementJob(t, repo, session.ID, session.QueueIncarnationID)
				assertRetentionDeliveryEvidence(t, repo, session.ID)
				purgeDeliveryRetirementJobs(t, repo)
				assertDeliveryRetirementEvidencePurged(t, repo, session.ID)
			})

			t.Run("managed_finalization", func(t *testing.T) {
				repo := newDeliveryRetentionDeletionRepo(t, engine)
				ctx := context.Background()
				const workspaceID, taskID, sessionID = "retention-managed-delete-workspace", "retention-managed-delete-task", "retention-managed-delete-session"
				identity := managed.Identity{
					TaskID: taskID, InstallationID: "retention-installation",
					WorkspaceID: workspaceID, InstanceKey: "retention-instance",
				}
				if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Retention"}); err != nil {
					t.Fatalf("create workspace: %v", err)
				}
				task := &models.Task{
					ID: taskID, WorkspaceID: workspaceID, Title: "Managed deletion",
					Metadata: map[string]interface{}{
						models.MetaKeyManagedRetained: true, "kandev.ephemeral": true,
						models.MetaKeyManagedInstallationID: identity.InstallationID,
						"kandev.workspace_id":               identity.WorkspaceID,
						models.MetaKeyManagedInstanceKey:    identity.InstanceKey,
					},
				}
				if err := repo.CreateTask(ctx, task); err != nil {
					t.Fatalf("create managed task: %v", err)
				}
				session := createRetentionSessionForExistingTask(t, repo, ctx, taskID, sessionID, false, "")
				seedRetentionDeliveryEvidence(t, repo, ctx, session)
				storedTask, err := repo.GetTask(ctx, taskID)
				if err != nil {
					t.Fatalf("read managed task: %v", err)
				}
				request := managed.DeleteRequest{
					Identity: identity, ExpectedRevision: managed.Revision(storedTask), TaskCreatedAt: storedTask.CreatedAt,
					OperationID: "retention-delete", PayloadDigest: "retention-delete-digest",
				}
				claim, err := repo.AdmitManagedDeletion(ctx, request, "retention-delete-owner")
				if err != nil {
					t.Fatalf("admit managed deletion: %v", err)
				}
				assertRetentionRowCount(t, repo, "agent_delivery_cleanup_jobs", "session_id = ?", session.ID, 0)
				sessions, err := repo.ListTaskSessions(ctx, taskID)
				if err != nil {
					t.Fatalf("list managed sessions: %v", err)
				}
				inventory, err := json.Marshal(struct {
					WorkspaceID string                `json:"workspace_id"`
					Sessions    []*models.TaskSession `json:"sessions"`
				}{workspaceID, sessions})
				if err != nil {
					t.Fatalf("encode managed deletion inventory: %v", err)
				}
				if _, err := repo.PrepareManagedDeletion(ctx, *claim, string(inventory)); err != nil {
					t.Fatalf("prepare managed deletion: %v", err)
				}
				assertRetentionRowCount(t, repo, "agent_delivery_cleanup_jobs", "session_id = ?", session.ID, 0)
				removeFailure := installRetentionDeleteFailure(t, repo, "tasks")
				if _, err := repo.FinalizeManagedDeletion(ctx, *claim); err == nil {
					t.Fatal("managed task finalization succeeded with a rejecting trigger")
				}
				removeFailure()
				assertRetentionRowCount(t, repo, "agent_delivery_cleanup_jobs", "session_id = ?", session.ID, 0)
				if _, err := repo.FinalizeManagedDeletion(ctx, *claim); err != nil {
					t.Fatalf("finalize managed deletion: %v", err)
				}
				committed, cleanup, err := repo.InspectManagedDeletion(ctx, request)
				if err != nil || committed == nil || committed.Phase != managed.DeleteCommitted || cleanup == nil {
					t.Fatalf("managed commit receipt after response loss = %+v, %+v, %v", committed, cleanup, err)
				}
				assertDeliveryRetirementJob(t, repo, session.ID, session.QueueIncarnationID)
				assertRetentionDeliveryEvidence(t, repo, session.ID)
				purgeDeliveryRetirementJobs(t, repo)
				assertDeliveryRetirementEvidencePurged(t, repo, session.ID)
			})

			t.Run("managed_release_preserves_delivery", func(t *testing.T) {
				testManagedReleasePreservesDelivery(t, engine)
			})
		})
	}
}

func testManagedReleasePreservesDelivery(t *testing.T, engine string) {
	t.Helper()
	repo := newDeliveryRetentionDeletionRepo(t, engine)
	ctx := context.Background()
	const workspaceID, taskID, sessionID = "retention-release-workspace", "retention-release-task", "retention-release-session"
	identity := managed.Identity{TaskID: taskID, InstallationID: "retention-release-installation", WorkspaceID: workspaceID, InstanceKey: "release"}
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Retention release"}); err != nil {
		t.Fatalf("create release workspace: %v", err)
	}
	task := &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: "Managed release", Metadata: map[string]interface{}{
		models.MetaKeyManagedRetained: true, "kandev.ephemeral": true,
		models.MetaKeyManagedInstallationID: identity.InstallationID,
		"kandev.workspace_id":               identity.WorkspaceID,
		models.MetaKeyManagedInstanceKey:    identity.InstanceKey,
	}}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create release task: %v", err)
	}
	session := createRetentionSessionForExistingTask(t, repo, ctx, taskID, sessionID, false, "")
	seedRetentionDeliveryEvidence(t, repo, ctx, session)
	stored, err := repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("read release task: %v", err)
	}
	request := managed.DeleteRequest{Identity: identity, ExpectedRevision: managed.Revision(stored), TaskCreatedAt: stored.CreatedAt,
		OperationID: "retention-release-operation", PayloadDigest: "retention-release-digest"}
	claim, err := repo.AdmitManagedDeletion(ctx, request, "retention-release-owner")
	if err != nil {
		t.Fatalf("reserve managed deletion: %v", err)
	}
	assertRetentionRowCount(t, repo, "agent_delivery_cleanup_jobs", "session_id = ?", sessionID, 0)
	sessions, err := repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		t.Fatalf("list release sessions: %v", err)
	}
	inventory, err := json.Marshal(struct {
		WorkspaceID string                `json:"workspace_id"`
		Sessions    []*models.TaskSession `json:"sessions"`
	}{workspaceID, sessions})
	if err != nil {
		t.Fatalf("encode release inventory: %v", err)
	}
	if _, err := repo.PrepareManagedDeletion(ctx, *claim, string(inventory)); err != nil {
		t.Fatalf("prepare managed deletion: %v", err)
	}
	assertRetentionRowCount(t, repo, "agent_delivery_cleanup_jobs", "session_id = ?", sessionID, 0)
	released, err := repo.ReleaseManagedDeletion(ctx, *claim)
	if err != nil || !released {
		t.Fatalf("release managed deletion = %v, %v", released, err)
	}
	assertRetentionRowCount(t, repo, "agent_delivery_cleanup_jobs", "session_id = ?", sessionID, 0)
	assertRetentionDeliveryEvidence(t, repo, sessionID)
	if _, err := repo.GetTaskSession(ctx, sessionID); err != nil {
		t.Fatalf("session disappeared after managed deletion release: %v", err)
	}
}

func purgeDeliveryRetirementJobs(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	for attempt := 0; attempt < 20; attempt++ {
		var pending int
		if err := repo.db.GetContext(ctx, &pending, `SELECT COUNT(*) FROM agent_delivery_cleanup_jobs`); err != nil {
			t.Fatalf("count delivery retirement jobs: %v", err)
		}
		if pending == 0 {
			return
		}
		if _, err := repo.RunAgentDeliveryRetirementBatch(ctx, models.AgentDeliveryRetirementBatchOptions{}); err != nil {
			t.Fatalf("purge delivery retirement batch: %v", err)
		}
	}
	t.Fatal("delivery retirement jobs did not finish within 20 bounded batches")
}

func TestDeliveryRetentionAuthenticatedAdmission(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo := newRetentionTestRepo(t, engine)
			ctx := context.Background()
			const (
				taskID        = "retention-owner-task"
				sessionID     = "retention-owner-session"
				incarnationID = "retention-owner-incarnation"
				executionID   = "retention-owner-execution"
				streamID      = "retention-owner-stream"
				submissionID  = "prompt:retention-owner"
			)
			if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Owner"}); err != nil {
				t.Fatalf("create task: %v", err)
			}
			session := &models.TaskSession{ID: sessionID, TaskID: taskID, QueueIncarnationID: incarnationID}
			if err := repo.CreateTaskSession(ctx, session); err != nil {
				t.Fatalf("create task session: %v", err)
			}
			if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
				ID: "retention-owner-running", SessionID: sessionID, TaskID: taskID,
				ExecutorID: "retention-owner-executor", AgentExecutionID: executionID, Status: "running",
			}); err != nil {
				t.Fatalf("create execution owner: %v", err)
			}
			now := time.Now().UTC()
			stored, err := repo.CommitHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
				SessionID: sessionID, IncarnationID: incarnationID, Generation: 2,
				NativeSessionID: "native-retention-owner", CreationReason: "initial",
				CreatedAt: now, CommittedAt: now,
			}, 0)
			if err != nil || !stored {
				t.Fatalf("commit harness owner = %v, err %v", stored, err)
			}
			if _, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
				ID: submissionID, SessionID: sessionID, IncarnationID: incarnationID,
				HarnessGeneration: 2, OwnerGeneration: 2, PayloadHash: "retention-owner-hash",
				Payload: []byte("prompt"), State: models.DeliverySubmissionDispatching,
			}); err != nil {
				t.Fatalf("prepare owner submission: %v", err)
			}
			valid := &models.AgentDeliveryEvent{
				SessionID: sessionID, IncarnationID: incarnationID, HarnessGeneration: 2,
				TaskID: taskID, ExecutionID: executionID, OwnerGeneration: 2, ExpectedStreamID: streamID,
				StreamID: streamID, Sequence: 1, SubmissionID: submissionID,
				EventType: "assistant_message", Payload: []byte("owner output"),
			}

			wrongExecution := *valid
			wrongExecution.ExecutionID = "replaced-execution"
			if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, &wrongExecution, 1); !errors.Is(err, repoerrors.ErrAgentDeliveryIdentityMismatch) || inserted {
				t.Fatalf("receive from stale execution = %v, err %v", inserted, err)
			}
			wrongSubmission := *valid
			wrongSubmission.SubmissionID = "prompt:unknown-owner"
			if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, &wrongSubmission, 1); !errors.Is(err, repoerrors.ErrAgentDeliveryIdentityMismatch) || inserted {
				t.Fatalf("receive for unknown submission = %v, err %v", inserted, err)
			}
			wrongStream := *valid
			wrongStream.StreamID = "retention-owner-replaced-stream"
			if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, &wrongStream, 1); !errors.Is(err, repoerrors.ErrAgentDeliveryIdentityMismatch) || inserted {
				t.Fatalf("receive on replaced stream = %v, err %v", inserted, err)
			}
			if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, valid, 1); err != nil || !inserted {
				t.Fatalf("receive from current owner = %v, err %v", inserted, err)
			}

			if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
				ID: "retention-owner-running", SessionID: sessionID, TaskID: taskID,
				ExecutorID: "retention-owner-executor", AgentExecutionID: "replacement-execution", Status: "running",
			}); err != nil {
				t.Fatalf("replace execution owner: %v", err)
			}
			staleSecondEvent := *valid
			staleSecondEvent.Sequence = 2
			staleSecondEvent.Payload = []byte("stale owner output")
			if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, &staleSecondEvent, 2); !errors.Is(err, repoerrors.ErrAgentDeliveryIdentityMismatch) || inserted {
				t.Fatalf("receive after owner transfer = %v, err %v", inserted, err)
			}
			assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id = ?", streamID, 1)
			cursor, err := repo.GetAgentDeliveryCursor(ctx, streamID)
			if err != nil || cursor.ReceivedSequence != 1 {
				t.Fatalf("cursor after stale owner transfer = %+v, err %v", cursor, err)
			}
		})
	}
}

func installRetentionDeleteFailure(t *testing.T, repo *Repository, table string) func() {
	t.Helper()
	if table != "tasks" && table != "task_sessions" {
		t.Fatalf("unsupported deletion trigger table %q", table)
	}
	const trigger, function = "retention_reject_delete", "retention_reject_delete_fn"
	if repo.db.DriverName() == "pgx" {
		if _, err := repo.db.Exec(`CREATE FUNCTION ` + function + `() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced retention delete failure'; END; $$`); err != nil {
			t.Fatalf("create PostgreSQL rejection function: %v", err)
		}
		if _, err := repo.db.Exec(`CREATE TRIGGER ` + trigger + ` BEFORE DELETE ON ` + table + ` FOR EACH ROW EXECUTE FUNCTION ` + function + `()`); err != nil {
			t.Fatalf("create PostgreSQL rejection trigger: %v", err)
		}
		return func() {
			t.Helper()
			if _, err := repo.db.Exec(`DROP TRIGGER IF EXISTS ` + trigger + ` ON ` + table); err != nil {
				t.Errorf("drop PostgreSQL rejection trigger: %v", err)
			}
			if _, err := repo.db.Exec(`DROP FUNCTION IF EXISTS ` + function + `()`); err != nil {
				t.Errorf("drop PostgreSQL rejection function: %v", err)
			}
		}
	}
	if _, err := repo.db.Exec(`CREATE TRIGGER ` + trigger + ` BEFORE DELETE ON ` + table + ` BEGIN SELECT RAISE(ABORT, 'forced retention delete failure'); END`); err != nil {
		t.Fatalf("create SQLite rejection trigger: %v", err)
	}
	return func() {
		t.Helper()
		if _, err := repo.db.Exec(`DROP TRIGGER IF EXISTS ` + trigger); err != nil {
			t.Errorf("drop SQLite rejection trigger: %v", err)
		}
	}
}

func newDeliveryRetentionDeletionRepo(t *testing.T, engine string) *Repository {
	t.Helper()
	repo := newRetentionTestRepo(t, engine)
	initializeRetentionDeletionQueue(t, repo)
	return repo
}

func initializeRetentionDeletionQueue(t *testing.T, repo *Repository) {
	t.Helper()
	if _, err := messagequeue.NewSQLiteRepository(repo.db, repo.db); err != nil {
		t.Fatalf("initialize queue schema: %v", err)
	}
}

func createRetentionSession(t *testing.T, repo *Repository, ctx context.Context, taskID, sessionID string, ephemeral bool, agentProfileID string) *models.TaskSession {
	t.Helper()
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Retention deletion", IsEphemeral: ephemeral}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	return createRetentionSessionForExistingTask(t, repo, ctx, taskID, sessionID, ephemeral, agentProfileID)
}

func createRetentionSessionForExistingTask(t *testing.T, repo *Repository, ctx context.Context, taskID, sessionID string, _ bool, agentProfileID string) *models.TaskSession {
	t.Helper()
	session := &models.TaskSession{ID: sessionID, TaskID: taskID, AgentProfileID: agentProfileID}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("create task session %s: %v", sessionID, err)
	}
	return session
}

func seedRetentionDeliveryEvidence(t *testing.T, repo *Repository, ctx context.Context, session *models.TaskSession) {
	t.Helper()
	submission := &models.AgentDeliverySubmission{
		ID: "prompt:" + session.ID, SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, DispatchAttemptID: "retention-delete-attempt",
		PayloadHash: "retention-delete-hash", Payload: []byte("retained prompt"),
		State: models.DeliverySubmissionPrepared,
	}
	if _, err := repo.PrepareAgentDeliverySubmission(ctx, submission); err != nil {
		t.Fatalf("prepare retained prompt: %v", err)
	}
	event := &models.AgentDeliveryEvent{
		SessionID: session.ID, IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1,
		StreamID: "stream:" + session.ID, Sequence: 1, SubmissionID: submission.ID,
		EventType: "assistant_message", Payload: []byte("retained event"),
	}
	if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, event, 1); err != nil || !inserted {
		t.Fatalf("receive retained event = %v, err %v", inserted, err)
	}
}

func assertDeliveryRetirementJob(t *testing.T, repo *Repository, sessionID, incarnationID string) {
	t.Helper()
	var got struct {
		SessionID     string    `db:"session_id"`
		IncarnationID string    `db:"incarnation_id"`
		RetiredAt     time.Time `db:"retired_at"`
		Phase         string    `db:"phase"`
	}
	if err := repo.db.Get(&got, repo.db.Rebind(`
		SELECT session_id, incarnation_id, retired_at, phase
		FROM agent_delivery_cleanup_jobs WHERE session_id = ? AND incarnation_id = ?`), sessionID, incarnationID); err != nil {
		t.Fatalf("read retirement job for %s/%s: %v", sessionID, incarnationID, err)
	}
	if got.SessionID != sessionID || got.IncarnationID != incarnationID || got.RetiredAt.IsZero() || got.Phase != "pending" {
		t.Fatalf("retirement job = %+v, want exact pending identity", got)
	}
	assertRetentionRowCount(t, repo, "agent_delivery_cleanup_jobs", "session_id = ?", sessionID, 1)
}

func assertRetentionDeliveryEvidence(t *testing.T, repo *Repository, sessionID string) {
	t.Helper()
	assertRetentionRowCount(t, repo, "agent_delivery_submissions", "session_id = ?", sessionID, 1)
	assertRetentionRowCount(t, repo, "agent_delivery_inbox", "session_id = ?", sessionID, 1)
}
