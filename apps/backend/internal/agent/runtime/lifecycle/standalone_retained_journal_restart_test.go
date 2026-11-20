package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	instancepkg "github.com/kandev/kandev/internal/agentctl/server/instance"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	agenttypes "github.com/kandev/kandev/pkg/agent"
)

// realStandaloneControl is an authenticated control surface over a real
// agentctl instance manager. It records the request sequence so the test can
// prove the pre-creation resolver ran before any journal open.
type realStandaloneControl struct {
	mgr *instancepkg.Manager

	mu                   sync.Mutex
	methods              []string
	failCreateResponse   int
	failDeleteBeforeStop map[string]bool
	failDeleteAfterStop  map[string]bool
}

func newRealStandaloneControl(mgr *instancepkg.Manager) *realStandaloneControl {
	return &realStandaloneControl{
		mgr:                  mgr,
		failDeleteBeforeStop: map[string]bool{},
		failDeleteAfterStop:  map[string]bool{},
	}
}

func (c *realStandaloneControl) record(method string) {
	c.mu.Lock()
	c.methods = append(c.methods, method)
	c.mu.Unlock()
}

func (c *realStandaloneControl) recorded() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.methods...)
}

func (c *realStandaloneControl) failNextCreateResponse() {
	c.mu.Lock()
	c.failCreateResponse++
	c.mu.Unlock()
}

func (c *realStandaloneControl) setDeleteFailure(id string, beforeStop, afterStop bool) {
	c.mu.Lock()
	c.failDeleteBeforeStop[id] = beforeStop
	c.failDeleteAfterStop[id] = afterStop
	c.mu.Unlock()
}

func (c *realStandaloneControl) listInstances() []*agentctl.InstanceInfo {
	c.record("LIST")
	infos := c.mgr.ListInstances()
	converted := make([]*agentctl.InstanceInfo, 0, len(infos))
	for _, info := range infos {
		converted = append(converted, &agentctl.InstanceInfo{
			ID: info.ID, Port: info.Port, Status: info.Status, WorkspacePath: info.WorkspacePath,
			Env: info.Env, SessionID: info.SessionID, TaskID: info.TaskID,
			WorkspaceSourceRoots: info.WorkspaceSourceRoots, ProviderSessionID: info.ProviderSessionID,
		})
	}
	return converted
}

func (c *realStandaloneControl) getInstance(id string) (*agentctl.InstanceInfo, error) {
	c.record("GET " + id)
	instance, ok := c.mgr.GetInstance(id)
	if !ok {
		return nil, fmt.Errorf("%w: %q", agentctl.ErrInstanceNotFound, id)
	}
	info := instance.Info()
	return &agentctl.InstanceInfo{
		ID: info.ID, Port: info.Port, Status: info.Status, WorkspacePath: info.WorkspacePath,
		Env: info.Env, SessionID: info.SessionID, TaskID: info.TaskID,
		WorkspaceSourceRoots: info.WorkspaceSourceRoots,
	}, nil
}

func (c *realStandaloneControl) createInstance(ctx context.Context, req *agentctl.CreateInstanceRequest) (*agentctl.CreateInstanceResponse, error) {
	c.record("CREATE " + req.ID)
	created, err := c.mgr.CreateInstance(ctx, &instancepkg.CreateRequest{
		ID: req.ID, WorkspacePath: req.WorkspacePath, Protocol: req.Protocol,
		SessionID: req.SessionID, TaskID: req.TaskID, Env: req.Env,
		DurableJournalPath:        req.DurableJournalPath,
		DeliveryStreamID:          req.DeliveryStreamID,
		DeliveryIncarnationID:     req.DeliveryIncarnationID,
		DeliveryHarnessGeneration: req.DeliveryHarnessGeneration,
	})
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	loseResponse := c.failCreateResponse > 0
	if loseResponse {
		c.failCreateResponse--
	}
	c.mu.Unlock()
	if loseResponse {
		return nil, errors.New("injected create response loss")
	}
	return &agentctl.CreateInstanceResponse{ID: created.ID, Port: created.Port}, nil
}

