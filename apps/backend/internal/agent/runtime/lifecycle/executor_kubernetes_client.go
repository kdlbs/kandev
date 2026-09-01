package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	kubernetesclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/tools/remotecommand"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

type kubernetesResourceClient interface {
	CreatePersistentVolumeClaim(context.Context, string, *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error)
	GetPersistentVolumeClaim(context.Context, string, string) (*corev1.PersistentVolumeClaim, error)
	DeletePersistentVolumeClaim(context.Context, string, string, types.UID, string) error
	CreatePod(context.Context, string, *corev1.Pod) (*corev1.Pod, error)
	GetPod(context.Context, string, string) (*corev1.Pod, error)
	WaitForPodRunning(context.Context, string, string, string) (*corev1.Pod, error)
	DeletePod(context.Context, string, string, types.UID, string) error
}

type kubernetesRuntimeClient struct {
	resources kubernetesResourceClient
	streams   *kubeexecutor.StreamOperations
}

type kubernetesRuntimeClientFactory func(kubeexecutor.ExecutorConfig) (*kubernetesRuntimeClient, error)
type kubernetesAgentctlBinaryResolver func(kubeexecutor.Platform) ([]byte, error)

func newKubernetesRuntimeClient(config kubeexecutor.ExecutorConfig) (*kubernetesRuntimeClient, error) {
	client, err := kubeexecutor.NewClient(config, kubeexecutor.ConfigLoader{}, nil)
	if err != nil {
		return nil, err
	}
	streamingConfig := kubernetesStreamingRESTConfig(client.RESTConfig)
	watchClient, err := kubernetesclient.NewForConfig(streamingConfig)
	if err != nil {
		return nil, fmt.Errorf("kubernetes lifecycle: create timeout-free watch client: %w", err)
	}
	resources := &liveKubernetesResources{client: client, watchClient: watchClient}
	streams := kubeexecutor.NewStreamOperations(
		&liveKubernetesExecutor{config: streamingConfig, resources: resources},
		&liveKubernetesForwarder{
			config: streamingConfig, resources: resources,
			handshakeTimeout: kubernetesPortForwardHandshakeTimeout(config.RequestTimeoutSeconds),
		},
	)
	return &kubernetesRuntimeClient{resources: resources, streams: streams}, nil
}

func kubernetesStreamingRESTConfig(apiConfig *rest.Config) *rest.Config {
	if apiConfig == nil {
		return nil
	}
	streamingConfig := rest.CopyConfig(apiConfig)
	streamingConfig.Timeout = 0
	return streamingConfig
}

type liveKubernetesResources struct {
	client             *kubeexecutor.Client
	watchClient        kubernetesclient.Interface
	watchRelistBackoff func(context.Context) error
}

const kubernetesWatchRelistBackoff = 100 * time.Millisecond

func (r *liveKubernetesResources) CreatePersistentVolumeClaim(
	ctx context.Context,
	namespace string,
	pvc *corev1.PersistentVolumeClaim,
) (*corev1.PersistentVolumeClaim, error) {
	return r.client.Clientset.CoreV1().PersistentVolumeClaims(namespace).Create(ctx, pvc, metav1.CreateOptions{})
}

func (r *liveKubernetesResources) GetPersistentVolumeClaim(
	ctx context.Context,
	namespace, name string,
) (*corev1.PersistentVolumeClaim, error) {
	return r.client.Clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (r *liveKubernetesResources) DeletePersistentVolumeClaim(
	ctx context.Context,
	namespace, name string,
	uid types.UID,
	resourceVersion string,
) error {
	propagation := metav1.DeletePropagationForeground
	return r.client.Clientset.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, name, metav1.DeleteOptions{
		Preconditions: kubernetesDeletePreconditions(uid, resourceVersion), PropagationPolicy: &propagation,
	})
}

func (r *liveKubernetesResources) CreatePod(
	ctx context.Context,
	namespace string,
	pod *corev1.Pod,
) (*corev1.Pod, error) {
	return r.client.Clientset.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
}

func (r *liveKubernetesResources) GetPod(ctx context.Context, namespace, name string) (*corev1.Pod, error) {
	return r.client.Clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (r *liveKubernetesResources) DeletePod(
	ctx context.Context,
	namespace, name string,
	uid types.UID,
	resourceVersion string,
) error {
	grace := int64(0)
	propagation := metav1.DeletePropagationForeground
	return r.client.Clientset.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{
		GracePeriodSeconds: &grace,
		Preconditions:      kubernetesDeletePreconditions(uid, resourceVersion),
		PropagationPolicy:  &propagation,
	})
}

