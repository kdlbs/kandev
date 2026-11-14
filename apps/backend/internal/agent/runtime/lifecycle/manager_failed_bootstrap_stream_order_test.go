package lifecycle

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
)

type orderedStopExecutor struct {
	*MockExecutor
	record func(string)
}

func (e *orderedStopExecutor) StopInstance(ctx context.Context, instance *ExecutorInstance, force bool) error {
	e.record("runtime_stop")
	return e.MockExecutor.StopInstance(ctx, instance, force)
}

func TestStopAgentWithReasonStopsFailedBootstrapBeforeAbandoningStream(t *testing.T) {
	ctx := t.Context()
	repo, _ := openRetentionLifecycleRepository(t, filepath.Join(t.TempDir(), "delivery.db"))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", Title: "Bootstrap cleanup"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session", TaskID: "task", QueueIncarnationID: "incarnation", State: models.TaskSessionStateWaitingForInput,
	}))
	committed, err := repo.CommitHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: "session", IncarnationID: "incarnation", Generation: 1,
		NativeSessionID: "native-one", CreationReason: "initial",
	}, 0)
	require.NoError(t, err)
	require.True(t, committed)

	var mu sync.Mutex
	var calls []string
	processStopped := false
	var requestErr error
	var abandonedStreamID string
	var abandonedGeneration uint64
	record := func(call string) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, call)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/stop":
			record("process_stop")
			mu.Lock()
			processStopped = true
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/agent/delivery/stream/abandon-bootstrap":
			record("abandon")
			mu.Lock()
			stopped := processStopped
			mu.Unlock()
			if !stopped {
				http.Error(w, "agent process is still running", http.StatusConflict)
				return
			}
			var request struct {
				StreamID          string `json:"stream_id"`
				HarnessGeneration uint64 `json:"harness_generation"`
			}
			if decodeErr := json.NewDecoder(r.Body).Decode(&request); decodeErr != nil {
				mu.Lock()
				requestErr = decodeErr
				mu.Unlock()
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
			mu.Lock()
			abandonedStreamID = request.StreamID
			abandonedGeneration = request.HarnessGeneration
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)
	host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)

	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	backend := &orderedStopExecutor{MockExecutor: &MockExecutor{name: executor.NameStandalone}, record: record}
	registry.Register(backend)
	manager := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, manager)
	manager.SetAgentDeliveryRepository(repo)
	execution := &AgentExecution{
		ID: "successor", TaskID: "task", SessionID: "session", RuntimeName: executor.NameStandalone,
		DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 2, DeliveryStreamID: "incarnation:g2",
		ForceContextContinuation: true, agentctl: agentctl.NewClient(host, port, log),
	}
	require.NoError(t, manager.executionStore.Add(execution))

	require.NoError(t, manager.StopAgentWithReason(ctx, execution.ID, StopReasonAgentBootstrapFailed, true))

	mu.Lock()
	got := append([]string(nil), calls...)
	gotStreamID := abandonedStreamID
	gotGeneration := abandonedGeneration
	gotRequestErr := requestErr
	mu.Unlock()
	require.Equal(t, []string{"process_stop", "abandon", "runtime_stop"}, got)
	require.NoError(t, gotRequestErr)
	require.Equal(t, "incarnation:g2", gotStreamID)
	require.Equal(t, uint64(2), gotGeneration)
}
