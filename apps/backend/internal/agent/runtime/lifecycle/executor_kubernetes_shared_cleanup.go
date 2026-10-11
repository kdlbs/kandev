package lifecycle

import (
	"context"
	"errors"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"

	"github.com/kandev/kandev/internal/task/models"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func (r *KubernetesExecutor) stopSharedKubernetesInstance(ctx context.Context, instance *ExecutorInstance) (returnedErr error) {
	unlock := r.lockInstance(instance.InstanceID)
	defer unlock()
	ctx, cancel := kubernetesDurableContext(ctx)
	defer cancel()
	attachments := r.sharedKubernetesSessionSnapshot(instance)
	defer func() {
		if returnedErr == nil {
			r.closeSharedKubernetesSessionSnapshot(attachments)
		}
	}()
	session := attachments[instance.InstanceID]
	if session == nil {
		for _, candidate := range attachments {
			session = candidate
			break
		}
	}
	if r.environmentStore == nil || r.secretStore == nil {
		return errors.New("kubernetes task environment store is unavailable")
	}
	environmentID := getMetadataString(instance.Metadata, MetadataKeyKubernetesResourceEnvironmentID)
	record, err := r.environmentStore.GetKubernetesEnvironment(ctx, environmentID)
	if errors.Is(err, models.ErrKubernetesEnvironmentNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if record.TaskID != instance.TaskID {
		return models.ErrWorkspaceReuseUnsafe
	}

	return r.stopSharedKubernetesRemote(ctx, instance, record, session)
}

func (r *KubernetesExecutor) stopSharedKubernetesRemote(ctx context.Context, instance *ExecutorInstance, record *models.KubernetesEnvironment, session *kubernetesSession) error {
	environmentID := getMetadataString(instance.Metadata, MetadataKeyKubernetesResourceEnvironmentID)
	var err error

	instance, err = r.taskKubernetesStopInstance(ctx, instance, record)
	if err != nil {
		return err
	}
	runtime, err := r.kubernetesCleanupRuntime(instance, session)
	if err != nil {
		return err
	}
	recorded, identity, err := kubernetesCleanupInventory(instance)
	if err != nil {
		return err
	}
	pod, err := runtime.resources.GetPod(ctx, recorded.namespace, recorded.podName)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := verifyRecordedPod(pod, recorded.namespace, recorded.podName, recorded.podUID, identity); err != nil {
		return err
	}
	if record.ControlSecretID == "" {
		return models.ErrWorkspaceReuseUnsafe
	}
	forward, control, err := r.connectHealthyKubernetesControl(ctx, runtime, pod)
	if err != nil {
		return err
	}
	defer func() { _ = forward.Close() }()
	req := &ExecutorCreateRequest{TaskEnvironmentID: environmentID, Metadata: cloneKubernetesMetadata(instance.Metadata)}
	req.Metadata[MetadataKeyAuthTokenSecret] = record.ControlSecretID
	req.Metadata[MetadataKeyBootstrapNonceSecret] = record.BootstrapSecretID
	_, err = r.withSharedKubernetesControlAuth(ctx, req, control, func() error {
		return control.DeleteInstance(ctx, getMetadataString(instance.Metadata, MetadataKeyKubernetesAgentctlInstanceID))
	})
	return err
}

// Rollback owns only the newly attached agent, even if credential persistence failed.
func (r *KubernetesExecutor) rollbackTaskKubernetesAttachment(ctx context.Context, instance *ExecutorInstance) error {
	session := r.closeKubernetesSession(instance.InstanceID)
	runtime, err := r.kubernetesCleanupRuntime(instance, session)
	if err != nil {
		return err
	}
	recorded, identity, err := kubernetesCleanupInventory(instance)
	if err != nil {
		return err
	}
	pod, err := runtime.resources.GetPod(ctx, recorded.namespace, recorded.podName)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := verifyRecordedPod(pod, recorded.namespace, recorded.podName, recorded.podUID, identity); err != nil {
		return err
	}
	forward, control, err := r.connectHealthyKubernetesControl(ctx, runtime, pod)
	if err != nil {
		return err
	}
	defer func() { _ = forward.Close() }()
	control.SetAuthToken(instance.AuthToken)
	return control.DeleteInstance(ctx, getMetadataString(instance.Metadata, MetadataKeyKubernetesAgentctlInstanceID))
}

func (r *KubernetesExecutor) resolveTaskKubernetesStopOwnership(ctx context.Context, instance *ExecutorInstance) error {
	if getMetadataBool(instance.Metadata, metadataKubernetesTaskOwned) || r.environmentStore == nil {
		return nil
	}
	envID := getMetadataString(instance.Metadata, MetadataKeyKubernetesResourceEnvironmentID)
	if envID == "" {
		return nil
	}
	record, err := r.environmentStore.GetKubernetesEnvironment(ctx, envID)
	if errors.Is(err, models.ErrKubernetesEnvironmentNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if record.TaskID != instance.TaskID || getMetadataString(record.Metadata, MetadataKeyKubernetesPodUID) != getMetadataString(instance.Metadata, MetadataKeyKubernetesPodUID) {
		return models.ErrWorkspaceReuseUnsafe
	}
	instance.Metadata[metadataKubernetesTaskOwned] = true
	return nil
}

func (r *KubernetesExecutor) taskKubernetesStopInstance(ctx context.Context, instance *ExecutorInstance, record *models.KubernetesEnvironment) (*ExecutorInstance, error) {
	current, err := r.environmentStore.GetExecutor(ctx, getMetadataString(record.Metadata, MetadataKeyKubernetesResourceExecutorID))
	if err != nil {
		return nil, err
	}
	if current == nil || current.Type != models.ExecutorTypeKubernetes {
		return nil, models.ErrWorkspaceReuseUnsafe
	}
	config, err := kubeexecutor.ParseExecutorConfig(current.Config)
	if err != nil {
		return nil, err
	}
	remoteID := getMetadataString(instance.Metadata, MetadataKeyKubernetesAgentctlInstanceID)
	copyInstance := *instance
	copyInstance.Metadata = overlayCurrentKubernetesConnectionMetadata(record.Metadata, current.Config, config)
	copyInstance.Metadata[MetadataKeyKubernetesAgentctlInstanceID] = remoteID
	return &copyInstance, nil
}

// Capture exact attachments before remote work; a later refresh must not be
// removed by completion of an older stop.
func (r *KubernetesExecutor) sharedKubernetesSessionSnapshot(instance *ExecutorInstance) map[string]*kubernetesSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	snapshot := make(map[string]*kubernetesSession)
	for id, session := range r.sessions {
		if id == instance.InstanceID || matchesSharedKubernetesSession(session, instance) {
			snapshot[id] = session
		}
	}
	return snapshot
}

func (r *KubernetesExecutor) closeSharedKubernetesSessionSnapshot(snapshot map[string]*kubernetesSession) {
	r.mu.Lock()
	var closing []*kubernetesSession
	for id, session := range snapshot {
		if r.sessions[id] == session {
			delete(r.sessions, id)
			closing = append(closing, session)
		}
	}
	r.mu.Unlock()
	for _, session := range closing {
		_ = closeKubernetesSessionResources(session)
	}
}

func matchesSharedKubernetesSession(session *kubernetesSession, instance *ExecutorInstance) bool {
	if session == nil || session.request == nil {
		return false
	}
	req := session.request
	remoteID := getMetadataString(req.Metadata, MetadataKeyKubernetesAgentctlInstanceID)
	if remoteID == "" {
		remoteID = req.InstanceID
	}
	return req.TaskID == instance.TaskID && req.SessionID == instance.SessionID &&
		req.TaskEnvironmentID == getMetadataString(instance.Metadata, MetadataKeyKubernetesResourceEnvironmentID) &&
		remoteID == getMetadataString(instance.Metadata, MetadataKeyKubernetesAgentctlInstanceID)
}
