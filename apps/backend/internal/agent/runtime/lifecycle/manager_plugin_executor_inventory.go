package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type pluginExecutorInventoryWriter interface {
	CheckpointPluginExecutorInventory(context.Context, *models.ExecutorRunning) error
}

type pluginExecutorUnrecordedWriter interface {
	RecordUnrecordedPluginExecutor(context.Context, *models.UnrecordedPluginExecutor) error
}

func (m *Manager) wirePluginExecutorInventoryPersistence(req *ExecutorCreateRequest, executorType string) {
	if req == nil || executorType != string(models.ExecutorTypePluginRemote) || m.runningWriter == nil {
		return
	}
	req.CheckpointRuntimeInventory = func(ctx context.Context, metadata map[string]interface{}) error {
		return m.checkpointPluginExecutorRuntimeInventory(ctx, req, metadata)
	}
	req.RecordUnrecordedRuntimeInventory = func(ctx context.Context, metadata map[string]interface{}) error {
		return m.recordUnrecordedPluginExecutorInventory(ctx, req, metadata)
	}
	if req.PreviousExecutionID == "" {
		req.ReleaseRuntimeInventory = func(ctx context.Context) error {
			return m.releasePluginExecutorRuntimeInventory(ctx, req)
		}
	}
}

func (m *Manager) checkpointPluginExecutorRuntimeInventory(
	ctx context.Context,
	req *ExecutorCreateRequest,
	runtimeMetadata map[string]interface{},
) error {
	writer, ok := m.runningWriter.(pluginExecutorInventoryWriter)
	if !ok {
		return errors.New("checkpoint plugin executor inventory: narrow CAS writer is unavailable")
	}
	if req == nil || strings.TrimSpace(req.InstanceID) == "" || strings.TrimSpace(req.TaskID) == "" ||
		strings.TrimSpace(req.SessionID) == "" || strings.TrimSpace(req.AgentProfileID) == "" {
		return errors.New("checkpoint plugin executor inventory: launch identity is incomplete")
	}
	persistCtx, cancelPersist := kubernetesDurableContext(ctx)
	defer cancelPersist()
	prior, revision, err := m.readPriorPluginExecutorRuntimeInventory(persistCtx, req.SessionID)
	if err != nil {
		return err
	}
	running, err := pluginExecutorLaunchRunning(req, runtimeMetadata, prior)
	if err != nil {
		return err
	}
	if prior != nil {
		running.ExpectedPluginExecutorRevision = revision
	}
	if err := writer.CheckpointPluginExecutorInventory(persistCtx, running); err != nil {
		return fmt.Errorf("checkpoint plugin executor inventory: %w", err)
	}
	return nil
}

// recordUnrecordedPluginExecutorInventory hands the launch's inventory to the
// environment cleanup when the inventory row could not be written.
func (m *Manager) recordUnrecordedPluginExecutorInventory(ctx context.Context, req *ExecutorCreateRequest, runtimeMetadata map[string]interface{}) error {
	writer, ok := m.runningWriter.(pluginExecutorUnrecordedWriter)
	if !ok {
		return errors.New("record unrecorded plugin executor: durable handoff is unavailable")
	}
	persistCtx, cancelPersist := kubernetesDurableContext(ctx)
	defer cancelPersist()
	running, err := pluginExecutorLaunchRunning(req, runtimeMetadata, nil)
	if err != nil {
		return err
	}
	return writer.RecordUnrecordedPluginExecutor(persistCtx, &models.UnrecordedPluginExecutor{
		ExecutionID: req.InstanceID, SessionID: req.SessionID, TaskID: req.TaskID,
		EnvironmentID: req.TaskEnvironmentID, Running: running,
	})
}

func pluginExecutorLaunchRunning(req *ExecutorCreateRequest, runtimeMetadata map[string]interface{}, prior *models.ExecutorRunning) (*models.ExecutorRunning, error) {
	metadata, err := mergePluginExecutorRuntimeMetadata(req.Metadata, runtimeMetadata)
	if err != nil {
		return nil, err
	}
	execution := &AgentExecution{
		ID: req.InstanceID, TaskID: req.TaskID, SessionID: req.SessionID,
		TaskEnvironmentID: req.TaskEnvironmentID, AgentProfileID: req.AgentProfileID,
		RuntimeName: agentruntime.RuntimePluginRemote, Status: v1.AgentStatusStarting, metadata: metadata,
	}
	running := buildRunningFromExecution(execution, prior)
	running.Runtime = agentruntime.RuntimePluginRemote
	running.Status = models.ExecutorRunningStatusStarting
	return running, nil
}

func mergePluginExecutorRuntimeMetadata(current, runtime map[string]interface{}) (map[string]interface{}, error) {
	metadata := clonePluginExecutorMetadata(current)
	for key, value := range runtime {
		metadata[key] = value
	}
	if _, exists := metadata[MetadataKeyPluginExecutor]; !exists {
		return nil, errors.New("checkpoint plugin executor inventory: typed envelope is missing")
	}
	return metadata, nil
}

func (m *Manager) readPriorPluginExecutorRuntimeInventory(ctx context.Context, sessionID string) (*models.ExecutorRunning, uint64, error) {
	reader, ok := m.runningWriter.(executorRunningReader)
	if !ok {
		return nil, 0, nil
	}
	prior, err := reader.GetExecutorRunningBySessionID(ctx, sessionID)
	if errors.Is(err, models.ErrExecutorRunningNotFound) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("checkpoint plugin executor inventory: read prior row: %w", err)
	}
	if prior == nil {
		return nil, 0, nil
	}
	inventory, err := decodePluginExecutorInventory(prior.Metadata)
	if err != nil {
		return nil, 0, fmt.Errorf("checkpoint plugin executor inventory: decode prior envelope: %w", err)
	}
	return prior, inventory.Revision, nil
}

func (m *Manager) releasePluginExecutorRuntimeInventory(ctx context.Context, req *ExecutorCreateRequest) error {
	if req == nil {
		return nil
	}
	reader, hasReader := m.runningWriter.(executorRunningReader)
	casWriter, hasCAS := m.runningWriter.(executorRunningCASWriter)
	if !hasReader || !hasCAS {
		return errors.New("release plugin executor inventory: exact CAS persistence is unavailable")
	}
	persistCtx, cancelPersist := kubernetesDurableContext(ctx)
	defer cancelPersist()
	current, err := reader.GetExecutorRunningBySessionID(persistCtx, req.SessionID)
	if errors.Is(err, models.ErrExecutorRunningNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("release plugin executor inventory: read row: %w", err)
	}
	if current == nil || current.AgentExecutionID != req.InstanceID {
		return fmt.Errorf("release plugin executor inventory: %w", models.ErrExecutionRotated)
	}
	if err := casWriter.DeleteExecutorRunningIfCurrent(persistCtx, req.SessionID, req.InstanceID, current.UpdatedAt); err != nil && !errors.Is(err, models.ErrExecutorRunningNotFound) {
		return fmt.Errorf("release plugin executor inventory: delete row: %w", err)
	}
	return nil
}
