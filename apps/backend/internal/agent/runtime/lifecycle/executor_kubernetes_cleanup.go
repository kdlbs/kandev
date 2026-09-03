package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/scriptengine"
	"go.uber.org/zap"
)

func (r *KubernetesExecutor) StopInstance(ctx context.Context, instance *ExecutorInstance, force bool) error {
	if instance == nil {
		return nil
	}
	unlock := r.lockInstance(instance.InstanceID)
	defer unlock()

	session := r.closeKubernetesSession(instance.InstanceID)
	if !force && !shouldRunExecutorCleanup(instance.StopReason) {
		return nil
	}
	runtime, err := r.kubernetesCleanupRuntime(instance, session)
	if err != nil {
		return err
	}
	if runtime == nil || runtime.resources == nil {
		return errors.New("kubernetes lifecycle: cleanup resource client is unavailable")
	}
	recorded, identity, err := kubernetesCleanupInventory(instance)
	if err != nil {
		return err
	}
	verifyCtx, cancelVerify := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
	if err := verifyKubernetesCleanupTargets(verifyCtx, runtime.resources, recorded, identity); err != nil {
		cancelVerify()
		return err
	}
	cancelVerify()
	r.runKubernetesTerminalCleanupScript(ctx, runtime.streams, instance, recorded)
	deleteCtx, cancelDelete := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
	defer cancelDelete()
	return deleteKubernetesResources(deleteCtx, runtime.resources, recorded, identity)
}

func (r *KubernetesExecutor) closeKubernetesSession(instanceID string) *kubernetesSession {
	session := r.takeSession(instanceID)
	_ = closeKubernetesSessionResources(session)
	return session
}

func closeKubernetesSessionResources(session *kubernetesSession) error {
	if session == nil {
		return nil
	}
	if session.client != nil {
		session.client.Close()
	}
	if session.forward != nil {
		return session.forward.Close()
	}
	return nil
}

// Close is the backend-shutdown safety net. It closes process-local clients
// and forwards only; Kubernetes resources stay preserved for resume.
func (r *KubernetesExecutor) Close() error {
	r.mu.Lock()
	sessions := make([]*kubernetesSession, 0, len(r.sessions))
	for instanceID, session := range r.sessions {
		sessions = append(sessions, session)
		delete(r.sessions, instanceID)
	}
	r.mu.Unlock()

	var closeErrors []error
	for _, session := range sessions {
		if err := closeKubernetesSessionResources(session); err != nil {
			closeErrors = append(closeErrors, err)
		}
	}
	return errors.Join(closeErrors...)
}

func (r *KubernetesExecutor) kubernetesCleanupRuntime(
	instance *ExecutorInstance,
	session *kubernetesSession,
) (*kubernetesRuntimeClient, error) {
	executorConfig, err := kubernetesExecutorConfigFromMetadata(instance.Metadata)
	if err != nil {
		if session != nil && session.runtime != nil {
			return session.runtime, nil
		}
		return nil, fmt.Errorf("kubernetes lifecycle: cleanup configuration: %w", err)
	}
	if r.clientFactory == nil {
		return nil, errors.New("kubernetes lifecycle: cleanup client factory is not configured")
	}
	runtime, err := r.clientFactory(executorConfig)
	if err != nil {
		return nil, fmt.Errorf("kubernetes lifecycle: cleanup client: %w", err)
	}
	return runtime, nil
}

func deleteKubernetesResources(
	ctx context.Context,
	resources kubernetesResourceClient,
	recorded kubernetesRecordedState,
	identity kubeexecutor.ResourceIdentity,
) error {
	if recorded.podUID != "" {
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Namespace: recorded.namespace, Name: recorded.podName, UID: recorded.podUID,
		}}
		var err error
		if recorded.podAdmissionVerified() {
			err = deleteKubernetesPodIfExact(ctx, resources, pod, identity)
		} else {
			err = deleteKubernetesPodByRecordedUID(ctx, resources, pod)
		}
		if err != nil {
			return err
		}
	}
	if !recorded.pvcCreated {
		return nil
	}
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
		Namespace: recorded.namespace, Name: recorded.pvcName, UID: recorded.pvcUID,
	}}
	if recorded.pvcAdmissionVerified() {
		return deleteKubernetesPVCIfExact(ctx, resources, pvc, identity)
	}
	return deleteKubernetesPVCByRecordedUID(ctx, resources, pvc)
}

