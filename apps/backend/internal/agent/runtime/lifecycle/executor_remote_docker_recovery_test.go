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
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/docker"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

func TestIsAgentctlAuthErrorClassifiesHealthUnauthorizedOnly(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "health unauthorized", err: errors.New("health check failed: 401"), want: true},
		{name: "status unauthorized", err: errors.New("request failed with status 401"), want: true},
		{name: "service unavailable", err: errors.New("health check failed: 503"), want: false},
		{name: "timeout", err: context.DeadlineExceeded, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, isAgentctlAuthError(test.err))
		})
	}
}

func TestRemoteDockerRecoveryAttachesOnlyTheRecordedLiveContainerAndInstance(t *testing.T) {
	fixture := newRemoteDockerRecoveryFixture(t, "running", remoteDockerRecoveryInstance{})
	exec := fixture.executor

	instances, err := exec.RecoverInstances(context.Background(), []*models.ExecutorRunning{
		fixture.record("saved-token", ""),
	})

	require.NoError(t, err)
	require.Len(t, instances, 1)
	got := instances[0]
	require.Equal(t, "execution-1", got.InstanceID)
	require.Equal(t, "session-1", got.SessionID)
	require.Equal(t, "task-1", got.TaskID)
	require.Equal(t, "container-preserved", got.ContainerID)
	require.NotNil(t, got.Client)
	require.Equal(t, "saved-token", got.AuthToken)
	require.True(t, fixture.statusWasChecked())
	require.True(t, fixture.controlOnlyReadRecordedInstance())
	require.True(t, fixture.engineOnlyInspectedRecordedContainer(), "engine requests=%+v", fixture.engine.requests())
	require.True(t, fixture.connectContextHadDeadline, "remote recovery bounds the host connection attempt")
	require.Zero(t, fixture.createInstanceCalls(), "recovery must not create an agentctl instance")
	require.Zero(t, fixture.engineMutationCalls(), "recovery must not start or mutate the Docker container")
	status, err := got.Client.GetStatus(context.Background())
	require.NoError(t, err, "the authenticated client remains usable after the bounded attempt returns")
	require.True(t, status.IsAgentRunning())

	exec.mu.Lock()
	trackedSession := exec.sessions[got.InstanceID]
	exec.mu.Unlock()
	require.Same(t, fixture.remoteSession, trackedSession, "the successful SSH-side transport stays tracked")
	require.NotNil(t, got.DiscardRecovery)
	successor := &remoteDockerSession{}
	successorTarget := map[string]interface{}{MetadataKeySSHHost: "successor-host"}
	exec.mu.Lock()
	exec.sessions[got.InstanceID] = successor
	exec.targets[got.InstanceID] = successorTarget
	exec.mu.Unlock()
	got.DiscardRecovery()
	exec.mu.Lock()
	require.Same(t, successor, exec.sessions[got.InstanceID], "discard cannot remove a replacement session")
	require.Equal(t, successorTarget, exec.targets[got.InstanceID], "discard cannot remove a replacement target")
	exec.mu.Unlock()
	require.Nil(t, fixture.remoteSession.takeAgentctlClient(), "discard closes the old attempt's authenticated client")
}

func TestRemoteDockerRecoveryDoesNotStartStoppedContainer(t *testing.T) {
	fixture := newRemoteDockerRecoveryFixture(t, "exited", remoteDockerRecoveryInstance{})

	instances, outcomes := recoverRemoteDockerCandidates(t, fixture.executor, []*models.ExecutorRunning{
		fixture.record("saved-token", ""),
	})
	require.Empty(t, instances)
	require.Equal(t, RecoveryOutcomeNoMatchingInstance, outcomes["session-1"])
	require.True(t, fixture.engineOnlyInspectedRecordedContainer())
	require.Empty(t, fixture.controlRequests(), "a stopped container must not be started to inspect agentctl")
	require.Zero(t, fixture.engineMutationCalls(), "startup must preserve, not restart, the stopped container")
}

func TestRemoteDockerRecoveryDoesNotRecreateMissingAgentctlInstance(t *testing.T) {
	fixture := newRemoteDockerRecoveryFixture(t, "running", remoteDockerRecoveryInstance{missing: true})

	instances, outcomes := recoverRemoteDockerCandidates(t, fixture.executor, []*models.ExecutorRunning{
		fixture.record("saved-token", ""),
	})
	require.Empty(t, instances)
	require.Equal(t, RecoveryOutcomeNoMatchingInstance, outcomes["session-1"])
	require.True(t, fixture.controlReadWasFor("execution-1"))
	require.Zero(t, fixture.createInstanceCalls(), "a missing instance must not be recreated")
	require.Zero(t, fixture.engineMutationCalls(), "a missing agentctl instance must not mutate its container")
}

