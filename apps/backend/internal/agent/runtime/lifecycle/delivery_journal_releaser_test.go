package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// TestNoDirectJournalReleaseCalls fails if production code outside the
// releaser asks agentctl to discard retained journal data.
func TestNoDirectJournalReleaseCalls(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	forbidden := map[string]struct{}{
		"AcknowledgeDelivery":                 {},
		"RetireDeliverySubmission":            {},
		"RetireInterruptedDeliverySubmission": {},
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") ||
			name == "delivery_journal_releaser.go" {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if _, ok := forbidden[selector.Sel.Name]; !ok {
				return true
			}
			t.Errorf("%s calls %s directly; use deliveryJournalReleaser",
				fset.Position(call.Pos()), selector.Sel.Name)
			return true
		})
	}
}

type recordingJournalReleaseClient struct {
	calls []string
	err   error
}

func (c *recordingJournalReleaseClient) record(name string) error {
	c.calls = append(c.calls, name)
	return c.err
}

func (c *recordingJournalReleaseClient) AcknowledgeDelivery(context.Context, string, uint64) error {
	return c.record("ack")
}

func (c *recordingJournalReleaseClient) RetireDeliverySubmission(context.Context, string) error {
	return c.record("retire")
}

func (c *recordingJournalReleaseClient) RetireInterruptedDeliverySubmission(context.Context, string, string, uint64) error {
	return c.record("retire_interrupted")
}

type journalReleaseCase struct {
	name    string
	release func(deliveryJournalReleaser, *recordingJournalReleaseClient) error
}

func journalReleaseCases() []journalReleaseCase {
	return []journalReleaseCase{
		{"ack", func(r deliveryJournalReleaser, c *recordingJournalReleaseClient) error {
			return r.acknowledge(context.Background(), c, "stream-1", 3)
		}},
		{"duplicate ack", func(r deliveryJournalReleaser, c *recordingJournalReleaseClient) error {
			return r.acknowledge(context.Background(), c, "stream-1", 3)
		}},
		{"retention ack", func(r deliveryJournalReleaser, c *recordingJournalReleaseClient) error {
			return r.acknowledge(context.Background(), c, "stream-1", 9)
		}},
		{"retire", func(r deliveryJournalReleaser, c *recordingJournalReleaseClient) error {
			return r.retire(context.Background(), c, "submission-1")
		}},
		{"retire interrupted", func(r deliveryJournalReleaser, c *recordingJournalReleaseClient) error {
			return r.retireInterrupted(context.Background(), c, "submission-1", "stream-1", 2)
		}},
	}
}

// @covers AC-PLATFORM-DELIVERY-WRITE-COST-001.3
func TestJournalReleaseAwaitsDurability(t *testing.T) {
	for _, tc := range journalReleaseCases() {
		t.Run(tc.name, func(t *testing.T) {
			order := make([]string, 0, 2)
			client := &recordingJournalReleaseClient{}
			releaser := deliveryJournalReleaser{durability: func(context.Context) error {
				order = append(order, "barrier")
				return nil
			}}
			if err := tc.release(releaser, client); err != nil {
				t.Fatalf("release: %v", err)
			}
			order = append(order, "release:"+client.calls[0])
			want := "barrier release:" + client.calls[0]
			if got := strings.Join(order, " "); got != want {
				t.Fatalf("order = %q, want %q", got, want)
			}
		})
	}
}

// @covers AC-PLATFORM-DELIVERY-WRITE-COST-001.3
func TestJournalReleaseBarrierFailureRetainsProgress(t *testing.T) {
	barrierErr := errors.New("fsync failed")
	for _, tc := range journalReleaseCases() {
		t.Run(tc.name, func(t *testing.T) {
			client := &recordingJournalReleaseClient{}
			releaser := deliveryJournalReleaser{durability: func(context.Context) error {
				return barrierErr
			}}
			err := tc.release(releaser, client)
			if !errors.Is(err, errDeliveryDurabilityBarrier) || !errors.Is(err, barrierErr) {
				t.Fatalf("error = %v, want a durability barrier error", err)
			}
			if len(client.calls) != 0 {
				t.Fatalf("release requests = %v, want none after a barrier failure", client.calls)
			}
		})
	}

	worker := newDurableDeliveryAckWorker(func(context.Context, uint64) error {
		return fmt.Errorf("%w: %w", errDeliveryDurabilityBarrier, barrierErr)
	})
	worker.schedule(5, false)
	worker.cancel()
	if worker.sendPending(5) {
		t.Fatal("ACK worker retried after owner cancellation")
	}
	worker.mu.Lock()
	pending, acknowledged := worker.pending, worker.acknowledged
	worker.mu.Unlock()
	if pending != 5 || acknowledged != 0 {
		t.Fatalf("pending=%d acknowledged=%d, want the pending cursor retained", pending, acknowledged)
	}
	if reason := deliveryAckFailureReason(fmt.Errorf("%w: %w", errDeliveryDurabilityBarrier, barrierErr)); reason != "durability_barrier" {
		t.Fatalf("failure reason = %q, want durability_barrier", reason)
	}
}