func (r *KubernetesExecutor) runKubernetesTerminalCleanupScript(
	ctx context.Context,
	streams *kubeexecutor.StreamOperations,
	instance *ExecutorInstance,
	recorded kubernetesRecordedState,
) {
	if streams == nil || instance == nil || !shouldRunExecutorCleanup(instance.StopReason) ||
		recorded.podUID == "" || !recorded.podAdmissionVerified() {
		return
	}
	script := strings.TrimSpace(getMetadataString(instance.Metadata, MetadataKeyCleanupScript))
	if script == "" {
		return
	}
	resolved := scriptengine.NewResolver().
		WithStatic(map[string]string{kubernetesRepositoryCloneURL: shellQuote("")}).
		WithProvider(scriptengine.WorkspaceProvider(kubernetesWorkspacePath)).
		WithProvider(scriptengine.AgentctlProviderWithOptions(
			int(kubeexecutor.DefaultAgentctlPort), kubernetesWorkspacePath,
			scriptengine.AgentctlProviderOptions{BinaryPath: kubernetesAgentctlPath, Start: false},
		)).
		WithProvider(scriptengine.GitIdentityProvider(instance.Metadata)).
		WithProvider(scriptengine.RepositoryProvider(
			instance.Metadata, nil, getGitRemoteURL, injectGitHubTokenIntoCloneURL,
		)).
		Resolve(script)
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
	defer cancel()
	err := streams.Exec(cleanupCtx, kubeexecutor.ExecRequest{
		Namespace: recorded.namespace,
		Pod:       recorded.podName,
		Container: recorded.mainContainer,
		Command:   []string{"sh", "-c", resolved},
		Stdout:    io.Discard,
		Stderr:    io.Discard,
	})
	if err != nil {
		r.logger.Warn("Kubernetes cleanup script failed",
			zap.String("instance_id", instance.InstanceID),
			zap.String("reason", instance.StopReason),
			zap.Error(routingerr.SanitizeError(err)))
		return
	}
	r.logger.Debug("Kubernetes cleanup script completed",
		zap.String("instance_id", instance.InstanceID),
		zap.String("reason", instance.StopReason))
}

func verifyKubernetesCleanupTargets(
	ctx context.Context,
	resources kubernetesResourceClient,
	recorded kubernetesRecordedState,
	identity kubeexecutor.ResourceIdentity,
) error {
	if err := verifyKubernetesCleanupPod(ctx, resources, recorded, identity); err != nil {
		return err
	}
	return verifyKubernetesCleanupPVC(ctx, resources, recorded, identity)
}

func verifyKubernetesCleanupPod(
	ctx context.Context,
	resources kubernetesResourceClient,
	recorded kubernetesRecordedState,
	identity kubeexecutor.ResourceIdentity,
) error {
	if recorded.podUID == "" {
		return nil
	}
	pod, err := resources.GetPod(ctx, recorded.namespace, recorded.podName)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("kubernetes lifecycle: inspect Pod before cleanup: %w", err)
	}
	if err == nil {
		verifyErr := verifyKubernetesCleanupPodIdentity(pod, recorded, identity)
		if verifyErr != nil {
			return verifyErr
		}
	}
	return nil
}

