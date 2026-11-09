package lifecycle

import (
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/api"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

const (
	freshStartRecoveryTask        = "fresh-start-task"
	freshStartRecoverySession     = "fresh-start-session"
	freshStartRecoveryExecution   = "fresh-start-execution"
	freshStartRecoveryIncarnation = "fresh-start-incarnation"
	freshStartRecoveryStreamOne   = freshStartRecoveryIncarnation + ":g1"
	freshStartRecoveryStreamTwo   = freshStartRecoveryIncarnation + ":g2"
	freshStartRecoveryNativeOne   = "predecessor-native"
	freshStartRecoveryNativeTwo   = "replacement-native"
)

// freshStartRecoveryFixture retains an uncommitted successor stream next to a
// projected predecessor. The journal is reopened by a replacement manager.
type freshStartRecoveryFixture struct {
	repo          *tasksqlite.Repository
	db            *sqlx.DB
	cfg           *config.InstanceConfig
	predecessor   []*models.Message
	predecessorID *models.AgentDeliveryCursor
}

func newFreshStartRecoveryFixture(t *testing.T) *freshStartRecoveryFixture {
	t.Helper()
	ctx := context.Background()
	repo, database := runOwnerTestRepository(t)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: freshStartRecoveryTask, Title: "Fresh start recovery"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: freshStartRecoverySession, TaskID: freshStartRecoveryTask, QueueIncarnationID: freshStartRecoveryIncarnation,
	}))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: freshStartRecoverySession, SessionID: freshStartRecoverySession, TaskID: freshStartRecoveryTask,
		AgentExecutionID: freshStartRecoveryExecution, ResumeToken: freshStartRecoveryNativeTwo,
		Status: models.ExecutorRunningStatusRunning,
	}))
	committed, err := repo.CommitHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: freshStartRecoverySession, IncarnationID: freshStartRecoveryIncarnation, Generation: 1,
		NativeSessionID: freshStartRecoveryNativeOne, CreationReason: "native_started",
	}, 0)
	require.NoError(t, err)
	require.True(t, committed)

	predecessorPayload, err := json.Marshal(agentctl.AgentEvent{Type: streams.EventTypeMessageChunk, Text: "predecessor answer"})
	require.NoError(t, err)
	predecessorEvent := &models.AgentDeliveryEvent{
		SessionID: freshStartRecoverySession, IncarnationID: freshStartRecoveryIncarnation,
		HarnessGeneration: 1, StreamID: freshStartRecoveryStreamOne, Sequence: 1,
		EventType: streams.EventTypeMessageChunk, Payload: predecessorPayload,
		TaskID: freshStartRecoveryTask, ExecutionID: freshStartRecoveryExecution,
		OwnerGeneration: 1, ExpectedStreamID: freshStartRecoveryStreamOne,
	}
	inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, predecessorEvent, 1)
	require.NoError(t, err)
	require.True(t, inserted)
	now := time.Now().UTC()
	_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, predecessorEvent, &models.AgentDeliveryEffect{
		EffectKey: "agent_delivery.event:" + freshStartRecoveryStreamOne + ":1",
		StreamID:  freshStartRecoveryStreamOne, Sequence: 1, EffectType: "agent_delivery.event",
		State: models.DeliveryEffectCompleted, CreatedAt: now, CompletedAt: &now,
	})
	require.NoError(t, err)
	predecessorMessages, err := repo.ListMessages(ctx, freshStartRecoverySession)
	require.NoError(t, err)
	require.NotEmpty(t, predecessorMessages, "the predecessor stream must be projected before recovery")
	predecessorCursor, err := repo.GetAgentDeliveryCursor(ctx, freshStartRecoveryStreamOne)
	require.NoError(t, err)

	cfg := &config.InstanceConfig{
		WorkDir: t.TempDir(), SessionID: freshStartRecoverySession,
		DeliveryIncarnationID: freshStartRecoveryIncarnation, DeliveryHarnessGeneration: 2,
		DeliveryStreamID: freshStartRecoveryStreamTwo, DurableJournalPath: filepath.Join(t.TempDir(), "delivery.bbolt"),
	}
	oldPeer := process.NewManager(cfg, newTestLogger())
	oldPeerStopped := false
	t.Cleanup(func() {
		if !oldPeerStopped {
			_ = oldPeer.StopForTeardown(context.Background())
		}
	})
	retained, err := oldPeer.DeliveryJournal()
	require.NoError(t, err)
	_, err = retained.Append(ctx, journal.Event{
		SessionID: freshStartRecoverySession, IncarnationID: freshStartRecoveryIncarnation,
		HarnessGeneration: 1, StreamID: freshStartRecoveryStreamOne,
		Type: streams.EventTypeMessageChunk, Payload: predecessorPayload,
	})
	require.NoError(t, err)
	require.NoError(t, retained.Acknowledge(ctx, freshStartRecoveryStreamOne, 1))
	_, err = retained.PutSubmission(ctx, journal.Submission{
		ID: "predecessor-submission", SessionID: freshStartRecoverySession,
		IncarnationID: freshStartRecoveryIncarnation, HarnessGeneration: 1,
		StreamID: freshStartRecoveryStreamOne, State: journal.SubmissionCompleted,
		TerminalEventRetained: true, TerminalSequence: 1,
		Hash: "predecessor-hash", Payload: []byte("historical instruction"), CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)

	oldManagerID := oldPeer.ProcessManagerID()
	setupEvent := agentctl.AgentEvent{Type: streams.EventTypeAgentCapabilities}
	setupPayload, err := json.Marshal(setupEvent)
	require.NoError(t, err)
	_, err = retained.Append(ctx, journal.Event{
		SessionID: freshStartRecoverySession, IncarnationID: freshStartRecoveryIncarnation,
		HarnessGeneration: 2, StreamID: freshStartRecoveryStreamTwo,
		Type: streams.EventTypeAgentCapabilities, Payload: setupPayload,
	})
	require.NoError(t, err)
	exitEvent := agentctl.AgentEvent{
		Type: streams.EventTypeError, Error: "predecessor process exited",
		Data: map[string]any{"process_generation": uint64(1), "process_manager_id": oldManagerID},
	}
	exitPayload, err := json.Marshal(exitEvent)
	require.NoError(t, err)
	_, err = retained.Append(ctx, journal.Event{
		SessionID: freshStartRecoverySession, IncarnationID: freshStartRecoveryIncarnation,
		HarnessGeneration: 2, StreamID: freshStartRecoveryStreamTwo,
		Type: streams.EventTypeError, Payload: exitPayload,
	})
	require.NoError(t, err)
	require.NoError(t, oldPeer.StopForTeardown(ctx))
	oldPeerStopped = true

	return &freshStartRecoveryFixture{
		repo: repo, db: database, cfg: cfg,
		predecessor: predecessorMessages, predecessorID: predecessorCursor,
	}
}

