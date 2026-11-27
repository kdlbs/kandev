package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestDeliveryRetentionDeletion(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo := newDeliveryRetentionDeletionRepo(t, engine)
			ctx := context.Background()
			const taskID, sessionID = "retention-purge-task", "retention-purge-session"
			session := createRetentionSession(t, repo, ctx, taskID, sessionID, false, "")
			seedRetentionDeliveryEvidence(t, repo, ctx, session)
			seedLegacyWorkflowReceipt(t, repo, session)
			seedLegacyGenericReceipt(t, repo, session)
			if err := repo.DeleteTaskSession(ctx, session); err != nil {
				t.Fatalf("delete task session: %v", err)
			}
			assertLegacyWorkflowReceiptBound(t, repo, session)
			if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID}); !errors.Is(err, repoerrors.ErrAgentDeliveryIdentityRetiring) {
				t.Fatalf("recreate before purge error = %v, want retirement conflict", err)
			}

			for attempt := 0; attempt < 8; attempt++ {
				result, err := repo.RunAgentDeliveryRetirementBatch(ctx, models.AgentDeliveryRetirementBatchOptions{MaxCandidates: 1})
				if err != nil {
					t.Fatalf("retirement batch %d: %v", attempt, err)
				}
				if result.Completed == 1 {
					break
				}
			}
			assertDeliveryRetirementEvidencePurged(t, repo, sessionID)
			assertRetentionRowCount(t, repo, "agent_delivery_effects", "effect_key = ?", "generic:"+sessionID, 0)
			assertRetentionRowCount(t, repo, "agent_delivery_cleanup_jobs", "session_id = ?", sessionID, 0)

			replacement := &models.TaskSession{ID: sessionID, TaskID: taskID}
			if err := repo.CreateTaskSession(ctx, replacement); err != nil {
				t.Fatalf("recreate after purge: %v", err)
			}
			if replacement.QueueIncarnationID == session.QueueIncarnationID {
				t.Fatal("replacement reused the retired incarnation")
			}
		})
	}
}

