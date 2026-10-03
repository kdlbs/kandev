package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	_ "github.com/mattn/go-sqlite3"
)

func TestDeliveryTerminalSettlesMatchingBlock(t *testing.T) {
	ctx := context.Background()
	service, repo := newServiceWithRealRepo(t)
	seedSession(t, repo, "task-delivery-settlement", "session-delivery-settlement", "step-1")
	queue, queueDB := newWorkflowTransferQueue(t, t.TempDir()+"/queue.db")
	t.Cleanup(func() { _ = queueDB.Close() })
	service.messageQueue = queue

	queued, err := queue.QueueMessage(ctx, "session-delivery-settlement", "task-delivery-settlement", "original prompt", "", messagequeue.QueuedByUser, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	claim, ok := queue.ReserveQueued(ctx, queued.SessionID)
	if !ok {
		t.Fatal("queue prompt was not reserved")
	}
	_, dispatchAttemptID, _ := claim.DeliverySubmission()
	if dispatchAttemptID == "" {
		t.Fatal("reserved prompt has no stable delivery identity")
	}
	if err := queue.SetPendingQueueDispatchDelivery(ctx, claim, messagequeue.DeliveryProtocolV1, dispatchAttemptID, "payload-hash"); err != nil {
		t.Fatalf("bind durable queue claim: %v", err)
	}
	submissionID := normalizeDeliverySubmissionID(dispatchAttemptID)
	if _, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: submissionID, SessionID: claim.SessionID, IncarnationID: claim.SessionID,
		HarnessGeneration: 1, OwnerGeneration: 1, DispatchAttemptID: dispatchAttemptID,
		PayloadHash: "payload-hash", Payload: []byte("original prompt"),
		State: models.DeliverySubmissionDispatching,
	}); err != nil {
		t.Fatalf("persist SQL submission: %v", err)
	}
	block := &models.SessionRecoveryBlock{
		ID: "delivery-block", SessionID: claim.SessionID, IncarnationID: claim.SessionID,
		ExpectedGeneration: 1, Reason: "unknown_prompt_outcome", State: models.RecoveryBlockOpen,
		ConsumerReference: "agent_delivery", DeliverySubmissionID: submissionID,
	}
	if err := repo.UpsertSessionRecoveryBlock(ctx, block); err != nil {
		t.Fatalf("persist recovery block: %v", err)
	}

	deliveryJournal, err := journal.Open(journal.Config{Path: t.TempDir() + "/agentctl.db"})
	if err != nil {
		t.Fatalf("open agentctl journal: %v", err)
	}
	t.Cleanup(func() { _ = deliveryJournal.Close() })
	if _, err := deliveryJournal.PutSubmission(ctx, journal.Submission{
		ID: submissionID, StreamID: "session-delivery-settlement:g1", SessionID: claim.SessionID,
		IncarnationID: claim.SessionID, HarnessGeneration: 1, Hash: "payload-hash",
		Payload: []byte("original prompt"), State: journal.SubmissionDispatching,
	}); err != nil {
		t.Fatalf("persist agentctl submission: %v", err)
	}
	payload, _ := json.Marshal(map[string]string{"type": "complete", "turn_id": "turn-delivery-settlement"})
	terminal, err := deliveryJournal.Append(ctx, journal.Event{
		SessionID: claim.SessionID, IncarnationID: claim.SessionID, HarnessGeneration: 1,
		StreamID: "session-delivery-settlement:g1", SubmissionID: submissionID,
		Type: "complete", Payload: payload, Terminal: true,
	})
	if err != nil {
		t.Fatalf("append terminal to agentctl journal: %v", err)
	}
	replayed, _, err := deliveryJournal.Replay(ctx, terminal.StreamID, 0, 10)
	if err != nil || len(replayed) != 1 {
		t.Fatalf("replay terminal from agentctl journal = %d events, %v", len(replayed), err)
	}
	event := &models.AgentDeliveryEvent{
		SessionID: replayed[0].SessionID, IncarnationID: replayed[0].IncarnationID,
		HarnessGeneration: int64(replayed[0].HarnessGeneration), StreamID: replayed[0].StreamID,
		Sequence: int64(replayed[0].Sequence), SubmissionID: replayed[0].SubmissionID,
		EventType: replayed[0].Type, Payload: replayed[0].Payload, Terminal: replayed[0].Terminal,
		ReceivedAt: replayed[0].CreatedAt,
	}
	if _, err := repo.ReceiveAgentDeliveryEvent(ctx, event, event.Sequence); err != nil {
		t.Fatalf("receive SQL inbox event: %v", err)
	}
	if _, err := repo.ProjectAgentDeliveryEvent(ctx, event, nil); err != nil {
		t.Fatalf("project SQL inbox event: %v", err)
	}

	if err := service.reconcileAgentDeliverySettlements(ctx, claim.SessionID); err != nil {
		t.Fatalf("reconcile delivery terminal settlement: %v", err)
	}
	settledBlock, err := repo.GetSessionRecoveryBlock(ctx, block.ID)
	if err != nil || settledBlock.State != models.RecoveryBlockResolved || settledBlock.DeliverySubmissionID != submissionID || settledBlock.DeliveryStreamID != terminal.StreamID {
		t.Fatalf("settled recovery block = %+v, %v", settledBlock, err)
	}
	submission, err := repo.GetAgentDeliverySubmission(ctx, submissionID)
	if err != nil || submission.State != models.DeliverySubmissionCompleted {
		t.Fatalf("settled submission = %+v, %v", submission, err)
	}
	pending, err := queue.ListPendingQueueDispatches(ctx)
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending queue claims = %+v, %v; want exact completed claim removed", pending, err)
	}
	if visible := queue.GetStatus(ctx, claim.SessionID); visible.Count != 0 {
		t.Fatalf("terminal prompt became visible for resend: %+v", visible.Entries)
	}
	if err := service.reconcileAgentDeliverySettlements(ctx, claim.SessionID); err != nil {
		t.Fatalf("repeat reconciliation after queue cleanup: %v", err)
	}
	remaining, err := repo.ListPendingAgentDeliverySettlements(ctx, claim.SessionID)
	if err != nil || len(remaining) != 0 {
		t.Fatalf("pending terminal effects = %+v, %v; want none after idempotent cleanup", remaining, err)
	}
}

