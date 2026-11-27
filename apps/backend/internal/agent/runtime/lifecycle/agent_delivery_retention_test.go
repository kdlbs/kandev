package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

var errRetentionAckResponseLost = errors.New("delivery ACK response lost")

func TestDeliveryRetentionAuthenticatedCursorRejectsMissingExecution(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("authenticated cursor panicked for missing execution: %v", recovered)
		}
	}()
	_, err := authenticatedDeliveryReplayCursor(context.Background(), nil, nil, &agentctlDeliveryStatusForRetentionTest)
	if err == nil {
		t.Fatal("authenticated cursor accepted a missing execution")
	}
}

func TestDeliveryRetentionStreamOwnerIdentity(t *testing.T) {
	execution := &AgentExecution{
		DeliveryMode: DurableDeliveryV1, DeliveryStreamID: "retention-stream",
		DeliveryIncarnationID: "retention-incarnation", DeliveryHarnessGeneration: 3,
	}
	event := agentctl.AgentEvent{
		DeliveryStreamID: "retention-stream", DeliveryIncarnationID: "retention-incarnation",
		DeliveryHarnessGeneration: 3, DeliverySequence: 1,
	}
	if durable, err := durableAgentEventIdentity(execution, event); err != nil || !durable {
		t.Fatalf("matching owner identity = %v, err %v", durable, err)
	}
	tests := []struct {
		name      string
		execution *AgentExecution
		event     agentctl.AgentEvent
	}{
		{name: "missing execution", event: event},
		{name: "wrong stream", execution: execution, event: agentctl.AgentEvent{
			DeliveryStreamID: "replaced-stream", DeliveryIncarnationID: event.DeliveryIncarnationID,
			DeliveryHarnessGeneration: event.DeliveryHarnessGeneration, DeliverySequence: event.DeliverySequence,
		}},
		{name: "wrong incarnation", execution: execution, event: agentctl.AgentEvent{
			DeliveryStreamID: event.DeliveryStreamID, DeliveryIncarnationID: "replaced-incarnation",
			DeliveryHarnessGeneration: event.DeliveryHarnessGeneration, DeliverySequence: event.DeliverySequence,
		}},
		{name: "wrong generation", execution: execution, event: agentctl.AgentEvent{
			DeliveryStreamID: event.DeliveryStreamID, DeliveryIncarnationID: event.DeliveryIncarnationID,
			DeliveryHarnessGeneration: 4, DeliverySequence: event.DeliverySequence,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if durable, err := durableAgentEventIdentity(tt.execution, tt.event); durable || !errors.Is(err, ErrDurableAdoptionIdentityMismatch) {
				t.Fatalf("mismatched owner identity = %v, err %v", durable, err)
			}
		})
	}
}

func TestDeliveryRetentionReplayRequiresCheckpointWhenPeerHasHistory(t *testing.T) {
	repository := &adoptionDeliveryRepository{}
	stream := &journal.Stream{HighWater: 5, FirstRetained: 1}
	if _, err := adoptionReplayCursor(context.Background(), repository, "stream-1", stream); !errors.Is(err, repoerrors.ErrAgentDeliveryCheckpointMissing) {
		t.Fatalf("missing SQL cursor error = %v, want missing-checkpoint error", err)
	}
}

