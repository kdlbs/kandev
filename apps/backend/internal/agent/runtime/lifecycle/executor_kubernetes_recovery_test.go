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

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

func TestKubernetesRecoveryAttachesSavedPodAndIdleInitializedInstance(t *testing.T) {
	fixture := newKubernetesRecoveryFixture(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	instances, outcomes := recoverKubernetesCandidates(t, fixture.executor, ctx, fixture.row)
	cancel()
	fixture.logRecoveryState(t, instances, outcomes)

	require.Len(t, instances, 1)
	require.Empty(t, outcomes)
	got := instances[0]
	require.Equal(t, fixture.created.InstanceID, got.InstanceID)
	require.Equal(t, fixture.created.SessionID, got.SessionID)
	require.Equal(t, fixture.created.TaskID, got.TaskID)
	require.Equal(t, fixture.created.Metadata[MetadataKeyKubernetesPodName], got.Metadata[MetadataKeyKubernetesPodName])
	require.Equal(t, fixture.created.Metadata[MetadataKeyKubernetesPodUID], got.Metadata[MetadataKeyKubernetesPodUID])
	require.Equal(t, fixture.created.Metadata[MetadataKeyKubernetesResourceInstanceID], got.Metadata[MetadataKeyKubernetesResourceInstanceID])
	require.Equal(t, fixture.created.AuthToken, got.AuthToken)
	require.Equal(t, "native-session", got.ProviderSessionID)
	require.Equal(t, map[string]string{"LIVE_ENV": "preserved"}, got.Env)
	require.Equal(t, []string{"/workspace", "/workspace/src"}, got.WorkspaceSourceRoots)
	require.NotNil(t, got.Client)
	require.Empty(t, fixture.recoveryExecs.requests, "adoption must not exec bootstrap or agent start")
	require.Equal(t, []uint16{uint16(kubeexecutor.DefaultAgentctlPort), 41001}, fixture.forwards.remotePorts())
	require.True(t, fixture.forwards.lastSession().isOpen(), "the successful forward outlives the bounded attempt context")
	require.Equal(t, 1, fixture.resources.createdPodsCount(), "recovery must not create a replacement Pod")
	require.Equal(t, 0, fixture.resources.createdPVCsCount())
	require.True(t, fixture.servers.readExactInstance(fixture.created.InstanceID))
	require.True(t, fixture.servers.statusWasRead())
	require.Zero(t, fixture.servers.instanceCreateCalls(), "adoption cannot post a create request")
	require.Empty(t, fixture.resources.deletedPods)
	require.Empty(t, fixture.resources.deletedPVCs)

	// The attempt context was canceled above. The returned transport remains live
	// until the executor or the common recovery rejection path closes it.
	status, err := got.Client.GetStatus(context.Background())
	require.NoError(t, err)
	require.True(t, status.IsAgentRunning(), "an idle initialized agent process remains adoptable")
	require.NotNil(t, got.DiscardRecovery)
	firstForward := fixture.forwards.lastSession()
	got.DiscardRecovery()
	require.True(t, firstForward.isClosed())
	fixture.executor.mu.Lock()
	require.Empty(t, fixture.executor.sessions)
	fixture.executor.mu.Unlock()

	retried, retryOutcomes := recoverKubernetesCandidates(t, fixture.executor, context.Background(), fixture.row)
	require.Len(t, retried, 1, "a discarded attempt must permit a fresh recovery")
	require.Empty(t, retryOutcomes)
	newForward := fixture.forwards.lastSession()
	require.NotSame(t, firstForward, newForward)
	require.True(t, newForward.isOpen())
	successor := &kubernetesSession{}
	fixture.executor.mu.Lock()
	fixture.executor.sessions[got.InstanceID] = successor
	fixture.executor.mu.Unlock()
	retried[0].DiscardRecovery()
	require.True(t, newForward.isClosed())
	fixture.executor.mu.Lock()
	require.Same(t, successor, fixture.executor.sessions[got.InstanceID], "discard cannot remove a replacement session")
	fixture.executor.mu.Unlock()
	require.Empty(t, fixture.resources.deletedPods, "discarding a rejected adoption only closes host transport")
	require.Equal(t, 1, fixture.resources.createdPodsCount())
}

func TestKubernetesRecoveryDoesNotRecreateMissingPod(t *testing.T) {
	fixture := newKubernetesRecoveryFixture(t, false)
	fixture.resources.mu.Lock()
	fixture.resources.pod = nil
	fixture.resources.mu.Unlock()

	instances, outcomes := recoverKubernetesCandidates(t, fixture.executor, context.Background(), fixture.row)
	fixture.logRecoveryState(t, instances, outcomes)

	require.Empty(t, instances)
	require.Equal(t, RecoveryOutcomeNoMatchingInstance, outcomes[fixture.row.SessionID])
	require.True(t, fixture.resources.getPodWasRequested(
		"kandev-agents/"+getMetadataString(fixture.created.Metadata, MetadataKeyKubernetesPodName),
	))
	require.Equal(t, 1, fixture.resources.createdPodsCount(), "a missing Pod must not be silently replaced")
	require.Zero(t, fixture.servers.totalCalls(), "missing compute must not contact a guessed control endpoint")
	require.Zero(t, fixture.servers.instanceCreateCalls())
	require.Empty(t, fixture.resources.deletedPods)
}

func TestKubernetesRecoveryRejectsReplacementPodUIDOrOwnership(t *testing.T) {
	for _, mutate := range []struct {
		name string
		fn   func(*corev1.Pod)
	}{
		{name: "uid", fn: func(pod *corev1.Pod) { pod.UID = "replacement-pod-uid" }},
		{name: "ownership label", fn: func(pod *corev1.Pod) { pod.Labels["kandev.ai/session-id"] = "another-session" }},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			fixture := newKubernetesRecoveryFixture(t, false)
			fixture.resources.mu.Lock()
			mutate.fn(fixture.resources.pod)
			fixture.resources.mu.Unlock()

			instances, outcomes := recoverKubernetesCandidates(t, fixture.executor, context.Background(), fixture.row)

			require.Empty(t, instances)
			require.Equal(t, RecoveryOutcomeUnknown, outcomes[fixture.row.SessionID])
			require.Empty(t, fixture.forwards.remotePorts(), "identity mismatch must be rejected before opening a forward")
			require.Zero(t, fixture.servers.totalCalls())
			require.Equal(t, 1, fixture.resources.createdPodsCount())
			require.Empty(t, fixture.resources.deletedPods)
		})
	}
}