type freshStartDispatchCounter struct {
	next  http.Handler
	count atomic.Int64
}

func (c *freshStartDispatchCounter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/v1/agent/stream") ||
		(r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/agent/submissions")) {
		c.count.Add(1)
	}
	c.next.ServeHTTP(w, r)
}

type freshStartRecoveryServer struct {
	peer     *process.Manager
	retained *journal.Journal
	client   *agentctl.Client
	dispatch *freshStartDispatchCounter
}

func (f *freshStartRecoveryFixture) startServer(t *testing.T) *freshStartRecoveryServer {
	t.Helper()
	replacement := process.NewManager(f.cfg, newTestLogger())
	t.Cleanup(func() { require.NoError(t, replacement.StopForTeardown(context.Background())) })
	retained, err := replacement.DeliveryJournal()
	require.NoError(t, err)
	counter := &freshStartDispatchCounter{next: api.NewServer(f.cfg, replacement, nil, nil, newTestLogger()).Router()}
	server := httptest.NewServer(counter)
	t.Cleanup(server.Close)
	host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	client := agentctl.NewClient(host, port, newTestLogger())
	t.Cleanup(client.Close)
	return &freshStartRecoveryServer{peer: replacement, retained: retained, client: client, dispatch: counter}
}

func (f *freshStartRecoveryFixture) execution(server *freshStartRecoveryServer) *AgentExecution {
	execution := &AgentExecution{
		ID: freshStartRecoveryExecution, TaskID: freshStartRecoveryTask,
		SessionID: freshStartRecoverySession, DeliveryMode: DurableDeliveryV1,
		DeliveryIncarnationID:     freshStartRecoveryIncarnation,
		DeliveryHarnessGeneration: 2, DeliveryStreamID: freshStartRecoveryStreamTwo,
		ReplacesNativeConversation: true,
	}
	execution.agentctl = server.client
	return execution
}