func TestDeliveryRetentionLostAck(t *testing.T) {
	tests := []struct {
		name       string
		loseBefore bool
		loseAfter  bool
		preAck     bool
		wantCalls  int
	}{
		{name: "lost request is retried from projected cursor", loseBefore: true, wantCalls: 2},
		{name: "lost success response is discovered from peer state", loseAfter: true, wantCalls: 1},
		{name: "expired peer replay window is already acknowledged", preAck: true, wantCalls: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			const (
				sessionID     = "retention-lost-ack-session"
				incarnationID = "retention-lost-ack-incarnation"
				streamID      = "retention-lost-ack-stream"
			)
			databasePath := filepath.Join(t.TempDir(), "delivery.db")
			journalPath := filepath.Join(t.TempDir(), "delivery.bbolt")
			repository, database := openRetentionLifecycleRepository(t, databasePath)
			if err := repository.CreateWorkspace(ctx, &models.Workspace{ID: "lost-ack-workspace", Name: "Lost acknowledgement"}); err != nil {
				t.Fatal(err)
			}
			if err := repository.CreateTask(ctx, &models.Task{ID: "lost-ack-task", WorkspaceID: "lost-ack-workspace", Title: "Lost acknowledgement"}); err != nil {
				t.Fatal(err)
			}
			if err := repository.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: "lost-ack-task", QueueIncarnationID: incarnationID}); err != nil {
				t.Fatal(err)
			}
			retainedJournal := openRetentionLifecycleJournal(t, journalPath)
			wire := streams.AgentEvent{
				Type: streams.EventTypeMessageChunk, Text: "projected once",
				DeliveryStreamID: streamID, DeliveryIncarnationID: incarnationID,
				DeliveryHarnessGeneration: 2, DeliverySequence: 1,
			}
			payload, err := json.Marshal(wire)
			if err != nil {
				t.Fatal(err)
			}
			event := &models.AgentDeliveryEvent{
				SessionID: sessionID, IncarnationID: incarnationID, HarnessGeneration: 2,
				StreamID: streamID, Sequence: 1, EventType: wire.Type, Payload: payload,
			}
			if _, err := retainedJournal.Append(ctx, journal.Event{
				SessionID: sessionID, IncarnationID: incarnationID, HarnessGeneration: 2,
				StreamID: streamID, Type: wire.Type, Payload: payload,
			}); err != nil {
				t.Fatalf("append retained event: %v", err)
			}
			if inserted, err := repository.ReceiveAgentDeliveryEvent(ctx, event, 1); err != nil || !inserted {
				t.Fatalf("receive event = %v, err %v", inserted, err)
			}
			if _, err := repository.ProjectAgentDeliveryEvent(ctx, event, nil); err != nil {
				t.Fatalf("project event: %v", err)
			}
			if _, err := database.Exec(database.Rebind(`
				UPDATE agent_delivery_cursors SET retired_sequence = 1 WHERE stream_id = ?`), streamID); err != nil {
				t.Fatalf("retire projected receipt: %v", err)
			}
			if _, err := database.Exec(database.Rebind(`
				DELETE FROM agent_delivery_inbox WHERE stream_id = ? AND sequence = 1`), streamID); err != nil {
				t.Fatalf("remove retired receipt: %v", err)
			}
			if err := database.Close(); err != nil {
				t.Fatalf("close delivery database: %v", err)
			}
			if err := retainedJournal.Close(); err != nil {
				t.Fatalf("close retained journal: %v", err)
			}

			repository, _ = openRetentionLifecycleRepository(t, databasePath)
			retainedJournal = openRetentionLifecycleJournal(t, journalPath)
			peer := &retentionLifecycleJournalPeer{
				journal: retainedJournal, sessionID: sessionID, incarnationID: incarnationID,
				harnessGeneration: 2, loseBefore: tt.loseBefore, loseAfter: tt.loseAfter,
			}
			if tt.preAck {
				if err := retainedJournal.Acknowledge(ctx, streamID, 1); err != nil {
					t.Fatalf("pre-acknowledge retained event: %v", err)
				}
			}
			execution := &AgentExecution{
				SessionID: sessionID, DeliveryStreamID: streamID,
				DeliveryMode: DurableDeliveryV1, DeliveryIncarnationID: incarnationID,
				DeliveryHarnessGeneration: 2,
			}
			if tt.loseBefore || tt.loseAfter {
				if _, err := reconcileExpiredAgentDeliveryReplay(ctx, execution, event, repository, peer, peer, nil); !errors.Is(err, errRetentionAckResponseLost) {
					t.Fatalf("first lost-ACK recovery error = %v", err)
				}
			}
			if _, err := reconcileExpiredAgentDeliveryReplay(ctx, execution, event, repository, peer, peer, nil); err != nil {
				t.Fatalf("recover projected cursor: %v", err)
			}
			if peer.ackCalls != tt.wantCalls {
				t.Fatalf("ACK calls = %d, want %d", peer.ackCalls, tt.wantCalls)
			}
			cursor, err := repository.GetAgentDeliveryCursor(ctx, streamID)
			if err != nil || cursor.ProjectedSequence != 1 || cursor.RetiredSequence != 1 {
				t.Fatalf("local cursor after recovery = %+v, err %v", cursor, err)
			}
			replayed, remote, err := retainedJournal.Replay(ctx, streamID, 0, 10)
			if !errors.Is(err, journal.ErrCursorExpired) || len(replayed) != 0 || remote.Acknowledged != 1 {
				t.Fatalf("remote replay = %d events, stream %+v, err %v", len(replayed), remote, err)
			}
		})
	}
}

