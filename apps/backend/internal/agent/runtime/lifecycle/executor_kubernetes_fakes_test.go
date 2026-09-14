package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/stretchr/testify/require"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
)

func setManagedKubernetesWorkspace(req *ExecutorCreateRequest) {
	req.Metadata[MetadataKeyKubernetesWorkspaceMode] = "managed_pvc"
	req.Metadata[MetadataKeyKubernetesWorkspaceSize] = "1Gi"
	req.Metadata[MetadataKeyKubernetesWorkspaceAccessModes] = `["ReadWriteOnce"]`
}

func kubernetesReconnectRequest(created *ExecutorInstance) *ExecutorCreateRequest {
	req := validKubernetesCreateRequest()
	req.InstanceID = "new-execution-id"
	req.PreviousExecutionID = created.InstanceID
	req.AuthToken = created.AuthToken
	req.BootstrapNonce = created.BootstrapNonce
	for key, value := range created.Metadata {
		req.Metadata[key] = value
	}
	return req
}

func newFakeKubernetesExecutor(
	t *testing.T,
	resources *fakeKubernetesResources,
	execs *recordingKubernetesExec,
	ports map[uint16]uint16,
) *KubernetesExecutor {
	t.Helper()
	return newFakeKubernetesExecutorWithForwarder(
		resources, execs, &recordingKubernetesForwarder{localPorts: ports},
	)
}

func newFakeKubernetesExecutorWithForwarder(
	resources *fakeKubernetesResources,
	execs *recordingKubernetesExec,
	forwards *recordingKubernetesForwarder,
) *KubernetesExecutor {
	executor := NewKubernetesExecutor(nil, newTestLogger())
	executor.clientFactory = func(kubeexecutor.ExecutorConfig) (*kubernetesRuntimeClient, error) {
		return &kubernetesRuntimeClient{
			resources: resources,
			streams:   kubeexecutor.NewStreamOperations(execs, forwards),
		}, nil
	}
	executor.resolveBinary = func(kubeexecutor.Platform) ([]byte, error) { return []byte("agentctl"), nil }
	return executor
}

func requireKubernetesUploadedFile(t *testing.T, execs *recordingKubernetesExec, path, content string) {
	t.Helper()
	execs.mu.Lock()
	defer execs.mu.Unlock()
	for _, recorded := range execs.requests {
		if strings.Contains(strings.Join(recorded.request.Command, " "), path) && string(recorded.stdin) == content {
			return
		}
	}
	t.Fatalf("remote file %s with content %q was not materialized; execs = %#v", path, content, execs.requests)
}

func requireKubernetesPathNotUsed(t *testing.T, execs *recordingKubernetesExec, forbidden string) {
	t.Helper()
	execs.mu.Lock()
	defer execs.mu.Unlock()
	for _, recorded := range execs.requests {
		if strings.Contains(strings.Join(recorded.request.Command, " "), forbidden) {
			t.Fatalf("untrusted remote auth path %q reached Kubernetes exec: %#v", forbidden, recorded.request)
		}
	}
}

func validKubernetesCreateRequest() *ExecutorCreateRequest {
	return &ExecutorCreateRequest{
		InstanceID:        "instance-1",
		TaskID:            "task-1",
		SessionID:         "session-1",
		TaskEnvironmentID: "environment-1",
		Metadata: map[string]interface{}{
			"executor_id":             "executor-1",
			"executor_profile_id":     "profile-1",
			"auth_mode":               "in_cluster",
			"namespace":               "kandev-agents",
			"request_timeout_seconds": "30",
			"platform":                "linux/amd64",
			"main_container":          "kandev-agent",
			"pod_template_yaml":       "apiVersion: v1\nkind: PodTemplate\ntemplate:\n  spec:\n    containers:\n      - name: kandev-agent\n        image: example.test/agent:latest\n",
			"workspace.mode":          "empty_dir",
		},
	}
}

type fakeKubernetesResources struct {
	mu                           sync.Mutex
	createdPVCs                  []*corev1.PersistentVolumeClaim
	createdPods                  []*corev1.Pod
	deletedPVCs                  []string
	deletedPods                  []string
	deletedPVCResourceVersions   []string
	deletedPodResourceVersions   []string
	deletePodContextDeadline     time.Time
	deletionOrder                []string
	nextPodUID                   types.UID
	nextPVCUID                   types.UID
	pvc                          *corev1.PersistentVolumeClaim
	pod                          *corev1.Pod
	getPodErr                    error
	getPodRequests               []string
	getPVCErr                    error
	getPVCRequests               []string
	createPodErr                 error
	createPVCErr                 error
	createPodBeforeCommitErr     error
	createPVCBeforeCommitErr     error
	deletePodErr                 error
	deletePVCErr                 error
	beforeDeletePod              func(*corev1.Pod)
	beforeDeletePVC              func(*corev1.PersistentVolumeClaim)
	retainPodAfterDelete         bool
	retainPVCAfterDelete         bool
	podDeleteIssued              bool
	pvcDeleteIssued              bool
	podPostDeleteGetsUntilAbsent int
	pvcPostDeleteGetsUntilAbsent int
	mutateCreatedPod             func(*corev1.Pod)
	mutateCreatedPVC             func(*corev1.PersistentVolumeClaim)
	waitForPodRunning            func(context.Context, string, string, string) (*corev1.Pod, error)
	rejectCanceledGetContexts    bool
}

