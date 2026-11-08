package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestDeliveryRetentionConcurrencyDifferentStreamSharedSession(t *testing.T) {
	repo, old := newRetentionSharedBarrierFixture(t)
	release, collectorDone := holdRetentionSharedCollector(t, repo)
	defer release()
	next := retentionSharedEvent(t, old, "shared-stream-b", "new live output")
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer cancel()
	inserted, receiveErr := repo.ReceiveAgentDeliveryEvent(ctx, next, 1)
	var projectErr error
	if receiveErr == nil {
		_, projectErr = repo.ProjectCanonicalAgentDeliveryEvent(ctx, next, completedRetentionEventEffect(next))
	}
	release()
	if err := <-collectorDone; err != nil {
		t.Fatalf("collector commit: %v", err)
	}
	if receiveErr != nil || !inserted || projectErr != nil {
		t.Fatalf("different-stream authenticated receive/projection waited for event-only retention: inserted=%v receive=%v project=%v", inserted, receiveErr, projectErr)
	}
	assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id = ? AND projected_at IS NOT NULL", next.StreamID, 1)
	if got := retentionCanonicalContent(t, repo, next.SessionID); got != "old canonical outputnew live output" {
		t.Fatalf("canonical output after independent stream projection: %q", got)
	}
}

func TestDeliveryRetentionConcurrencySameStreamRemainsExclusive(t *testing.T) {
	repo, old := newRetentionSharedBarrierFixture(t)
	release, collectorDone := holdRetentionSharedCollector(t, repo)
	defer release()
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	_, err := repo.ReceiveAgentDeliveryEvent(ctx, old, 1)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("same-stream receive escaped held retention stream lock: %v", err)
	}
	release()
	if err := <-collectorDone; err != nil {
		t.Fatalf("collector commit: %v", err)
	}
	if _, err := repo.ReceiveAgentDeliveryEvent(t.Context(), old, 1); !errors.Is(err, repoerrors.ErrAgentDeliveryReplayExpired) {
		t.Fatalf("old same-stream replay after checkpoint: %v", err)
	}
	cursor, err := repo.GetAgentDeliveryCursor(t.Context(), old.StreamID)
	if err != nil || cursor.RetiredSequence != 1 || cursor.ProjectedSequence != 1 {
		t.Fatalf("same-stream monotonic checkpoint: %+v %v", cursor, err)
	}
	if got := retentionCanonicalContent(t, repo, old.SessionID); got != "old canonical output" {
		t.Fatalf("expired replay changed canonical output: %q", got)
	}
}

func TestDeliveryRetentionConcurrencyMatchingGenericOwnershipShared(t *testing.T) {
	repo, old := newRetentionSharedBarrierFixture(t)
	submission := seedRetentionSharedTerminal(t, repo, old, false).SubmissionID
	next := retentionSharedEvent(t, old, "shared-stream-b", "owned output")
	next.SubmissionID = submission
	effect := completedRetentionEventEffect(next)
	effect.SessionID, effect.IncarnationID = next.SessionID, next.IncarnationID
	effect.HarnessGeneration, effect.SubmissionID = next.HarnessGeneration, next.SubmissionID
	release, collectorDone := holdRetentionSharedCollector(t, repo)
	defer release()
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer cancel()
	inserted, receiveErr := repo.ReceiveAgentDeliveryEvent(ctx, next, 1)
	var projectErr error
	if receiveErr == nil {
		_, projectErr = repo.ProjectCanonicalAgentDeliveryEvent(ctx, next, effect)
	}
	release()
	if err := <-collectorDone; err != nil {
		t.Fatalf("collector commit: %v", err)
	}
	if receiveErr != nil || !inserted || projectErr != nil {
		t.Fatalf("exact matching generic ownership blocked independent delivery: inserted=%v receive=%v project=%v", inserted, receiveErr, projectErr)
	}
	assertRetentionRowCount(t, repo, "agent_delivery_effects", "effect_key = ? AND session_id = 'shared-barrier-session' AND submission_id = 'shared-terminal-submission'", effect.EffectKey, 1)
}