func verifyKubernetesCleanupPVC(
	ctx context.Context,
	resources kubernetesResourceClient,
	recorded kubernetesRecordedState,
	identity kubeexecutor.ResourceIdentity,
) error {
	if recorded.workspaceMode == kubeexecutor.WorkspaceModeEmptyDir || recorded.pvcName == "" {
		return nil
	}
	pvc, err := resources.GetPersistentVolumeClaim(ctx, recorded.namespace, recorded.pvcName)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("kubernetes lifecycle: inspect PVC before cleanup: %w", err)
	}
	if pvc == nil || pvc.Namespace != recorded.namespace || pvc.Name != recorded.pvcName || pvc.UID != recorded.pvcUID {
		return errors.New("kubernetes lifecycle: PVC UID or name does not match recorded inventory")
	}
	if !recorded.pvcCreated || !recorded.pvcAdmissionVerified() {
		return nil
	}
	labels, err := kubeexecutor.OwnershipLabels(identity)
	if err != nil {
		return err
	}
	if err := kubeexecutor.ValidateOwnershipLabels(pvc.Labels, labels); err != nil {
		return fmt.Errorf("kubernetes lifecycle: PVC ownership labels: %w", err)
	}
	return nil
}

func (r *KubernetesExecutor) takeSession(instanceID string) *kubernetesSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	session := r.sessions[instanceID]
	delete(r.sessions, instanceID)
	return session
}

func kubernetesCleanupInventory(
	instance *ExecutorInstance,
) (kubernetesRecordedState, kubeexecutor.ResourceIdentity, error) {
	if instance == nil {
		return kubernetesRecordedState{}, kubeexecutor.ResourceIdentity{}, errors.New(
			"kubernetes lifecycle: cleanup instance is nil",
		)
	}
	metadata := instance.Metadata
	namespace := getMetadataString(metadata, MetadataKeyKubernetesNamespace)
	executorConfig := kubeexecutor.ExecutorConfig{
		AuthMode: kubeexecutor.AuthModeInCluster, Namespace: namespace,
		RequestTimeoutSeconds: kubeexecutor.DefaultRequestTimeoutSeconds,
	}
	req := &ExecutorCreateRequest{
		TaskID:              instance.TaskID,
		SessionID:           instance.SessionID,
		PreviousExecutionID: getMetadataString(metadata, MetadataKeyKubernetesResourceInstanceID),
		Metadata:            metadata,
	}
	return kubernetesRecordedCleanupInventory(req, executorConfig, false)
}

func kubernetesRecordedCleanupInventory(
	req *ExecutorCreateRequest,
	executorConfig kubeexecutor.ExecutorConfig,
	validateConnectionIdentity bool,
) (kubernetesRecordedState, kubeexecutor.ResourceIdentity, error) {
	state := getMetadataString(req.Metadata, MetadataKeyKubernetesInventoryState)
	if state == "" || state == KubernetesInventoryStateReady {
		return kubernetesRecordedInventory(req, validateConnectionIdentity)
	}
	return kubernetesRecordedProvisionalInventory(req, executorConfig, validateConnectionIdentity, state)
}

func kubernetesRecordedProvisionalInventory(
	req *ExecutorCreateRequest,
	executorConfig kubeexecutor.ExecutorConfig,
	validateConnectionIdentity bool,
	state string,
) (kubernetesRecordedState, kubeexecutor.ResourceIdentity, error) {
	recorded := decodeKubernetesRecordedStateBase(req.Metadata)
	identity, err := kubernetesRecordedResourceIdentity(req, validateConnectionIdentity)
	if err != nil {
		return kubernetesRecordedState{}, kubeexecutor.ResourceIdentity{}, err
	}
	if err := validateKubernetesProvisionalInventory(recorded, identity, executorConfig.Namespace, state); err != nil {
		return kubernetesRecordedState{}, kubeexecutor.ResourceIdentity{}, err
	}
	return recorded, identity, nil
}

