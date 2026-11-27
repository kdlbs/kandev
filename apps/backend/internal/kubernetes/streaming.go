package kubernetes

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/tools/remotecommand"

	agentkubernetes "github.com/kandev/kandev/internal/agent/kubernetes"
)

func probeStreamingTransport(
	ctx context.Context,
	client *agentkubernetes.Client,
	namespace string,
	podName string,
	mainContainer string,
) error {
	if client == nil || client.RESTConfig == nil || client.Clientset == nil {
		return errors.New("streaming transport is unavailable")
	}
	if err := probeExecUpgrade(ctx, client, namespace, podName, mainContainer); err != nil {
		return fmt.Errorf("pods/exec upgrade failed: %w", err)
	}
	if err := probePortForwardUpgrade(ctx, client, namespace, podName); err != nil {
		return fmt.Errorf("pods/portforward upgrade failed: %w", err)
	}
	return nil
}

func probeExecUpgrade(
	ctx context.Context,
	client *agentkubernetes.Client,
	namespace string,
	podName string,
	mainContainer string,
) error {
	executor, err := newStreamingProbeExecutor(client, namespace, podName, &corev1.PodExecOptions{
		Container: mainContainer, Command: []string{"sh", "-c", ":"}, Stdout: true,
	})
	if err != nil {
		return err
	}
	return executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdout: io.Discard})
}

// streamingProbePayloadBytes is large enough to span several stream frames.
const streamingProbePayloadBytes = 64 << 10

// probeExecStreamingTransport proves the exec transport's data path: random
// bytes written to an exec stream's stdin must come back unchanged on its
// stdout. A handshake-only check passes on runtimes that cannot carry data.
func probeExecStreamingTransport(
	ctx context.Context,
	client *agentkubernetes.Client,
	namespace string,
	podName string,
	mainContainer string,
) error {
	if client == nil || client.RESTConfig == nil || client.Clientset == nil {
		return errors.New("streaming transport is unavailable")
	}
	payload := make([]byte, streamingProbePayloadBytes)
	if _, err := rand.Read(payload); err != nil {
		return fmt.Errorf("generate exec stream probe payload: %w", err)
	}
	executor, err := newStreamingProbeExecutor(client, namespace, podName, &corev1.PodExecOptions{
		Container: mainContainer, Command: []string{"cat"}, Stdin: true, Stdout: true,
	})
	if err != nil {
		return fmt.Errorf("pods/exec stream setup failed: %w", err)
	}
	return runExecEchoProbe(ctx, executor, payload, streamingProbeEchoWait)
}

// streamingProbeEchoWait bounds how long the probe keeps stdin open while it
// waits for the full echo.
const streamingProbeEchoWait = 10 * time.Second

// runExecEchoProbe writes payload to the stream's stdin and keeps stdin open
// until the whole echo has arrived, or echoWait passes, before closing it.
// Some sandboxed runtimes (seen with Kata Containers) drop stdout that is
// still in flight when stdin reaches EOF; closing stdin right after the
// payload made the probe report a short echo on a working data path.
func runExecEchoProbe(
	ctx context.Context,
	executor remotecommand.Executor,
	payload []byte,
	echoWait time.Duration,
) error {
	stdin, stdinWriter := io.Pipe()
	echoed := &limitedProbeWriter{
		buffer: &bytes.Buffer{}, limit: len(payload) + 1, want: len(payload), full: make(chan struct{}),
	}
	streamDone := make(chan struct{})
	go func() {
		if _, err := stdinWriter.Write(payload); err != nil {
			return
		}
		timer := time.NewTimer(echoWait)
		defer timer.Stop()
		select {
		case <-echoed.full:
		case <-timer.C:
		case <-streamDone:
		case <-ctx.Done():
		}
		_ = stdinWriter.Close()
	}()
	err := executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdin: stdin, Stdout: echoed})
	close(streamDone)
	_ = stdin.Close()
	if err != nil {
		return fmt.Errorf("pods/exec byte stream failed: %w", err)
	}
	return verifyStreamingProbeEcho(payload, echoed.received())
}

func verifyStreamingProbeEcho(sent, received []byte) error {
	if !bytes.Equal(sent, received) {
		return fmt.Errorf("pods/exec byte stream returned %d of %d bytes unchanged", len(received), len(sent))
	}
	return nil
}

// limitedProbeWriter keeps at most limit bytes of the echo. When full is set,
// it is closed once want bytes have arrived.
type limitedProbeWriter struct {
	mu     sync.Mutex
	buffer *bytes.Buffer
	limit  int
	want   int
	full   chan struct{}
	closed bool
}