func TestRemoteDockerRecoveryAuthFailureWithoutNonceStaysUnknown(t *testing.T) {
	fixture := newRemoteDockerRecoveryFixture(t, "running", remoteDockerRecoveryInstance{requiredToken: "rotated-token"})

	instances, outcomes := recoverRemoteDockerCandidates(t, fixture.executor, []*models.ExecutorRunning{
		fixture.record("stale-token", ""),
	})
	require.Empty(t, instances)
	require.Equal(t, RecoveryOutcomeUnknown, outcomes["session-1"])
	require.True(t, fixture.controlReadWasFor("execution-1"))
	require.Zero(t, fixture.handshakeCalls(), "without the saved nonce the adapter cannot refresh auth")
	require.Zero(t, fixture.createInstanceCalls(), "auth failure must never fall through to instance creation")
	require.Zero(t, fixture.engineMutationCalls())
	require.Empty(t, fixture.executor.sessions, "failed recovery must close its temporary host transport")
}

func TestRemoteDockerRecoveryRefreshesAuthOnSameSavedInstance(t *testing.T) {
	fixture := newRemoteDockerRecoveryFixture(t, "running", remoteDockerRecoveryInstance{requiredToken: "rotated-token"})
	fixture.nonce = "saved-bootstrap-nonce"

	instances, outcomes := recoverRemoteDockerCandidates(t, fixture.executor, []*models.ExecutorRunning{
		fixture.record("stale-token", fixture.nonce),
	})
	require.Len(t, instances, 1)
	require.Empty(t, outcomes)
	require.Equal(t, "rotated-token", instances[0].AuthToken)
	require.Equal(t, "execution-1", instances[0].InstanceID)
	require.Equal(t, "container-preserved", instances[0].ContainerID)
	require.Equal(t, 1, fixture.handshakeCalls(), "control requests=%+v instance requests=%+v",
		fixture.control.snapshot(), fixture.instance.snapshot())
	require.True(t, fixture.handshakeUsedNonce(fixture.nonce))
	require.True(t, fixture.controlReadWasFor("execution-1"))
	require.True(t, fixture.statusWasChecked())
	require.Zero(t, fixture.createInstanceCalls(), "credential refresh may only reattach the exact recorded instance")
	require.Zero(t, fixture.engineMutationCalls())
}

func TestRemoteDockerRecoveryRejectsChangedInstancePortDuringAuthRefresh(t *testing.T) {
	fixture := newRemoteDockerRecoveryFixture(t, "running", remoteDockerRecoveryInstance{
		instanceRequiredToken: "rotated-token",
		refreshedPort:         41002,
	})
	fixture.nonce = "saved-bootstrap-nonce"

	instances, outcomes := recoverRemoteDockerCandidates(t, fixture.executor, []*models.ExecutorRunning{
		fixture.record("stale-token", fixture.nonce),
	})

	require.Empty(t, instances)
	require.Equal(t, RecoveryOutcomeUnknown, outcomes["session-1"])
	require.Equal(t, 1, fixture.handshakeCalls(), "control requests=%+v instance requests=%+v",
		fixture.control.snapshot(), fixture.instance.snapshot())
	require.True(t, fixture.handshakeUsedNonce(fixture.nonce))
	require.False(t, fixture.statusWasChecked(), "a changed controller port cannot reuse the stale endpoint")
	require.Zero(t, fixture.createInstanceCalls())
	require.Zero(t, fixture.engineMutationCalls())
	require.Empty(t, fixture.executor.sessions)
}

func recoverRemoteDockerCandidates(
	t *testing.T,
	exec *RemoteDockerExecutor,
	records []*models.ExecutorRunning,
) ([]*ExecutorInstance, map[string]RecoveryCandidateOutcome) {
	t.Helper()
	recovery, ok := any(exec).(DetailedRecoveryBackend)
	require.True(t, ok, "remote Docker recovery must preserve per-candidate uncertainty")
	instances, outcomes, err := recovery.RecoverInstancesDetailed(context.Background(), records)
	require.NoError(t, err)
	return instances, outcomes
}

type remoteDockerRecoveryInstance struct {
	missing               bool
	requiredToken         string
	instanceRequiredToken string
	refreshedPort         int
	wrongOwner            bool
}