func kubernetesDeletePreconditions(uid types.UID, resourceVersion string) *metav1.Preconditions {
	preconditions := &metav1.Preconditions{UID: &uid}
	if resourceVersion != "" {
		preconditions.ResourceVersion = &resourceVersion
	}
	return preconditions
}

func (r *liveKubernetesResources) WaitForPodRunning(
	ctx context.Context,
	namespace, name, mainContainer string,
) (*corev1.Pod, error) {
	var latestDiagnostic error
	for {
		pod, err := r.GetPod(ctx, namespace, name)
		if err != nil {
			return nil, kubernetesPodWaitError(ctx, err, latestDiagnostic)
		}
		latestDiagnostic = kubernetesLatestPodDiagnostic(pod, latestDiagnostic)
		if ready, terminalErr := kubernetesPodReady(pod, mainContainer); ready || terminalErr != nil {
			return pod, terminalErr
		}
		outcome := r.watchKubernetesPodTransition(
			ctx, namespace, name, mainContainer, pod.ResourceVersion, latestDiagnostic,
		)
		latestDiagnostic = outcome.latestDiagnostic
		if outcome.relist {
			if err := r.waitBeforeWatchRelist(ctx); err != nil {
				return nil, kubernetesPodWaitError(ctx, err, latestDiagnostic)
			}
			continue
		}
		return outcome.pod, outcome.err
	}
}

func (r *liveKubernetesResources) waitBeforeWatchRelist(ctx context.Context) error {
	if r.watchRelistBackoff != nil {
		return r.watchRelistBackoff(ctx)
	}
	timer := time.NewTimer(kubernetesWatchRelistBackoff)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type kubernetesPodWatchOutcome struct {
	pod              *corev1.Pod
	latestDiagnostic error
	err              error
	relist           bool
	done             bool
}

func (r *liveKubernetesResources) watchKubernetesPodTransition(
	ctx context.Context,
	namespace, name, mainContainer, resourceVersion string,
	latestDiagnostic error,
) kubernetesPodWatchOutcome {
	watcher, err := r.watchPod(ctx, namespace, name, resourceVersion)
	if err != nil {
		return kubernetesPodWatchOutcome{
			latestDiagnostic: latestDiagnostic,
			err:              kubernetesPodWaitError(ctx, err, latestDiagnostic),
		}
	}
	defer watcher.Stop()
	for {
		select {
		case <-ctx.Done():
			return kubernetesPodWatchOutcome{
				latestDiagnostic: latestDiagnostic,
				err:              kubernetesPodWaitError(ctx, ctx.Err(), latestDiagnostic),
			}
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return kubernetesPodWatchOutcome{latestDiagnostic: latestDiagnostic, relist: true}
			}
			outcome := evaluateKubernetesPodWatchEvent(
				ctx, event, namespace, name, mainContainer, latestDiagnostic,
			)
			latestDiagnostic = outcome.latestDiagnostic
			if outcome.done || outcome.relist {
				return outcome
			}
		}
	}
}

func evaluateKubernetesPodWatchEvent(
	ctx context.Context,
	event watch.Event,
	namespace, name, mainContainer string,
	latestDiagnostic error,
) kubernetesPodWatchOutcome {
	if event.Type == watch.Error {
		eventErr := apierrors.FromObject(event.Object)
		if apierrors.IsResourceExpired(eventErr) || apierrors.IsGone(eventErr) {
			return kubernetesPodWatchOutcome{latestDiagnostic: latestDiagnostic, relist: true}
		}
		return kubernetesPodWatchOutcome{
			latestDiagnostic: latestDiagnostic,
			err:              kubernetesPodWaitError(ctx, eventErr, latestDiagnostic),
			done:             true,
		}
	}
	current, ok := event.Object.(*corev1.Pod)
	if !ok || current.Name != name || current.Namespace != namespace {
		return kubernetesPodWatchOutcome{latestDiagnostic: latestDiagnostic}
	}
	latestDiagnostic = kubernetesLatestPodDiagnostic(current, latestDiagnostic)
	if event.Type == watch.Deleted {
		return kubernetesPodWatchOutcome{
			pod: current, latestDiagnostic: latestDiagnostic,
			err: kubernetesDeletedPodError(current, mainContainer, latestDiagnostic), done: true,
		}
	}
	ready, terminalErr := kubernetesPodReady(current, mainContainer)
	return kubernetesPodWatchOutcome{
		pod: current, latestDiagnostic: latestDiagnostic, err: terminalErr,
		done: ready || terminalErr != nil,
	}
}

