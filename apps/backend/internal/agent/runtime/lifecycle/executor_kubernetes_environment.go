package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/executor"
	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

const metadataKubernetesTaskOwned = kubeexecutor.MetadataKeyTaskOwned

// KubernetesEnvironmentStore separates physical inventory from session execution rows.
type KubernetesEnvironmentStore interface {
	GetExecutor(context.Context, string) (*models.Executor, error)
	GetTaskEnvironment(context.Context, string) (*models.TaskEnvironment, error)
	GetKubernetesEnvironment(context.Context, string) (*models.KubernetesEnvironment, error)
	ClaimKubernetesEnvironment(context.Context, string, string, int64, string) (*models.KubernetesEnvironment, error)
	ClaimKubernetesEnvironmentCleanup(context.Context, string, string, int64, string) (*models.KubernetesEnvironment, error)
	SaveKubernetesEnvironment(context.Context, *models.KubernetesEnvironment, bool) error
	DeleteKubernetesEnvironment(context.Context, *models.KubernetesEnvironment) error
	ListExecutorsRunningByTaskID(context.Context, string) ([]*models.ExecutorRunning, error)
}

func (m *Manager) wireKubernetesEnvironmentStore() {
	if m.executorRegistry == nil {
		return
	}
	backend, err := m.executorRegistry.GetBackend(executor.NameKubernetes)
	if err != nil {
		return
	}
	runtime, ok := backend.(*KubernetesExecutor)
	if !ok {
		return
	}
	runtime.environmentStore, _ = m.runningWriter.(KubernetesEnvironmentStore)
	runtime.secretStore = m.secretStore
}

func (r *KubernetesExecutor) createTaskInstance(ctx context.Context, original *ExecutorCreateRequest) (_ *ExecutorInstance, returnedErr error) {
	unlock := r.lockInstance("environment:" + original.TaskEnvironmentID)
	defer unlock()
	legacy, err := r.isUnadoptedKubernetesResume(ctx, original)
	if err != nil {
		return nil, err
	}
	if legacy {
		return r.createSessionInstance(ctx, original)
	}
	record, err := r.claimTaskKubernetesEnvironment(ctx, original)
	if err != nil {
		return nil, err
	}
	defer func() {
		if record.OperationID != "" {
			persistCtx, cancel := kubernetesDurableContext(ctx)
			defer cancel()
			returnedErr = errors.Join(returnedErr, r.environmentStore.SaveKubernetesEnvironment(persistCtx, record, true))
		}
	}()
	req, err := r.taskKubernetesCreateRequest(ctx, original, record)
	if err != nil {
		return nil, err
	}
	instance, err := r.createSessionInstance(ctx, req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if returnedErr != nil {
			rollbackCtx, cancel := kubernetesDurableContext(ctx)
			defer cancel()
			returnedErr = errors.Join(returnedErr, r.rollbackTaskKubernetesAttachment(rollbackCtx, instance))
		}
	}()
	instance.Metadata[metadataKubernetesTaskOwned] = true
	instance.Metadata[MetadataKeyCleanupScript] = getMetadataString(req.Metadata, MetadataKeyCleanupScript)
	record.Metadata = cloneKubernetesMetadata(instance.Metadata)
	if err := r.persistKubernetesEnvironmentSecrets(ctx, record, instance); err != nil {
		return nil, err
	}
	if err := r.environmentStore.SaveKubernetesEnvironment(ctx, record, true); err != nil {
		return nil, err
	}
	return instance, nil
}

func (r *KubernetesExecutor) adoptKubernetesEnvironment(ctx context.Context, req *ExecutorCreateRequest, record *models.KubernetesEnvironment) error {
	rows, err := r.environmentStore.ListExecutorsRunningByTaskID(ctx, req.TaskID)
	if err != nil {
		return err
	}
	var candidate *models.ExecutorRunning
	for _, row := range rows {
		if row == nil || row.Runtime != executor.NameKubernetes {
			continue
		}
		if getMetadataString(row.Metadata, MetadataKeyKubernetesResourceEnvironmentID) != req.TaskEnvironmentID {
			return models.ErrWorkspaceReuseUnsafe
		}
		if getMetadataString(row.Metadata, MetadataKeyKubernetesPodUID) == "" {
			return models.ErrWorkspaceReuseUnsafe
		}
		if candidate != nil && !kubernetesLegacyInventoriesAgree(candidate.Metadata, row.Metadata) {
			return models.ErrWorkspaceReuseUnsafe
		}
		candidate = row
	}
	if candidate == nil {
		return nil
	}
	record.Metadata = cloneKubernetesMetadata(candidate.Metadata)
	record.ControlSecretID = getMetadataString(record.Metadata, MetadataKeyAuthTokenSecret)
	record.BootstrapSecretID = getMetadataString(record.Metadata, MetadataKeyBootstrapNonceSecret)
	return nil
}

