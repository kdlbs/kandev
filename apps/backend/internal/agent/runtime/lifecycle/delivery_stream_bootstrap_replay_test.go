package lifecycle

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
)

type absentBootstrapCursorRepository struct {
	*retentionBootstrapRepository
}

func (*absentBootstrapCursorRepository) GetAgentDeliveryCursor(context.Context, string) (*models.AgentDeliveryCursor, error) {
	return nil, sql.ErrNoRows
}

func bootstrapReplayClient(
	t *testing.T,
	ctx context.Context,
	repo *retentionBootstrapRepository,
	acknowledged chan<- struct{},
) *agentctl.Client {
	t.Helper()
	var startGeneration atomic.Uint64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/agent/delivery":
			_ = json.NewEncoder(w).Encode(journal.RecoveryDescriptor{
				StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Durable: true},
				SessionID:         "bootstrap-session",
				IncarnationID:     "bootstrap-incarnation",
				HarnessGeneration: 1,
				StreamID:          "bootstrap-stream",
				ProcessManagerID:  "replacement-manager",
				Stream: &journal.Stream{
					SessionID: "bootstrap-session", IncarnationID: "bootstrap-incarnation",
					HarnessGeneration: 1, StreamID: "bootstrap-stream", HighWater: 1, FirstRetained: 1,
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/start":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true, "process_generation": startGeneration.Add(1),
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/agent/delivery/stream/ack":
			if repo.generation.Load() == nil || repo.receipts.Load() != 1 || repo.projections.Load() != 1 {
				http.Error(w, "ACK arrived before replacement generation commit", http.StatusConflict)
				return
			}
			acknowledged <- struct{}{}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	address, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(address.Port())
	if err != nil {
		t.Fatal(err)
	}
	client := agentctl.NewClient(address.Hostname(), port, newTestLogger())
	if _, err := client.Start(ctx); err != nil {
		t.Fatalf("start replacement child: %v", err)
	}
	return client
}

func requireReplacementGenerationFence(t *testing.T, ctx context.Context, client *agentctl.Client, execution *AgentExecution, event agentctl.AgentEvent) {
	t.Helper()
	if _, err := client.Start(ctx); err != nil {
		t.Fatalf("start next child in replacement manager: %v", err)
	}
	event.Data["process_manager_id"] = "replacement-manager"
	if !agentEventBelongsToRetiredProcess(execution, event) {
		t.Fatal("older generation in the same process manager was not retired")
	}
	event.Data["process_generation"] = uint64(2)
	if agentEventBelongsToRetiredProcess(execution, event) {
		t.Fatal("current generation in the same process manager was retired")
	}
}

func TestBootstrapReplayIgnoresExitFromFailedPredecessorManager(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	repo := &retentionBootstrapRepository{firstRead: make(chan struct{})}
	delivery := &absentBootstrapCursorRepository{retentionBootstrapRepository: repo}
	acknowledged := make(chan struct{}, 1)
	lifecycleCallback := make(chan bool, 1)
	client := bootstrapReplayClient(t, ctx, repo, acknowledged)

	execution, event := retentionBootstrapOwner()
	execution.ID = "replacement-execution"
	execution.isResumedSession = true
	execution.agentctl = client
	event.Type = "error"
	event.Error = "predecessor process exited"
	event.Data = map[string]any{"process_generation": uint64(1), "process_manager_id": "failed-manager"}
	streamManager := NewStreamManager(newTestLogger(), StreamCallbacks{
		OnAgentEventWithGeneration: func(execution *AgentExecution, event agentctl.AgentEvent, _ uint64) {
			lifecycleCallback <- agentEventBelongsToRetiredProcess(execution, event)
		},
	}, nil, nil)
	defer streamManager.Wait()
	after, err := streamManager.initialDeliveryReplayCursor(ctx, execution, delivery, client, execution.DeliveryStreamID)
	if err != nil || after != 0 {
		t.Fatalf("replacement bootstrap preflight = (%d, %v), want (0, nil)", after, err)
	}

	processor := newStreamEventProcessor(streamManager, ctx, execution, client, delivery, execution.startupAttemptSnapshot())
	defer func() { _ = processor.close() }()
	if !processor.enqueue(event) {
		t.Fatal("retained predecessor exit event was refused")
	}

	select {
	case <-repo.firstRead:
	case <-ctx.Done():
		t.Fatal("replay did not wait for replacement generation authority")
	}
	if repo.receipts.Load() != 0 || repo.projections.Load() != 0 {
		t.Fatal("replay wrote delivery evidence before replacement generation commit")
	}
	select {
	case <-acknowledged:
		t.Fatal("replay acknowledged the predecessor event before replacement generation commit")
	default:
	}
	select {
	case <-lifecycleCallback:
		t.Fatal("replay reached lifecycle before replacement generation commit")
	default:
	}

	repo.generation.Store(retentionBootstrapGeneration())
	select {
	case retired := <-lifecycleCallback:
		if !retired {
			t.Fatal("predecessor exit error was treated as an event from the replacement child")
		}
	case <-ctx.Done():
		t.Fatal("committed replacement did not release the lifecycle callback")
	}
	select {
	case <-acknowledged:
	case <-ctx.Done():
		t.Fatal("retained predecessor event was not acknowledged after the generation commit")
	}
	delete(event.Data, "process_manager_id")
	if !agentEventBelongsToRetiredProcess(execution, event) {
		t.Fatal("unmarked retained exit was not retired against an identified manager")
	}
	requireReplacementGenerationFence(t, ctx, client, execution, event)
	if err := processor.close(); err != nil {
		t.Fatal(err)
	}
}