type remoteDockerRecoveryFixture struct {
	executor                  *RemoteDockerExecutor
	remoteSession             *remoteDockerSession
	engine                    *remoteDockerRecoveryEngine
	control                   *remoteDockerRecoveryHTTP
	instance                  *remoteDockerRecoveryHTTP
	nonce                     string
	containerState            string
	connectContextHadDeadline bool
}

func newRemoteDockerRecoveryFixture(
	t *testing.T,
	containerState string,
	instance remoteDockerRecoveryInstance,
) *remoteDockerRecoveryFixture {
	t.Helper()
	fixture := &remoteDockerRecoveryFixture{containerState: containerState}
	fixture.control = newRemoteDockerRecoveryHTTP(t, true, instance)
	fixture.instance = newRemoteDockerRecoveryHTTP(t, false, instance)
	controlPort := fixture.control.port(t)
	instancePort := fixture.instance.port(t)
	fixture.engine = newRemoteDockerRecoveryEngine(t, containerState)
	dockerClient, err := docker.NewRemoteClient(docker.RemoteTransport{
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", fixture.engine.address())
		},
	}, dialerTestLogger(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = dockerClient.Close() })

	fixture.executor = NewRemoteDockerExecutor(dialerTestLogger(t))
	fixture.remoteSession = &remoteDockerSession{
		dockerClient: dockerClient,
		endpoints: remoteDockerRecoveryEndpoints{
			AgentCtlPort: controlPort,
			41001:        instancePort,
			41002:        instancePort,
		},
	}
	fixture.executor.connect = func(ctx context.Context, req *ExecutorCreateRequest) (*remoteDockerSession, error) {
		_, fixture.connectContextHadDeadline = ctx.Deadline()
		if req.InstanceID != "execution-1" || req.SessionID != "session-1" || req.TaskID != "task-1" {
			return nil, fmt.Errorf("unexpected saved identity in recovery request: %#v", req)
		}
		return fixture.remoteSession, nil
	}
	fixture.executor.watchTransport = func(string, *remoteDockerSession) {}
	fixture.executor.reconnect = func(context.Context, *remoteDockerSession, *ExecutorCreateRequest) (*ExecutorInstance, error) {
		t.Fatal("startup recovery called the resume reconnect path")
		return nil, nil
	}
	fixture.executor.launch = func(context.Context, *remoteDockerSession, *ExecutorCreateRequest) (*ExecutorInstance, error) {
		t.Fatal("startup recovery attempted to launch a container")
		return nil, nil
	}
	t.Cleanup(func() { _ = fixture.executor.Close() })
	return fixture
}

func (f *remoteDockerRecoveryFixture) record(token, nonce string) *models.ExecutorRunning {
	return &models.ExecutorRunning{
		ID: "inventory-row", AgentExecutionID: "execution-1", SessionID: "session-1", TaskID: "task-1",
		Runtime: agentruntime.RuntimeRemoteDocker, ContainerID: "container-preserved",
		TransientAuthToken: token, TransientBootstrapNonce: nonce,
		Metadata: map[string]interface{}{
			MetadataKeySSHHost:            "build-box",
			MetadataKeySSHHostFingerprint: "SHA256:pinned",
			MetadataKeyContainerID:        "container-preserved",
		},
	}
}

type remoteDockerRecoveryEndpoints map[int]uint16

func (e remoteDockerRecoveryEndpoints) Resolve(
	_ context.Context, _ string, containerPort int, _ string,
) (string, int, error) {
	port := e[containerPort]
	if port == 0 {
		return "", 0, fmt.Errorf("no test endpoint for container port %d", containerPort)
	}
	return "127.0.0.1", int(port), nil
}

func (remoteDockerRecoveryEndpoints) Close() error { return nil }

type remoteDockerRecoveryEngine struct {
	server *httptest.Server
	mu     sync.Mutex
	seen   []remoteDockerEngineRequest
}

type remoteDockerEngineRequest struct {
	method string
	path   string
}

func newRemoteDockerRecoveryEngine(t *testing.T, state string) *remoteDockerRecoveryEngine {
	t.Helper()
	engine := &remoteDockerRecoveryEngine{}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, request *http.Request) {
		engine.mu.Lock()
		engine.seen = append(engine.seen, remoteDockerEngineRequest{method: request.Method, path: request.URL.Path})
		engine.mu.Unlock()
		w.Header().Set("Api-Version", "1.51")
		w.Header().Set("Ostype", "linux")
		if request.Method == http.MethodHead && strings.HasSuffix(request.URL.Path, "/_ping") {
			w.WriteHeader(http.StatusOK)
			return
		}
		if strings.HasSuffix(request.URL.Path, "/version") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ApiVersion":"1.51","Arch":"amd64"}`))
			return
		}
		if strings.HasSuffix(request.URL.Path, "/containers/container-preserved/json") && request.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"Id": "container-preserved", "Name": "/kandev-preserved",
				"Config": map[string]interface{}{"Image": "example.test/agent:latest"},
				"State":  map[string]interface{}{"Status": state},
			})
			return
		}
		http.NotFound(w, request)
	})
	engine.server = httptest.NewServer(mux)
	t.Cleanup(engine.server.Close)
	return engine
}