type combinedBatchRepository struct {
	recordingAgentDeliveryRepository
	combinedCalls int
	combinedErr   error
	order         *[]string
	alreadyForced bool
	receiveErr    error
	receiveCalls  int
}

func (r *combinedBatchRepository) ReceiveAndProjectCanonicalAgentDeliveryEvents(
	_ context.Context,
	events []*models.AgentDeliveryEvent,
	_ []*models.AgentDeliveryEffect,
	_ int64,
) ([]models.CanonicalAgentDeliveryProjectionResult, error) {
	r.combinedCalls++
	if r.order != nil {
		*r.order = append(*r.order, "combined")
	}
	if r.combinedErr != nil {
		return nil, r.combinedErr
	}
	r.projected = append(r.projected, events...)
	results := make([]models.CanonicalAgentDeliveryProjectionResult, len(events))
	for index := range events {
		results[index] = models.CanonicalAgentDeliveryProjectionResult{
			CanonicalMessageID: "projected-message",
			Appended:           index > 0,
			AlreadyProjected:   r.alreadyForced,
		}
	}
	return results, nil
}

func (r *combinedBatchRepository) ProjectCanonicalAgentDeliveryEvent(
	_ context.Context,
	event *models.AgentDeliveryEvent,
	_ *models.AgentDeliveryEffect,
) (models.CanonicalAgentDeliveryProjectionResult, error) {
	r.projected = append(r.projected, event)
	return models.CanonicalAgentDeliveryProjectionResult{CanonicalMessageID: "projected-message"}, nil
}

func (r *combinedBatchRepository) ReceiveAgentDeliveryEvent(
	ctx context.Context,
	event *models.AgentDeliveryEvent,
	remoteHighWater int64,
) (bool, error) {
	r.receiveCalls++
	if r.receiveErr != nil {
		return false, r.receiveErr
	}
	return r.recordingAgentDeliveryRepository.ReceiveAgentDeliveryEvent(ctx, event, remoteHighWater)
}

type barrierRecordingDeliveryRepository struct {
	combinedBatchRepository
	barrierCalls int
	barrierErr   error
	order        *[]string
}

func (r *barrierRecordingDeliveryRepository) AwaitAgentDeliveryDurability(context.Context) error {
	r.barrierCalls++
	if r.order != nil {
		*r.order = append(*r.order, "barrier")
	}
	return r.barrierErr
}

// @covers AC-PLATFORM-DELIVERY-WRITE-COST-001.3
func TestDeliveryAckSenderAwaitsDurability(t *testing.T) {
	order := make([]string, 0, 2)
	repository := &barrierRecordingDeliveryRepository{order: &order}
	ack := &recordingAgentDeliveryAcknowledger{onAcknowledge: func() {
		order = append(order, "ack")
	}}
	events := combinedProcessorEvents()
	execution, _, _ := deliveryProjectionFixture(t, events[1], ack)
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	defer sm.Wait()
	sm.setAgentDeliveryRepository(repository)

	if err := sm.deliveryAckSender(execution, "stream-1")(context.Background(), 2); err != nil {
		t.Fatalf("send ACK: %v", err)
	}
	if repository.barrierCalls != 1 {
		t.Fatalf("barrier calls = %d, want 1", repository.barrierCalls)
	}
	if got := strings.Join(order, " "); got != "barrier ack" {
		t.Fatalf("order = %q, want barrier before ack", got)
	}

	order = order[:0]
	repository.barrierErr = errors.New("fsync failed")
	err := sm.deliveryAckSender(execution, "stream-1")(context.Background(), 2)
	if !errors.Is(err, errDeliveryDurabilityBarrier) {
		t.Fatalf("error = %v, want a durability barrier error", err)
	}
	if got := strings.Join(order, " "); got != "barrier" {
		t.Fatalf("order = %q, want no ACK after a barrier failure", got)
	}
}

