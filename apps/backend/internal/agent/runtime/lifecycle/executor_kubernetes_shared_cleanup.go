package lifecycle

import (
	"context"
	"errors"
	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"

	"github.com/kandev/kandev/internal/task/models"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func (r *KubernetesExecutor) stopSharedKubernetesInstance(ctx context.Context, instance *ExecutorInstance) error {
	unlock := r.lockInstance(instance.InstanceID)
	defer unlock()
	ctx, cancel := kubernetesDurableContext(ctx)
	defer cancel()
	session := r.closeSharedKubernetesSessions(instance)
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
	token, err := r.secretStore.Reveal(ctx, record.ControlSecretID)
	if err != nil {
		return err
	}
	forward, control, err := r.connectHealthyKubernetesControl(ctx, runtime, pod)
	if err != nil {
		return err
	}
	defer func() { _ = forward.Close() }()
	control.SetAuthToken(token)
	return control.DeleteInstance(ctx, getMetadataString(instance.Metadata, MetadataKeyKubernetesAgentctlInstanceID))
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

// A resumed execution can have a new local ID while agentctl retains the remote
// session ID. Release every local connection for that exact remote session.
func (r *KubernetesExecutor) closeSharedKubernetesSessions(instance *ExecutorInstance) *kubernetesSession {
	r.mu.Lock()
	var closing []*kubernetesSession
	for id, session := range r.sessions {
		if id == instance.InstanceID || matchesSharedKubernetesSession(session, instance) {
			delete(r.sessions, id)
			closing = append(closing, session)
		}
	}
	r.mu.Unlock()
	for _, session := range closing {
		_ = closeKubernetesSessionResources(session)
	}
	if len(closing) == 0 {
		return nil
	}
	// Every matching connection uses the same pod and executor configuration;
	// any one supplies the fallback runtime client after its forward is closed.
	return closing[0]
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