func TestDeliveryRetentionConcurrencyStoredTerminalCannotBecomeShared(t *testing.T) {
	repo, old := newRetentionSharedBarrierFixture(t)
	terminal := seedRetentionSharedTerminal(t, repo, old, true)
	forged := *terminal
	forged.Terminal = false
	release, collectorDone := holdRetentionSharedCollector(t, repo)
	defer release()
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer cancel()
	_, receiveErr := repo.ReceiveAgentDeliveryEvent(ctx, &forged, 1)
	_, projectErr := repo.ProjectCanonicalAgentDeliveryEvent(ctx, &forged, completedRetentionEventEffect(&forged))
	release()
	if err := <-collectorDone; err != nil {
		t.Fatalf("collector commit: %v", err)
	}
	if !errors.Is(receiveErr, repoerrors.ErrAgentDeliveryEventConflict) || !errors.Is(projectErr, repoerrors.ErrAgentDeliveryEventConflict) {
		t.Fatalf("false terminal flag bypassed stored identity: receive=%v project=%v", receiveErr, projectErr)
	}
	assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id = ? AND terminal = 1 AND projected_at IS NULL", terminal.StreamID, 1)
	assertRetentionRowCount(t, repo, "agent_delivery_effects", "stream_id = ?", terminal.StreamID, 0)
}

func TestDeliveryRetentionConcurrencyCanonicalEffectScopeFallback(t *testing.T) {
	for _, kind := range []string{"cross_stream", "custom_key", "pending", "missing_completed_at", "foreign_owner", "foreign_incarnation", "foreign_generation", "foreign_submission", "nil"} {
		t.Run(kind, func(t *testing.T) {
			repo, old := newRetentionSharedBarrierFixture(t)
			next := retentionSharedEvent(t, old, "shared-stream-b", "scoped output")
			if inserted, err := repo.ReceiveAgentDeliveryEvent(t.Context(), next, 1); err != nil || !inserted {
				t.Fatalf("prepare different-stream receipt: %v %v", inserted, err)
			}
			effect := retentionSharedUnsafeEffect(next, old, kind)
			release, collectorDone := holdRetentionSharedCollector(t, repo)
			defer release()
			ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
			_, err := repo.ProjectCanonicalAgentDeliveryEvent(ctx, next, effect)
			cancel()
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("%s effect scope escaped exclusive session fallback: %v", kind, err)
			}
			release()
			if err := <-collectorDone; err != nil {
				t.Fatalf("collector commit: %v", err)
			}
			if _, err := repo.ProjectCanonicalAgentDeliveryEvent(t.Context(), next, effect); err != nil {
				t.Fatalf("%s existing projection behavior after exclusive fence: %v", kind, err)
			}
			assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id = ? AND projected_at IS NOT NULL", next.StreamID, 1)
		})
	}
}

func retentionSharedUnsafeEffect(next, old *models.AgentDeliveryEvent, kind string) *models.AgentDeliveryEffect {
	effect := completedRetentionEventEffect(next)
	switch kind {
	case "cross_stream":
		effect.StreamID, effect.Sequence, effect.EffectKey = old.StreamID, old.Sequence, "foreign-dependency"
		effect.EffectType, effect.State, effect.CompletedAt = "unknown-dependency", models.DeliveryEffectPending, nil
	case "custom_key":
		effect.EffectKey = "custom-generic-receipt"
	case "pending":
		effect.State = models.DeliveryEffectPending
	case "missing_completed_at":
		effect.CompletedAt = nil
	case "foreign_owner":
		effect.SessionID = "foreign-effect-owner"
	case "foreign_incarnation":
		effect.IncarnationID = "foreign-effect-incarnation"
	case "foreign_generation":
		effect.HarnessGeneration = next.HarnessGeneration + 1
	case "foreign_submission":
		effect.SubmissionID = "foreign-effect-submission"
	case "nil":
		return nil
	}
	return effect
}

