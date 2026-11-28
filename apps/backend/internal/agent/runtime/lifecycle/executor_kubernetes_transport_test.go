package lifecycle

import (
	"context"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
)

// bridgingKubernetesExec answers `agentctl tcp-stdio` exec streams by
// bridging them to local test agentctl servers, and records every other exec
// through the shared recording fake.
type bridgingKubernetesExec struct {
	files *recordingKubernetesExec
	ports map[string]uint16

	mu      sync.Mutex
	bridges []kubeexecutor.ExecRequest
}

func (b *bridgingKubernetesExec) Exec(ctx context.Context, request kubeexecutor.ExecRequest) error {
	if len(request.Command) < 2 || request.Command[1] != kubeexecutor.TCPStdioSubcommand {
		return b.files.Exec(ctx, request)
	}
	b.mu.Lock()
	b.bridges = append(b.bridges, request)
	b.mu.Unlock()
	port := b.ports[request.Command[len(request.Command)-1]]
	upstream, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(int(port))))
	if err != nil {
		return err
	}
	defer func() { _ = upstream.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = upstream.Close() })
	defer stop()
	go func() {
		_, _ = io.Copy(upstream, request.Stdin)
		_ = upstream.(*net.TCPConn).CloseWrite()
	}()
	_, err = io.Copy(request.Stdout, upstream)
	return err
}

func (b *bridgingKubernetesExec) bridgeRequests() []kubeexecutor.ExecRequest {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]kubeexecutor.ExecRequest(nil), b.bridges...)
}

type execTransportLaunch struct {
	executor     *KubernetesExecutor
	execs        *bridgingKubernetesExec
	files        *recordingKubernetesExec
	portForwards *recordingKubernetesForwarder
	configured   *kubeexecutor.ExecutorConfig
}

func newExecTransportLaunch(t *testing.T) *execTransportLaunch {
	t.Helper()
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	launch := &execTransportLaunch{
		files:        &recordingKubernetesExec{},
		portForwards: &recordingKubernetesForwarder{},
		configured:   &kubeexecutor.ExecutorConfig{},
	}
	launch.execs = &bridgingKubernetesExec{files: launch.files, ports: map[string]uint16{
		strconv.Itoa(int(kubeexecutor.DefaultAgentctlPort)): controlPort,
		"41001": instancePort,
	}}
	launch.executor = NewKubernetesExecutor(nil, newTestLogger())
	launch.executor.clientFactory = func(config kubeexecutor.ExecutorConfig) (*kubernetesRuntimeClient, error) {
		*launch.configured = config
		return &kubernetesRuntimeClient{
			resources: resources,
			streams: kubeexecutor.NewStreamOperations(
				launch.execs, newKubernetesForwarder(config, launch.execs, launch.portForwards, nil),
			),
		}, nil
	}
	launch.executor.resolveBinary = func(context.Context, *ExecutorCreateRequest, kubeexecutor.Platform) ([]byte, error) {
		return []byte("agentctl-binary"), nil
	}
	return launch
}

// requireAllExecsTarget checks that bootstrap uploads and every bridge stream
// went to the main container, and that both agentctl ports were bridged.
func (l *execTransportLaunch) requireAllExecsTarget(t *testing.T, mainContainer string) {
	t.Helper()
	bridges := l.execs.bridgeRequests()
	require.NotEmpty(t, bridges)
	remotePorts := map[string]bool{}
	for _, bridge := range bridges {
		require.Equal(t, mainContainer, bridge.Container, "exec bridge must target the main container")
		require.Equal(t, kubernetesAgentctlPath, bridge.Command[0])
		remotePorts[bridge.Command[len(bridge.Command)-1]] = true
	}
	require.True(t, remotePorts[strconv.Itoa(int(kubeexecutor.DefaultAgentctlPort))], "control port bridged")
	require.True(t, remotePorts["41001"], "instance port bridged")
	l.files.mu.Lock()
	defer l.files.mu.Unlock()
	for _, upload := range l.files.requests {
		require.Equal(t, mainContainer, upload.request.Container, "bootstrap exec must target the main container")
	}
}