func validateKubernetesProvisionalInventory(
	recorded kubernetesRecordedState,
	identity kubeexecutor.ResourceIdentity,
	namespace, state string,
) error {
	if recorded.agentctlInstanceID != identity.InstanceID {
		return errors.New(
			"kubernetes lifecycle: recorded agentctl instance does not match resource identity",
		)
	}
	if recorded.namespace == "" || recorded.namespace != namespace || recorded.mainContainer == "" {
		return errors.New(
			"kubernetes lifecycle: provisional namespace or container inventory is incomplete",
		)
	}
	if err := validateKubernetesRecordedWorkspace(recorded); err != nil {
		return err
	}
	podComplete := recorded.podName != "" && recorded.podUID != ""
	if (recorded.podName == "") != (recorded.podUID == "") {
		return errors.New(
			"kubernetes lifecycle: provisional Pod inventory is incomplete",
		)
	}
	return validateKubernetesProvisionalInventoryState(state, recorded.workspaceMode, podComplete)
}

func validateKubernetesProvisionalInventoryState(
	state string,
	workspaceMode kubeexecutor.WorkspaceMode,
	podComplete bool,
) error {
	switch state {
	case KubernetesInventoryStatePVCCreated, KubernetesInventoryStatePVCAdmitted:
		if podComplete || workspaceMode != kubeexecutor.WorkspaceModeManagedPVC {
			return errors.New(
				"kubernetes lifecycle: provisional PVC inventory state is inconsistent",
			)
		}
	case KubernetesInventoryStatePodCreated, KubernetesInventoryStatePodAdmitted:
		if !podComplete {
			return errors.New(
				"kubernetes lifecycle: provisional Pod inventory is incomplete",
			)
		}
	default:
		return errors.New(
			"kubernetes lifecycle: provisional inventory state is invalid",
		)
	}
	return nil
}

func (recorded kubernetesRecordedState) podAdmissionVerified() bool {
	return recorded.inventoryState != KubernetesInventoryStatePodCreated
}

func (recorded kubernetesRecordedState) pvcAdmissionVerified() bool {
	return recorded.inventoryState != KubernetesInventoryStatePVCCreated
}

func verifyKubernetesCleanupPodIdentity(
	pod *corev1.Pod,
	recorded kubernetesRecordedState,
	identity kubeexecutor.ResourceIdentity,
) error {
	if recorded.podAdmissionVerified() {
		return verifyRecordedPod(pod, recorded.namespace, recorded.podName, recorded.podUID, identity)
	}
	if pod == nil || pod.Namespace != recorded.namespace || pod.Name != recorded.podName ||
		pod.UID != recorded.podUID || recorded.podUID == "" {
		return errors.New("kubernetes lifecycle: Pod UID or name does not match recorded inventory")
	}
	return nil
}

func deleteKubernetesPodByRecordedUID(
	ctx context.Context,
	resources kubernetesResourceClient,
	recorded *corev1.Pod,
) error {
	return deleteKubernetesResourceByRecordedUID(ctx, kubernetesPodDeletionTarget(resources, recorded))
}

func deleteKubernetesPVCByRecordedUID(
	ctx context.Context,
	resources kubernetesResourceClient,
	recorded *corev1.PersistentVolumeClaim,
) error {
	return deleteKubernetesResourceByRecordedUID(ctx, kubernetesPVCDeletionTarget(resources, recorded))
}

func deleteKubernetesResourceByRecordedUID(ctx context.Context, target kubernetesDeletionTarget) error {
	current, err := target.get(ctx)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf(
			"kubernetes lifecycle: inspect provisional %s before deletion: %w", target.kind, err,
		)
	}
	if current.namespace != target.namespace || current.name != target.name ||
		current.uid != target.uid || target.uid == "" {
		return fmt.Errorf(
			"kubernetes lifecycle: provisional %s UID or name does not match recorded inventory", target.kind,
		)
	}
	if err := target.delete(ctx, target.uid, current.resourceVersion); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("kubernetes lifecycle: delete provisional %s: %w", target.kind, err)
	}
	return target.wait(ctx, target.uid)
}
