package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/executor"
	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

// DestroyKubernetesEnvironment is called only by task resource teardown after
// session termination and the task cleanup barrier have excluded new launches.
func (m *Manager) DestroyKubernetesEnvironment(ctx context.Context, env *models.TaskEnvironment) error {
	if env == nil {
		return nil
	}
	if m.executorRegistry == nil {
		return errors.New("kubernetes runtime registry is unavailable")
	}
	backend, err := m.executorRegistry.GetBackend(executor.NameKubernetes)
	if err != nil {
		return err
	}
	runtime, ok := backend.(*KubernetesExecutor)
	if !ok {
		return errors.New("kubernetes runtime is unavailable")
	}
	return runtime.destroyTaskEnvironment(ctx, env)
}

func (r *KubernetesExecutor) destroyTaskEnvironment(ctx context.Context, env *models.TaskEnvironment) (returnedErr error) {
	if r.environmentStore == nil {
		return errors.New("kubernetes environment inventory is unavailable")
	}
	unlock := r.lockInstance("environment:" + env.ID)
	defer unlock()
	if _, err := r.environmentStore.GetKubernetesEnvironment(ctx, env.ID); errors.Is(err, models.ErrKubernetesEnvironmentNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	if err := r.ensureKubernetesEnvironmentStopped(ctx, env); err != nil {
		return err
	}
	record, err := r.environmentStore.ClaimKubernetesEnvironmentCleanup(ctx, env.ID, env.TaskID, env.OwnershipGeneration, uuid.NewString())
	if err != nil {
		return err
	}
	defer func() {
		if returnedErr != nil {
			persistCtx, cancel := kubernetesDurableContext(ctx)
			defer cancel()
			returnedErr = errors.Join(returnedErr, r.environmentStore.SaveKubernetesEnvironment(persistCtx, record, true))
		}
	}()
	if err := r.deleteTaskKubernetesResources(ctx, record); err != nil {
		return err
	}
	for _, id := range []string{record.ControlSecretID, record.BootstrapSecretID} {
		if id == "" {
			continue
		}
		if r.secretStore == nil {
			return errors.New("kubernetes environment secret store is unavailable")
		}
		if err := r.secretStore.Delete(ctx, id); err != nil && !errors.Is(err, secrets.ErrNotFound) {
			return err
		}
	}
	return r.environmentStore.DeleteKubernetesEnvironment(ctx, record)
}

func (r *KubernetesExecutor) ensureKubernetesEnvironmentStopped(ctx context.Context, env *models.TaskEnvironment) error {
	r.mu.Lock()
	for _, session := range r.sessions {
		if session.request != nil && session.request.TaskEnvironmentID == env.ID {
			r.mu.Unlock()
			return models.ErrWorkspaceReuseUnsafe
		}
	}
	r.mu.Unlock()
	rows, err := r.environmentStore.ListExecutorsRunningByTaskID(ctx, env.TaskID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row == nil || getMetadataString(row.Metadata, MetadataKeyKubernetesResourceEnvironmentID) != env.ID {
			continue
		}
		switch row.Status {
		case models.ExecutorRunningStatusStopped, models.ExecutorRunningStatusFailed, models.ExecutorRunningStatusComplete:
		default:
			return models.ErrWorkspaceReuseUnsafe
		}
	}
	return nil
}

func (r *KubernetesExecutor) deleteTaskKubernetesResources(ctx context.Context, record *models.KubernetesEnvironment) error {
	if len(record.Metadata) == 0 {
		return nil
	}
	current, err := r.environmentStore.GetExecutor(ctx, getMetadataString(record.Metadata, MetadataKeyKubernetesResourceExecutorID))
	if err != nil {
		return err
	}
	if current == nil || current.Type != models.ExecutorTypeKubernetes {
		return models.ErrWorkspaceReuseUnsafe
	}
	config, err := kubeexecutor.ParseExecutorConfig(current.Config)
	if err != nil {
		return err
	}
	metadata := overlayCurrentKubernetesConnectionMetadata(record.Metadata, current.Config, config)
	instance := &ExecutorInstance{TaskID: record.TaskID, SessionID: getMetadataString(metadata, MetadataKeyKubernetesResourceSessionID), Metadata: metadata, StopReason: StopReasonTaskDeleted}
	runtime, err := r.kubernetesCleanupRuntime(instance, nil)
	if err != nil {
		return err
	}
	recorded, identity, err := kubernetesCleanupInventory(instance)
	if err != nil {
		return err
	}
	if err := verifyKubernetesCleanupTargets(ctx, runtime.resources, recorded, identity); err != nil {
		return err
	}
	r.runKubernetesTerminalCleanupScript(ctx, runtime.streams, instance, recorded)
	if err := deleteKubernetesResources(ctx, runtime.resources, recorded, identity); err != nil {
		return fmt.Errorf("delete task Kubernetes resources: %w", err)
	}
	return nil
}