func TestDeliveryRetentionConcurrencyTerminalReceiptRemainsExclusive(t *testing.T) {
	for _, boundary := range []string{"receive", "canonical_projection"} {
		t.Run(boundary, func(t *testing.T) {
			repo, old := newRetentionSharedBarrierFixture(t)
			terminal := seedRetentionSharedTerminal(t, repo, old, boundary == "canonical_projection")
			release, collectorDone := holdRetentionSharedCollector(t, repo)
			defer release()
			ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
			var err error
			if boundary == "receive" {
				_, err = repo.ReceiveAgentDeliveryEvent(ctx, terminal, 1)
			} else {
				_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, terminal, completedRetentionEventEffect(terminal))
			}
			cancel()
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("terminal %s escaped exclusive session fence: %v", boundary, err)
			}
			release()
			if err := <-collectorDone; err != nil {
				t.Fatalf("collector commit: %v", err)
			}
			if boundary == "receive" {
				if inserted, err := repo.ReceiveAgentDeliveryEvent(t.Context(), terminal, 1); err != nil || !inserted {
					t.Fatalf("terminal receive after fence: %v %v", inserted, err)
				}
			}
			if _, err := repo.ProjectCanonicalAgentDeliveryEvent(t.Context(), terminal, completedRetentionEventEffect(terminal)); err != nil {
				t.Fatalf("terminal projection after fence: %v", err)
			}
			effect, err := repo.GetAgentDeliveryEffect(t.Context(), terminalRetentionEffectKey(*terminal))
			if err != nil || effect.State != models.DeliveryEffectPending || effect.SubmissionID != terminal.SubmissionID || effect.TurnID != "shared-terminal-turn" {
				t.Fatalf("actual derived terminal receipt: %+v %v", effect, err)
			}
		})
	}
}

func seedRetentionSharedTerminal(t *testing.T, repo *Repository, owner *models.AgentDeliveryEvent, receive bool) *models.AgentDeliveryEvent {
	t.Helper()
	submission := &models.AgentDeliverySubmission{ID: "shared-terminal-submission", SessionID: owner.SessionID, IncarnationID: owner.IncarnationID, HarnessGeneration: owner.HarnessGeneration, OwnerGeneration: owner.OwnerGeneration, DispatchAttemptID: "shared-terminal-dispatch", PayloadHash: "shared-terminal-hash", Payload: []byte("live terminal prompt"), State: models.DeliverySubmissionDispatching}
	if prepared, err := repo.PrepareAgentDeliverySubmission(t.Context(), submission); err != nil || !prepared {
		t.Fatalf("prepare actual terminal submission: %v %v", prepared, err)
	}
	event := retentionSharedEvent(t, owner, "shared-stream-b", "")
	event.SubmissionID, event.Terminal, event.EventType = submission.ID, true, streams.EventTypeComplete
	payload, err := json.Marshal(streams.AgentEvent{Type: streams.EventTypeComplete, TurnID: "shared-terminal-turn"})
	if err != nil {
		t.Fatal(err)
	}
	event.Payload = payload
	if receive {
		if inserted, err := repo.ReceiveAgentDeliveryEvent(t.Context(), event, 1); err != nil || !inserted {
			t.Fatalf("prepare actual terminal inbox: %v %v", inserted, err)
		}
	}
	return event
}

func TestDeliveryRetentionConcurrencySharedSessionMutationFences(t *testing.T) {
	for _, kind := range []string{"owner", "harness_and_block", "submission", "recovery", "terminal_settlement", "workflow_outcome", "deletion", "retirement", "recreation"} {
		t.Run(kind, func(t *testing.T) {
			repo, old := newRetentionSharedBarrierFixture(t)
			mutate, verify := retentionSharedMutation(t, repo, old, kind)
			var release func()
			var collectorDone <-chan error
			if kind == "owner" {
				release, collectorDone = holdRetentionOwnerCollector(t, repo)
			} else {
				release, collectorDone = holdRetentionSharedCollector(t, repo)
			}
			defer release()
			if kind == "owner" {
				assertRetentionOwnerMutationBlocked(t, repo, old.SessionID, mutate, release)
			} else {
				ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
				err := mutate(ctx)
				cancel()
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Errorf("%s mutation escaped event retention's shared session fence: %v", kind, err)
				}
			}
			release()
			if err := <-collectorDone; err != nil {
				t.Fatalf("collector commit: %v", err)
			}
			if err := mutate(t.Context()); err != nil {
				t.Fatalf("%s positive mutation after retention commit: %v", kind, err)
			}
			verify()
		})
	}
}