func (c *realStandaloneControl) deleteInstance(ctx context.Context, id string) error {
	c.record("DELETE " + id)
	c.mu.Lock()
	beforeStop := c.failDeleteBeforeStop[id]
	afterStop := c.failDeleteAfterStop[id]
	c.mu.Unlock()
	if beforeStop {
		return errors.New("injected teardown failure")
	}
	if err := c.mgr.StopInstance(ctx, id); err != nil {
		return err
	}
	if afterStop {
		return errors.New("injected delete response loss")
	}
	return nil
}

func newRealStandaloneControlServer(t *testing.T, control *realStandaloneControl) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/health":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/api/v1/instances" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"instances": control.listInstances()})
		case r.URL.Path == "/api/v1/instances" && r.Method == http.MethodPost:
			var request agentctl.CreateInstanceRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			response, err := control.createInstance(r.Context(), &request)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			_ = json.NewEncoder(w).Encode(response)
		case len(r.URL.Path) > len("/api/v1/instances/"):
			id := r.URL.Path[len("/api/v1/instances/"):]
			switch r.Method {
			case http.MethodGet:
				info, err := control.getInstance(id)
				if errors.Is(err, agentctl.ErrInstanceNotFound) {
					http.NotFound(w, r)
					return
				}
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				_ = json.NewEncoder(w).Encode(info)
			case http.MethodDelete:
				if err := control.deleteInstance(r.Context(), id); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
			default:
				http.NotFound(w, r)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// realInstanceStatuses serves the per-instance status and delivery routes over
// a real agentctl instance manager.
type realInstanceStatuses struct {
	mu       sync.Mutex
	running  map[string]bool
	delivery map[string]*journal.RecoveryDescriptor
}

func (s *realInstanceStatuses) setRunning(id string, running bool) {
	s.mu.Lock()
	s.running[id] = running
	s.mu.Unlock()
}

func (s *realInstanceStatuses) setDelivery(id string, descriptor *journal.RecoveryDescriptor) {
	s.mu.Lock()
	s.delivery[id] = descriptor
	s.mu.Unlock()
}

func (s *realInstanceStatuses) factory(cfg *config.InstanceConfig, _ *process.Manager, _ *logger.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/status", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.Lock()
		running := s.running[cfg.InstanceID]
		s.mu.Unlock()
		status := "stopped"
		if running {
			status = "running"
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"agent_status": status})
	})
	mux.HandleFunc("/api/v1/agent/delivery", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		descriptor := s.delivery[cfg.InstanceID]
		s.mu.Unlock()
		if descriptor == nil {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(descriptor)
	})
	return mux
}

func newRealInstanceManager(t *testing.T, ports int) (*instancepkg.Manager, *realInstanceStatuses) {
	t.Helper()
	base := bindBindablePortRange(t, ports)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	statuses := &realInstanceStatuses{running: map[string]bool{}, delivery: map[string]*journal.RecoveryDescriptor{}}
	mgr := instancepkg.NewManager(&config.Config{
		Ports:    config.PortConfig{Base: base, Max: base + ports - 1},
		Defaults: config.InstanceDefaults{Protocol: agenttypes.ProtocolACP},
	}, log)
	mgr.SetServerFactory(statuses.factory)
	t.Cleanup(func() {
		done := make(chan struct{})
		go func() {
			_ = mgr.Shutdown(context.Background())
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(20 * time.Second):
			t.Error("instance manager shutdown did not finish")
		}
	})
	return mgr, statuses
}

func bindBindablePortRange(t *testing.T, size int) int {
	t.Helper()
	for attempt := 0; attempt < 10; attempt++ {
		probe, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen for port range: %v", err)
		}
		base := probe.Addr().(*net.TCPAddr).Port
		_ = probe.Close()
		if base+size-1 > 65535 {
			continue
		}
		ok := true
		for port := base; port < base+size; port++ {
			listener, listenErr := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
			if listenErr != nil {
				ok = false
				break
			}
			_ = listener.Close()
		}
		if ok {
			return base
		}
	}
	t.Skip("could not find a bindable loopback port range")
	return 0
}

