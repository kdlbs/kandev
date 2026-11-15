package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
)

// journalReconnectControl models the authenticated agentctl control API with a
// real bbolt journal, so a duplicate open fails for the same reason production
// does instead of through a synthetic flag.
type journalReconnectControl struct {
	mu              sync.Mutex
	journalPathFor  func(sessionID string) string
	instances       map[string]*agentctl.InstanceInfo
	running         map[string]bool
	statusFailures  map[string]bool
	deliverySession map[string]string
	deliveryFails   map[string]bool
	held            map[string]*journal.Journal
	methods         []string
	listErr         error
	deleteErr       error
	createErr       error
	nextPort        int
}

func newJournalReconnectControl(pathFor func(string) string) *journalReconnectControl {
	return &journalReconnectControl{
		journalPathFor:  pathFor,
		instances:       map[string]*agentctl.InstanceInfo{},
		running:         map[string]bool{},
		statusFailures:  map[string]bool{},
		deliverySession: map[string]string{},
		deliveryFails:   map[string]bool{},
		held:            map[string]*journal.Journal{},
		nextPort:        42001,
	}
}

func (c *journalReconnectControl) addInstance(id, taskID, sessionID string, port int, running bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.instances[id] = &agentctl.InstanceInfo{ID: id, TaskID: taskID, SessionID: sessionID, Port: port}
	c.running[id] = running
	c.deliverySession[id] = sessionID
}

func (c *journalReconnectControl) holdJournal(t *testing.T, id, path string) {
	t.Helper()
	opened, err := journal.Open(journal.Config{Path: path})
	require.NoError(t, err)
	t.Cleanup(func() { _ = opened.Close() })
	c.mu.Lock()
	c.held[id] = opened
	c.mu.Unlock()
}

func (c *journalReconnectControl) recordedMethods() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.methods...)
}

func (c *journalReconnectControl) hasMethod(prefix string) bool {
	for _, method := range c.recordedMethods() {
		if strings.HasPrefix(method, prefix) {
			return true
		}
	}
	return false
}

func (c *journalReconnectControl) GetInstance(_ context.Context, id string) (*agentctl.InstanceInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.methods = append(c.methods, "GET "+id)
	info := c.instances[id]
	if info == nil {
		return nil, fmt.Errorf("%w: %q", agentctl.ErrInstanceNotFound, id)
	}
	copied := *info
	return &copied, nil
}