func (e *remoteDockerRecoveryEngine) address() string {
	return strings.TrimPrefix(e.server.URL, "http://")
}

func (e *remoteDockerRecoveryEngine) mutationCalls() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	count := 0
	for _, request := range e.seen {
		if request.method != http.MethodGet && !isDockerEnginePing(request) {
			count++
		}
	}
	return count
}

type remoteDockerRecoveryHTTP struct {
	server  *httptest.Server
	mu      sync.Mutex
	seen    []remoteDockerHTTPRequest
	control bool
	config  remoteDockerRecoveryInstance
	nonce   string
}

type remoteDockerHTTPRequest struct {
	method string
	path   string
	token  string
	nonce  string
}

func newRemoteDockerRecoveryHTTP(
	t *testing.T,
	control bool,
	config remoteDockerRecoveryInstance,
) *remoteDockerRecoveryHTTP {
	t.Helper()
	httpFixture := &remoteDockerRecoveryHTTP{control: control, config: config}
	mux := http.NewServeMux()
	mux.HandleFunc("/", httpFixture.serve)
	httpFixture.server = httptest.NewServer(mux)
	t.Cleanup(httpFixture.server.Close)
	return httpFixture
}

func (h *remoteDockerRecoveryHTTP) serve(w http.ResponseWriter, request *http.Request) {
	seen, refreshed := h.recordRequest(request)
	if h.serveHealth(w, request, seen) {
		return
	}
	if h.control && h.serveControl(w, request, seen, refreshed) {
		return
	}
	if !h.control && h.serveInstance(w, request, seen) {
		return
	}
	http.NotFound(w, request)
}

func (h *remoteDockerRecoveryHTTP) recordRequest(request *http.Request) (remoteDockerHTTPRequest, bool) {
	seen := remoteDockerHTTPRequest{
		method: request.Method, path: request.URL.Path,
		token: strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer "),
	}
	if request.URL.Path == "/auth/handshake" && request.Method == http.MethodPost {
		var body map[string]string
		_ = json.NewDecoder(request.Body).Decode(&body)
		seen.nonce = body["nonce"]
	}
	h.mu.Lock()
	h.seen = append(h.seen, seen)
	if h.control && request.URL.Path == "/auth/handshake" && request.Method == http.MethodPost {
		h.nonce = seen.nonce
	}
	refreshed := h.nonce != ""
	h.mu.Unlock()
	return seen, refreshed
}

func (h *remoteDockerRecoveryHTTP) serveHealth(
	w http.ResponseWriter,
	request *http.Request,
	seen remoteDockerHTTPRequest,
) bool {
	if request.URL.Path != "/health" || request.Method != http.MethodGet {
		return false
	}
	if !h.control && !h.authorizedInstanceRequest(w, seen) {
		return true
	}
	w.WriteHeader(http.StatusOK)
	return true
}

func (h *remoteDockerRecoveryHTTP) serveControl(
	w http.ResponseWriter,
	request *http.Request,
	seen remoteDockerHTTPRequest,
	refreshed bool,
) bool {
	if request.Method == http.MethodPost && request.URL.Path == "/auth/handshake" {
		if seen.nonce == "" {
			http.Error(w, `{"error":"nonce required"}`, http.StatusUnauthorized)
			return true
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "rotated-token"})
		return true
	}
	if request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/api/v1/instances/") {
		h.serveControlInstance(w, request, seen, refreshed)
		return true
	}
	if request.Method == http.MethodPost && request.URL.Path == "/api/v1/instances" {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "execution-1", "port": 41001})
		return true
	}
	return false
}