func replayFreshStartEvents(t *testing.T, ctx context.Context, client *agentctl.Client) []agentctl.AgentEvent {
	t.Helper()
	committed, stream, err := client.ReplayDelivery(ctx, freshStartRecoveryStreamTwo, 0, 10)
	require.NoError(t, err)
	require.Equal(t, uint64(2), stream.HighWater)
	require.Len(t, committed, 2)
	events := make([]agentctl.AgentEvent, 0, len(committed))
	for _, retained := range committed {
		var event agentctl.AgentEvent
		require.NoError(t, json.Unmarshal(retained.Payload, &event))
		if event.Type == "" {
			event.Type = retained.Type
		}
		event.DeliveryStreamID = retained.StreamID
		event.DeliveryIncarnationID = retained.IncarnationID
		event.DeliveryHarnessGeneration = retained.HarnessGeneration
		event.DeliverySequence = retained.Sequence
		event.DeliverySubmissionID = retained.SubmissionID
		event.Recovered = true
		events = append(events, event)
	}
	return events
}

type freshStartRecoveryRepository struct {
	*tasksqlite.Repository
	admissionRead chan struct{}
	once          sync.Once
}

func (r *freshStartRecoveryRepository) GetCurrentHarnessSessionGeneration(
	ctx context.Context, sessionID, incarnationID string,
) (*models.HarnessSessionGeneration, error) {
	r.once.Do(func() { close(r.admissionRead) })
	return r.Repository.GetCurrentHarnessSessionGeneration(ctx, sessionID, incarnationID)
}

func requireNoFreshStartCursor(t *testing.T, repo *tasksqlite.Repository) {
	t.Helper()
	_, err := repo.GetAgentDeliveryCursor(context.Background(), freshStartRecoveryStreamTwo)
	require.ErrorIs(t, err, sql.ErrNoRows, "no successor checkpoint may exist before its commit")
}

func requireFreshStartInboxCount(t *testing.T, db *sqlx.DB, want int) {
	t.Helper()
	var count int
	require.NoError(t, db.QueryRow(
		"SELECT count(*) FROM agent_delivery_inbox WHERE stream_id = ?", freshStartRecoveryStreamTwo,
	).Scan(&count))
	require.Equal(t, want, count)
}

func requireNoFreshStartAcknowledgement(t *testing.T, ctx context.Context, retained *journal.Journal) {
	t.Helper()
	stream, err := retained.GetStream(ctx, freshStartRecoveryStreamTwo)
	require.NoError(t, err)
	require.Zero(t, stream.Acknowledged, "retained successor output must stay unacknowledged")
}

