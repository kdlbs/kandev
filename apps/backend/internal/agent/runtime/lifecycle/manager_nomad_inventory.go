package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agent/nomad"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type nomadExecutorInventoryWriter interface {
	CheckpointNomadExecutorInventory(context.Context, *models.ExecutorRunning) error
}

type nomadExecutorRunningLister interface {
	ListExecutorsRunningNomad(context.Context) ([]*models.ExecutorRunning, error)
}

// wireNomadInventoryPersistence attaches typed metadata CAS only to the
// built-in Nomad backend. No ordinary upsert or local executor fallback can
// create a Nomad allocation record.
func (m *Manager) wireNomadInventoryPersistence(req *ExecutorCreateRequest, executorType string) {
	if req == nil || executorType != string(models.ExecutorTypeNomad) || m.runningWriter == nil {
		return
	}
	req.CheckpointRuntimeInventory = func(ctx context.Context, metadata map[string]interface{}) error {
		return m.checkpointNomadRuntimeInventory(ctx, req, metadata)
	}
	req.PersistNomadRuntimeSecret = func(ctx context.Context, metadataKey, value string) error {
		return m.persistNomadRuntimeSecret(ctx, req, metadataKey, value)
	}
	req.ListNomadInventory = func(ctx context.Context) ([]nomad.EnvironmentState, error) {
		return m.listNomadInventoryStates(ctx)
	}
	if req.PreviousExecutionID == "" {
		req.ReleaseRuntimeInventory = func(ctx context.Context) error {
			return m.releaseNomadRuntimeInventory(ctx, req)
		}
	}
}

// persistNomadRuntimeSecret preserves a one-time bootstrap nonce or agentctl
// token without allowing a retry to rotate a credential already associated
// with this execution. The caller checkpoints the returned reference together
// with the current typed Nomad state before performing the next side effect.
func (m *Manager) persistNomadRuntimeSecret(ctx context.Context, req *ExecutorCreateRequest, metadataKey, value string) error {
	if req == nil || req.InstanceID == "" || value == "" || m.secretStore == nil {
		return errors.New("persist Nomad runtime secret: identity, value, or secret store is unavailable")
	}
	var namePrefix string
	switch metadataKey {
	case MetadataKeyBootstrapNonceSecret:
		namePrefix = nomadCredentialPrefixBootstrap
	case MetadataKeyAuthTokenSecret:
		namePrefix = nomadCredentialPrefixAuth
	default:
		return errors.New("persist Nomad runtime secret: unsupported secret reference")
	}
	if req.Metadata == nil {
		req.Metadata = make(map[string]interface{})
	}
	secretID := getMetadataString(req.Metadata, metadataKey)
	if secretID == "" {
		secretID = kubernetesRuntimeSecretID(req.InstanceID, namePrefix)
	}
	if err := m.ensureNomadRuntimeSecret(ctx, req, secretID, namePrefix, value); err != nil {
		return err
	}
	req.Metadata[metadataKey] = secretID
	return nil
}

// ensureNomadRuntimeSecret verifies an existing credential reference matches the
// value or creates it when absent.
func (m *Manager) ensureNomadRuntimeSecret(ctx context.Context, req *ExecutorCreateRequest, secretID, namePrefix, value string) error {
	_, err := m.secretStore.Get(ctx, secretID)
	switch {
	case err == nil:
		existing, revealErr := m.secretStore.Reveal(ctx, secretID)
		if revealErr != nil {
			return fmt.Errorf("persist Nomad runtime secret: reveal existing reference: %w", revealErr)
		}
		if existing != value {
			return errors.New("persist Nomad runtime secret: existing operation credential does not match")
		}
	case errors.Is(err, secrets.ErrNotFound):
		if err := m.secretStore.Create(ctx, &secrets.SecretWithValue{
			Secret: secrets.Secret{ID: secretID, Name: fmt.Sprintf("%s-%s", namePrefix, truncateID(req.InstanceID, 12))},
			Value:  value,
		}); err != nil {
			return fmt.Errorf("persist Nomad runtime secret: create reference: %w", err)
		}
	default:
		return fmt.Errorf("persist Nomad runtime secret: inspect existing reference: %w", err)
	}
	return nil
}