func (h *remoteDockerRecoveryHTTP) serveControlInstance(
	w http.ResponseWriter,
	request *http.Request,
	seen remoteDockerHTTPRequest,
	refreshed bool,
) {
	if h.config.requiredToken != "" && seen.token != h.config.requiredToken {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	if h.config.missing {
		http.NotFound(w, request)
		return
	}
	id := strings.TrimPrefix(request.URL.Path, "/api/v1/instances/")
	ownerSession, ownerTask := "session-1", "task-1"
	if h.config.wrongOwner {
		ownerSession = "another-session"
	}
	port := 41001
	if refreshed && h.config.refreshedPort > 0 {
		port = h.config.refreshedPort
	}
	_ = json.NewEncoder(w).Encode(agentctl.InstanceInfo{
		ID: id, Port: port, SessionID: ownerSession, TaskID: ownerTask,
	})
}

func (h *remoteDockerRecoveryHTTP) serveInstance(
	w http.ResponseWriter,
	request *http.Request,
	seen remoteDockerHTTPRequest,
) bool {
	if request.Method != http.MethodGet || request.URL.Path != "/api/v1/status" {
		return false
	}
	if !h.authorizedInstanceRequest(w, seen) {
		return true
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"agent_status": "running"})
	return true
}

func (h *remoteDockerRecoveryHTTP) authorizedInstanceRequest(
	w http.ResponseWriter,
	seen remoteDockerHTTPRequest,
) bool {
	requiredToken := h.config.requiredToken
	if h.config.instanceRequiredToken != "" {
		requiredToken = h.config.instanceRequiredToken
	}
	if requiredToken != "" && seen.token != requiredToken {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return false
	}
	return true
}

func (h *remoteDockerRecoveryHTTP) port(t *testing.T) uint16 {
	t.Helper()
	_, rawPort, err := net.SplitHostPort(strings.TrimPrefix(h.server.URL, "http://"))
	require.NoError(t, err)
	port, err := strconv.Atoi(rawPort)
	require.NoError(t, err)
	return uint16(port)
}

func (h *remoteDockerRecoveryHTTP) snapshot() []remoteDockerHTTPRequest {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]remoteDockerHTTPRequest(nil), h.seen...)
}

func (f *remoteDockerRecoveryFixture) controlRequests() []remoteDockerHTTPRequest {
	return f.control.snapshot()
}

func (f *remoteDockerRecoveryFixture) controlReadWasFor(id string) bool {
	for _, request := range f.control.snapshot() {
		if request.method == http.MethodGet && request.path == "/api/v1/instances/"+id {
			return true
		}
	}
	return false
}

func (f *remoteDockerRecoveryFixture) controlOnlyReadRecordedInstance() bool {
	seenInstance := false
	for _, request := range f.control.snapshot() {
		if request.path == "/health" && request.method == http.MethodGet {
			continue
		}
		if request.method == http.MethodGet && request.path == "/api/v1/instances/execution-1" {
			seenInstance = true
			continue
		}
		if request.method == http.MethodPost && request.path == "/auth/handshake" && request.nonce != "" {
			continue
		}
		return false
	}
	return seenInstance
}

func (f *remoteDockerRecoveryFixture) createInstanceCalls() int {
	count := 0
	for _, request := range f.control.snapshot() {
		if request.method == http.MethodPost && request.path == "/api/v1/instances" {
			count++
		}
	}
	return count
}

func (f *remoteDockerRecoveryFixture) handshakeCalls() int {
	count := 0
	for _, request := range f.control.snapshot() {
		if request.method == http.MethodPost && request.path == "/auth/handshake" {
			count++
		}
	}
	return count
}

func (f *remoteDockerRecoveryFixture) handshakeUsedNonce(nonce string) bool {
	for _, request := range f.control.snapshot() {
		if request.method == http.MethodPost && request.path == "/auth/handshake" && request.nonce == nonce {
			return true
		}
	}
	return false
}

func (f *remoteDockerRecoveryFixture) statusWasChecked() bool {
	for _, request := range f.instance.snapshot() {
		if request.method == http.MethodGet && request.path == "/api/v1/status" {
			return true
		}
	}
	return false
}

func (f *remoteDockerRecoveryFixture) engineOnlyInspectedRecordedContainer() bool {
	for _, request := range f.engine.requests() {
		if isDockerEnginePing(request) {
			continue
		}
		if request.method != http.MethodGet || !strings.HasSuffix(request.path, "/containers/container-preserved/json") &&
			!strings.HasSuffix(request.path, "/version") {
			return false
		}
	}
	return true
}

func isDockerEnginePing(request remoteDockerEngineRequest) bool {
	return request.method == http.MethodHead && strings.HasSuffix(request.path, "/_ping")
}

func (f *remoteDockerRecoveryFixture) engineMutationCalls() int { return f.engine.mutationCalls() }

func (e *remoteDockerRecoveryEngine) requests() []remoteDockerEngineRequest {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]remoteDockerEngineRequest(nil), e.seen...)
}