func (f *fakeKubernetesResources) CreatePersistentVolumeClaim(
	_ context.Context,
	namespace string,
	pvc *corev1.PersistentVolumeClaim,
) (*corev1.PersistentVolumeClaim, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createPVCBeforeCommitErr != nil {
		return nil, f.createPVCBeforeCommitErr
	}
	created := pvc.DeepCopy()
	created.Namespace = namespace
	created.UID = f.nextPVCUID
	if created.UID == "" {
		created.UID = types.UID("pvc-uid")
	}
	if created.ResourceVersion == "" {
		created.ResourceVersion = "pvc-rv-1"
	}
	if f.mutateCreatedPVC != nil {
		f.mutateCreatedPVC(created)
	}
	created.Status.Phase = corev1.ClaimBound
	f.pvc = created.DeepCopy()
	f.createdPVCs = append(f.createdPVCs, created.DeepCopy())
	return created, f.createPVCErr
}

func (f *fakeKubernetesResources) GetPersistentVolumeClaim(
	ctx context.Context,
	namespace, name string,
) (*corev1.PersistentVolumeClaim, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getPVCRequests = append(f.getPVCRequests, namespace+"/"+name)
	if f.rejectCanceledGetContexts && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if f.pvcDeleteIssued && f.retainPVCAfterDelete && f.pvcPostDeleteGetsUntilAbsent >= 0 {
		if f.pvcPostDeleteGetsUntilAbsent == 0 {
			f.pvc = nil
		} else {
			f.pvcPostDeleteGetsUntilAbsent--
		}
	}
	if f.getPVCErr != nil {
		return nil, f.getPVCErr
	}
	if f.pvc == nil {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "persistentvolumeclaims"}, "missing")
	}
	return f.pvc.DeepCopy(), nil
}

func (f *fakeKubernetesResources) DeletePersistentVolumeClaim(
	_ context.Context,
	_, name string,
	uid types.UID,
	resourceVersion string,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.beforeDeletePVC != nil {
		f.beforeDeletePVC(f.pvc)
	}
	if f.deletePVCErr != nil {
		return f.deletePVCErr
	}
	if f.pvc != nil && (f.pvc.UID != uid || (resourceVersion != "" && f.pvc.ResourceVersion != resourceVersion)) {
		return apierrors.NewConflict(
			schema.GroupResource{Resource: "persistentvolumeclaims"}, name, errors.New("precondition failed"),
		)
	}
	f.deletedPVCs = append(f.deletedPVCs, name+":"+string(uid))
	f.deletedPVCResourceVersions = append(f.deletedPVCResourceVersions, resourceVersion)
	f.deletionOrder = append(f.deletionOrder, "pvc")
	f.pvcDeleteIssued = true
	if !f.retainPVCAfterDelete {
		f.pvc = nil
	}
	return nil
}

func (f *fakeKubernetesResources) CreatePod(
	_ context.Context,
	namespace string,
	pod *corev1.Pod,
) (*corev1.Pod, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createPodBeforeCommitErr != nil {
		return nil, f.createPodBeforeCommitErr
	}
	created := pod.DeepCopy()
	created.Namespace = namespace
	created.UID = f.nextPodUID
	if created.UID == "" {
		created.UID = types.UID("pod-uid")
	}
	if created.ResourceVersion == "" {
		created.ResourceVersion = "pod-rv-1"
	}
	f.nextPodUID = ""
	if f.mutateCreatedPod != nil {
		f.mutateCreatedPod(created)
	}
	f.pod = runningKubernetesPod(created)
	f.createdPods = append(f.createdPods, created.DeepCopy())
	return created, f.createPodErr
}

func (f *fakeKubernetesResources) GetPod(ctx context.Context, namespace, name string) (*corev1.Pod, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getPodRequests = append(f.getPodRequests, namespace+"/"+name)
	if f.rejectCanceledGetContexts && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if f.podDeleteIssued && f.retainPodAfterDelete && f.podPostDeleteGetsUntilAbsent >= 0 {
		if f.podPostDeleteGetsUntilAbsent == 0 {
			f.pod = nil
		} else {
			f.podPostDeleteGetsUntilAbsent--
		}
	}
	if f.getPodErr != nil {
		return nil, f.getPodErr
	}
	if f.pod == nil {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, "missing")
	}
	return f.pod.DeepCopy(), nil
}

func (f *fakeKubernetesResources) WaitForPodRunning(
	ctx context.Context,
	namespace string,
	name string,
	mainContainer string,
) (*corev1.Pod, error) {
	f.mu.Lock()
	hook := f.waitForPodRunning
	pod := f.pod.DeepCopy()
	f.mu.Unlock()
	if hook != nil {
		return hook(ctx, namespace, name, mainContainer)
	}
	return pod, nil
}