// @covers AC-PLATFORM-DELIVERY-WRITE-COST-001.2
func TestStreamProcessorBatchUsesOneWriterCommit(t *testing.T) {
	order := make([]string, 0, 2)
	repository := &combinedBatchRepository{order: &order}
	ack := &recordingAgentDeliveryAcknowledger{
		onAcknowledge: func() { order = append(order, "ack") },
	}
	events := combinedProcessorEvents()
	execution, client, acked := deliveryProjectionFixture(t, events[1], ack)

	sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	defer sm.Wait()
	processor := newStreamEventProcessor(sm, context.Background(), execution, client, repository, 0)
	for _, event := range events {
		if !processor.enqueue(event) {
			t.Fatal("processor rejected an event")
		}
	}
	if err := processor.close(); err != nil {
		t.Fatalf("process batch: %v", err)
	}
	select {
	case <-acked:
	case <-time.After(3 * time.Second):
		t.Fatal("batch was not acknowledged")
	}
	if repository.combinedCalls != 1 {
		t.Fatalf("combined writer transactions = %d, want 1", repository.combinedCalls)
	}
	if repository.receiveCalls != 0 {
		t.Fatalf("individual receipts = %d, want 0", repository.receiveCalls)
	}
	if got := strings.Join(order, " "); got != "combined ack" {
		t.Fatalf("order = %q, want combined before ack", got)
	}
}

func combinedProcessorEvents() []agentctl.AgentEvent {
	base := agentctl.AgentEvent{
		Type: streams.EventTypeMessageChunk, DeliveryStreamID: "stream-1",
		DeliveryIncarnationID: "owner", DeliveryHarnessGeneration: 1, ProtocolMessageID: "message-1",
	}
	first := base
	first.Text, first.DeliverySequence = "first", 1
	second := base
	second.Text, second.DeliverySequence = "second", 2
	return []agentctl.AgentEvent{first, second}
}

// @covers AC-PLATFORM-DELIVERY-WRITE-COST-001.2
func TestCombinedBatchDuplicateDoesNotNotify(t *testing.T) {
	repository := &combinedBatchRepository{alreadyForced: true}
	events := combinedProcessorEvents()
	execution := &AgentExecution{SessionID: "session-1"}
	prepared := make([]preparedAgentEvent, len(events))
	for index, event := range events {
		prepared[index] = preparedAgentEvent{
			event:           event,
			durableEvent:    durableAgentEvent(execution, event),
			receiptDeferred: true,
		}
	}
	var notifications int
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{
		OnAgentEvent: func(*AgentExecution, agentctl.AgentEvent) { notifications++ },
	}, nil, nil)
	if err := sm.commitPreparedAgentEvents(context.Background(), execution, prepared, repository, nil, 0); err != nil {
		t.Fatalf("commit duplicate combined batch: %v", err)
	}
	if repository.combinedCalls != 1 {
		t.Fatalf("combined calls = %d, want 1", repository.combinedCalls)
	}
	if notifications != 0 {
		t.Fatalf("notifications = %d, want none for already-projected events", notifications)
	}
}

// @covers AC-PLATFORM-DELIVERY-WRITE-COST-001.2
func TestCombinedBatchAdmissionFailureFallsBack(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"replay expiry", repoerrors.ErrAgentDeliveryReplayExpired},
		{"owner mismatch", repoerrors.ErrAgentDeliveryIdentityMismatch},
		{"sequence gap", repoerrors.ErrAgentDeliveryCheckpointMissing},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository := &combinedBatchRepository{combinedErr: tc.err, receiveErr: tc.err}
			events := combinedProcessorEvents()
			execution, client, _ := deliveryProjectionFixture(t, events[1], &recordingAgentDeliveryAcknowledger{})
			if tc.err == repoerrors.ErrAgentDeliveryReplayExpired {
				execution.DeliveryReplayCursor = 0
				repository.cursor = &models.AgentDeliveryCursor{
					StreamID: "stream-1", SessionID: "session-1", IncarnationID: "owner",
					HarnessGeneration: 1, ReceivedSequence: 1, ProjectedSequence: 1, RetiredSequence: 1,
				}
			}
			sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
			prepared := make([]preparedAgentEvent, len(events))
			for index, event := range events {
				prepared[index] = preparedAgentEvent{
					event:           event,
					durableEvent:    durableAgentEvent(execution, event),
					receiptDeferred: true,
				}
			}
			err := sm.commitPreparedAgentEvents(context.Background(), execution, prepared, repository, client, 0)
			if tc.err == repoerrors.ErrAgentDeliveryReplayExpired {
				if !errors.Is(err, errDeliveryReplayReconciled) {
					t.Fatalf("error = %v, want the existing reconciliation outcome", err)
				}
				if execution.DeliveryReplayCursor != 1 {
					t.Fatalf("replay cursor = %d, want 1", execution.DeliveryReplayCursor)
				}
			} else if !errors.Is(err, tc.err) {
				t.Fatalf("error = %v, want %v", err, tc.err)
			}
			if repository.combinedCalls != 1 {
				t.Fatalf("combined calls = %d, want 1", repository.combinedCalls)
			}
			if repository.receiveCalls == 0 {
				t.Fatal("combined failure did not replay through the two-phase path")
			}
		})
	}
}