func (w *limitedProbeWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if remaining := w.limit - w.buffer.Len(); remaining > 0 {
		if len(p) > remaining {
			w.buffer.Write(p[:remaining])
		} else {
			w.buffer.Write(p)
		}
	}
	if w.full != nil && !w.closed && w.buffer.Len() >= w.want {
		w.closed = true
		close(w.full)
	}
	return len(p), nil
}

func (w *limitedProbeWriter) received() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]byte(nil), w.buffer.Bytes()...)
}

func newStreamingProbeExecutor(
	client *agentkubernetes.Client,
	namespace string,
	podName string,
	options *corev1.PodExecOptions,
) (remotecommand.Executor, error) {
	streamingConfig, err := streamingRESTConfig(client)
	if err != nil {
		return nil, err
	}
	requestURL := client.Clientset.CoreV1().RESTClient().Post().
		Namespace(namespace).Resource("pods").Name(podName).SubResource("exec").
		VersionedParams(options, scheme.ParameterCodec).URL()
	websocketExecutor, err := remotecommand.NewWebSocketExecutor(
		streamingConfig, http.MethodGet, requestURL.String(),
	)
	if err != nil {
		return nil, err
	}
	spdyExecutor, err := remotecommand.NewSPDYExecutor(
		streamingConfig, http.MethodPost, requestURL,
	)
	if err != nil {
		return nil, err
	}
	return remotecommand.NewFallbackExecutor(
		websocketExecutor, spdyExecutor, shouldFallbackStreaming,
	)
}

func probePortForwardUpgrade(
	ctx context.Context,
	client *agentkubernetes.Client,
	namespace string,
	podName string,
) error {
	streamingConfig, err := streamingRESTConfig(client)
	if err != nil {
		return err
	}
	requestURL := client.Clientset.CoreV1().RESTClient().Get().
		Namespace(namespace).Resource("pods").Name(podName).SubResource("portforward").URL()
	dialer, cancelDialer, err := agentkubernetes.NewPortForwardDialer(
		ctx, streamingConfig, requestURL, streamingPortForwardHandshakeTimeout(ctx),
	)
	if err != nil {
		return err
	}
	defer cancelDialer()
	connection, protocol, err := dialer.Dial(portforward.PortForwardProtocolV1Name)
	if connection != nil {
		_ = connection.Close()
	}
	if err != nil {
		return err
	}
	if protocol != portforward.PortForwardProtocolV1Name {
		return fmt.Errorf("unexpected port-forward protocol %q", protocol)
	}
	return nil
}

func streamingPortForwardHandshakeTimeout(ctx context.Context) time.Duration {
	maximum := time.Duration(agentkubernetes.DefaultRequestTimeoutSeconds) * time.Second
	deadline, ok := ctx.Deadline()
	if !ok {
		return maximum
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return time.Nanosecond
	}
	if remaining < maximum {
		return remaining
	}
	return maximum
}

func streamingRESTConfig(client *agentkubernetes.Client) (*rest.Config, error) {
	if client == nil || client.RESTConfig == nil {
		return nil, errors.New("streaming REST configuration is unavailable")
	}
	config := rest.CopyConfig(client.RESTConfig)
	config.Timeout = 0
	return config, nil
}

func shouldFallbackStreaming(err error) bool {
	return agentkubernetes.ShouldFallbackStream(err)
}

// streamingPermissions lists the stream subresources the selected transport
// needs; the exec transport never opens pods/portforward.
func streamingPermissions(transport agentkubernetes.StreamTransport) []requiredPermission {
	permissions := []requiredPermission{
		{verb: verbGet, resource: resourcePods, subresource: "exec"},
		{verb: verbCreate, resource: resourcePods, subresource: "exec"},
	}
	if transport == agentkubernetes.StreamTransportExec {
		return permissions
	}
	return append(permissions,
		requiredPermission{verb: verbGet, resource: resourcePods, subresource: "portforward"},
		requiredPermission{verb: verbCreate, resource: resourcePods, subresource: "portforward"},
	)
}

func (h *Handler) streamingProbeFor(transport agentkubernetes.StreamTransport) (StreamingProbe, string) {
	if transport == agentkubernetes.StreamTransportExec {
		probe := h.probeExecStreaming
		if probe == nil {
			probe = probeExecStreamingTransport
		}
		return probe, "Exec transport carried a byte round trip against a disposable Pod"
	}
	return h.probeStreaming, "Exec and port-forward transports succeeded against a disposable Pod"
}

