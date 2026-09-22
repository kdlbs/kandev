package lifecycle

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type taskPodControl struct {
	mu           sync.Mutex
	active       map[string]agentctl.CreateInstanceRequest
	created      []agentctl.CreateInstanceRequest
	handshakes   int
	token        string
	failID       string
	beforeDelete func()
}

func (c *taskPodControl) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete && c.beforeDelete != nil {
		c.beforeDelete()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if r.URL.Path == "/health" {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.URL.Path == "/auth/handshake" {
		c.handshakes++
		if c.handshakes > 1 {
			http.Error(w, "nonce already used", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"token": c.token})
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+c.token {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		return
	}
	if r.URL.Path == "/api/v1/instances" {
		var request agentctl.CreateInstanceRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if request.ID == c.failID {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "injected create failure"})
			return
		}
		c.created = append(c.created, request)
		c.active[request.ID] = request
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(agentctl.CreateInstanceResponse{ID: request.ID, Port: 41001})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/instances/")
	request, exists := c.active[id]
	if !exists {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodDelete {
		delete(c.active, id)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	_ = json.NewEncoder(w).Encode(agentctl.InstanceInfo{ID: id, Port: 41001, WorkspacePath: request.WorkspacePath})
}

func (c *taskPodControl) snapshot() ([]agentctl.CreateInstanceRequest, map[string]agentctl.CreateInstanceRequest, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	active := make(map[string]agentctl.CreateInstanceRequest, len(c.active))
	for k, v := range c.active {
		active[k] = v
	}
	return append([]agentctl.CreateInstanceRequest(nil), c.created...), active, c.handshakes
}

func (c *taskPodControl) reboot() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.active = map[string]agentctl.CreateInstanceRequest{}
	c.handshakes = 0
	c.token += "-restarted"
}

type taskPodFixture struct {
	runtime     *KubernetesExecutor
	manager     *Manager
	repo        *tasksqlite.Repository
	database    *sqlx.DB
	resources   *fakeKubernetesResources
	execs       *recordingKubernetesExec
	control     *taskPodControl
	secretStore secrets.SecretStore
	port        uint16
}

func newTaskPodFixture(t *testing.T) *taskPodFixture {
	t.Helper()
	repo, database := runOwnerTestRepository(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task-1", Title: "shared pod"}))
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: "environment-1", TaskID: "task-1", ExecutorType: "k8s", ExecutorID: "executor-1", ExecutorProfileID: "profile-1", Status: models.TaskEnvironmentStatusCreating, OwnershipGeneration: 1}))
	require.NoError(t, repo.CreateExecutor(ctx, &models.Executor{ID: "executor-1", Name: "cluster", Type: models.ExecutorTypeKubernetes, Config: map[string]string{"auth_mode": "in_cluster", "namespace": "kandev-agents", "request_timeout_seconds": "30"}}))
	control := &taskPodControl{active: map[string]agentctl.CreateInstanceRequest{}, token: "shared-control-token"}
	server := httptest.NewServer(control)
	t.Cleanup(server.Close)
	_, raw, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(raw)
	require.NoError(t, err)
	f := &taskPodFixture{repo: repo, database: database, resources: &fakeKubernetesResources{}, execs: &recordingKubernetesExec{}, control: control, secretStore: newInMemorySecretStore(), port: uint16(port)}
	f.restartBackend(t)
	return f
}

func (f *taskPodFixture) restartBackend(t *testing.T) {
	t.Helper()
	if f.runtime != nil {
		require.NoError(t, f.runtime.Close())
	}
	f.runtime = newFakeKubernetesExecutor(t, f.resources, f.execs, map[uint16]uint16{uint16(kubeexecutor.DefaultAgentctlPort): f.port, 41001: f.port})
	current := f.runtime
	t.Cleanup(func() { require.NoError(t, current.Close()) })
	f.manager = newTestManager(t)
	f.manager.executorRegistry = NewExecutorRegistry(f.manager.logger)
	f.manager.executorRegistry.Register(f.runtime)
	f.manager.SetExecutorRunningWriter(f.repo)
	f.manager.SetSecretStore(f.secretStore)
}

func taskPodRequest(number int) *ExecutorCreateRequest {
	req := validKubernetesCreateRequest()
	setManagedKubernetesWorkspace(req)
	req.InstanceID = "instance-" + strconv.Itoa(number)
	req.SessionID = "session-" + strconv.Itoa(number)
	req.Env = map[string]string{"SESSION_CREDENTIAL": "credential-" + strconv.Itoa(number)}
	req.WorkspaceReuseRequired = number > 1
	return req
}

func (f *taskPodFixture) launch(t *testing.T, number int) *ExecutorInstance {
	t.Helper()
	instance, err := f.runtime.CreateInstance(context.Background(), taskPodRequest(number))
	require.NoError(t, err)
	if number == 1 {
		_, err = f.database.Exec(`UPDATE task_environments SET status = 'ready' WHERE id = 'environment-1'`)
		require.NoError(t, err)
	}
	return instance
}