func TestKubernetesCreateInstanceReachesAgentctlOverExecTransport(t *testing.T) {
	launch := newExecTransportLaunch(t)
	req := validKubernetesCreateRequest()
	req.Metadata[MetadataKeyKubernetesTransport] = "exec"

	instance, err := launch.executor.CreateInstance(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, instance.Client)
	require.Equal(t, "handshake-token", instance.AuthToken)
	require.Equal(t, kubeexecutor.StreamTransportExec, launch.configured.EffectiveTransport())
	require.Empty(t, launch.portForwards.remotePorts(), "exec transport must not open pods/portforward")
	launch.requireAllExecsTarget(t, "kandev-agent")
	require.NoError(t, instance.Client.Health(context.Background()))
	require.NotNil(t, launch.executor.closeKubernetesSession(instance.InstanceID), "the exec forward belongs to the session")
}

// TestKubernetesExecTransportTargetsMainContainerAmongSidecars puts sidecars
// before and after the main container, as a mesh-injected or Docker-in-Docker
// Pod has them, and checks that every exec stream opens in the configured
// main container: the default kandev-agent and a custom name.
func TestKubernetesExecTransportTargetsMainContainerAmongSidecars(t *testing.T) {
	for _, tc := range []struct {
		name          string
		mainContainer string
	}{
		{name: "default main container", mainContainer: kubeexecutor.DefaultMainContainerName},
		{name: "custom main container", mainContainer: "agent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			launch := newExecTransportLaunch(t)
			req := validKubernetesCreateRequest()
			req.Metadata[MetadataKeyKubernetesTransport] = "exec"
			req.Metadata[MetadataKeyKubernetesProfileMainContainer] = tc.mainContainer
			req.Metadata[MetadataKeyKubernetesPodTemplateYAML] = "apiVersion: v1\nkind: PodTemplate\ntemplate:\n" +
				"  spec:\n    containers:\n" +
				"      - name: istio-proxy\n        image: example.test/proxy:latest\n" +
				"      - name: " + tc.mainContainer + "\n        image: example.test/agent:latest\n" +
				"      - name: docker-engine\n        image: example.test/docker:latest\n"

			instance, err := launch.executor.CreateInstance(context.Background(), req)

			require.NoError(t, err)
			launch.requireAllExecsTarget(t, tc.mainContainer)
			require.NoError(t, instance.Client.Health(context.Background()))
			require.NotNil(t, launch.executor.closeKubernetesSession(instance.InstanceID))
		})
	}
}

func sidecarSessionPod(mainPort bool) *corev1.Pod {
	main := corev1.Container{Name: "kandev-agent"}
	if mainPort {
		main.Ports = []corev1.ContainerPort{{
			Name: kubeexecutor.AgentctlContainerPort, ContainerPort: kubeexecutor.DefaultAgentctlPort,
		}}
	}
	pod := &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
		{Name: "istio-proxy", Ports: []corev1.ContainerPort{{Name: "http-envoy-prom", ContainerPort: 15090}}},
		main,
	}}}
	pod.Namespace, pod.Name = "kandev-agents", "session-pod"
	return pod
}

func TestStartKubernetesForwardRejectsPodWithoutMainContainerOverExec(t *testing.T) {
	execs := &recordingKubernetesExec{}
	streams := kubeexecutor.NewStreamOperations(execs, newKubernetesForwarder(
		kubeexecutor.ExecutorConfig{Transport: kubeexecutor.StreamTransportExec}, execs, &recordingKubernetesForwarder{}, nil,
	))

	_, err := startKubernetesForward(context.Background(), streams, sidecarSessionPod(false), 8765)

	require.ErrorContains(t, err, "has no main container with the kandev-agentctl port")
	require.Empty(t, execs.requests, "no exec stream may open in a sidecar")
}