func (f *fakeKubernetesResources) DeletePod(
	ctx context.Context,
	_, name string,
	uid types.UID,
	resourceVersion string,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if deadline, ok := ctx.Deadline(); ok {
		f.deletePodContextDeadline = deadline
	}
	if f.beforeDeletePod != nil {
		f.beforeDeletePod(f.pod)
	}
	if f.deletePodErr != nil {
		return f.deletePodErr
	}
	if f.pod != nil && (f.pod.UID != uid || (resourceVersion != "" && f.pod.ResourceVersion != resourceVersion)) {
		return apierrors.NewConflict(schema.GroupResource{Resource: "pods"}, name, errors.New("precondition failed"))
	}
	f.deletedPods = append(f.deletedPods, name+":"+string(uid))
	f.deletedPodResourceVersions = append(f.deletedPodResourceVersions, resourceVersion)
	f.deletionOrder = append(f.deletionOrder, "pod")
	f.podDeleteIssued = true
	if !f.retainPodAfterDelete {
		f.pod = nil
	}
	return nil
}

func runningKubernetesPod(pod *corev1.Pod) *corev1.Pod {
	running := pod.DeepCopy()
	running.Status.Phase = corev1.PodRunning
	running.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name:  "kandev-agent",
		Ready: true,
		State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
	}}
	return running
}

type recordedKubernetesExec struct {
	request kubeexecutor.ExecRequest
	stdin   []byte
}

type recordingKubernetesExec struct {
	mu       sync.Mutex
	requests []recordedKubernetesExec
	err      error
}

func (r *recordingKubernetesExec) Exec(_ context.Context, request kubeexecutor.ExecRequest) error {
	var data []byte
	if request.Stdin != nil {
		var err error
		data, err = io.ReadAll(request.Stdin)
		if err != nil {
			return err
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, recordedKubernetesExec{request: request, stdin: data})
	return r.err
}

type recordingKubernetesForwarder struct {
	mu         sync.Mutex
	localPorts map[uint16]uint16
	requests   []kubeexecutor.PortForwardRequest
	sessions   []*fakeKubernetesForwardSession
}

func (f *recordingKubernetesForwarder) Forward(
	_ context.Context,
	request kubeexecutor.PortForwardRequest,
) (kubeexecutor.PortForwardSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, request)
	session := newFakeKubernetesForwardSession(f.localPorts[request.RemotePort])
	f.sessions = append(f.sessions, session)
	return session, nil
}

func (f *recordingKubernetesForwarder) lastSession() *fakeKubernetesForwardSession {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sessions[len(f.sessions)-1]
}

func (f *recordingKubernetesForwarder) remotePorts() []uint16 {
	f.mu.Lock()
	defer f.mu.Unlock()
	ports := make([]uint16, 0, len(f.requests))
	for _, request := range f.requests {
		ports = append(ports, request.RemotePort)
	}
	return ports
}

type fakeKubernetesForwardSession struct {
	mu        sync.Mutex
	localPort uint16
	ready     chan struct{}
	done      chan error
	closed    bool
	closeErr  error
}

func newFakeKubernetesForwardSession(localPort uint16) *fakeKubernetesForwardSession {
	ready := make(chan struct{})
	close(ready)
	return &fakeKubernetesForwardSession{localPort: localPort, ready: ready, done: make(chan error, 1)}
}

func (s *fakeKubernetesForwardSession) LocalPort() uint16      { return s.localPort }
func (s *fakeKubernetesForwardSession) Ready() <-chan struct{} { return s.ready }
func (s *fakeKubernetesForwardSession) Done() <-chan error     { return s.done }
func (s *fakeKubernetesForwardSession) Close() error {
	s.mu.Lock()
	s.closed = true
	err := s.closeErr
	s.mu.Unlock()
	return err
}
func (s *fakeKubernetesForwardSession) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func startKubernetesAgentctlServer(t *testing.T, control bool, instanceRemotePort int) uint16 {
	return startKubernetesAgentctlServerWithToken(t, control, instanceRemotePort, "handshake-token")
}

func startKubernetesAgentctlServerWithToken(
	t *testing.T,
	control bool,
	instanceRemotePort int,
	token string,
) uint16 {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	if control {
		mux.HandleFunc("/auth/handshake", func(w http.ResponseWriter, request *http.Request) {
			var body map[string]string
			require.NoError(t, json.NewDecoder(request.Body).Decode(&body))
			require.NotEmpty(t, body["nonce"])
			_ = json.NewEncoder(w).Encode(map[string]string{"token": token})
		})
		mux.HandleFunc("/api/v1/instances", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "instance-1", "port": instanceRemotePort})
		})
	}
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	_, rawPort, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(rawPort)
	require.NoError(t, err)
	return uint16(port)
}

func startKubernetesUnhealthyServer(t *testing.T) uint16 {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)
	_, rawPort, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(rawPort)
	require.NoError(t, err)
	return uint16(port)
}