func TestKubernetesRecoveryRequiresRecordedPVCUIDAndOwnership(t *testing.T) {
	for _, mutate := range []struct {
		name string
		fn   func(*corev1.PersistentVolumeClaim)
	}{
		{name: "uid", fn: func(pvc *corev1.PersistentVolumeClaim) { pvc.UID = "replacement-pvc-uid" }},
		{name: "ownership label", fn: func(pvc *corev1.PersistentVolumeClaim) { pvc.Labels["kandev.ai/session-id"] = "another-session" }},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			fixture := newKubernetesRecoveryFixture(t, true)
			fixture.resources.mu.Lock()
			mutate.fn(fixture.resources.pvc)
			fixture.resources.mu.Unlock()

			instances, outcomes := recoverKubernetesCandidates(t, fixture.executor, context.Background(), fixture.row)

			require.Empty(t, instances)
			require.Equal(t, RecoveryOutcomeUnknown, outcomes[fixture.row.SessionID])
			require.Empty(t, fixture.forwards.remotePorts())
			require.Zero(t, fixture.servers.totalCalls())
			require.Equal(t, 1, fixture.resources.createdPodsCount())
			require.Equal(t, 1, fixture.resources.createdPVCsCount())
			require.Empty(t, fixture.resources.deletedPods)
			require.Empty(t, fixture.resources.deletedPVCs)
		})
	}
}