func holdRetentionOwnerCollector(t *testing.T, repo *Repository) (func(), <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	reached, resume, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(resume) }) }
	done := make(chan error, 1)
	repo.agentDeliveryRetentionFailpoint = func(point string) error {
		if point == "before_payload" {
			close(reached)
			<-resume
		}
		return nil
	}
	t.Cleanup(func() {
		cancel()
		release()
		<-finished
	})
	go func() {
		defer close(finished)
		result, err := repo.RunAgentDeliveryRetentionBatch(ctx, models.AgentDeliveryRetentionBatchOptions{MaxCandidates: 1})
		if err == nil && result.EventRowsRetired != 1 {
			err = fmt.Errorf("collector retired %d events, want 1", result.EventRowsRetired)
		}
		done <- err
	}()
	select {
	case <-reached:
	case err := <-done:
		t.Fatalf("collector did not reach locked payload boundary: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("collector did not reach locked payload boundary")
	}
	return release, done
}

func assertRetentionOwnerMutationBlocked(t *testing.T, repo *Repository, sessionID string, mutate func(context.Context) error, release func()) {
	t.Helper()
	var collectorPID int
	if err := repo.db.QueryRowxContext(t.Context(), `SELECT pid FROM pg_locks
		WHERE locktype = 'advisory' AND granted AND mode = 'ShareLock'
		  AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
		  AND classid = ((hashtextextended($1, 0) >> 32) & 4294967295)::oid
		  AND objid = (hashtextextended($1, 0) & 4294967295)::oid AND objsubid = 1`,
		agentDeliverySessionLockNamespace+sessionID).Scan(&collectorPID); err != nil {
		t.Fatalf("read collector's shared session lock: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	var mutationErr error
	go func() {
		defer close(done)
		mutationErr = mutate(ctx)
	}()
	defer func() {
		cancel()
		release()
		<-done
	}()
	waitForRetentionOwnerWaiter(t, repo, sessionID, collectorPID, done)
	cancel()
	<-done
	if !errors.Is(mutationErr, context.Canceled) {
		t.Fatalf("cancel owner mutation after observed session fence: %v", mutationErr)
	}
}

func waitForRetentionOwnerWaiter(t *testing.T, repo *Repository, sessionID string, collectorPID int, done <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiterPID int
		err := repo.db.QueryRowxContext(ctx, `SELECT w.pid FROM pg_locks w JOIN pg_locks b
			ON b.locktype = w.locktype AND b.database = w.database
			AND b.classid = w.classid AND b.objid = w.objid AND b.objsubid = w.objsubid
			WHERE w.locktype = 'advisory' AND NOT w.granted AND w.mode = 'ExclusiveLock'
			  AND b.granted AND b.mode = 'ShareLock' AND b.pid = $2
			  AND w.database = (SELECT oid FROM pg_database WHERE datname = current_database())
			  AND w.classid = ((hashtextextended($1, 0) >> 32) & 4294967295)::oid
			  AND w.objid = (hashtextextended($1, 0) & 4294967295)::oid AND w.objsubid = 1
			  AND b.pid = ANY(pg_blocking_pids(w.pid)) LIMIT 1`,
			agentDeliverySessionLockNamespace+sessionID, collectorPID).Scan(&waiterPID)
		if err == nil {
			return
		}
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("observe owner session fence: %v", err)
		}
		select {
		case <-done:
			t.Fatal("owner mutation escaped before observed session fence")
		case <-ctx.Done():
			t.Fatal("owner did not wait on collector's shared session fence")
		case <-ticker.C:
		}
	}
}