func TestDeliverySettlementRepairsAfterQueueClaimCleanup(t *testing.T) {
	ctx := context.Background()
	service, repo := newServiceWithRealRepo(t)
	seedSession(t, repo, "task-delivery-claim-crash", "session-delivery-claim-crash", "step-1")
	queue, queueDB := newWorkflowTransferQueue(t, t.TempDir()+"/queue.db")
	t.Cleanup(func() { _ = queueDB.Close() })
	service.messageQueue = queue

	queued, err := queue.QueueMessage(ctx, "session-delivery-claim-crash", "task-delivery-claim-crash", "original prompt", "", messagequeue.QueuedByUser, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	claim, ok := queue.ReserveQueued(ctx, queued.SessionID)
	if !ok {
		t.Fatal("queue prompt was not reserved")
	}
	_, dispatchAttemptID, _ := claim.DeliverySubmission()
	if err := queue.SetPendingQueueDispatchDelivery(ctx, claim, messagequeue.DeliveryProtocolV1, dispatchAttemptID, "crash-hash"); err != nil {
		t.Fatalf("bind durable queue claim: %v", err)
	}
	submissionID := normalizeDeliverySubmissionID(dispatchAttemptID)
	if _, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: submissionID, SessionID: claim.SessionID, IncarnationID: claim.SessionID,
		HarnessGeneration: 1, OwnerGeneration: 1, DispatchAttemptID: dispatchAttemptID,
		PayloadHash: "crash-hash", Payload: []byte("original prompt"), State: models.DeliverySubmissionDispatching,
	}); err != nil {
		t.Fatalf("persist SQL submission: %v", err)
	}
	if err := repo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{
		ID: "delivery-block-claim-crash", SessionID: claim.SessionID, IncarnationID: claim.SessionID,
		ExpectedGeneration: 1, Reason: "unknown_prompt_outcome", State: models.RecoveryBlockOpen,
		ConsumerReference: "agent_delivery", DeliverySubmissionID: submissionID,
	}); err != nil {
		t.Fatalf("persist recovery block: %v", err)
	}
	event := persistedTerminalEvent(claim.SessionID, claim.SessionID, 1, submissionID, dispatchAttemptID, "complete")
	projectOrchestratorDeliveryEvent(t, repo, ctx, event)

	if settled, err := repo.SettleAgentDeliveryTerminal(ctx, event.StreamID, event.Sequence, models.DeliverySubmissionCompleted, time.Now().UTC()); err != nil || !settled {
		t.Fatalf("settle SQL outcome before simulated crash = %v, %v", settled, err)
	}
	if err := queue.AcknowledgeDurablePendingQueueDispatch(ctx, claim); err != nil {
		t.Fatalf("commit queue-claim cleanup before simulated crash: %v", err)
	}

	if err := service.reconcileAgentDeliverySettlements(ctx, claim.SessionID); err != nil {
		t.Fatalf("repair outbox after claim cleanup: %v", err)
	}
	effect, err := repo.GetAgentDeliveryEffect(ctx, "agent_delivery.terminal:"+event.StreamID+":1")
	if err != nil || effect.State != models.DeliveryEffectCompleted {
		t.Fatalf("repaired settlement effect = %+v, %v; want completed", effect, err)
	}
	pending, err := queue.ListPendingQueueDispatches(ctx)
	if err != nil || len(pending) != 0 {
		t.Fatalf("queue claim after recovered settlement = %+v, %v", pending, err)
	}
}