func (r *KubernetesExecutor) attachKubernetesEnvironmentRequest(ctx context.Context, req *ExecutorCreateRequest, record *models.KubernetesEnvironment) error {
	if getMetadataString(req.Metadata, "executor_id") != getMetadataString(record.Metadata, MetadataKeyKubernetesResourceExecutorID) || getMetadataString(req.Metadata, MetadataKeyExecutorProfileID) != getMetadataString(record.Metadata, MetadataKeyKubernetesResourceProfileID) {
		return models.ErrWorkspaceReuseUnsafe
	}
	if record.ControlSecretID == "" || record.BootstrapSecretID == "" {
		return models.ErrWorkspaceReuseUnsafe
	}
	token, err := r.secretStore.Reveal(ctx, record.ControlSecretID)
	if err != nil {
		return err
	}
	nonce, err := r.secretStore.Reveal(ctx, record.BootstrapSecretID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(token) == "" || strings.TrimSpace(nonce) == "" {
		return models.ErrWorkspaceReuseUnsafe
	}
	remoteID := req.InstanceID
	if req.PreviousExecutionID != "" {
		if previousID := getMetadataString(req.Metadata, MetadataKeyKubernetesAgentctlInstanceID); previousID != "" {
			remoteID = previousID
		}
	}
	for key, value := range record.Metadata {
		req.Metadata[key] = value
	}
	req.Metadata[metadataKubernetesTaskOwned] = true
	// A resumed session retains its own instance independently of sibling inventory.
	req.Metadata[MetadataKeyKubernetesAgentctlInstanceID] = remoteID
	req.AuthToken = token
	req.BootstrapNonce = nonce
	return nil
}

func (r *KubernetesExecutor) persistKubernetesEnvironmentSecrets(ctx context.Context, record *models.KubernetesEnvironment, instance *ExecutorInstance) error {
	refs := []struct {
		target           *string
		name, value, key string
	}{
		{&record.ControlSecretID, "agentctl-auth", instance.AuthToken, MetadataKeyAuthTokenSecret},
		{&record.BootstrapSecretID, "agentctl-bootstrap", instance.BootstrapNonce, MetadataKeyBootstrapNonceSecret},
	}
	for _, item := range refs {
		if item.value == "" {
			return errors.New("kubernetes environment control credential is empty")
		}
		*item.target = kubernetesRuntimeSecretID(record.EnvironmentID, item.name)
		name := item.name + "-" + record.EnvironmentID
		if _, err := r.secretStore.Get(ctx, *item.target); errors.Is(err, secrets.ErrNotFound) {
			if err := r.secretStore.Create(ctx, &secrets.SecretWithValue{Secret: secrets.Secret{ID: *item.target, Name: name}, Value: item.value}); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		instance.Metadata[item.key] = *item.target
		record.Metadata[item.key] = *item.target
	}
	return nil
}

func kubernetesLegacyInventoriesAgree(first, second map[string]interface{}) bool {
	keys := []string{
		MetadataKeyKubernetesPodUID, MetadataKeyKubernetesPodName, MetadataKeyKubernetesNamespace,
		MetadataKeyKubernetesPVCUID, MetadataKeyKubernetesPVCName, MetadataKeyKubernetesResourceExecutorID,
		MetadataKeyKubernetesResourceProfileID, MetadataKeyKubernetesResourceInstanceID,
		MetadataKeyKubernetesResourceTaskID, MetadataKeyKubernetesResourceSessionID,
		MetadataKeyKubernetesResourceEnvironmentID, MetadataKeyKubernetesProfileSnapshot,
		MetadataKeyKubernetesInventoryState, MetadataKeyAuthTokenSecret, MetadataKeyBootstrapNonceSecret,
	}
	for _, key := range keys {
		if getMetadataString(first, key) != getMetadataString(second, key) {
			return false
		}
	}
	return getMetadataBool(first, MetadataKeyKubernetesPVCCreated) == getMetadataBool(second, MetadataKeyKubernetesPVCCreated)
}

func (r *KubernetesExecutor) claimTaskKubernetesEnvironment(ctx context.Context, original *ExecutorCreateRequest) (*models.KubernetesEnvironment, error) {
	env, err := r.environmentStore.GetTaskEnvironment(ctx, original.TaskEnvironmentID)
	if err != nil {
		return nil, fmt.Errorf("load Kubernetes task environment: %w", err)
	}
	if env == nil || env.TaskID != original.TaskID || env.ExecutorType != "k8s" {
		return nil, models.ErrWorkspaceReuseUnsafe
	}
	if original.WorkspaceReuseRequired && env.Status != models.TaskEnvironmentStatusReady {
		if env.Status == models.TaskEnvironmentStatusCreating {
			return nil, models.ErrWorkspacePreparing
		}
		return nil, models.ErrWorkspaceReuseUnsafe
	}
	if r.secretStore == nil {
		return nil, errors.New("kubernetes environment secret store is unavailable")
	}
	record, err := r.environmentStore.ClaimKubernetesEnvironment(ctx, env.ID, env.TaskID, env.OwnershipGeneration, uuid.NewString())
	if err != nil {
		return nil, fmt.Errorf("%w: claim Kubernetes runtime: %w", models.ErrWorkspaceReuseUnsafe, err)
	}
	return record, nil
}

func (r *KubernetesExecutor) taskKubernetesCreateRequest(ctx context.Context, original *ExecutorCreateRequest, record *models.KubernetesEnvironment) (*ExecutorCreateRequest, error) {
	req := cloneKubernetesCreateRequest(original)
	req.Metadata[metadataKubernetesTaskOwned] = true
	req.Env = kubernetesSessionEnvironment(req)
	if len(record.Metadata) == 0 {
		if err := r.adoptKubernetesEnvironment(ctx, req, record); err != nil {
			return nil, err
		}
	}
	if len(record.Metadata) > 0 {
		if err := r.attachKubernetesEnvironmentRequest(ctx, req, record); err != nil {
			return nil, err
		}
	} else if req.WorkspaceReuseRequired {
		return nil, models.ErrWorkspaceReuseUnsafe
	}
	previousCheckpoint := req.CheckpointRuntimeInventory
	req.CheckpointRuntimeInventory = func(checkpointCtx context.Context, metadata map[string]interface{}) error {
		metadata[metadataKubernetesTaskOwned] = true
		metadata[MetadataKeyCleanupScript] = getMetadataString(req.Metadata, MetadataKeyCleanupScript)
		record.Metadata = cloneKubernetesMetadata(metadata)
		if err := r.environmentStore.SaveKubernetesEnvironment(checkpointCtx, record, false); err != nil {
			return err
		}
		if previousCheckpoint != nil {
			return previousCheckpoint(checkpointCtx, metadata)
		}
		return nil
	}
	return req, nil
}

func (r *KubernetesExecutor) isUnadoptedKubernetesResume(ctx context.Context, req *ExecutorCreateRequest) (bool, error) {
	if req.PreviousExecutionID == "" || getMetadataBool(req.Metadata, metadataKubernetesTaskOwned) || getMetadataString(req.Metadata, MetadataKeyKubernetesPodUID) == "" {
		return false, nil
	}
	_, err := r.environmentStore.GetKubernetesEnvironment(ctx, req.TaskEnvironmentID)
	if !errors.Is(err, models.ErrKubernetesEnvironmentNotFound) {
		return false, err
	}
	env, err := r.environmentStore.GetTaskEnvironment(ctx, req.TaskEnvironmentID)
	if err != nil {
		return false, err
	}
	if env == nil || env.TaskID != req.TaskID || env.ExecutorType != "k8s" {
		return false, models.ErrWorkspaceReuseUnsafe
	}
	return true, nil
}