func TestDeliveryRetentionReplacement(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine+"/concurrent_create_and_purge", func(t *testing.T) {
			repo := newDeliveryRetentionDeletionRepo(t, engine)
			ctx := context.Background()
			const taskID, sessionID = "retention-race-task", "retention-race-session"
			old := createRetentionSession(t, repo, ctx, taskID, sessionID, false, "")
			seedRetentionDeliveryEvidence(t, repo, ctx, old)
			if err := repo.DeleteTaskSession(ctx, old); err != nil {
				t.Fatalf("retire original session: %v", err)
			}

			start := make(chan struct{})
			createDone := make(chan error, 1)
			purgeDone := make(chan error, 1)
			go func() {
				<-start
				purgeDone <- func() error {
					_, err := repo.RunAgentDeliveryRetirementBatch(ctx, models.AgentDeliveryRetirementBatchOptions{})
					return err
				}()
			}()
			replacement := &models.TaskSession{ID: sessionID, TaskID: taskID}
			go func() {
				<-start
				createDone <- repo.CreateTaskSession(ctx, replacement)
			}()
			close(start)
			createErr := <-createDone
			if err := <-purgeDone; err != nil {
				t.Fatalf("concurrent retirement batch: %v", err)
			}
			if createErr != nil && !errors.Is(createErr, repoerrors.ErrAgentDeliveryIdentityRetiring) {
				t.Fatalf("concurrent replacement error = %v", createErr)
			}
			if errors.Is(createErr, repoerrors.ErrAgentDeliveryIdentityRetiring) {
				purgeDeliveryRetirementJobs(t, repo)
				if err := repo.CreateTaskSession(ctx, replacement); err != nil {
					t.Fatalf("retry replacement after purge: %v", err)
				}
			}
			assertDeliveryRetirementEvidencePurged(t, repo, sessionID)
			assertRetentionRowCount(t, repo, "agent_delivery_cleanup_jobs", "session_id = ?", sessionID, 0)
			if replacement.QueueIncarnationID == old.QueueIncarnationID {
				t.Fatal("concurrent replacement reused the retired incarnation")
			}
		})

		t.Run(engine+"/orphan_mismatch_preserves_live_identity", func(t *testing.T) {
			repo := newDeliveryRetentionDeletionRepo(t, engine)
			ctx := context.Background()
			const taskID, sessionID = "retention-mismatch-task", "retention-mismatch-session"
			createRetentionSession(t, repo, ctx, taskID, sessionID, false, "")
			_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
				INSERT INTO agent_delivery_submissions
				(id, session_id, incarnation_id, harness_generation, owner_generation, dispatch_attempt_id,
				 payload_hash, payload, state, created_at, updated_at)
				VALUES (?, ?, ?, 1, 1, 'stale-attempt', 'stale-hash', ?, 'completed', ?, ?)`),
				"stale:"+sessionID, sessionID, "old-incarnation", []byte("stale payload"), repo.nowUTC(), repo.nowUTC())
			if err != nil {
				t.Fatalf("seed mismatched old incarnation: %v", err)
			}

			result, err := repo.RunAgentDeliveryOrphanBatch(ctx, models.AgentDeliveryOrphanBatchOptions{})
			if err != nil || result.JobsScheduled != 0 {
				t.Fatalf("orphan scan against live session = %+v, %v; want no job", result, err)
			}
			assertRetentionRowCount(t, repo, "agent_delivery_submissions", "id = ?", "stale:"+sessionID, 1)
			assertRetentionRowCount(t, repo, "agent_delivery_cleanup_jobs", "session_id = ?", sessionID, 0)
		})

		t.Run(engine+"/live_identity_suspends_purge", func(t *testing.T) {
			repo := newDeliveryRetentionDeletionRepo(t, engine)
			ctx := context.Background()
			const taskID, sessionID = "retention-live-task", "retention-live-session"
			session := createRetentionSession(t, repo, ctx, taskID, sessionID, false, "")
			seedRetentionDeliveryEvidence(t, repo, ctx, session)
			_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
				INSERT INTO agent_delivery_cleanup_jobs
				(session_id, incarnation_id, retired_at, phase, created_at, updated_at)
				VALUES (?, ?, ?, 'pending', ?, ?)`), sessionID, "stale-incarnation", repo.nowUTC(), repo.nowUTC(), repo.nowUTC())
			if err != nil {
				t.Fatalf("seed stale retirement job: %v", err)
			}

			result, err := repo.RunAgentDeliveryRetirementBatch(ctx, models.AgentDeliveryRetirementBatchOptions{})
			if err != nil {
				t.Fatalf("run retirement against live replacement: %v", err)
			}
			if result.Suspended != 1 || result.RowsPurged != 0 || result.Completed != 0 {
				t.Fatalf("live identity retirement result = %+v", result)
			}
			assertRetentionDeliveryEvidence(t, repo, sessionID)
			assertRetentionRowCount(t, repo, "agent_delivery_cleanup_jobs", "session_id = ?", sessionID, 1)
		})

		t.Run(engine+"/rollback_after_job_delete", func(t *testing.T) {
			repo := newDeliveryRetentionDeletionRepo(t, engine)
			ctx := context.Background()
			const taskID, sessionID = "retention-retry-task", "retention-retry-session"
			session := createRetentionSession(t, repo, ctx, taskID, sessionID, false, "")
			seedRetentionDeliveryEvidence(t, repo, ctx, session)
			if err := repo.DeleteTaskSession(ctx, session); err != nil {
				t.Fatalf("delete task session: %v", err)
			}
			injected := errors.New("injected failure before retirement commit")
			repo.agentDeliveryRetentionFailpoint = func(point string) error {
				if point == "retirement_after_job_delete" {
					return injected
				}
				return nil
			}
			if _, err := repo.RunAgentDeliveryRetirementBatch(ctx, models.AgentDeliveryRetirementBatchOptions{}); !errors.Is(err, injected) {
				t.Fatalf("injected retirement failure = %v", err)
			}
			repo.agentDeliveryRetentionFailpoint = nil
			assertRetentionDeliveryEvidence(t, repo, sessionID)
			assertRetentionRowCount(t, repo, "agent_delivery_cleanup_jobs", "session_id = ?", sessionID, 1)
			result, err := repo.RunAgentDeliveryRetirementBatch(ctx, models.AgentDeliveryRetirementBatchOptions{})
			if err != nil || result.Completed != 1 {
				t.Fatalf("retry after rolled-back final commit = %+v, %v", result, err)
			}
			retry, err := repo.RunAgentDeliveryRetirementBatch(ctx, models.AgentDeliveryRetirementBatchOptions{})
			if err != nil || retry.Completed != 0 {
				t.Fatalf("retry after committed retirement = %+v, %v; want idempotent no-op", retry, err)
			}
			assertDeliveryRetirementEvidencePurged(t, repo, sessionID)
		})
	}
}