func standaloneRestartRequest(root, requestedID, previousID string) *ExecutorCreateRequest {
	return &ExecutorCreateRequest{
		InstanceID:                requestedID,
		PreviousExecutionID:       previousID,
		TaskID:                    "task-restart",
		SessionID:                 "session-restart",
		WorkspacePath:             root,
		DurableJournalHostRoot:    root,
		DurableJournalOwnerID:     "session-restart",
		DeliveryStreamID:          "stream-1",
		DeliveryIncarnationID:     "incarnation-1",
		DeliveryHarnessGeneration: 1,
	}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.20
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.21
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.23
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.24
func TestStandaloneRetainedJournalRestartRecovery(t *testing.T) {
	t.Run("stopped owner replacement survives repeated restart", func(t *testing.T) {
		root := t.TempDir()
		location, err := journal.ResolveLocation(root, "session-restart")
		require.NoError(t, err)
		event, submission := seedRetainedJournalForSession(t, location.Path, "session-restart")

		mgr, statuses := newRealInstanceManager(t, 8)
		first, err := mgr.CreateInstance(context.Background(), &instancepkg.CreateRequest{
			ID: "retained-1", WorkspacePath: root, SessionID: "session-restart",
			TaskID: "task-restart", DurableJournalPath: location.Path,
			DeliveryStreamID: "stream-1", DeliveryIncarnationID: "incarnation-1", DeliveryHarnessGeneration: 1,
		})
		require.NoError(t, err)
		statuses.setRunning(first.ID, false)

		control := newRealStandaloneControl(mgr)
		server := newRealStandaloneControlServer(t, control)

		firstExecutor := newStandaloneReconnectExecutor(t, server)
		replacement, err := firstExecutor.CreateInstance(context.Background(), standaloneRestartRequest(root, "replacement-1", first.ID))
		require.NoError(t, err)
		require.Equal(t, "replacement-1", replacement.StandaloneInstanceID)
		require.Contains(t, control.recorded(), "DELETE "+first.ID)
		_, firstStillPresent := mgr.GetInstance(first.ID)
		require.False(t, firstStillPresent)

		secondExecutor := newStandaloneReconnectExecutor(t, newRealStandaloneControlServer(t, control))
		secondReplacement, err := secondExecutor.CreateInstance(context.Background(), standaloneRestartRequest(root, "replacement-2", "replacement-1"))
		require.NoError(t, err)
		require.Equal(t, "replacement-2", secondReplacement.StandaloneInstanceID)
		require.Contains(t, control.recorded(), "DELETE replacement-1")

		require.NoError(t, mgr.StopInstance(context.Background(), "replacement-2"))
		reopened, err := journal.Open(journal.Config{Path: location.Path, ReadOnly: true})
		require.NoError(t, err)
		defer func() { require.NoError(t, reopened.Close()) }()
		events, _, err := reopened.Replay(context.Background(), event.StreamID, 0, 10)
		require.NoError(t, err)
		require.Len(t, events, 1)
		require.Equal(t, event.Payload, events[0].Payload)
		retained, err := reopened.GetSubmission(context.Background(), submission.ID)
		require.NoError(t, err)
		require.Equal(t, submission.Hash, retained.Hash)
		require.Equal(t, journal.SubmissionInterruptedUnknown, retained.State, "uncertain work must remain unresolved")
	})

	t.Run("live owner is adopted without dispatch", func(t *testing.T) {
		root := t.TempDir()
		location, err := journal.ResolveLocation(root, "session-restart")
		require.NoError(t, err)
		mgr, statuses := newRealInstanceManager(t, 4)
		live, err := mgr.CreateInstance(context.Background(), &instancepkg.CreateRequest{
			ID: "retained-live", WorkspacePath: root, SessionID: "session-restart",
			TaskID: "task-restart", DurableJournalPath: location.Path,
		})
		require.NoError(t, err)
		statuses.setRunning(live.ID, true)
		statuses.setDelivery(live.ID, &journal.RecoveryDescriptor{
			StorageCapability: journal.StorageCapability{Version: 2, Durable: true},
			SessionID:         "session-restart",
			StreamID:          "stream-1",
			IncarnationID:     "incarnation-1",
			HarnessGeneration: 1,
		})

		control := newRealStandaloneControl(mgr)
		server := newRealStandaloneControlServer(t, control)
		instance, err := newStandaloneReconnectExecutor(t, server).CreateInstance(context.Background(), standaloneRestartRequest(root, "replacement-live", live.ID))
		require.NoError(t, err)
		require.Equal(t, "retained-live", instance.InstanceID)
		require.Equal(t, "retained-live", instance.StandaloneInstanceID)
		require.Equal(t, true, instance.Metadata[MetadataKeyReuseExistingProcess])
		require.Contains(t, control.recorded(), "LIST")
		for _, method := range control.recorded() {
			require.NotEqual(t, "CREATE replacement-live", method)
			require.NotEqual(t, "DELETE retained-live", method)
		}
		_, stillPresent := mgr.GetInstance("retained-live")
		require.True(t, stillPresent)
	})

	t.Run("create interruption after disposal is recoverable", func(t *testing.T) {
		root := t.TempDir()
		location, err := journal.ResolveLocation(root, "session-restart")
		require.NoError(t, err)
		mgr, _ := newRealInstanceManager(t, 4)
		first, err := mgr.CreateInstance(context.Background(), &instancepkg.CreateRequest{
			ID: "retained-interrupted", WorkspacePath: root, SessionID: "session-restart",
			TaskID: "task-restart", DurableJournalPath: location.Path,
		})
		require.NoError(t, err)
		control := newRealStandaloneControl(mgr)
		control.failNextCreateResponse()
		server := newRealStandaloneControlServer(t, control)

		_, err = newStandaloneReconnectExecutor(t, server).CreateInstance(context.Background(), standaloneRestartRequest(root, "replacement-interrupted", first.ID))
		require.Error(t, err)
		_, oldPresent := mgr.GetInstance(first.ID)
		require.False(t, oldPresent, "the stopped owner must have been disposed before the interrupted create")
		_, replacementPresent := mgr.GetInstance("replacement-interrupted")
		require.True(t, replacementPresent, "the failed response must leave the journal owner created by that request")

		recovered, err := newStandaloneReconnectExecutor(t, server).CreateInstance(context.Background(), standaloneRestartRequest(root, "replacement-interrupted", first.ID))
		require.NoError(t, err)
		require.Equal(t, "replacement-interrupted", recovered.StandaloneInstanceID)
		require.Contains(t, control.recorded(), "DELETE replacement-interrupted",
			"retry must reconcile the owner created by the lost response before reopening its journal")
	})

	t.Run("blocked teardown preserves the owner and unrelated sessions", func(t *testing.T) {
		root := t.TempDir()
		location, err := journal.ResolveLocation(root, "session-restart")
		require.NoError(t, err)
		mgr, _ := newRealInstanceManager(t, 6)
		blocked, err := mgr.CreateInstance(context.Background(), &instancepkg.CreateRequest{
			ID: "retained-blocked", WorkspacePath: root, SessionID: "session-restart",
			TaskID: "task-restart", DurableJournalPath: location.Path,
		})
		require.NoError(t, err)
		otherRoot := t.TempDir()
		otherLocation, err := journal.ResolveLocation(otherRoot, "other-session")
		require.NoError(t, err)
		other, err := mgr.CreateInstance(context.Background(), &instancepkg.CreateRequest{
			ID: "other-session-instance", WorkspacePath: otherRoot, SessionID: "other-session",
			TaskID: "task-restart", DurableJournalPath: otherLocation.Path,
		})
		require.NoError(t, err)
		control := newRealStandaloneControl(mgr)
		control.setDeleteFailure(blocked.ID, true, false)
		server := newRealStandaloneControlServer(t, control)

		_, err = newStandaloneReconnectExecutor(t, server).CreateInstance(context.Background(), standaloneRestartRequest(root, "replacement-blocked", blocked.ID))
		require.ErrorIs(t, err, errStandaloneJournalOwnerBlocked)
		_, stillPresent := mgr.GetInstance(blocked.ID)
		require.True(t, stillPresent, "an unconfirmed teardown must not authorize another journal open")
		_, otherPresent := mgr.GetInstance(other.ID)
		require.True(t, otherPresent, "a different session keeps its own instance")

		control.setDeleteFailure(blocked.ID, false, true)
		recovered, err := newStandaloneReconnectExecutor(t, server).CreateInstance(context.Background(), standaloneRestartRequest(root, "replacement-blocked", blocked.ID))
		require.NoError(t, err, "a lost DELETE response reconciled by exact absence permits recovery")
		require.Equal(t, "replacement-blocked", recovered.StandaloneInstanceID)
		_, otherStillPresent := mgr.GetInstance(other.ID)
		require.True(t, otherStillPresent)
	})
}