func retentionSharedMutation(t *testing.T, repo *Repository, old *models.AgentDeliveryEvent, kind string) (func(context.Context) error, func()) {
	t.Helper()
	switch kind {
	case "owner":
		return func(ctx context.Context) error {
			return repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "replacement-running", SessionID: old.SessionID, TaskID: old.TaskID, ExecutorID: "shared-executor", AgentExecutionID: "replacement-execution", Status: "running"})
		}, func() { assertRetentionSharedOldOwnerRefused(t, repo, old) }
	case "harness_and_block":
		block := &models.SessionRecoveryBlock{ID: "shared-generation-block", SessionID: old.SessionID, IncarnationID: old.IncarnationID, ExpectedGeneration: 1, Reason: "native_state_missing", State: models.RecoveryBlockOpen, ConsumerReference: "agent_delivery"}
		if err := repo.UpsertSessionRecoveryBlock(t.Context(), block); err != nil {
			t.Fatal(err)
		}
		return func(ctx context.Context) error {
				stored, err := repo.CommitHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{SessionID: old.SessionID, IncarnationID: old.IncarnationID, Generation: 2, PredecessorGeneration: 1, NativeSessionID: "replacement-native", CreationReason: "continuation"}, 1)
				return retentionSharedAccepted(stored, err)
			}, func() {
				assertRetentionRowCount(t, repo, "session_recovery_blocks", "id = ? AND expected_generation = 2 AND state = 'open'", block.ID, 1)
				assertRetentionSharedOldOwnerRefused(t, repo, old)
			}
	case "submission":
		return func(ctx context.Context) error {
				stored, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{ID: "shared-new-submission", SessionID: old.SessionID, IncarnationID: old.IncarnationID, HarnessGeneration: 1, OwnerGeneration: 1, DispatchAttemptID: "new-dispatch", PayloadHash: "new-hash", Payload: []byte("new prompt")})
				return retentionSharedAccepted(stored, err)
			}, func() {
				assertRetentionRowCount(t, repo, "agent_delivery_submissions", "id = ?", "shared-new-submission", 1)
			}
	case "recovery":
		terminal := seedRetentionSharedTerminal(t, repo, old, false)
		return func(ctx context.Context) error {
				return repo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{ID: "shared-recovery-block", SessionID: old.SessionID, IncarnationID: old.IncarnationID, ExpectedGeneration: 1, Reason: "unknown_prompt_outcome", State: models.RecoveryBlockOpen, ConsumerReference: "agent_delivery", DeliverySubmissionID: terminal.SubmissionID, DeliveryStreamID: terminal.StreamID})
			}, func() {
				assertRetentionRowCount(t, repo, "session_recovery_blocks", "id = ? AND state = 'open'", "shared-recovery-block", 1)
			}
	case "terminal_settlement", "workflow_outcome":
		return retentionSharedTerminalMutation(t, repo, old, kind)
	case "deletion":
		if _, err := messagequeue.NewSQLiteRepository(repo.db, repo.db); err != nil {
			t.Fatalf("initialize actual deletion queue schema: %v", err)
		}
		session, err := repo.GetTaskSession(t.Context(), old.SessionID)
		if err != nil {
			t.Fatal(err)
		}
		return func(ctx context.Context) error { return repo.DeleteTaskSession(ctx, session) }, func() {
			assertRetentionRowCount(t, repo, "task_sessions", "id = ?", old.SessionID, 0)
			assertRetentionRowCount(t, repo, "agent_delivery_cleanup_jobs", "session_id = ? AND phase = 'pending'", old.SessionID, 1)
		}
	case "retirement":
		seedRetentionAdmissionJob(t, repo, old.SessionID, old.IncarnationID)
		return func(ctx context.Context) error {
				result, err := repo.RunAgentDeliveryRetirementBatch(ctx, models.AgentDeliveryRetirementBatchOptions{})
				return retentionSharedAccepted(result.Suspended == 1 && result.RowsPurged == 0, err)
			}, func() {
				assertRetentionRowCount(t, repo, "agent_delivery_cleanup_jobs", "session_id = ? AND error_code = 'live_session_conflict'", old.SessionID, 1)
			}
	case "recreation":
		return func(ctx context.Context) error {
				err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: old.SessionID, TaskID: old.TaskID, QueueIncarnationID: "replacement-incarnation"})
				if errors.Is(err, repoerrors.ErrAgentDeliveryIdentityRetiring) {
					return nil
				}
				if err == nil {
					return errors.New("recreation consumed retained identity")
				}
				return err
			}, func() {
				assertRetentionRowCount(t, repo, "task_sessions", "id = ? AND queue_incarnation_id = 'shared-barrier-incarnation'", old.SessionID, 1)
			}
	}
	t.Fatalf("unknown shared fence mutation %q", kind)
	return nil, nil
}