func TestKubernetesRecoveryDoesNotRecreateMissingAgentctlInstance(t *testing.T) {
	fixture := newKubernetesRecoveryFixture(t, false)
	fixture.servers.setMissingInstance(true)
	fixture.servers.reset()

	instances, outcomes := recoverKubernetesCandidates(t, fixture.executor, context.Background(), fixture.row)

	require.Empty(t, instances)
	require.Equal(t, RecoveryOutcomeNoMatchingInstance, outcomes[fixture.row.SessionID])
	require.True(t, fixture.servers.readExactInstance(fixture.created.InstanceID))
	require.Zero(t, fixture.servers.instanceCreateCalls(), "missing agentctl instance must not be recreated")
	require.Empty(t, fixture.recoveryExecs.requests)
	require.Equal(t, 1, fixture.resources.createdPodsCount())
	require.Empty(t, fixture.resources.deletedPods)
}

func TestKubernetesRecoveryAuthFailureWithoutNonceStaysUnknown(t *testing.T) {
	fixture := newKubernetesRecoveryFixture(t, false)
	fixture.servers.setExpectedToken("rotated-token")
	fixture.servers.reset()
	fixture.row.TransientBootstrapNonce = ""

	instances, outcomes := recoverKubernetesCandidates(t, fixture.executor, context.Background(), fixture.row)

	require.Empty(t, instances)
	require.Equal(t, RecoveryOutcomeUnknown, outcomes[fixture.row.SessionID])
	require.True(t, fixture.servers.readExactInstance(fixture.created.InstanceID))
	require.Zero(t, fixture.servers.handshakeCalls(), "missing nonce must not try unauthenticated replacement")
	require.Zero(t, fixture.servers.instanceCreateCalls(), "auth failure must not create a native instance")
	require.Empty(t, fixture.recoveryExecs.requests)
	require.Empty(t, fixture.executor.sessions, "failed recovery must close its temporary forward")
	require.Empty(t, fixture.resources.deletedPods)
}

func TestKubernetesRecoveryRefreshesAuthAndRechecksSameInstance(t *testing.T) {
	fixture := newKubernetesRecoveryFixture(t, false)
	fixture.servers.setExpectedToken("rotated-token")
	fixture.servers.reset()

	instances, outcomes := recoverKubernetesCandidates(t, fixture.executor, context.Background(), fixture.row)

	require.Len(t, instances, 1)
	require.Empty(t, outcomes)
	require.Equal(t, "rotated-token", instances[0].AuthToken)
	require.Equal(t, fixture.created.InstanceID, instances[0].InstanceID)
	require.Equal(t, fixture.created.Metadata[MetadataKeyKubernetesAgentctlRemotePort], instances[0].Metadata[MetadataKeyKubernetesAgentctlRemotePort])
	require.Equal(t, 1, fixture.servers.handshakeCalls())
	require.True(t, fixture.servers.handshakeUsedNonce(fixture.created.BootstrapNonce))
	require.Equal(t, 2, fixture.servers.instanceReadCount(fixture.created.InstanceID), "refresh must recheck the exact instance after handshake")
	require.True(t, fixture.servers.statusWasRead())
	require.Zero(t, fixture.servers.instanceCreateCalls(), "credential refresh cannot create/restart the instance")
	require.Empty(t, fixture.recoveryExecs.requests)
	require.Equal(t, 1, fixture.resources.createdPodsCount())
}

func TestKubernetesRecoveryRejectsMismatchedNativeOwner(t *testing.T) {
	fixture := newKubernetesRecoveryFixture(t, false)
	fixture.servers.setWrongOwner(true)
	fixture.servers.reset()

	instances, outcomes := recoverKubernetesCandidates(t, fixture.executor, context.Background(), fixture.row)

	require.Empty(t, instances)
	require.Equal(t, RecoveryOutcomeUnknown, outcomes[fixture.row.SessionID])
	require.True(t, fixture.servers.readExactInstance(fixture.created.InstanceID))
	require.Zero(t, fixture.servers.instanceCreateCalls())
	require.Empty(t, fixture.recoveryExecs.requests)
	require.Empty(t, fixture.executor.sessions)
}