func (h *Handler) runStreamingStep(
	ctx context.Context,
	client *agentkubernetes.Client,
	namespace string,
	transport agentkubernetes.StreamTransport,
	profile *agentkubernetes.ProfileConfig,
	prerequisitesPassed bool,
) TestStep {
	permissions := streamingPermissions(transport)
	step := runPermissionStep(ctx, client.Clientset, namespace, "streaming", permissions)
	if !step.Success {
		return step
	}
	if profile == nil {
		step.Detail = "Streaming permissions succeeded; live transport probe was not run without a profile"
		return step
	}
	if !prerequisitesPassed {
		step.Success = false
		step.Detail = "Pod permissions and dry-run admission must succeed before the live streaming probe"
		step.Error = "streaming probe prerequisites failed"
		return step
	}
	probe, successDetail := h.streamingProbeFor(transport)
	started := time.Now()
	if err := h.runLiveStreamingProbeWith(ctx, client, namespace, *profile, probe); err != nil {
		detail := "Streaming upgrade transport is incompatible"
		if errors.Is(err, errStreamingProbeCleanup) {
			detail = "Streaming probe cleanup failed"
		}
		return failedStep("streaming", started, detail, err)
	}
	step.DurationMS += time.Since(started).Milliseconds()
	step.Detail = successDetail
	return step
}

func (h *Handler) runLiveStreamingProbe(
	ctx context.Context,
	client *agentkubernetes.Client,
	namespace string,
	profile agentkubernetes.ProfileConfig,
) error {
	return h.runLiveStreamingProbeWith(ctx, client, namespace, profile, h.probeStreaming)
}

func (h *Handler) runLiveStreamingProbeWith(
	ctx context.Context,
	client *agentkubernetes.Client,
	namespace string,
	profile agentkubernetes.ProfileConfig,
	probe StreamingProbe,
) (returnErr error) {
	probeName := "kandev-stream-probe-" + uuid.NewString()[:8]
	pod, err := buildStreamingProbePod(namespace, probeName, profile)
	if err != nil {
		return err
	}
	if err := agentkubernetes.StampCreateNonce(pod); err != nil {
		return fmt.Errorf("generate streaming probe create nonce: %w", err)
	}
	created, err := client.Clientset.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		if !agentkubernetes.IsAmbiguousCreateError(err) {
			return err
		}
		return reconcileStreamingProbeCreateError(
			ctx, client.Clientset, pod, profile.MainContainer, err,
		)
	}
	if created == nil || created.Namespace == "" || created.Name == "" || created.UID == "" {
		return fmt.Errorf("%w: created Pod identity is incomplete", errStreamingProbeCleanup)
	}
	createdNamespace := created.Namespace
	createdName := created.Name
	uid := created.UID
	defer func() {
		if cleanupErr := deleteStreamingProbePod(
			ctx, client.Clientset, createdNamespace, createdName, uid,
		); cleanupErr != nil {
			cleanupErr = fmt.Errorf("%w: %v", errStreamingProbeCleanup, cleanupErr)
			returnErr = errors.Join(returnErr, cleanupErr)
		}
	}()
	if err := agentkubernetes.ValidateAdmittedPod(created, pod, profile.MainContainer); err != nil {
		return err
	}
	running, err := waitForStreamingProbePod(
		ctx, client.Clientset, createdNamespace, createdName, profile.MainContainer, created,
	)
	if err != nil {
		return err
	}
	return probe(ctx, client, running.Namespace, running.Name, profile.MainContainer)
}

func reconcileStreamingProbeCreateError(
	ctx context.Context,
	client kubeclient.Interface,
	desired *corev1.Pod,
	mainContainer string,
	createErr error,
) error {
	reconcileCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	admitted, getErr := client.CoreV1().Pods(desired.Namespace).Get(
		reconcileCtx, desired.Name, metav1.GetOptions{},
	)
	switch {
	case apierrors.IsNotFound(getErr):
		return createErr
	case getErr != nil:
		return errors.Join(
			createErr,
			fmt.Errorf("reconcile streaming probe create: inspect exact Pod: %w", getErr),
		)
	}
	if admitted == nil || admitted.Namespace == "" || admitted.Name == "" || admitted.UID == "" {
		return errors.Join(
			createErr,
			errors.New("reconcile streaming probe create: returned Pod identity is incomplete"),
		)
	}
	if err := agentkubernetes.ValidateAdmittedPod(admitted, desired, mainContainer); err != nil {
		return errors.Join(
			createErr,
			fmt.Errorf("reconcile streaming probe create: %w", err),
		)
	}
	if err := deleteStreamingProbePod(
		reconcileCtx, client, admitted.Namespace, admitted.Name, admitted.UID,
	); err != nil {
		return errors.Join(
			createErr,
			fmt.Errorf("%w: %v", errStreamingProbeCleanup, err),
		)
	}
	return createErr
}