func (r *liveKubernetesResources) watchPod(
	ctx context.Context,
	namespace, name, resourceVersion string,
) (watch.Interface, error) {
	clientset := r.client.Clientset
	if r.watchClient != nil {
		clientset = r.watchClient
	}
	selector := fields.OneTermEqualSelector("metadata.name", name).String()
	return clientset.CoreV1().Pods(namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector: selector, ResourceVersion: resourceVersion,
	})
}

func kubernetesLatestPodDiagnostic(pod *corev1.Pod, previous error) error {
	if diagnostic := kubernetesSchedulingError(pod); diagnostic != nil {
		return diagnostic
	}
	return previous
}

func kubernetesPodWaitError(ctx context.Context, err, diagnostic error) error {
	if ctx.Err() != nil {
		err = ctx.Err()
	} else {
		err = routingerr.SanitizeError(err)
	}
	if diagnostic == nil {
		return err
	}
	return fmt.Errorf("%w: latest Pod diagnostic: %v", err, diagnostic)
}

func kubernetesPodReady(pod *corev1.Pod, mainContainer string) (bool, error) {
	if pod == nil {
		return false, errors.New("kubernetes Pod is nil")
	}
	switch pod.Status.Phase {
	case corev1.PodFailed:
		return false, routingerr.SanitizeError(fmt.Errorf("pod failed: %s: %s", pod.Status.Reason, pod.Status.Message))
	case corev1.PodSucceeded:
		return false, errors.New("pod completed before agentctl started")
	}
	for _, status := range pod.Status.InitContainerStatuses {
		if err := kubernetesContainerImagePullError(status, "init container "+status.Name); err != nil {
			return false, err
		}
	}
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name != mainContainer {
			continue
		}
		if status.State.Terminated != nil {
			return false, routingerr.SanitizeError(fmt.Errorf(
				"main container terminated: %s: %s",
				status.State.Terminated.Reason,
				status.State.Terminated.Message,
			))
		}
		if err := kubernetesContainerImagePullError(status, "main container"); err != nil {
			return false, err
		}
		if status.State.Running != nil {
			return true, nil
		}
		break
	}
	return false, nil
}

func kubernetesContainerImagePullError(status corev1.ContainerStatus, container string) error {
	if status.State.Waiting == nil {
		return nil
	}
	reason := status.State.Waiting.Reason
	if reason != "ImagePullBackOff" && reason != "ErrImagePull" {
		return nil
	}
	return routingerr.SanitizeError(fmt.Errorf(
		"%s image pull failed: %s: %s", container, reason, status.State.Waiting.Message,
	))
}

func kubernetesSchedulingError(pod *corev1.Pod) error {
	for _, condition := range pod.Status.Conditions {
		if condition.Type != corev1.PodScheduled || condition.Status != corev1.ConditionFalse {
			continue
		}
		return routingerr.SanitizeError(fmt.Errorf(
			"pod scheduling failed: %s: %s", condition.Reason, condition.Message,
		))
	}
	return nil
}

func kubernetesDeletedPodError(pod *corev1.Pod, mainContainer string, latestDiagnostic error) error {
	if _, diagnostic := kubernetesPodReady(pod, mainContainer); diagnostic != nil {
		return fmt.Errorf("kubernetes Pod deleted before main container started: %w", diagnostic)
	}
	if latestDiagnostic != nil {
		return fmt.Errorf("kubernetes Pod deleted before main container started: %w", latestDiagnostic)
	}
	return errors.New("kubernetes Pod deleted before main container started")
}

type liveKubernetesExecutor struct {
	config    *rest.Config
	resources *liveKubernetesResources
}

func (e *liveKubernetesExecutor) Exec(ctx context.Context, request kubeexecutor.ExecRequest) error {
	targetURL := e.resources.client.Clientset.CoreV1().RESTClient().Get().
		Resource("pods").Namespace(request.Namespace).Name(request.Pod).SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: request.Container, Command: request.Command,
			Stdin: request.Stdin != nil, Stdout: request.Stdout != nil, Stderr: request.Stderr != nil, TTY: request.TTY,
		}, scheme.ParameterCodec).URL()
	executor, err := newKubernetesRemoteCommandExecutor(e.config, targetURL)
	if err != nil {
		return err
	}
	return executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin: request.Stdin, Stdout: request.Stdout, Stderr: request.Stderr, Tty: request.TTY,
	})
}