// @covers AC-PLATFORM-DELIVERY-WRITE-COST-001.2
func TestStreamProcessorCombinedBatchWithSQLiteRepository(t *testing.T) {
	ctx := context.Background()
	database, err := db.OpenSQLite(filepath.Join(t.TempDir(), "combined-live.db"))
	if err != nil {
		t.Fatal(err)
	}
	writer := sqlx.NewDb(database, "sqlite3")
	t.Cleanup(func() { _ = writer.Close() })
	repository, err := tasksqlite.NewWithDB(writer, writer, nil)
	if err != nil {
		t.Fatal(err)
	}
	execution := &AgentExecution{
		ID: "execution-combined-live", TaskID: "task-combined-live", SessionID: "session-combined-live",
		DeliveryMode: DurableDeliveryV1, DeliveryStreamID: "stream-combined-live",
		DeliveryIncarnationID: "incarnation-combined-live", DeliveryHarnessGeneration: 1,
	}
	if err := repository.CreateTask(ctx, &models.Task{ID: execution.TaskID, Title: "Combined live"}); err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateTaskSession(ctx, &models.TaskSession{
		ID: execution.SessionID, TaskID: execution.TaskID,
		QueueIncarnationID: execution.DeliveryIncarnationID, State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repository.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "running-combined-live", SessionID: execution.SessionID, TaskID: execution.TaskID,
		ExecutorID: "executor-combined-live", AgentExecutionID: execution.ID, Status: "running",
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if stored, err := repository.CommitHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: execution.SessionID, IncarnationID: execution.DeliveryIncarnationID,
		Generation: 1, NativeSessionID: "native-combined-live", CreationReason: "initial",
		CreatedAt: now, CommittedAt: now,
	}, 0); err != nil || !stored {
		t.Fatalf("harness generation stored=%t err=%v", stored, err)
	}
	if err := repository.CreateTurn(ctx, &models.Turn{
		ID: "turn-combined-live", TaskSessionID: execution.SessionID, TaskID: execution.TaskID,
		StartedAt: now, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	events := combinedProcessorEvents()
	for index := range events {
		events[index].DeliveryStreamID = execution.DeliveryStreamID
		events[index].DeliveryIncarnationID = execution.DeliveryIncarnationID
		events[index].DeliveryHarnessGeneration = execution.DeliveryHarnessGeneration
		events[index].CanonicalMessageBaseID = "message-combined-live"
		events[index].CanonicalMessageID = "message-combined-live"
	}
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	defer sm.Wait()
	processor := newStreamEventProcessor(sm, ctx, execution, nil, repository, 0)
	for _, event := range events {
		if !processor.enqueue(event) {
			t.Fatal("processor rejected an event")
		}
	}
	if err := processor.close(); err != nil {
		t.Fatalf("process combined batch: %v", err)
	}
	cursor, err := repository.GetAgentDeliveryCursor(ctx, execution.DeliveryStreamID)
	if err != nil {
		t.Fatalf("read cursor: %v", err)
	}
	if cursor.ReceivedSequence != 2 || cursor.ProjectedSequence != 2 {
		t.Fatalf("cursor = %+v, want received and projected at 2", cursor)
	}
	message, err := repository.GetMessage(ctx, "message-combined-live")
	if err != nil {
		t.Fatalf("read canonical message: %v", err)
	}
	if message.Content != "firstsecond" {
		t.Fatalf("message content = %q, want combined chunks", message.Content)
	}
}