func TestDeliveryRetentionOrphans(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo := newDeliveryRetentionDeletionRepo(t, engine)
			ctx := context.Background()
			const taskID, sessionID = "retention-orphan-task", "retention-orphan-session"
			session := createRetentionSession(t, repo, ctx, taskID, sessionID, false, "")
			seedRetentionDeliveryEvidence(t, repo, ctx, session)
			if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`DELETE FROM task_sessions WHERE id = ?`), sessionID); err != nil {
				t.Fatalf("remove canonical session to model pre-existing orphan: %v", err)
			}
			if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`DELETE FROM agent_delivery_cleanup_jobs WHERE session_id = ?`), sessionID); err != nil {
				t.Fatalf("remove retirement job to model pre-existing orphan: %v", err)
			}

			result, err := repo.RunAgentDeliveryOrphanBatch(ctx, models.AgentDeliveryOrphanBatchOptions{MaxCandidates: 10})
			if err != nil {
				t.Fatalf("orphan scan: %v", err)
			}
			if result.JobsScheduled != 1 {
				t.Fatalf("orphan scan result = %+v, want one job", result)
			}
			assertDeliveryRetirementJob(t, repo, sessionID, session.QueueIncarnationID)
			var unknownCount int
			if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
				INSERT INTO agent_delivery_effects
				(effect_key, stream_id, sequence, effect_type, state, created_at, session_id, incarnation_id)
				VALUES (?, ?, 1, 'workflow.on_turn_complete', 'completed', ?, '', '')`),
				"unbound:"+sessionID, "unknown-stream:"+sessionID, repo.nowUTC()); err != nil {
				t.Fatalf("seed unknown workflow receipt: %v", err)
			}
			orphanResult, err := repo.RunAgentDeliveryOrphanBatch(ctx, models.AgentDeliveryOrphanBatchOptions{})
			if err != nil {
				t.Fatalf("scan unknown workflow receipt: %v", err)
			}
			unknownCount = orphanResult.UnboundWorkflowRows
			if unknownCount != 1 {
				t.Fatalf("bounded unbound workflow count = %d, want 1", unknownCount)
			}
			if _, err := repo.RunAgentDeliveryRetirementBatch(ctx, models.AgentDeliveryRetirementBatchOptions{}); err != nil {
				t.Fatalf("purge discovered orphan: %v", err)
			}
			assertDeliveryRetirementEvidencePurged(t, repo, sessionID)
			assertRetentionRowCount(t, repo, "agent_delivery_effects", "effect_key = ?", "unbound:"+sessionID, 1)
		})
	}
}

func seedLegacyWorkflowReceipt(t *testing.T, repo *Repository, session *models.TaskSession) {
	t.Helper()
	const terminalType = models.DeliveryTerminalEffectType
	streamID := "stream:" + session.ID
	submissionID := "prompt:" + session.ID
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO agent_delivery_effects
		(effect_key, stream_id, sequence, effect_type, state, created_at, completed_at,
		 session_id, incarnation_id, harness_generation, submission_id, turn_id, outcome)
		VALUES (?, ?, 1, ?, 'completed', ?, ?, ?, ?, 1, ?, 'turn-retention', 'completed')`),
		"terminal:"+session.ID, streamID, terminalType, repo.nowUTC(), repo.nowUTC(),
		session.ID, session.QueueIncarnationID, submissionID)
	if err != nil {
		t.Fatalf("seed terminal effect: %v", err)
	}
	_, err = repo.db.Exec(repo.db.Rebind(`
		INSERT INTO agent_delivery_effects
		(effect_key, stream_id, sequence, effect_type, state, created_at, completed_at, submission_id)
		VALUES (?, ?, 1, 'workflow.on_turn_complete', 'completed', ?, ?, ?)`),
		"workflow:turn-retention", streamID, repo.nowUTC(), repo.nowUTC(), submissionID)
	if err != nil {
		t.Fatalf("seed legacy workflow effect: %v", err)
	}
}

func seedLegacyGenericReceipt(t *testing.T, repo *Repository, session *models.TaskSession) {
	t.Helper()
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO agent_delivery_effects
		(effect_key, stream_id, sequence, effect_type, state, created_at)
		VALUES (?, ?, 1, 'agent_delivery.event', 'completed', ?)`),
		"generic:"+session.ID, "stream:"+session.ID, repo.nowUTC())
	if err != nil {
		t.Fatalf("seed generic delivery receipt: %v", err)
	}
}

func assertLegacyWorkflowReceiptBound(t *testing.T, repo *Repository, session *models.TaskSession) {
	t.Helper()
	var got struct {
		SessionID     string `db:"session_id"`
		IncarnationID string `db:"incarnation_id"`
		Generation    int64  `db:"harness_generation"`
		SubmissionID  string `db:"submission_id"`
		TurnID        string `db:"turn_id"`
	}
	err := repo.db.Get(&got, repo.db.Rebind(`
		SELECT session_id, incarnation_id, harness_generation, submission_id, turn_id
		FROM agent_delivery_effects WHERE effect_key = ?`), "workflow:turn-retention")
	if err != nil {
		t.Fatalf("read backfilled workflow receipt: %v", err)
	}
	if got.SessionID != session.ID || got.IncarnationID != session.QueueIncarnationID ||
		got.Generation != 1 || got.SubmissionID != "prompt:"+session.ID || got.TurnID != "turn-retention" {
		t.Fatalf("workflow receipt identity = %+v", got)
	}
}

func assertDeliveryRetirementEvidencePurged(t *testing.T, repo *Repository, sessionID string) {
	t.Helper()
	for _, table := range []string{"agent_delivery_inbox", "agent_delivery_effects", "agent_delivery_submissions", "agent_delivery_cursors"} {
		assertRetentionRowCount(t, repo, table, "session_id = ?", sessionID, 0)
	}
}