func TestDeliverySettlementRetriesWhenQueueClaimChangesDuringAcknowledgement(t *testing.T) {
	ctx := context.Background()
	service, repo := newServiceWithRealRepo(t)
	seedSession(t, repo, "task-delivery-claim-conflict", "session-delivery-claim-conflict", "step-1")
	queueDBRaw, err := sql.Open("sqlite3", t.TempDir()+"/queue.db")
	if err != nil {
		t.Fatal(err)
	}
	queueDBRaw.SetMaxOpenConns(1)
	queueDB := sqlx.NewDb(queueDBRaw, "sqlite3")
	t.Cleanup(func() { _ = queueDB.Close() })
	queueRepo, err := messagequeue.NewSQLiteRepository(queueDB, queueDB)
	if err != nil {
		t.Fatalf("create SQLite queue repository: %v", err)
	}
	deliveryBinder, ok := queueRepo.(interface {
		SetPendingQueueDispatchDelivery(context.Context, *messagequeue.QueuedMessage, string, string, string) error
	})
	if !ok {
		t.Fatal("SQLite queue repository does not support durable dispatch binding")
	}
	dispatchState, ok := queueRepo.(interface {
		ListPendingQueueDispatches(context.Context) ([]messagequeue.PendingQueueDispatch, error)
		MarkPendingQueueDispatchAccepted(context.Context, *messagequeue.QueuedMessage) error
	})
	if !ok {
		t.Fatal("SQLite queue repository does not support durable dispatch recovery")
	}
	deleter, ok := queueRepo.(interface {
		DeletePendingQueueDispatch(context.Context, *messagequeue.QueuedMessage) error
	})
	if !ok {
		t.Fatal("SQLite queue repository does not support dispatch claim acknowledgement")
	}
	claimConflictRepo := &changedOnceQueueDispatchRepository{
		Repository:    queueRepo,
		bindPending:   deliveryBinder.SetPendingQueueDispatchDelivery,
		deletePending: deleter.DeletePendingQueueDispatch,
		listPending:   dispatchState.ListPendingQueueDispatches,
		markAccepted:  dispatchState.MarkPendingQueueDispatchAccepted,
		fail:          true,
	}
	queue := messagequeue.NewService(claimConflictRepo, messagequeue.DefaultMaxPerSession, testLogger())
	service.messageQueue = queue

	queued, err := queue.QueueMessage(ctx, "session-delivery-claim-conflict", "task-delivery-claim-conflict", "original prompt", "", messagequeue.QueuedByUser, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	claim, ok := queue.ReserveQueued(ctx, queued.SessionID)
	if !ok {
		t.Fatal("queue prompt was not reserved")
	}
	_, dispatchAttemptID, _ := claim.DeliverySubmission()
	if err := queue.SetPendingQueueDispatchDelivery(ctx, claim, messagequeue.DeliveryProtocolV1, dispatchAttemptID, "conflict-hash"); err != nil {
		t.Fatalf("bind durable queue claim: %v", err)
	}
	submissionID := normalizeDeliverySubmissionID(dispatchAttemptID)
	if _, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: submissionID, SessionID: claim.SessionID, IncarnationID: claim.SessionID,
		HarnessGeneration: 1, OwnerGeneration: 1, DispatchAttemptID: dispatchAttemptID,
		PayloadHash: "conflict-hash", Payload: []byte("original prompt"), State: models.DeliverySubmissionDispatching,
	}); err != nil {
		t.Fatalf("persist SQL submission: %v", err)
	}
	if err := repo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{
		ID: "delivery-block-claim-conflict", SessionID: claim.SessionID, IncarnationID: claim.SessionID,
		ExpectedGeneration: 1, Reason: "unknown_prompt_outcome", State: models.RecoveryBlockOpen,
		ConsumerReference: "agent_delivery", DeliverySubmissionID: submissionID,
	}); err != nil {
		t.Fatalf("persist recovery block: %v", err)
	}
	event := persistedTerminalEvent(claim.SessionID, claim.SessionID, 1, submissionID, dispatchAttemptID, "complete")
	projectOrchestratorDeliveryEvent(t, repo, ctx, event)

	if err := service.reconcileAgentDeliverySettlements(ctx, claim.SessionID); !errors.Is(err, messagequeue.ErrQueueDispatchClaimChanged) {
		t.Fatalf("reconcile after changed queue claim = %v, want claim conflict", err)
	}
	effect, err := repo.GetAgentDeliveryEffect(ctx, "agent_delivery.terminal:"+event.StreamID+":1")
	if err != nil || effect.State != models.DeliveryEffectBlockResolved {
		t.Fatalf("settlement after queue conflict = %+v, %v; want pending outbox", effect, err)
	}
	pending, err := queue.ListPendingQueueDispatches(ctx)
	if err != nil || len(pending) != 1 {
		t.Fatalf("queue claims after conflict = %+v, %v; original claim must remain", pending, err)
	}

	claimConflictRepo.fail = false
	if err := service.reconcileAgentDeliverySettlements(ctx, claim.SessionID); err != nil {
		t.Fatalf("retry terminal settlement after queue claim is available: %v", err)
	}
	effect, err = repo.GetAgentDeliveryEffect(ctx, "agent_delivery.terminal:"+event.StreamID+":1")
	if err != nil || effect.State != models.DeliveryEffectCompleted {
		t.Fatalf("settlement after retry = %+v, %v; want completed outbox", effect, err)
	}
	pending, err = queue.ListPendingQueueDispatches(ctx)
	if err != nil || len(pending) != 0 {
		t.Fatalf("queue claims after retry = %+v, %v; want exact claim removed", pending, err)
	}
}