type kubernetesRecoveryFixture struct {
	servers       *kubernetesRecoveryAgentctlServers
	resources     *fakeKubernetesResources
	created       *ExecutorInstance
	row           *models.ExecutorRunning
	recoveryExecs *recordingKubernetesExec
	forwards      *recordingKubernetesForwarder
	executor      *KubernetesExecutor
}

func (f *kubernetesRecoveryFixture) logRecoveryState(
	t *testing.T,
	instances []*ExecutorInstance,
	outcomes map[string]RecoveryCandidateOutcome,
) {
	t.Helper()
	f.resources.mu.Lock()
	podRequests := append([]string(nil), f.resources.getPodRequests...)
	f.resources.mu.Unlock()
	t.Logf("recovery instances=%d outcomes=%v pod-reads=%v control=%+v forwards=%v metadata=%#v",
		len(instances), outcomes, podRequests, f.servers.controlSnapshot(), f.forwards.remotePorts(), f.row.Metadata)
}

func newKubernetesRecoveryFixture(t *testing.T, managedPVC bool) *kubernetesRecoveryFixture {
	t.Helper()
	fixture := &kubernetesRecoveryFixture{
		servers:   newKubernetesRecoveryAgentctlServers(t),
		resources: &fakeKubernetesResources{},
	}
	initialExecs := &recordingKubernetesExec{}
	initial := newFakeKubernetesExecutor(t, fixture.resources, initialExecs, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): fixture.servers.controlPort(t),
		41001:                                    fixture.servers.instancePort(t),
	})
	request := validKubernetesCreateRequest()
	if managedPVC {
		setManagedKubernetesWorkspace(request)
	}
	created, err := initial.CreateInstance(context.Background(), request)
	require.NoError(t, err)
	fixture.created = created
	persistedMetadata := cloneKubernetesMetadata(request.Metadata)
	for key, value := range created.Metadata {
		persistedMetadata[key] = value
	}
	fixture.servers.reset()
	initialExecs.mu.Lock()
	initialExecs.requests = nil
	initialExecs.mu.Unlock()
	fixture.row = &models.ExecutorRunning{
		ID: "inventory-row", AgentExecutionID: created.InstanceID, SessionID: created.SessionID, TaskID: created.TaskID,
		Runtime: agentruntime.RuntimeKubernetes, TransientAuthToken: created.AuthToken,
		TransientBootstrapNonce: created.BootstrapNonce,
		Metadata:                persistedMetadata,
	}
	fixture.recoveryExecs = &recordingKubernetesExec{}
	fixture.forwards = &recordingKubernetesForwarder{localPorts: map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): fixture.servers.controlPort(t),
		41001:                                    fixture.servers.instancePort(t),
	}}
	fixture.executor = newFakeKubernetesExecutorWithForwarder(fixture.resources, fixture.recoveryExecs, fixture.forwards)
	t.Cleanup(func() { _ = fixture.executor.Close() })
	return fixture
}

func recoverKubernetesCandidates(
	t *testing.T,
	exec *KubernetesExecutor,
	ctx context.Context,
	row *models.ExecutorRunning,
) ([]*ExecutorInstance, map[string]RecoveryCandidateOutcome) {
	t.Helper()
	recovery, ok := any(exec).(DetailedRecoveryBackend)
	require.True(t, ok, "Kubernetes recovery must preserve per-candidate uncertainty")
	instances, outcomes, err := recovery.RecoverInstancesDetailed(ctx, []*models.ExecutorRunning{row})
	require.NoError(t, err)
	return instances, outcomes
}