func (m *Manager) checkpointNomadRuntimeInventory(ctx context.Context, req *ExecutorCreateRequest, runtimeMetadata map[string]interface{}) error {
	writer, ok := m.runningWriter.(nomadExecutorInventoryWriter)
	if !ok {
		return errors.New("checkpoint Nomad executor inventory: narrow CAS writer is unavailable")
	}
	if req == nil || strings.TrimSpace(req.InstanceID) == "" || strings.TrimSpace(req.TaskID) == "" ||
		strings.TrimSpace(req.SessionID) == "" || strings.TrimSpace(req.TaskEnvironmentID) == "" {
		return errors.New("checkpoint Nomad executor inventory: launch identity is incomplete")
	}
	persistCtx, cancelPersist := kubernetesDurableContext(ctx)
	defer cancelPersist()
	metadata := clonePluginExecutorMetadata(req.Metadata)
	for key, value := range runtimeMetadata {
		metadata[key] = value
	}
	state, err := nomad.EnvironmentStateFromMetadata(metadata)
	if err != nil {
		return fmt.Errorf("checkpoint Nomad executor inventory: typed state is missing or invalid: %w", err)
	}
	if state.ExecutionID != req.InstanceID || state.EnvironmentID != req.TaskEnvironmentID {
		return fmt.Errorf("checkpoint Nomad executor inventory: %w", models.ErrExecutionRotated)
	}
	prior, expectedRevision, err := m.readNomadPriorRunningRow(persistCtx, req)
	if err != nil {
		return err
	}
	execution := &AgentExecution{
		ID: req.InstanceID, TaskID: req.TaskID, SessionID: req.SessionID,
		TaskEnvironmentID: req.TaskEnvironmentID, AgentProfileID: req.AgentProfileID,
		OriginalWorkspacePath: req.OriginalWorkspacePath,
		RuntimeName:           agentruntime.RuntimeNomad, Status: v1.AgentStatusStarting, metadata: metadata,
	}
	running := buildRunningFromExecution(execution, prior)
	running.Runtime = agentruntime.RuntimeNomad
	running.Status = models.ExecutorRunningStatusStarting
	if prior != nil {
		running.ExpectedNomadExecutorRevision = expectedRevision
	}
	if err := writer.CheckpointNomadExecutorInventory(persistCtx, running); err != nil {
		return fmt.Errorf("checkpoint Nomad executor inventory: %w", err)
	}
	// The DAO assigns the authoritative CAS revision. Keep the in-flight
	// request synchronized so the next stage cannot checkpoint a stale revision.
	req.Metadata = clonePluginExecutorMetadata(running.Metadata)
	return nil
}

// readNomadPriorRunningRow returns the existing Nomad row for the session, if
// any, together with its typed CAS revision.
func (m *Manager) readNomadPriorRunningRow(ctx context.Context, req *ExecutorCreateRequest) (*models.ExecutorRunning, uint64, error) {
	reader, ok := m.runningWriter.(executorRunningReader)
	if !ok {
		return nil, 0, nil
	}
	existing, err := reader.GetExecutorRunningBySessionID(ctx, req.SessionID)
	switch {
	case err == nil:
		if existing == nil || existing.Runtime != agentruntime.RuntimeNomad {
			return nil, 0, fmt.Errorf("checkpoint Nomad executor inventory: existing row has another runtime: %w", models.ErrExecutionRotated)
		}
		priorState, err := nomad.EnvironmentStateFromMetadata(existing.Metadata)
		if err != nil {
			return nil, 0, fmt.Errorf("checkpoint Nomad executor inventory: decode prior state: %w", err)
		}
		return existing, priorState.Revision, nil
	case errors.Is(err, models.ErrExecutorRunningNotFound):
		return nil, 0, nil
	default:
		return nil, 0, fmt.Errorf("checkpoint Nomad executor inventory: read prior row: %w", err)
	}
}

func (m *Manager) releaseNomadRuntimeInventory(ctx context.Context, req *ExecutorCreateRequest) error {
	if m.runningWriter == nil || req == nil {
		return nil
	}
	reader, hasReader := m.runningWriter.(executorRunningReader)
	if !hasReader {
		return errors.New("release Nomad executor inventory: exact retained state is unavailable")
	}
	persistCtx, cancelPersist := kubernetesDurableContext(ctx)
	defer cancelPersist()
	current, err := reader.GetExecutorRunningBySessionID(persistCtx, req.SessionID)
	if errors.Is(err, models.ErrExecutorRunningNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("release Nomad executor inventory: read row: %w", err)
	}
	if current == nil || current.AgentExecutionID != req.InstanceID || current.Runtime != agentruntime.RuntimeNomad {
		return fmt.Errorf("release Nomad executor inventory: %w", models.ErrExecutionRotated)
	}
	state, err := nomad.EnvironmentStateFromMetadata(current.Metadata)
	if err != nil {
		return fmt.Errorf("release Nomad executor inventory: decode state: %w", err)
	}
	if state.ExecutionID != req.InstanceID || state.EnvironmentID != req.TaskEnvironmentID ||
		state.Phase != "absent" || state.DrainReceiptDigest == "" || state.DrainReceiptGeneration != state.Generation {
		return errors.New("release Nomad executor inventory: positive current-generation absence receipt is required")
	}
	// Keep the typed tombstone for restart reconciliation and successor fencing.
	return nil
}

func (m *Manager) ListLiveNomadExecutorsRunning(ctx context.Context) ([]*models.ExecutorRunning, error) {
	lister, ok := m.runningWriter.(nomadExecutorRunningLister)
	if !ok {
		return nil, nil
	}
	return lister.ListExecutorsRunningNomad(ctx)
}

// listNomadInventoryStates decodes every retained Nomad row. A row whose typed
// state cannot be decoded is an error because unknown inventory blocks admission.
func (m *Manager) listNomadInventoryStates(ctx context.Context) ([]nomad.EnvironmentState, error) {
	lister, ok := m.runningWriter.(nomadExecutorRunningLister)
	if !ok {
		return nil, errors.New("list Nomad inventory: executor-running store does not support Nomad inventory listing")
	}
	rows, err := lister.ListExecutorsRunningNomad(ctx)
	if err != nil {
		return nil, err
	}
	states := make([]nomad.EnvironmentState, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		state, err := nomad.EnvironmentStateFromMetadata(row.Metadata)
		if err != nil {
			return nil, fmt.Errorf("decode retained Nomad inventory row: %w", err)
		}
		states = append(states, state)
	}
	return states, nil
}