type changedOnceQueueDispatchRepository struct {
	messagequeue.Repository
	bindPending   func(context.Context, *messagequeue.QueuedMessage, string, string, string) error
	deletePending func(context.Context, *messagequeue.QueuedMessage) error
	listPending   func(context.Context) ([]messagequeue.PendingQueueDispatch, error)
	markAccepted  func(context.Context, *messagequeue.QueuedMessage) error
	fail          bool
}

func (r *changedOnceQueueDispatchRepository) SetPendingQueueDispatchDelivery(
	ctx context.Context,
	claim *messagequeue.QueuedMessage,
	protocol, submissionID, payloadHash string,
) error {
	return r.bindPending(ctx, claim, protocol, submissionID, payloadHash)
}

func (r *changedOnceQueueDispatchRepository) ListPendingQueueDispatches(
	ctx context.Context,
) ([]messagequeue.PendingQueueDispatch, error) {
	return r.listPending(ctx)
}

func (r *changedOnceQueueDispatchRepository) MarkPendingQueueDispatchAccepted(
	ctx context.Context,
	claim *messagequeue.QueuedMessage,
) error {
	return r.markAccepted(ctx, claim)
}

func (r *changedOnceQueueDispatchRepository) DeletePendingQueueDispatch(
	ctx context.Context,
	claim *messagequeue.QueuedMessage,
) error {
	if r.fail {
		return messagequeue.ErrQueueDispatchClaimChanged
	}
	return r.deletePending(ctx, claim)
}

