package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
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
	streamingConfig, err := streamingRESTConfig(client)
	if err != nil {
		return err
	}
	requestURL := client.Clientset.CoreV1().RESTClient().Post().
		Namespace(namespace).Resource("pods").Name(podName).SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: mainContainer, Command: []string{"sh", "-c", ":"}, Stdout: true,
		}, scheme.ParameterCodec).URL()
	websocketExecutor, err := remotecommand.NewWebSocketExecutor(
		streamingConfig, http.MethodGet, requestURL.String(),
	)
	if err != nil {
		return err
	}
	spdyExecutor, err := remotecommand.NewSPDYExecutor(
		streamingConfig, http.MethodPost, requestURL,
	)
	if err != nil {
		return err
	}
	executor, err := remotecommand.NewFallbackExecutor(
		websocketExecutor, spdyExecutor, shouldFallbackStreaming,
	)
	if err != nil {
		return err
	}
	return executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdout: io.Discard})
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

func (h *Handler) runStreamingStep(
	ctx context.Context,
	client *agentkubernetes.Client,
	namespace string,
	permissions []requiredPermission,
	profile *agentkubernetes.ProfileConfig,
	prerequisitesPassed bool,
) TestStep {
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
	started := time.Now()
	if err := h.runLiveStreamingProbe(ctx, client, namespace, *profile); err != nil {
		detail := "Streaming upgrade transport is incompatible"
		if errors.Is(err, errStreamingProbeCleanup) {
			detail = "Streaming probe cleanup failed"
		}
		return failedStep("streaming", started, detail, err)
	}
	step.DurationMS += time.Since(started).Milliseconds()
	step.Detail = "Exec and port-forward transports succeeded against a disposable Pod"
	return step
}

func (h *Handler) runLiveStreamingProbe(
	ctx context.Context,
	client *agentkubernetes.Client,
	namespace string,
	profile agentkubernetes.ProfileConfig,
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
	return h.probeStreaming(ctx, client, running.Namespace, running.Name, profile.MainContainer)
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