func newKubernetesRemoteCommandExecutor(
	config *rest.Config,
	targetURL *url.URL,
) (remotecommand.Executor, error) {
	websocketExecutor, err := remotecommand.NewWebSocketExecutor(config, http.MethodGet, targetURL.String())
	if err != nil {
		return nil, err
	}
	spdyExecutor, err := remotecommand.NewSPDYExecutor(config, http.MethodPost, targetURL)
	if err != nil {
		return nil, err
	}
	return remotecommand.NewFallbackExecutor(
		websocketExecutor, spdyExecutor, kubeexecutor.ShouldFallbackStream,
	)
}

type liveKubernetesForwarder struct {
	config           *rest.Config
	resources        *liveKubernetesResources
	handshakeTimeout time.Duration
}

func (f *liveKubernetesForwarder) Forward(
	ctx context.Context,
	request kubeexecutor.PortForwardRequest,
) (kubeexecutor.PortForwardSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	targetURL := f.resources.client.Clientset.CoreV1().RESTClient().Get().
		Resource("pods").Namespace(request.Namespace).Name(request.Pod).SubResource("portforward").URL()
	dialer, cancelForward, err := kubeexecutor.NewPortForwardDialer(
		ctx, f.config, targetURL, f.effectiveHandshakeTimeout(),
	)
	if err != nil {
		return nil, err
	}
	stop := make(chan struct{})
	internalReady := make(chan struct{})
	forwarder, err := portforward.NewOnAddresses(
		dialer, []string{request.LocalAddress},
		[]string{strconv.Itoa(int(request.LocalPort)) + ":" + strconv.Itoa(int(request.RemotePort))},
		stop, internalReady, io.Discard, io.Discard,
	)
	if err != nil {
		cancelForward()
		return nil, err
	}
	session := &liveKubernetesForwardSession{
		stop: stop, ready: make(chan struct{}), done: make(chan error, 1),
		cancel: cancelForward, finished: make(chan struct{}),
	}
	go session.run(ctx, forwarder, internalReady)
	return session, nil
}

func (f *liveKubernetesForwarder) effectiveHandshakeTimeout() time.Duration {
	if f.handshakeTimeout > 0 {
		return f.handshakeTimeout
	}
	return kubernetesPortForwardHandshakeTimeout(kubeexecutor.DefaultRequestTimeoutSeconds)
}

func kubernetesPortForwardHandshakeTimeout(requestTimeoutSeconds int) time.Duration {
	timeout := time.Duration(requestTimeoutSeconds) * time.Second
	maximum := time.Duration(kubeexecutor.DefaultRequestTimeoutSeconds) * time.Second
	if timeout <= 0 || timeout > maximum {
		return maximum
	}
	return timeout
}

type liveKubernetesForwardSession struct {
	mu        sync.RWMutex
	closeOnce sync.Once
	localPort uint16
	stop      chan struct{}
	ready     chan struct{}
	done      chan error
	cancel    context.CancelFunc
	finished  chan struct{}
}

func (s *liveKubernetesForwardSession) run(ctx context.Context, forwarder *portforward.PortForwarder, internalReady <-chan struct{}) {
	defer close(s.finished)
	forwardDone := make(chan error, 1)
	go func() { forwardDone <- forwarder.ForwardPorts() }()
	select {
	case <-ctx.Done():
		s.signalClose()
		<-forwardDone
		s.done <- ctx.Err()
	case err := <-forwardDone:
		s.done <- err
	case <-internalReady:
		ports, err := forwarder.GetPorts()
		if err != nil || len(ports) != 1 || ports[0].Local == 0 {
			s.signalClose()
			<-forwardDone
			if err == nil {
				err = errors.New("port-forward did not allocate one local port")
			}
			s.done <- err
			return
		}
		s.mu.Lock()
		s.localPort = ports[0].Local
		s.mu.Unlock()
		close(s.ready)
		s.done <- <-forwardDone
	}
}

func (s *liveKubernetesForwardSession) LocalPort() uint16 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.localPort
}

func (s *liveKubernetesForwardSession) Ready() <-chan struct{} { return s.ready }
func (s *liveKubernetesForwardSession) Done() <-chan error     { return s.done }
func (s *liveKubernetesForwardSession) Close() error {
	s.signalClose()
	if s.finished != nil {
		<-s.finished
	}
	return nil
}

func (s *liveKubernetesForwardSession) signalClose() {
	s.closeOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		close(s.stop)
	})
}