func TestDeliveryTerminalSettlesMatchingSendNowClaim(t *testing.T) {
	ctx := context.Background()
	service, repo := newServiceWithRealRepo(t)
	queue, queueDB, identity, _ := newIdentityWorkflowTransferQueue(t, t.TempDir()+"/queue.db", true)
	t.Cleanup(func() { _ = queueDB.Close() })
	seedSession(t, repo, identity.TaskID, identity.SessionID, "step-1")
	service.messageQueue = queue

	source, err := queue.QueueMessageWithMetadataForSession(
		ctx, identity, "send now prompt", "", messagequeue.QueuedByUser, false, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := queue.ClaimSendNowForSession(ctx, identity, []messagequeue.QueuedMessage{*source})
	if err != nil {
		t.Fatal(err)
	}
	_, dispatchAttemptID, _ := claim.DeliverySubmission()
	if err := queue.SetPendingSendNowClaimDelivery(ctx, claim, messagequeue.DeliveryProtocolV1, dispatchAttemptID, "send-now-hash"); err != nil {
		t.Fatalf("bind durable Send Now claim: %v", err)
	}
	submissionID := normalizeDeliverySubmissionID(dispatchAttemptID)
	if _, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: submissionID, SessionID: source.SessionID, IncarnationID: identity.SessionIncarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, DispatchAttemptID: dispatchAttemptID,
		PayloadHash: "send-now-hash", Payload: []byte("send now prompt"), State: models.DeliverySubmissionDispatching,
	}); err != nil {
		t.Fatalf("persist Send Now SQL submission: %v", err)
	}
	if err := repo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{
		ID: "delivery-block-send-now", SessionID: source.SessionID, IncarnationID: identity.SessionIncarnationID,
		ExpectedGeneration: 1, Reason: "unknown_prompt_outcome", State: models.RecoveryBlockOpen,
		ConsumerReference: "agent_delivery", DeliverySubmissionID: submissionID,
	}); err != nil {
		t.Fatalf("persist Send Now recovery block: %v", err)
	}
	event := persistedTerminalEvent(source.SessionID, identity.SessionIncarnationID, 1, submissionID, dispatchAttemptID, "complete")
	projectOrchestratorDeliveryEvent(t, repo, ctx, event)

	if err := service.reconcileAgentDeliverySettlements(ctx, source.SessionID); err != nil {
		t.Fatalf("settle Send Now terminal outcome: %v", err)
	}
	claims, err := queue.ListPendingSendNowClaims(ctx)
	if err != nil || len(claims) != 0 {
		t.Fatalf("pending Send Now claims after terminal settlement = %+v, %v; want none", claims, err)
	}
	if status := queue.GetStatus(ctx, source.SessionID); status.Count != 0 {
		t.Fatalf("settled Send Now prompt became visible for resend: %+v", status.Entries)
	}
}