func TestDeliveryRetentionNoResend(t *testing.T) {
	ctx := context.Background()
	const (
		sessionID     = "retention-no-resend-session"
		incarnationID = "retention-no-resend-incarnation"
		streamID      = "retention-no-resend-stream"
	)
	databasePath := filepath.Join(t.TempDir(), "delivery.db")
	journalPath := filepath.Join(t.TempDir(), "delivery.bbolt")
	repository, database := openRetentionLifecycleRepository(t, databasePath)
	retainedJournal := openRetentionLifecycleJournal(t, journalPath)
	const taskID, executionID, workspaceID = "retention-no-resend-task", "retention-no-resend-execution", "retention-no-resend-workspace"
	if err := repository.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Retention"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repository.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: "No resend"}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repository.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, QueueIncarnationID: incarnationID,
	}); err != nil {
		t.Fatalf("create task session: %v", err)
	}
	if err := repository.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: sessionID, SessionID: sessionID, TaskID: taskID,
		AgentExecutionID: executionID, Status: models.ExecutorRunningStatusRunning,
	}); err != nil {
		t.Fatalf("create execution owner: %v", err)
	}
	now := time.Now().UTC()
	if committed, err := repository.CommitHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: sessionID, IncarnationID: incarnationID, Generation: 2,
		NativeSessionID: "retention-no-resend-native", CreationReason: "initial",
		CreatedAt: now, CommittedAt: now,
	}, 0); err != nil || !committed {
		t.Fatalf("commit harness generation = %v, err %v", committed, err)
	}
	wire := agentctl.AgentEvent{
		Type:             streams.EventTypeComplete,
		DeliveryStreamID: streamID, DeliveryIncarnationID: incarnationID,
		DeliveryHarnessGeneration: 2, DeliverySequence: 1,
	}
	payload, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	event := &models.AgentDeliveryEvent{
		SessionID: sessionID, IncarnationID: incarnationID, HarnessGeneration: 2,
		TaskID: taskID, ExecutionID: executionID, OwnerGeneration: 2, ExpectedStreamID: streamID,
		StreamID: streamID, Sequence: 1, EventType: wire.Type, Terminal: true,
		Payload: payload,
	}
	if _, err := retainedJournal.Append(ctx, journal.Event{
		SessionID: sessionID, IncarnationID: incarnationID, HarnessGeneration: 2,
		StreamID: streamID, Type: wire.Type, Terminal: true, Payload: payload,
	}); err != nil {
		t.Fatalf("append terminal event: %v", err)
	}
	if inserted, err := repository.ReceiveAgentDeliveryEvent(ctx, event, 1); err != nil || !inserted {
		t.Fatalf("receive terminal event = %v, err %v", inserted, err)
	}
	if _, err := repository.ProjectAgentDeliveryEvent(ctx, event, nil); err != nil {
		t.Fatalf("project terminal event: %v", err)
	}
	if _, err := database.Exec(database.Rebind(`UPDATE agent_delivery_cursors SET retired_sequence = 1 WHERE stream_id = ?`), streamID); err != nil {
		t.Fatalf("retire projected terminal: %v", err)
	}
	if _, err := database.Exec(database.Rebind(`DELETE FROM agent_delivery_inbox WHERE stream_id = ?`), streamID); err != nil {
		t.Fatalf("remove retired terminal: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if err := retainedJournal.Close(); err != nil {
		t.Fatal(err)
	}
	repository, database = openRetentionLifecycleRepository(t, databasePath)
	retainedJournal = openRetentionLifecycleJournal(t, journalPath)
	ackCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agent/delivery":
			status, err := retainedJournal.RecoveryDescriptor(ctx, sessionID, incarnationID, 2, streamID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(status)
		case "/api/v1/agent/delivery/stream/ack":
			var request struct {
				StreamID string `json:"stream_id"`
				Sequence uint64 `json:"sequence"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			ackCalls++
			if err := retainedJournal.Acknowledge(ctx, request.StreamID, request.Sequence); err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	host, port, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatalf("split test peer address: %v", err)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("parse test peer port: %v", err)
	}
	client := agentctl.NewClient(host, portNumber, newTestLogger())
	callbackCalls := 0
	streamManager := NewStreamManager(newTestLogger(), StreamCallbacks{
		OnAgentEvent: func(*AgentExecution, agentctl.AgentEvent) { callbackCalls++ },
	}, nil, nil)
	execution := &AgentExecution{
		ID: executionID, TaskID: taskID, SessionID: sessionID, DeliveryStreamID: streamID,
		DeliveryMode: DurableDeliveryV1, DeliveryIncarnationID: incarnationID,
		DeliveryHarnessGeneration: 2,
	}
	err = streamManager.processAgentEvent(ctx, execution, client, repository, wire, 0)
	if !errors.Is(err, errDeliveryReplayReconciled) {
		t.Fatalf("expired terminal replay error = %v, want state-only reconciliation", err)
	}
	if callbackCalls != 0 {
		t.Fatalf("agent callbacks = %d, want no replayed terminal callback", callbackCalls)
	}
	if ackCalls != 1 || execution.DeliveryReplayCursor != 1 {
		t.Fatalf("ACK calls=%d replay cursor=%d, want one ACK at SQL cursor 1", ackCalls, execution.DeliveryReplayCursor)
	}
	var effects int
	if err := database.QueryRow(`SELECT COUNT(*) FROM agent_delivery_effects`).Scan(&effects); err != nil {
		t.Fatal(err)
	}
	if effects != 0 {
		t.Fatalf("terminal effects after expired replay = %d, want none", effects)
	}
}

type retentionLifecycleJournalPeer struct {
	journal           *journal.Journal
	sessionID         string
	incarnationID     string
	harnessGeneration uint64
	loseBefore        bool
	loseAfter         bool
	ackCalls          int
}

func (p *retentionLifecycleJournalPeer) GetDeliveryStatus(ctx context.Context, streamID string) (*agentctl.DeliveryStatus, error) {
	status, err := p.journal.RecoveryDescriptor(ctx, p.sessionID, p.incarnationID, p.harnessGeneration, streamID)
	return &status, err
}

func (p *retentionLifecycleJournalPeer) AcknowledgeDelivery(ctx context.Context, streamID string, sequence uint64) error {
	p.ackCalls++
	if p.loseBefore && p.ackCalls == 1 {
		return errRetentionAckResponseLost
	}
	if err := p.journal.Acknowledge(ctx, streamID, sequence); err != nil {
		return err
	}
	if p.loseAfter && p.ackCalls == 1 {
		return errRetentionAckResponseLost
	}
	return nil
}

func openRetentionLifecycleRepository(t *testing.T, path string) (*tasksqlite.Repository, *sqlx.DB) {
	t.Helper()
	connection, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	database := sqlx.NewDb(connection, "sqlite3")
	repository, err := tasksqlite.NewWithDB(database, database, nil)
	if err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return repository, database
}

func openRetentionLifecycleJournal(t *testing.T, path string) *journal.Journal {
	t.Helper()
	retainedJournal, err := journal.Open(journal.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = retainedJournal.Close() })
	return retainedJournal
}

var agentctlDeliveryStatusForRetentionTest = agentctl.DeliveryStatus{
	StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Durable: true},
	SessionID:         "session-1",
	IncarnationID:     "incarnation-1",
	HarnessGeneration: 2,
	StreamID:          "stream-1",
	Stream: &journal.Stream{
		SessionID: "session-1", IncarnationID: "incarnation-1", HarnessGeneration: 2,
		StreamID: "stream-1", HighWater: 5, FirstRetained: 1,
	},
}