func retentionSharedTerminalMutation(t *testing.T, repo *Repository, old *models.AgentDeliveryEvent, kind string) (func(context.Context) error, func()) {
	t.Helper()
	terminal := seedRetentionSharedTerminal(t, repo, old, true)
	if _, err := repo.ProjectCanonicalAgentDeliveryEvent(t.Context(), terminal, completedRetentionEventEffect(terminal)); err != nil {
		t.Fatal(err)
	}
	if kind == "terminal_settlement" {
		return func(ctx context.Context) error {
				settled, err := repo.SettleAgentDeliveryTerminal(ctx, terminal.StreamID, terminal.Sequence, models.DeliverySubmissionCompleted, time.Time{})
				return retentionSharedAccepted(settled, err)
			}, func() {
				assertRetentionRowCount(t, repo, "agent_delivery_submissions", "id = ? AND state = 'completed'", terminal.SubmissionID, 1)
			}
	}
	if settled, err := repo.SettleAgentDeliveryTerminal(t.Context(), terminal.StreamID, terminal.Sequence, models.DeliverySubmissionCompleted, time.Time{}); err != nil || !settled {
		t.Fatalf("prepare real workflow terminal settlement: %v %v", settled, err)
	}
	now := repo.nowUTC()
	effect := &models.AgentDeliveryEffect{EffectKey: "workflow.on_turn_complete:shared-terminal-turn", StreamID: terminal.StreamID, Sequence: terminal.Sequence, EffectType: "workflow.on_turn_complete", State: models.DeliveryEffectCompleted, TaskID: old.TaskID, SessionID: old.SessionID, IncarnationID: old.IncarnationID, HarnessGeneration: old.HarnessGeneration, SubmissionID: terminal.SubmissionID, TurnID: "shared-terminal-turn", Outcome: "not_required", CreatedAt: now, CompletedAt: &now}
	return func(ctx context.Context) error {
			recorded, err := repo.RecordAgentDeliveryWorkflowOutcome(ctx, old.TaskID, effect)
			return retentionSharedAccepted(recorded, err)
		}, func() {
			assertRetentionRowCount(t, repo, "agent_delivery_effects", "effect_key = ? AND state = 'completed'", effect.EffectKey, 1)
		}
}

func retentionSharedAccepted(accepted bool, err error) error {
	if err != nil {
		return err
	}
	if !accepted {
		return errors.New("prepared mutation did not reach its positive write boundary")
	}
	return nil
}

func assertRetentionSharedOldOwnerRefused(t *testing.T, repo *Repository, old *models.AgentDeliveryEvent) {
	t.Helper()
	if _, err := repo.ReceiveAgentDeliveryEvent(t.Context(), old, 1); !errors.Is(err, repoerrors.ErrAgentDeliveryIdentityMismatch) {
		t.Fatalf("stale admission reached old checkpoint after owner replacement: %v", err)
	}
	cursor, err := repo.GetAgentDeliveryCursor(t.Context(), old.StreamID)
	if err != nil || cursor.RetiredSequence != 1 || cursor.ProjectedSequence != 1 {
		t.Fatalf("stale owner changed compact checkpoint: %+v %v", cursor, err)
	}
}

func TestDeliveryRetentionConcurrencyMixedPromptSessionLockModes(t *testing.T) {
	repo, old := newRetentionSharedBarrierFixture(t)
	seedRetentionPostgresSettledPrompt(t, repo, old.TaskID)
	release, collectorDone := holdRetentionSharedCollectorWithOptions(t, repo, models.AgentDeliveryRetentionBatchOptions{MaxCandidates: 10}, 2)
	defer release()
	eventMode := retentionSharedGrantedSessionMode(t, repo, old.SessionID)
	promptMode := retentionSharedGrantedSessionMode(t, repo, "postgres-settled-session")
	release()
	if err := <-collectorDone; err != nil {
		t.Fatalf("mixed batch commit: %v", err)
	}
	if eventMode != "ShareLock" || promptMode != "ExclusiveLock" {
		t.Fatalf("mixed batch must share event-only session and exclude prompt session: event=%s prompt=%s", eventMode, promptMode)
	}
	assertRetentionRowCount(t, repo, "agent_delivery_submissions", "id = ? AND payload_pruned_at IS NOT NULL", "postgres-settled-prompt", 1)
}

func retentionSharedGrantedSessionMode(t *testing.T, repo *Repository, sessionID string) string {
	t.Helper()
	var mode string
	err := repo.db.QueryRowx(`SELECT mode FROM pg_locks
		WHERE locktype = 'advisory' AND granted AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
		  AND classid = ((hashtextextended($1, 0) >> 32) & 4294967295)::oid
		  AND objid = (hashtextextended($1, 0) & 4294967295)::oid AND objsubid = 1`,
		"agent-delivery-session:"+sessionID).Scan(&mode)
	if err != nil {
		t.Fatalf("read actual granted retention session mode: %v", err)
	}
	return mode
}