func (c *journalReconnectControl) ListInstances(context.Context) ([]*agentctl.InstanceInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.methods = append(c.methods, "LIST")
	if c.listErr != nil {
		return nil, c.listErr
	}
	ids := make([]string, 0, len(c.instances))
	for id := range c.instances {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	instances := make([]*agentctl.InstanceInfo, 0, len(ids))
	for _, id := range ids {
		copied := *c.instances[id]
		instances = append(instances, &copied)
	}
	return instances, nil
}

func (c *journalReconnectControl) DeleteInstance(_ context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.methods = append(c.methods, "DELETE "+id)
	if c.deleteErr != nil {
		return c.deleteErr
	}
	if held := c.held[id]; held != nil {
		if err := held.Close(); err != nil {
			return err
		}
		delete(c.held, id)
	}
	delete(c.instances, id)
	delete(c.running, id)
	return nil
}

func (c *journalReconnectControl) CreateInstance(_ context.Context, req *agentctl.CreateInstanceRequest) (*agentctl.CreateInstanceResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.methods = append(c.methods, "CREATE "+req.ID)
	if c.createErr != nil {
		return nil, c.createErr
	}
	opened, err := journal.Open(journal.Config{
		Path:        c.journalPathFor(req.SessionID),
		OpenTimeout: 100 * time.Millisecond,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create new instance: %w", err)
	}
	c.held[req.ID] = opened
	port := c.nextPort
	c.nextPort++
	c.instances[req.ID] = &agentctl.InstanceInfo{ID: req.ID, TaskID: req.TaskID, SessionID: req.SessionID, Port: port}
	c.running[req.ID] = false
	c.deliverySession[req.ID] = req.SessionID
	return &agentctl.CreateInstanceResponse{ID: req.ID, Port: port}, nil
}

func newReconnectStatusServer(t *testing.T, control *journalReconnectControl) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/status", func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Instance-ID")
		control.mu.Lock()
		fail := control.statusFailures[id]
		running := control.running[id]
		control.mu.Unlock()
		if fail {
			http.Error(w, "status unavailable", http.StatusInternalServerError)
			return
		}
		status := "stopped"
		if running {
			status = "running"
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"agent_status": status})
	})
	mux.HandleFunc("/api/v1/agent/delivery", func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Instance-ID")
		control.mu.Lock()
		fail := control.deliveryFails[id]
		sessionID := control.deliverySession[id]
		control.mu.Unlock()
		if fail {
			http.Error(w, "delivery unavailable", http.StatusInternalServerError)
			return
		}
		if sessionID == "" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"version": 2, "durable": true, "session_id": sessionID,
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func reconnectRequest(t *testing.T, server *httptest.Server, requestedID, previousID, sessionID string) (*DockerExecutor, *ExecutorCreateRequest, stubHostPortLookup) {
	t.Helper()
	host, port := parseHTTPURL(t, server.URL)
	return &DockerExecutor{logger: newTestDockerLogger()}, &ExecutorCreateRequest{
		InstanceID:          requestedID,
		PreviousExecutionID: previousID,
		TaskID:              "task-1",
		SessionID:           sessionID,
		DeliveryStreamID:    sessionID + ":g1",
	}, stubHostPortLookup{host: host, port: port}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.1
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.6
func TestDockerReconnectReconcilesPreviousJournalOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	control := newJournalReconnectControl(func(string) string { return path })
	control.addInstance("exec-old", "task-1", "session-1", 41001, false)
	control.holdJournal(t, "exec-old", path)
	server := newReconnectStatusServer(t, control)
	exec, req, docker := reconnectRequest(t, server, "exec-new", "exec-old", "session-1")

	port, reusing, instanceID, err := exec.findExistingInstance(
		context.Background(), docker, control, req, "container-1", "172.17.0.2", "exec-new", "token",
	)
	require.NoError(t, err)
	require.False(t, reusing, "a stopped owner must be disposed before a replacement is created")
	require.Equal(t, 42001, port)
	require.Equal(t, "exec-new", instanceID)
	require.Equal(t, []string{
		"GET exec-new",
		"LIST",
		"DELETE exec-old",
		"CREATE exec-new",
	}, control.recordedMethods())
	control.mu.Lock()
	_, oldPresent := control.instances["exec-old"]
	replacement := control.held["exec-new"]
	control.mu.Unlock()
	require.False(t, oldPresent, "the previous journal owner must be removed")
	require.NotNil(t, replacement, "the replacement must own the reopened journal")
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.4
func TestDockerReconnectAdoptsLivePreviousJournalOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	control := newJournalReconnectControl(func(string) string { return path })
	control.addInstance("exec-old", "task-1", "session-1", 41001, true)
	control.holdJournal(t, "exec-old", path)
	server := newReconnectStatusServer(t, control)
	exec, req, docker := reconnectRequest(t, server, "exec-new", "exec-old", "session-1")

	port, reusing, instanceID, err := exec.findExistingInstance(
		context.Background(), docker, control, req, "container-1", "172.17.0.2", "exec-new", "token",
	)
	require.NoError(t, err)
	require.True(t, reusing, "a live initialized owner must be adopted without a second journal open")
	require.Equal(t, 41001, port)
	require.Equal(t, "exec-old", instanceID, "adoption must keep the agentctl instance identity")
	require.Equal(t, []string{"GET exec-new", "LIST"}, control.recordedMethods())
	control.mu.Lock()
	defer control.mu.Unlock()
	require.Contains(t, control.held, "exec-old", "adoption must preserve the live owner's journal handle")
	require.NotContains(t, control.held, "exec-new")
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.1
func TestDockerReconnectKeepsDistinctSessionJournalOwner(t *testing.T) {
	dir := t.TempDir()
	control := newJournalReconnectControl(func(sessionID string) string {
		return filepath.Join(dir, sessionID+".bbolt")
	})
	control.addInstance("other-exec", "task-1", "other-session", 41001, false)
	control.holdJournal(t, "other-exec", filepath.Join(dir, "other-session.bbolt"))
	server := newReconnectStatusServer(t, control)
	exec, req, docker := reconnectRequest(t, server, "exec-new", "exec-old", "session-1")

	port, reusing, instanceID, err := exec.findExistingInstance(
		context.Background(), docker, control, req, "container-1", "172.17.0.2", "exec-new", "token",
	)
	require.NoError(t, err)
	require.False(t, reusing)
	require.Equal(t, 42001, port)
	require.Equal(t, "exec-new", instanceID)
	require.True(t, control.hasMethod("CREATE exec-new"))
	require.False(t, control.hasMethod("DELETE other-exec"), "a different session keeps its own journal owner")
	control.mu.Lock()
	defer control.mu.Unlock()
	require.Contains(t, control.instances, "other-exec")
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.6
func TestDockerReconnectBlocksOnUnresolvedOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	control := newJournalReconnectControl(func(string) string { return path })
	control.addInstance("stopped-exec", "task-1", "session-1", 41001, false)
	control.addInstance("unresolved-exec", "task-1", "session-1", 41002, true)
	control.holdJournal(t, "stopped-exec", path)
	control.mu.Lock()
	control.deliveryFails["unresolved-exec"] = true
	control.mu.Unlock()
	server := newReconnectStatusServer(t, control)
	exec, req, docker := reconnectRequest(t, server, "exec-new", "stopped-exec", "session-1")

	_, _, _, err := exec.findExistingInstance(
		context.Background(), docker, control, req, "container-1", "172.17.0.2", "exec-new", "token",
	)
	require.Error(t, err)
	require.ErrorIs(t, err, errReconnectOwnerBlocked)
	require.Contains(t, err.Error(), "classify agent instance")
	require.False(t, control.hasMethod("DELETE"), "an unresolved candidate must block every mutation")
	require.False(t, control.hasMethod("CREATE"), "blocked recovery must not create a replacement")
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.1
func TestDockerReconnectFailedDisposalBlocksReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	control := newJournalReconnectControl(func(string) string { return path })
	control.addInstance("exec-old", "task-1", "session-1", 41001, false)
	control.holdJournal(t, "exec-old", path)
	control.deleteErr = fmt.Errorf("teardown failed")
	server := newReconnectStatusServer(t, control)
	exec, req, docker := reconnectRequest(t, server, "exec-new", "exec-old", "session-1")

	_, _, _, err := exec.findExistingInstance(
		context.Background(), docker, control, req, "container-1", "172.17.0.2", "exec-new", "token",
	)
	require.Error(t, err)
	require.ErrorIs(t, err, errReconnectOwnerBlocked)
	require.Contains(t, err.Error(), "dispose stopped agent instance")
	require.False(t, control.hasMethod("CREATE"))
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.4
func TestJournalFailureCannotSelectLegacyReconnect(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	blocker, err := journal.Open(journal.Config{Path: path})
	require.NoError(t, err)
	t.Cleanup(func() { _ = blocker.Close() })
	control := newJournalReconnectControl(func(string) string { return path })
	control.addInstance("exec-old", "task-1", "session-1", 41001, false)
	server := newReconnectStatusServer(t, control)
	exec, req, docker := reconnectRequest(t, server, "exec-new", "", "session-1")

	_, _, _, err = exec.findExistingInstance(
		context.Background(), docker, control, req, "container-1", "172.17.0.2", "exec-new", "token",
	)
	require.Error(t, err, "a journal acquisition failure must not downgrade to legacy delivery")
	require.True(t, control.hasMethod("CREATE exec-new"))
	control.mu.Lock()
	defer control.mu.Unlock()
	require.NotContains(t, control.held, "exec-new")
}
