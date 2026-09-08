package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const kubernetesDurableWriteTimeout = time.Minute

func kubernetesDurableContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(ctx), kubernetesDurableWriteTimeout)
}

// wireKubernetesInventoryPersistence installs lifecycle-owned callbacks only
// for a fresh Kubernetes launch. Reconnect/replacement already has an
// authoritative row that must never be released as a provisional launch row.
func (m *Manager) wireKubernetesInventoryPersistence(req *ExecutorCreateRequest, executorType string) {
	if req == nil || executorType != string(models.ExecutorTypeKubernetes) || m.runningWriter == nil {
		return
	}
	req.CheckpointRuntimeInventory = func(ctx context.Context, metadata map[string]interface{}) error {
		return m.checkpointKubernetesRuntimeInventory(ctx, req, metadata)
	}
	if req.PreviousExecutionID == "" && getMetadataString(req.Metadata, MetadataKeyKubernetesPodName) == "" {
		req.ReleaseRuntimeInventory = func(ctx context.Context) error {
			return m.releaseKubernetesRuntimeInventory(ctx, req)
		}
	}
}

func (m *Manager) checkpointKubernetesRuntimeInventory(
	ctx context.Context,
	req *ExecutorCreateRequest,
	runtimeMetadata map[string]interface{},
) error {
	if m.runningWriter == nil {
		return nil
	}
	if req == nil || strings.TrimSpace(req.InstanceID) == "" || strings.TrimSpace(req.TaskID) == "" ||
		strings.TrimSpace(req.SessionID) == "" || strings.TrimSpace(req.AgentProfileID) == "" {
		return errors.New("checkpoint Kubernetes runtime inventory: launch identity is incomplete")
	}
	persistCtx, cancelPersist := kubernetesDurableContext(ctx)
	defer cancelPersist()
	ctx = persistCtx
	metadata := cloneKubernetesMetadata(req.Metadata)
	for key, value := range runtimeMetadata {
		metadata[key] = value
	}
	if strings.TrimSpace(getMetadataString(metadata, MetadataKeyKubernetesInventoryState)) == "" {
		return errors.New("checkpoint Kubernetes runtime inventory: state is missing")
	}

	var prior *models.ExecutorRunning
	if reader, ok := m.runningWriter.(executorRunningReader); ok {
		existing, err := reader.GetExecutorRunningBySessionID(ctx, req.SessionID)
		switch {
		case err == nil:
			prior = existing
		case errors.Is(err, models.ErrExecutorRunningNotFound):
		default:
			return fmt.Errorf("checkpoint Kubernetes runtime inventory: read prior row: %w", err)
		}
	}
	execution := &AgentExecution{
		ID: req.InstanceID, TaskID: req.TaskID, SessionID: req.SessionID,
		TaskEnvironmentID: req.TaskEnvironmentID, AgentProfileID: req.AgentProfileID,
		RuntimeName: agentruntime.RuntimeKubernetes, Status: v1.AgentStatusStarting,
		metadata: metadata,
	}
	running := buildRunningFromExecution(execution, prior)
	running.Runtime = agentruntime.RuntimeKubernetes
	running.Status = models.ExecutorRunningStatusStarting
	if err := m.runningWriter.UpsertExecutorRunning(ctx, running); err != nil {
		return fmt.Errorf("checkpoint Kubernetes runtime inventory: upsert row: %w", err)
	}
	return nil
}

func (m *Manager) releaseKubernetesRuntimeInventory(
	ctx context.Context,
	req *ExecutorCreateRequest,
) error {
	if m.runningWriter == nil || req == nil {
		return nil
	}
	persistCtx, cancelPersist := kubernetesDurableContext(ctx)
	defer cancelPersist()
	ctx = persistCtx
	reader, hasReader := m.runningWriter.(executorRunningReader)
	casWriter, hasCAS := m.runningWriter.(executorRunningCASWriter)
	if !hasReader || !hasCAS {
		return errors.New("release Kubernetes runtime inventory: exact CAS persistence is unavailable")
	}
	current, err := reader.GetExecutorRunningBySessionID(ctx, req.SessionID)
	if errors.Is(err, models.ErrExecutorRunningNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("release Kubernetes runtime inventory: read row: %w", err)
	}
	if current == nil || current.AgentExecutionID != req.InstanceID {
		return fmt.Errorf("release Kubernetes runtime inventory: %w", models.ErrExecutionRotated)
	}
	if err := casWriter.DeleteExecutorRunningIfCurrent(
		ctx, req.SessionID, req.InstanceID, current.UpdatedAt,
	); err != nil && !errors.Is(err, models.ErrExecutorRunningNotFound) {
		return fmt.Errorf("release Kubernetes runtime inventory: delete row: %w", err)
	}
	return nil
}

func releaseExecutorInstanceRuntimeInventory(ctx context.Context, instance *ExecutorInstance) error {
	if instance == nil || instance.ReleaseRuntimeInventory == nil {
		return nil
	}
	return instance.ReleaseRuntimeInventory(ctx)
}

func stopRuntimeInstanceAndRelease(
	ctx context.Context,
	runtime ExecutorBackend,
	instance *ExecutorInstance,
	force bool,
) error {
	if runtime == nil || instance == nil {
		return nil
	}
	if err := runtime.StopInstance(ctx, instance, force); err != nil {
		return err
	}
	if err := releaseExecutorInstanceRuntimeInventory(ctx, instance); err != nil {
		return fmt.Errorf("release stopped runtime inventory: %w", err)
	}
	return nil
}