func TestDeliveryRetentionConcurrencyMixedSessionsStayGloballySorted(t *testing.T) {
	repo, old := newRetentionSharedBarrierFixture(t)
	seedRetentionPostgresSettledPrompt(t, repo, old.TaskID)
	blocker, err := repo.db.BeginTxx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = blocker.Rollback() }()
	if _, err := blocker.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "agent-delivery-session:postgres-settled-session"); err != nil {
		t.Fatal(err)
	}
	reached, resume := make(chan struct{}), make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(resume) }) }
	defer release()
	repo.agentDeliveryRetentionFailpoint = func(point string) error {
		if point == "before_payload" {
			close(reached)
			<-resume
		}
		return nil
	}
	done := make(chan error, 1)
	go func() {
		_, err := repo.RunAgentDeliveryRetentionBatch(t.Context(), models.AgentDeliveryRetentionBatchOptions{MaxCandidates: 10})
		done <- err
	}()
	pid := retentionSharedWaitingSessionPID(t, repo, "postgres-settled-session")
	var laterLocked bool
	err = repo.db.QueryRowx(`SELECT EXISTS (SELECT 1 FROM pg_locks
		WHERE pid = $1 AND locktype = 'advisory' AND granted
		  AND classid = ((hashtextextended($2, 0) >> 32) & 4294967295)::oid
		  AND objid = (hashtextextended($2, 0) & 4294967295)::oid AND objsubid = 1)`, pid,
		"agent-delivery-session:"+old.SessionID).Scan(&laterLocked)
	if err != nil {
		t.Fatal(err)
	}
	if err := blocker.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-reached:
	case err := <-done:
		t.Fatalf("mixed collector failed before payload boundary: %v", err)
	case <-time.After(time.Second):
		t.Fatal("mixed collector did not resume after earlier session release")
	}
	release()
	if err := <-done; err != nil {
		t.Fatalf("mixed collector commit: %v", err)
	}
	if laterLocked {
		t.Fatal("mixed collector acquired later shared session before earlier exclusive prompt session")
	}
}

func retentionSharedWaitingSessionPID(t *testing.T, repo *Repository, sessionID string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		var pid int
		err := repo.db.QueryRowxContext(ctx, `SELECT COALESCE(MAX(pid), 0) FROM pg_locks
			WHERE locktype = 'advisory' AND NOT granted AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
			  AND classid = ((hashtextextended($1, 0) >> 32) & 4294967295)::oid
			  AND objid = (hashtextextended($1, 0) & 4294967295)::oid AND objsubid = 1`,
			"agent-delivery-session:"+sessionID).Scan(&pid)
		if err != nil {
			t.Fatalf("observe real mixed-session lock waiter: %v", err)
		}
		if pid != 0 {
			return pid
		}
		select {
		case <-ctx.Done():
			t.Fatal("mixed collector did not wait on earlier exclusive session")
		case <-ticker.C:
		}
	}
}

func TestDeliveryRetentionConcurrencySharedCollectorCancellation(t *testing.T) {
	repo, old := newRetentionSharedBarrierFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	release, collectorDone := holdRetentionSharedCollectorContext(t, ctx, repo, models.AgentDeliveryRetentionBatchOptions{MaxCandidates: 1}, 1)
	cancel()
	release()
	if err := <-collectorDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled shared collector result: %v", err)
	}
	assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id = ? AND payload_pruned_at IS NULL", old.StreamID, 1)
	assertRetentionRowCount(t, repo, "agent_delivery_effects", "stream_id = ?", old.StreamID, 1)
	cursor, err := repo.GetAgentDeliveryCursor(t.Context(), old.StreamID)
	if err != nil || cursor.RetiredSequence != 0 {
		t.Fatalf("canceled collector advanced checkpoint: %+v %v", cursor, err)
	}
	repo.agentDeliveryRetentionFailpoint = nil
	if result, err := repo.RunAgentDeliveryRetentionBatch(t.Context(), models.AgentDeliveryRetentionBatchOptions{MaxCandidates: 1}); err != nil || result.EventRowsRetired != 1 {
		t.Fatalf("collector retry after canceled shared locks: %+v %v", result, err)
	}
}