func buildStreamingProbePod(
	namespace string,
	name string,
	profile agentkubernetes.ProfileConfig,
) (*corev1.Pod, error) {
	probeProfile := profile
	probeProfile.Workspace = agentkubernetes.WorkspaceConfig{Mode: agentkubernetes.WorkspaceModeEmptyDir}
	template, err := agentkubernetes.ParsePodTemplate(probeProfile.PodTemplateYAML)
	if err != nil {
		return nil, err
	}
	identity := agentkubernetes.ResourceIdentity{
		ExecutorID: connectionTestIdentity, ProfileID: connectionTestIdentity,
		InstanceID: name, TaskID: connectionTestIdentity,
		SessionID: name, EnvironmentID: connectionTestIdentity,
	}
	pod, _, err := agentkubernetes.ComposePod(template, probeProfile, agentkubernetes.PodOptions{
		Name: name, Namespace: namespace, Identity: identity,
		Command: []string{"sh"}, Args: []string{"-c", "while :; do sleep 30; done"},
		WorkingDir: "/workspace", AgentctlPort: agentkubernetes.DefaultAgentctlPort,
	})
	return pod, err
}

func matchesStreamingProbePod(current, admitted *corev1.Pod, mainContainer string) bool {
	if current == nil || admitted == nil {
		return false
	}
	runtimePod := current
	if current.Spec.NodeName != admitted.Spec.NodeName {
		if admitted.Spec.NodeName != "" || current.Spec.NodeName == "" {
			return false
		}
		runtimePod = current.DeepCopy()
		runtimePod.Spec.NodeName = ""
	}
	return agentkubernetes.ValidateAdmittedPod(runtimePod, admitted, mainContainer) == nil
}

func waitForStreamingProbePod(
	ctx context.Context,
	client kubeclient.Interface,
	namespace string,
	name string,
	mainContainer string,
	current *corev1.Pod,
) (*corev1.Pod, error) {
	if current == nil || current.UID == "" {
		return nil, errors.New("streaming probe Pod identity is unavailable")
	}
	expected := current.DeepCopy()
	assignedNodeName := expected.Spec.NodeName
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		if current.Spec.NodeName != "" {
			if assignedNodeName == "" {
				assignedNodeName = current.Spec.NodeName
			} else if current.Spec.NodeName != assignedNodeName {
				return nil, errors.New("streaming probe Pod identity changed while waiting")
			}
		} else if assignedNodeName != "" {
			return nil, errors.New("streaming probe Pod identity changed while waiting")
		}
		if current.UID != expected.UID || !matchesStreamingProbePod(current, expected, mainContainer) {
			return nil, errors.New("streaming probe Pod identity changed while waiting")
		}
		ready, err := streamingProbePodReady(current, mainContainer)
		if ready || err != nil {
			return current, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
		current, err = client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
	}
}

func streamingProbePodReady(pod *corev1.Pod, mainContainer string) (bool, error) {
	if pod == nil {
		return false, errors.New("streaming probe Pod is unavailable")
	}
	switch pod.Status.Phase {
	case corev1.PodFailed, corev1.PodSucceeded:
		return false, errors.New("streaming probe Pod terminated before transport checks")
	}
	if pod.Status.Phase != corev1.PodRunning {
		return false, nil
	}
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name != mainContainer {
			continue
		}
		if status.State.Terminated != nil {
			return false, errors.New("streaming probe main container terminated")
		}
		return status.State.Running != nil, nil
	}
	return false, nil
}

func deleteStreamingProbePod(
	ctx context.Context,
	client kubeclient.Interface,
	namespace string,
	name string,
	uid types.UID,
) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	grace := int64(0)
	propagation := metav1.DeletePropagationForeground
	err := client.CoreV1().Pods(namespace).Delete(cleanupCtx, name, metav1.DeleteOptions{
		GracePeriodSeconds: &grace,
		PropagationPolicy:  &propagation,
		Preconditions:      &metav1.Preconditions{UID: &uid},
	})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		pod, getErr := client.CoreV1().Pods(namespace).Get(cleanupCtx, name, metav1.GetOptions{})
		switch {
		case apierrors.IsNotFound(getErr):
			return nil
		case getErr != nil:
			return getErr
		case pod == nil || pod.UID != uid:
			return nil
		}
		select {
		case <-cleanupCtx.Done():
			return cleanupCtx.Err()
		case <-ticker.C:
		}
	}
}