// TestFreshStartBootstrapRegisteredJournalRecovery drives the real agentctl
// routes and journal against real SQLite admission: preflight replays the
// retained successor from zero, every effect waits for the successor commit,
// and replay is ordered and idempotent without redispatching an instruction.
func TestFreshStartBootstrapRegisteredJournalRecovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	f := newFreshStartRecoveryFixture(t)
	server := f.startServer(t)

	status, err := server.client.GetDeliveryStatus(ctx, freshStartRecoveryStreamTwo)
	require.NoError(t, err)
	require.False(t, status.Unresolved, "retained setup events without unsettled prompts are not an unresolved capability")
	require.Zero(t, status.SubmissionCount)
	require.Empty(t, status.Submissions)
	require.False(t, status.SubmissionsTruncated)
	require.NotNil(t, status.Stream)
	require.Equal(t, uint64(2), status.Stream.HighWater)
	require.Zero(t, status.Stream.Acknowledged)
	require.Equal(t, uint64(1), status.Stream.FirstRetained)
	require.Equal(t, server.peer.ProcessManagerID(), status.ProcessManagerID)

	execution := f.execution(server)
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	t.Cleanup(sm.Wait)
	after, err := sm.initialDeliveryReplayCursor(ctx, execution, f.repo, server.client, freshStartRecoveryStreamTwo)
	require.NoError(t, err, "the registered successor stream must pass preflight after a failed committed predecessor")
	require.Zero(t, after)
	requireNoFreshStartCursor(t, f.repo)
	current, err := f.repo.GetCurrentHarnessSessionGeneration(ctx, freshStartRecoverySession, freshStartRecoveryIncarnation)
	require.NoError(t, err)
	require.EqualValues(t, 1, current.Generation, "recovery must reuse the successor without advancing another generation")

	events := replayFreshStartEvents(t, ctx, server.client)
	callbacks := make(chan agentctl.AgentEvent, 8)
	retired := make(chan bool, 2)
	sm = NewStreamManager(newTestLogger(), StreamCallbacks{
		OnAgentEvent: func(_ *AgentExecution, event agentctl.AgentEvent) {
			retired <- agentEventBelongsToRetiredProcess(execution, event)
			callbacks <- event
		},
	}, nil, nil)
	t.Cleanup(sm.Wait)
	reader := &freshStartRecoveryRepository{Repository: f.repo, admissionRead: make(chan struct{})}
	processor := newStreamEventProcessor(sm, ctx, execution, server.client, reader, execution.startupAttemptSnapshot(), nil)
	defer func() { _ = processor.close() }()
	for _, event := range events {
		require.True(t, processor.enqueue(event))
	}
	select {
	case <-reader.admissionRead:
	case <-ctx.Done():
		t.Fatal("successor admission did not inspect SQL authority")
	}
	requireNoFreshStartCursor(t, f.repo)
	requireFreshStartInboxCount(t, f.db, 0)
	requireNoFreshStartAcknowledgement(t, ctx, server.retained)
	select {
	case event := <-callbacks:
		t.Fatalf("lifecycle callback %q ran before the successor commit", event.Type)
	default:
	}

	successor := &models.HarnessSessionGeneration{
		SessionID: freshStartRecoverySession, IncarnationID: freshStartRecoveryIncarnation,
		Generation: 2, PredecessorGeneration: 1, NativeSessionID: freshStartRecoveryNativeTwo,
		CreationReason: "native_replaced",
	}
	committed, err := f.repo.CommitHarnessSessionGenerationForExecution(
		ctx, successor, 1, freshStartRecoveryTask, freshStartRecoveryExecution,
	)
	require.NoError(t, err)
	require.True(t, committed, "the replacement execution still owns the successor token")

	require.Eventually(t, func() bool {
		cursor, err := f.repo.GetAgentDeliveryCursor(ctx, freshStartRecoveryStreamTwo)
		return err == nil && cursor.ReceivedSequence == 2 && cursor.ProjectedSequence == 2
	}, 5*time.Second, 10*time.Millisecond)
	requireFreshStartInboxCount(t, f.db, 2)
	var replayed []agentctl.AgentEvent
	for len(replayed) < 2 {
		select {
		case event := <-callbacks:
			replayed = append(replayed, event)
		case <-ctx.Done():
			t.Fatal("committed successor did not release retained events")
		}
	}
	require.Equal(t, []string{streams.EventTypeAgentCapabilities, streams.EventTypeError},
		[]string{replayed[0].Type, replayed[1].Type}, "replay must preserve journal order")
	require.Equal(t, []uint64{1, 2}, []uint64{replayed[0].DeliverySequence, replayed[1].DeliverySequence})
	require.False(t, <-retired)
	require.True(t, <-retired, "the old manager's exit error must be retired")
	require.Eventually(t, func() bool {
		stream, err := server.retained.GetStream(ctx, freshStartRecoveryStreamTwo)
		return err == nil && stream.Acknowledged == 2
	}, 5*time.Second, 10*time.Millisecond)
	require.Zero(t, server.dispatch.count.Load(), "delivery and acknowledgement are not provider dispatches")

	predecessorMessages, err := f.repo.ListMessages(ctx, freshStartRecoverySession)
	require.NoError(t, err)
	require.Equal(t, f.predecessor, predecessorMessages, "projected predecessor history must stay unchanged")
	predecessorCursor, err := f.repo.GetAgentDeliveryCursor(ctx, freshStartRecoveryStreamOne)
	require.NoError(t, err)
	require.Equal(t, f.predecessorID.ProjectedSequence, predecessorCursor.ProjectedSequence)
	require.Equal(t, f.predecessorID.ReceivedSequence, predecessorCursor.ReceivedSequence)

	duplicateReader := &freshStartRecoveryRepository{Repository: f.repo, admissionRead: make(chan struct{})}
	duplicate := newStreamEventProcessor(sm, ctx, execution, server.client, duplicateReader, execution.startupAttemptSnapshot(), nil)
	for _, event := range events {
		require.True(t, duplicate.enqueue(event))
	}
	require.NoError(t, duplicate.close())
	requireFreshStartInboxCount(t, f.db, 2)
	cursor, err := f.repo.GetAgentDeliveryCursor(ctx, freshStartRecoveryStreamTwo)
	require.NoError(t, err)
	require.EqualValues(t, 2, cursor.ReceivedSequence)
	require.EqualValues(t, 2, cursor.ProjectedSequence)
	select {
	case event := <-callbacks:
		t.Fatalf("duplicate replay notified lifecycle callback %q", event.Type)
	default:
	}
	require.Zero(t, server.dispatch.count.Load(), "idempotent replay must not dispatch an instruction")
	require.NoError(t, processor.close())
}