type kubernetesRecoveryAgentctlServers struct {
	mu               sync.Mutex
	control          *httptest.Server
	instance         *httptest.Server
	controlRequests  []kubernetesRecoveryHTTPRequest
	instanceRequests []kubernetesRecoveryHTTPRequest
	instanceID       string
	missingInstance  bool
	wrongOwner       bool
	expectedToken    string
	handshakeToken   string
}

type kubernetesRecoveryHTTPRequest struct {
	method string
	path   string
	token  string
	nonce  string
}

func newKubernetesRecoveryAgentctlServers(t *testing.T) *kubernetesRecoveryAgentctlServers {
	t.Helper()
	servers := &kubernetesRecoveryAgentctlServers{expectedToken: "handshake-token", handshakeToken: "handshake-token"}
	servers.control = httptest.NewServer(http.HandlerFunc(servers.serveControl))
	servers.instance = httptest.NewServer(http.HandlerFunc(servers.serveInstance))
	t.Cleanup(servers.control.Close)
	t.Cleanup(servers.instance.Close)
	return servers
}

func (s *kubernetesRecoveryAgentctlServers) serveControl(w http.ResponseWriter, request *http.Request) {
	seen := kubernetesRecoveryHTTPRequest{method: request.Method, path: request.URL.Path, token: recoveryRequestToken(request)}
	if request.Method == http.MethodPost && request.URL.Path == "/auth/handshake" {
		var body map[string]string
		_ = json.NewDecoder(request.Body).Decode(&body)
		seen.nonce = body["nonce"]
	}
	s.mu.Lock()
	s.controlRequests = append(s.controlRequests, seen)
	if request.Method == http.MethodPost && request.URL.Path == "/api/v1/instances" {
		var body agentctl.CreateInstanceRequest
		_ = json.NewDecoder(request.Body).Decode(&body)
		s.instanceID = body.ID
	}
	instanceID := s.instanceID
	missing, wrongOwner, expectedToken, handshakeToken := s.missingInstance, s.wrongOwner, s.expectedToken, s.handshakeToken
	s.mu.Unlock()

	switch {
	case request.Method == http.MethodGet && request.URL.Path == "/health":
		w.WriteHeader(http.StatusOK)
	case request.Method == http.MethodPost && request.URL.Path == "/auth/handshake":
		if seen.nonce == "" {
			http.Error(w, `{"error":"nonce required"}`, http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"token": handshakeToken})
	case request.Method == http.MethodPost && request.URL.Path == "/api/v1/instances":
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": instanceID, "port": 41001})
	case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/api/v1/instances/"):
		if expectedToken != "" && seen.token != expectedToken {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		id := strings.TrimPrefix(request.URL.Path, "/api/v1/instances/")
		if missing || id != instanceID {
			http.NotFound(w, request)
			return
		}
		sessionID := "session-1"
		if wrongOwner {
			sessionID = "another-session"
		}
		_ = json.NewEncoder(w).Encode(agentctl.InstanceInfo{
			ID: id, Port: 41001, WorkspacePath: kubernetesWorkspacePath,
			SessionID: sessionID, TaskID: "task-1", ProviderSessionID: "native-session",
			Env:                  map[string]string{"LIVE_ENV": "preserved"},
			WorkspaceSourceRoots: []string{"/workspace", "/workspace/src"},
		})
	default:
		http.NotFound(w, request)
	}
}

func (s *kubernetesRecoveryAgentctlServers) serveInstance(w http.ResponseWriter, request *http.Request) {
	seen := kubernetesRecoveryHTTPRequest{method: request.Method, path: request.URL.Path, token: recoveryRequestToken(request)}
	s.mu.Lock()
	s.instanceRequests = append(s.instanceRequests, seen)
	expectedToken := s.expectedToken
	s.mu.Unlock()
	if request.Method == http.MethodGet && request.URL.Path == "/health" {
		w.WriteHeader(http.StatusOK)
		return
	}
	if request.Method == http.MethodGet && request.URL.Path == "/api/v1/status" {
		if expectedToken != "" && seen.token != expectedToken {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"agent_status": "running"})
		return
	}
	http.NotFound(w, request)
}

