package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
)

type retentionBootstrapRepository struct {
	recordingAgentDeliveryRepository
	generation  atomic.Pointer[models.HarnessSessionGeneration]
	firstRead   chan struct{}
	readOnce    sync.Once
	receipts    atomic.Int32
	projections atomic.Int32
}

func (r *retentionBootstrapRepository) GetCurrentHarnessSessionGeneration(context.Context, string, string) (*models.HarnessSessionGeneration, error) {
	generation := r.generation.Load()
	r.readOnce.Do(func() { close(r.firstRead) })
	if generation == nil {
		return nil, models.ErrTaskSessionNotFound
	}
	copy := *generation
	return &copy, nil
}

func (r *retentionBootstrapRepository) ReceiveAgentDeliveryEvent(ctx context.Context, event *models.AgentDeliveryEvent, highWater int64) (bool, error) {
	if r.generation.Load() == nil {
		return false, errors.New("bootstrap receipt written before committed generation")
	}
	r.receipts.Add(1)
	return r.recordingAgentDeliveryRepository.ReceiveAgentDeliveryEvent(ctx, event, highWater)
}

func (r *retentionBootstrapRepository) ProjectAgentDeliveryEvent(ctx context.Context, event *models.AgentDeliveryEvent, effect *models.AgentDeliveryEffect) (bool, error) {
	if r.generation.Load() == nil {
		return false, errors.New("bootstrap projection written before committed generation")
	}
	r.projections.Add(1)
	return r.recordingAgentDeliveryRepository.ProjectAgentDeliveryEvent(ctx, event, effect)
}

func retentionBootstrapOwner() (*AgentExecution, agentctl.AgentEvent) {
	execution := &AgentExecution{ID: "bootstrap-execution", TaskID: "bootstrap-task", SessionID: "bootstrap-session", DeliveryMode: DurableDeliveryV1, DeliveryStreamID: "bootstrap-stream", DeliveryIncarnationID: "bootstrap-incarnation", DeliveryHarnessGeneration: 1}
	execution.deliveryBootstrap.Store(true)
	event := agentctl.AgentEvent{Type: "session_config_options", DeliveryStreamID: execution.DeliveryStreamID, DeliverySequence: 1, DeliveryIncarnationID: execution.DeliveryIncarnationID, DeliveryHarnessGeneration: 1}
	return execution, event
}

func retentionBootstrapGeneration() *models.HarnessSessionGeneration {
	return &models.HarnessSessionGeneration{SessionID: "bootstrap-session", IncarnationID: "bootstrap-incarnation", Generation: 1, NativeSessionID: "bootstrap-native", CreationReason: "native_started"}
}

func TestDurableRetentionBootstrapWaitsForCommittedGeneration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	repo := &retentionBootstrapRepository{firstRead: make(chan struct{})}
	ack := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(journal.RecoveryDescriptor{
				StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Durable: true},
				SessionID:         "bootstrap-session", IncarnationID: "bootstrap-incarnation", HarnessGeneration: 1, StreamID: "bootstrap-stream",
				Stream: &journal.Stream{SessionID: "bootstrap-session", IncarnationID: "bootstrap-incarnation", HarnessGeneration: 1, StreamID: "bootstrap-stream", HighWater: 1, FirstRetained: 1},
			})
			return
		}
		if request.URL.Path != "/api/v1/agent/delivery/stream/ack" || repo.generation.Load() == nil || repo.projections.Load() == 0 {
			http.Error(w, "ACK arrived without committed owner and projection", http.StatusConflict)
			return
		}
		ack <- struct{}{}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	address, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(address.Port())
	if err != nil {
		t.Fatal(err)
	}
	client := agentctl.NewClient(address.Hostname(), port, newTestLogger())
	manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	defer manager.Wait()
	execution, event := retentionBootstrapOwner()
	execution.agentctl = client
	processor := newStreamEventProcessor(manager, ctx, execution, client, repo, 0)
	defer func() { _ = processor.close() }()
	if !processor.enqueue(event) {
		t.Fatal("bootstrap frame was refused before processing")
	}
	select {
	case <-repo.firstRead:
	case <-ctx.Done():
		t.Fatal("bootstrap did not query authoritative generation")
	}
	if repo.receipts.Load() != 0 || repo.projections.Load() != 0 {
		t.Fatal("bootstrap touched delivery evidence before committed authority")
	}
	repo.generation.Store(retentionBootstrapGeneration())
	select {
	case <-ack:
	case <-processor.done:
		t.Fatalf("bootstrap processor stopped before committed owner: %v", processor.close())
	case <-ctx.Done():
		t.Fatal("bootstrap did not project and acknowledge after committed owner")
	}
	if err := processor.close(); err != nil {
		t.Fatal(err)
	}
	if execution.deliveryBootstrap.Load() {
		t.Fatal("successful authoritative generation read left bootstrap admission open")
	}
	if repo.receipts.Load() != 1 || repo.projections.Load() != 1 {
		t.Fatalf("bootstrap durable writes: receipts=%d projections=%d", repo.receipts.Load(), repo.projections.Load())
	}
}

func TestDurableRetentionBootstrapCancellationPreservesMissingAuthority(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	repo := &retentionBootstrapRepository{firstRead: make(chan struct{})}
	execution, event := retentionBootstrapOwner()
	manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	result := make(chan error, 1)
	go func() {
		_, err := manager.prepareAgentEvent(ctx, execution, nil, repo, event)
		result <- err
	}()
	select {
	case <-repo.firstRead:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("bootstrap cancellation fixture did not query authority")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("missing bootstrap authority cancellation: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("missing authority did not honor cancellation")
	}
	if repo.receipts.Load() != 0 || repo.projections.Load() != 0 {
		t.Fatal("missing authority admitted delivery evidence")
	}
}

func TestDurableRetentionBootstrapForeignOwnerFailsClosed(t *testing.T) {
	for _, foreignFrame := range []bool{false, true} {
		t.Run(strconv.FormatBool(foreignFrame), func(t *testing.T) {
			repo := &retentionBootstrapRepository{firstRead: make(chan struct{})}
			generation := retentionBootstrapGeneration()
			generation.Generation = 2
			repo.generation.Store(generation)
			execution, event := retentionBootstrapOwner()
			if foreignFrame {
				event.DeliveryIncarnationID = "foreign-incarnation"
			}
			manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
			_, err := manager.prepareAgentEvent(context.Background(), execution, nil, repo, event)
			if !errors.Is(err, ErrDurableAdoptionIdentityMismatch) {
				t.Fatalf("foreign bootstrap owner: %v", err)
			}
			if repo.receipts.Load() != 0 || repo.projections.Load() != 0 {
				t.Fatal("foreign bootstrap owner touched delivery evidence")
			}
		})
	}
}

func TestDurableRetentionBootstrapUnmarkedMissingOwnerFailsClosed(t *testing.T) {
	repo := &retentionBootstrapRepository{firstRead: make(chan struct{})}
	execution, event := retentionBootstrapOwner()
	execution.deliveryBootstrap.Store(false)
	manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	_, err := manager.prepareAgentEvent(context.Background(), execution, nil, repo, event)
	if !errors.Is(err, ErrDurableAdoptionIdentityMismatch) {
		t.Fatalf("missing authority without fresh bootstrap admission: %v", err)
	}
	if repo.receipts.Load() != 0 || repo.projections.Load() != 0 {
		t.Fatal("unmarked missing owner touched delivery evidence")
	}
}