func TestStartKubernetesForwardLeavesPortForwardRequestUnchanged(t *testing.T) {
	portForwards := &recordingKubernetesForwarder{localPorts: map[uint16]uint16{8765: 41000}}
	streams := kubeexecutor.NewStreamOperations(&recordingKubernetesExec{}, newKubernetesForwarder(
		kubeexecutor.ExecutorConfig{}, &recordingKubernetesExec{}, portForwards, nil,
	))

	forward, err := startKubernetesForward(context.Background(), streams, sidecarSessionPod(false), 8765)

	require.NoError(t, err)
	defer func() { _ = forward.Close() }()
	require.Len(t, portForwards.requests, 1)
	require.Empty(t, portForwards.requests[0].Container, "port-forward requests carry no container")
}

func TestNewKubernetesForwarderDefaultsToPortForward(t *testing.T) {
	portForwarder := &recordingKubernetesForwarder{}
	selected := newKubernetesForwarder(kubeexecutor.ExecutorConfig{}, &recordingKubernetesExec{}, portForwarder, nil)
	require.Same(t, portForwarder, selected)

	selected = newKubernetesForwarder(
		kubeexecutor.ExecutorConfig{Transport: kubeexecutor.StreamTransportExec}, &recordingKubernetesExec{}, portForwarder, zap.NewNop(),
	)
	execForwarder, ok := selected.(*kubeexecutor.ExecPortForwarder)
	require.True(t, ok)
	require.Equal(t, kubernetesAgentctlPath, execForwarder.BridgePath)
	require.NotNil(t, execForwarder.Logger, "the exec transport logs through the executor logger")
}

func TestKubernetesExecutorConfigFromMetadataReadsTransport(t *testing.T) {
	metadata := validKubernetesCreateRequest().Metadata
	config, err := kubernetesExecutorConfigFromMetadata(metadata)
	require.NoError(t, err)
	require.Equal(t, kubeexecutor.StreamTransportPortForward, config.EffectiveTransport())

	metadata[MetadataKeyKubernetesTransport] = "exec"
	config, err = kubernetesExecutorConfigFromMetadata(metadata)
	require.NoError(t, err)
	require.Equal(t, kubeexecutor.StreamTransportExec, config.EffectiveTransport())

	metadata[MetadataKeyKubernetesTransport] = "vsock"
	_, err = kubernetesExecutorConfigFromMetadata(metadata)
	require.ErrorContains(t, err, "config.transport")
}

// TestKubernetesExecutorConfigHashUnchangedWithoutTransport pins the recorded
// executor-config hash of existing executors: without `transport` the JSON,
// and therefore the hash, must equal the pre-transport encoding, or every
// running session would look reconfigured after the upgrade.
func TestKubernetesExecutorConfigHashUnchangedWithoutTransport(t *testing.T) {
	config := kubeexecutor.ExecutorConfig{
		AuthMode: kubeexecutor.AuthModeInCluster, Namespace: "kandev-agents", RequestTimeoutSeconds: 30,
	}
	preTransport := `{"auth_mode":"in_cluster","namespace":"kandev-agents","request_timeout_seconds":30}`

	require.Equal(t, kubernetesStringHash(preTransport), kubernetesConfigHash(config))

	config.Transport = kubeexecutor.StreamTransportExec
	require.NotEqual(t, kubernetesStringHash(preTransport), kubernetesConfigHash(config))
}

func TestKubernetesTransportIsTrustedConnectionMetadata(t *testing.T) {
	require.True(t, isTrustedExecutorConfigKey(MetadataKeyKubernetesTransport))
	require.Contains(t, kubernetesConnectionMetadataKeys[:], MetadataKeyKubernetesTransport)
	require.True(t, persistentMetadataKeys[MetadataKeyKubernetesTransport])
}