func recoveryRequestToken(request *http.Request) string {
	return strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
}

func (s *kubernetesRecoveryAgentctlServers) controlPort(t *testing.T) uint16 {
	return kubernetesRecoveryPort(t, s.control)
}

func (s *kubernetesRecoveryAgentctlServers) instancePort(t *testing.T) uint16 {
	return kubernetesRecoveryPort(t, s.instance)
}

func kubernetesRecoveryPort(t *testing.T, server *httptest.Server) uint16 {
	t.Helper()
	_, rawPort, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	require.NoError(t, err)
	port, err := strconv.Atoi(rawPort)
	require.NoError(t, err)
	return uint16(port)
}

func (s *kubernetesRecoveryAgentctlServers) reset() {
	s.mu.Lock()
	s.controlRequests = nil
	s.instanceRequests = nil
	s.mu.Unlock()
}

func (s *kubernetesRecoveryAgentctlServers) setMissingInstance(missing bool) {
	s.mu.Lock()
	s.missingInstance = missing
	s.mu.Unlock()
}

func (s *kubernetesRecoveryAgentctlServers) setWrongOwner(wrong bool) {
	s.mu.Lock()
	s.wrongOwner = wrong
	s.mu.Unlock()
}

func (s *kubernetesRecoveryAgentctlServers) setExpectedToken(token string) {
	s.mu.Lock()
	s.expectedToken = token
	s.handshakeToken = token
	s.mu.Unlock()
}

func (s *kubernetesRecoveryAgentctlServers) controlSnapshot() []kubernetesRecoveryHTTPRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]kubernetesRecoveryHTTPRequest(nil), s.controlRequests...)
}

func (s *kubernetesRecoveryAgentctlServers) totalCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.controlRequests) + len(s.instanceRequests)
}

func (s *kubernetesRecoveryAgentctlServers) readExactInstance(id string) bool {
	for _, request := range s.controlSnapshot() {
		if request.method == http.MethodGet && request.path == "/api/v1/instances/"+id {
			return true
		}
	}
	return false
}

func (s *kubernetesRecoveryAgentctlServers) instanceCreateCalls() int {
	count := 0
	for _, request := range s.controlSnapshot() {
		if request.method == http.MethodPost && request.path == "/api/v1/instances" {
			count++
		}
	}
	return count
}

func (s *kubernetesRecoveryAgentctlServers) handshakeCalls() int {
	count := 0
	for _, request := range s.controlSnapshot() {
		if request.method == http.MethodPost && request.path == "/auth/handshake" {
			count++
		}
	}
	return count
}

func (s *kubernetesRecoveryAgentctlServers) handshakeUsedNonce(nonce string) bool {
	for _, request := range s.controlSnapshot() {
		if request.method == http.MethodPost && request.path == "/auth/handshake" && request.nonce == nonce {
			return true
		}
	}
	return false
}

func (s *kubernetesRecoveryAgentctlServers) instanceReadCount(id string) int {
	count := 0
	for _, request := range s.controlSnapshot() {
		if request.method == http.MethodGet && request.path == "/api/v1/instances/"+id {
			count++
		}
	}
	return count
}

func (s *kubernetesRecoveryAgentctlServers) statusWasRead() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, request := range s.instanceRequests {
		if request.method == http.MethodGet && request.path == "/api/v1/status" {
			return true
		}
	}
	return false
}

func (r *fakeKubernetesResources) createdPodsCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.createdPods)
}

func (r *fakeKubernetesResources) createdPVCsCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.createdPVCs)
}

func (r *fakeKubernetesResources) getPodWasRequested(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, request := range r.getPodRequests {
		if request == name {
			return true
		}
	}
	return false
}

func (s *fakeKubernetesForwardSession) isOpen() bool {
	return !s.isClosed()
}