// TestFreshStartBootstrapCancellationLeavesUncommittedStream proves that
// cancellation and rejected commits preserve the retained successor and its
// predecessor authority, and that a later eligible retry still recovers it.
func TestFreshStartBootstrapCancellationLeavesUncommittedStream(t *testing.T) {
	for _, outcome := range []string{"cancelled", "rejected commit", "failed commit"} {
		t.Run(outcome, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			f := newFreshStartRecoveryFixture(t)
			server := f.startServer(t)
			execution := f.execution(server)
			sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
			t.Cleanup(sm.Wait)
			after, err := sm.initialDeliveryReplayCursor(ctx, execution, f.repo, server.client, freshStartRecoveryStreamTwo)
			require.NoError(t, err)
			require.Zero(t, after)
			events := replayFreshStartEvents(t, ctx, server.client)
			reader := &freshStartRecoveryRepository{Repository: f.repo, admissionRead: make(chan struct{})}
			procCtx, procCancel := context.WithCancel(ctx)
			processor := newStreamEventProcessor(sm, procCtx, execution, server.client, reader, execution.startupAttemptSnapshot(), nil)
			t.Cleanup(func() { procCancel(); _ = processor.close() })
			for _, event := range events {
				require.True(t, processor.enqueue(event))
			}
			select {
			case <-reader.admissionRead:
			case <-ctx.Done():
				t.Fatal("successor admission did not inspect SQL authority")
			}
			successor := &models.HarnessSessionGeneration{
				SessionID: freshStartRecoverySession, IncarnationID: freshStartRecoveryIncarnation,
				Generation: 2, PredecessorGeneration: 1, NativeSessionID: freshStartRecoveryNativeTwo,
				CreationReason: "native_replaced",
			}
			switch outcome {
			case "cancelled":
				cancel()
				require.ErrorIs(t, processor.close(), context.Canceled)
			case "rejected commit":
				successor.NativeSessionID = "obsolete-native"
				committed, commitErr := f.repo.CommitHarnessSessionGenerationForExecution(
					ctx, successor, 1, freshStartRecoveryTask, freshStartRecoveryExecution,
				)
				require.NoError(t, commitErr)
				require.False(t, committed, "a replaced execution token cannot commit the successor")
				procCancel()
				require.ErrorIs(t, processor.close(), context.Canceled)
			case "failed commit":
				_, triggerErr := f.db.Exec(`CREATE TRIGGER reject_fresh_start_successor BEFORE INSERT ON harness_session_generations
					WHEN NEW.generation = 2 BEGIN SELECT RAISE(ABORT, 'successor insert rejected'); END`)
				require.NoError(t, triggerErr)
				_, commitErr := f.repo.CommitHarnessSessionGenerationForExecution(
					ctx, successor, 1, freshStartRecoveryTask, freshStartRecoveryExecution,
				)
				require.ErrorContains(t, commitErr, "successor insert rejected")
				procCancel()
				require.ErrorIs(t, processor.close(), context.Canceled)
				_, triggerErr = f.db.Exec(`DROP TRIGGER reject_fresh_start_successor`)
				require.NoError(t, triggerErr)
			}
			successor.NativeSessionID = freshStartRecoveryNativeTwo
			checkCtx, checkCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer checkCancel()
			requireNoFreshStartCursor(t, f.repo)
			requireFreshStartInboxCount(t, f.db, 0)
			requireNoFreshStartAcknowledgement(t, checkCtx, server.retained)
			stream, err := server.retained.GetStream(checkCtx, freshStartRecoveryStreamTwo)
			require.NoError(t, err)
			require.Equal(t, uint64(2), stream.HighWater, "retained successor events must survive the failed attempt")
			require.Equal(t, uint64(1), stream.FirstRetained)
			predecessorCursor, err := f.repo.GetAgentDeliveryCursor(checkCtx, freshStartRecoveryStreamOne)
			require.NoError(t, err)
			require.Equal(t, f.predecessorID.ProjectedSequence, predecessorCursor.ProjectedSequence)
			predecessorMessages, err := f.repo.ListMessages(checkCtx, freshStartRecoverySession)
			require.NoError(t, err)
			require.Equal(t, f.predecessor, predecessorMessages)

			committed, err := f.repo.CommitHarnessSessionGenerationForExecution(
				checkCtx, successor, 1, freshStartRecoveryTask, freshStartRecoveryExecution,
			)
			require.NoError(t, err)
			require.True(t, committed)
			retryReader := &freshStartRecoveryRepository{Repository: f.repo, admissionRead: make(chan struct{})}
			retry := newStreamEventProcessor(sm, checkCtx, execution, server.client, retryReader, execution.startupAttemptSnapshot(), nil)
			for _, event := range events {
				require.True(t, retry.enqueue(event))
			}
			require.NoError(t, retry.close(), "a later eligible retry of the same retained stream must succeed")
			requireFreshStartInboxCount(t, f.db, 2)
			require.Eventually(t, func() bool {
				cursor, err := f.repo.GetAgentDeliveryCursor(checkCtx, freshStartRecoveryStreamTwo)
				return err == nil && cursor.ReceivedSequence == 2 && cursor.ProjectedSequence == 2
			}, 5*time.Second, 10*time.Millisecond)
			require.Eventually(t, func() bool {
				stream, err := server.retained.GetStream(checkCtx, freshStartRecoveryStreamTwo)
				return err == nil && stream.Acknowledged == 2
			}, 5*time.Second, 10*time.Millisecond)

			staleEvent := events[0]
			staleEvent.DeliverySequence = 3
			staleEvent.DeliveryStreamID = freshStartRecoveryStreamTwo
			execution.ID = "stale-execution"
			_, err = sm.prepareAgentEvent(checkCtx, execution, server.client, f.repo, staleEvent)
			require.ErrorIs(t, err, repoerrors.ErrAgentDeliveryIdentityMismatch, "a stale owner must not admit successor output")
			execution.ID = freshStartRecoveryExecution

			delayed := createPredecessorAgentEvent()
			execution.DeliveryHarnessGeneration, execution.DeliveryStreamID = 1, freshStartRecoveryStreamOne
			execution.ReplacesNativeConversation = false
			_, err = sm.prepareAgentEvent(checkCtx, execution, server.client, f.repo, delayed)
			require.ErrorIs(t, err, ErrDurableAdoptionIdentityMismatch, "a delayed predecessor event must fail after the successor")
			requireFreshStartInboxCount(t, f.db, 2)
		})
	}
}

func createPredecessorAgentEvent() agentctl.AgentEvent {
	return agentctl.AgentEvent{
		Type: streams.EventTypeMessageChunk, Text: "delayed predecessor",
		DeliveryStreamID: freshStartRecoveryStreamOne, DeliveryIncarnationID: freshStartRecoveryIncarnation,
		DeliveryHarnessGeneration: 1, DeliverySequence: 2,
	}
}