func TestDeliveryRecoveryBlocksKeepIndependentCausesAndSubmissionBinding(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-delivery-blocks", "session-delivery-blocks", "step-1")
	session, err := repo.GetTaskSession(ctx, "session-delivery-blocks")
	if err != nil {
		t.Fatal(err)
	}
	incarnationID := session.QueueIncarnationID
	if incarnationID == "" {
		incarnationID = session.ID
	}
	if err := repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: session.ID, IncarnationID: incarnationID, Generation: 1,
		NativeSessionID: "native-delivery-blocks", CreationReason: "test",
	}); err != nil {
		t.Fatalf("create current harness generation: %v", err)
	}
	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	nativeErr := &sessionRecoveryRequiredError{Block: &models.SessionRecoveryBlock{
		ID: "native-block", Reason: "native_state_missing",
	}}
	if err := service.recordSessionRecoveryBlock(ctx, "session-delivery-blocks", "operator", nativeErr); err != nil {
		t.Fatalf("record independent native cause: %v", err)
	}
	if _, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: "prompt:attempt-bound", SessionID: "session-delivery-blocks", IncarnationID: incarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, DispatchAttemptID: "attempt-bound",
		PayloadHash: "bound-hash", Payload: []byte("prompt"), State: models.DeliverySubmissionInterruptedUnknown,
	}); err != nil {
		t.Fatalf("persist exact submission: %v", err)
	}
	deliveryErr := service.deliveryRecoveryError(ctx, "session-delivery-blocks", "unknown_prompt_outcome", &agentDeliverySubmissionRuntime{
		id: "prompt:attempt-bound",
	})
	var persistedDeliveryErr *sessionRecoveryRequiredError
	if !errors.As(deliveryErr, &persistedDeliveryErr) {
		t.Fatalf("delivery recovery error = %v, want typed recovery error", deliveryErr)
	}
	if persistedDeliveryErr.Block.ID == "" {
		t.Fatal("delivery recovery error lost the persisted block identity")
	}
	event := persistedTerminalEvent(
		"session-delivery-blocks", incarnationID, 1,
		"prompt:attempt-bound", "stream-independent-causes", "complete",
	)
	projectOrchestratorDeliveryEvent(t, repo, ctx, event)
	if err := service.reconcileAgentDeliverySettlements(ctx, event.SessionID); err != nil {
		t.Fatalf("settle only delivery recovery cause: %v", err)
	}
	nativeBlock, err := repo.GetSessionRecoveryBlock(ctx, nativeErr.Block.ID)
	if err != nil || nativeBlock.State != models.RecoveryBlockOpen {
		t.Fatalf("native block after independent delivery error = %+v, %v", nativeBlock, err)
	}
	deliveryBlock, err := repo.GetSessionRecoveryBlock(ctx, persistedDeliveryErr.Block.ID)
	if err != nil || deliveryBlock.State != models.RecoveryBlockResolved || deliveryBlock.DeliverySubmissionID != "prompt:attempt-bound" {
		t.Fatalf("delivery recovery block = %+v, %v; want only the bound delivery block resolved", deliveryBlock, err)
	}
	if err := service.checkSessionRecoveryBlock(ctx, "session-delivery-blocks"); !errors.Is(err, ErrSessionRecoveryRequired) {
		t.Fatalf("recovery admission after delivery settlement = %v; want independent native cause to keep work blocked", err)
	}
}

func persistedTerminalEvent(
	sessionID, incarnationID string,
	generation int64,
	submissionID, streamID, eventType string,
) models.AgentDeliveryEvent {
	payload, _ := json.Marshal(map[string]string{"type": eventType, "turn_id": "turn-" + submissionID})
	return models.AgentDeliveryEvent{
		SessionID: sessionID, IncarnationID: incarnationID, HarnessGeneration: generation,
		StreamID: streamID, Sequence: 1, SubmissionID: submissionID,
		EventType: eventType, Payload: payload, Terminal: true, ReceivedAt: time.Now().UTC(),
	}
}

func projectOrchestratorDeliveryEvent(
	t *testing.T,
	repo *tasksqlite.Repository,
	ctx context.Context,
	event models.AgentDeliveryEvent,
) {
	t.Helper()
	if _, err := repo.ReceiveAgentDeliveryEvent(ctx, &event, event.Sequence); err != nil {
		t.Fatalf("receive terminal delivery event: %v", err)
	}
	if _, err := repo.ProjectAgentDeliveryEvent(ctx, &event, nil); err != nil {
		t.Fatalf("project terminal delivery event: %v", err)
	}
}