func newRetentionSharedBarrierFixture(t *testing.T) (*Repository, *models.AgentDeliveryEvent) {
	t.Helper()
	repo := newRetentionTestRepo(t, "Postgres")
	repo.db.SetMaxOpenConns(4)
	repo.db.SetMaxIdleConns(4)
	ctx := t.Context()
	const taskID, sessionID = "shared-barrier-task", "shared-barrier-session"
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: retentionTestWorkspaceID(t, repo), Title: "Shared retention barrier"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID, QueueIncarnationID: "shared-barrier-incarnation"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "shared-running", SessionID: sessionID, TaskID: taskID, ExecutorID: "shared-executor", AgentExecutionID: "shared-execution", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	old := now.Add(-48 * time.Hour)
	if stored, err := repo.CommitHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{SessionID: sessionID, IncarnationID: "shared-barrier-incarnation", Generation: 1, NativeSessionID: "shared-native", CreationReason: "initial", CreatedAt: old, CommittedAt: old}, 0); err != nil || !stored {
		t.Fatalf("create authenticated generation: %v %v", stored, err)
	}
	repo.clockNow = func() time.Time { return old }
	event := retentionSharedEvent(t, &models.AgentDeliveryEvent{TaskID: taskID, SessionID: sessionID, IncarnationID: "shared-barrier-incarnation", HarnessGeneration: 1, ExecutionID: "shared-execution", OwnerGeneration: 1, ReceivedAt: old}, "shared-stream-a", "old canonical output")
	if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, event, 1); err != nil || !inserted {
		t.Fatalf("seed authenticated old event: %v %v", inserted, err)
	}
	if _, err := repo.ProjectCanonicalAgentDeliveryEvent(ctx, event, completedRetentionEventEffect(event)); err != nil {
		t.Fatal(err)
	}
	repo.clockNow = func() time.Time { return now }
	return repo, event
}

func retentionSharedEvent(t *testing.T, owner *models.AgentDeliveryEvent, streamID, text string) *models.AgentDeliveryEvent {
	t.Helper()
	payload, err := json.Marshal(streams.AgentEvent{Type: streams.EventTypeMessageChunk, Text: text, CanonicalMessageID: "canonical-" + streamID})
	if err != nil {
		t.Fatal(err)
	}
	event := *owner
	event.StreamID, event.ExpectedStreamID = streamID, streamID
	event.Sequence, event.EventType, event.Payload = 1, streams.EventTypeMessageChunk, payload
	event.ContentHash, event.HashVersion, event.PayloadBytes = "", 0, 0
	return &event
}

func holdRetentionSharedCollector(t *testing.T, repo *Repository) (func(), <-chan error) {
	t.Helper()
	return holdRetentionSharedCollectorWithOptions(t, repo, models.AgentDeliveryRetentionBatchOptions{MaxCandidates: 1}, 1)
}

func holdRetentionSharedCollectorWithOptions(t *testing.T, repo *Repository, options models.AgentDeliveryRetentionBatchOptions, wantRetired int) (func(), <-chan error) {
	t.Helper()
	return holdRetentionSharedCollectorContext(t, t.Context(), repo, options, wantRetired)
}

func holdRetentionSharedCollectorContext(t *testing.T, ctx context.Context, repo *Repository, options models.AgentDeliveryRetentionBatchOptions, wantRetired int) (func(), <-chan error) {
	t.Helper()
	// Lock proofs deliberately park the transaction; the public work budget is
	// verified separately without charging this test-controlled pause to it.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	reached, resume := make(chan struct{}), make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(resume) }) }
	repo.agentDeliveryRetentionFailpoint = func(point string) error {
		if point == "before_payload" {
			close(reached)
			select {
			case <-resume:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}
	done := make(chan error, 1)
	finished := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		release()
		<-finished
	})
	go func() {
		defer close(finished)
		result, err := repo.runAgentDeliveryRetentionBatch(ctx, normalizeAgentDeliveryRetentionOptions(options))
		if err == nil && result.EventRowsRetired != wantRetired {
			err = fmt.Errorf("collector retired %d events, want %d", result.EventRowsRetired, wantRetired)
		}
		done <- err
	}()
	select {
	case <-reached:
	case err := <-done:
		release()
		t.Fatalf("collector did not reach locked payload boundary: %v", err)
	case <-time.After(2 * time.Second):
		release()
		t.Fatal("collector did not reach locked payload boundary")
	}
	return release, done
}
