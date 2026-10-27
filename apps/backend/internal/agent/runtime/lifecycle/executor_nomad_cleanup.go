package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/nomad"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

const (
	nomadCleanupClaimSuffix   = ":cleanup"
	nomadClaimReleaseTimeout  = 10 * time.Second
	nomadLaunchRollbackBudget = 3 * time.Minute
)

// nomadCleanupCoordinator is installed by the Manager, which owns inventory
// persistence and the durable environment claim every cleanup write requires.
type nomadCleanupCoordinator interface {
	// DestroyInstance runs positive cleanup for a connected or retained instance.
	DestroyInstance(ctx context.Context, instance *ExecutorInstance) error
	// DestroyLaunch cleans up a launch that failed after its job was registered.
	DestroyLaunch(ctx context.Context, req *ExecutorCreateRequest) error
	// ResumeCleanup repeats the idempotent tail of a retained cleanup. It
	// reports whether the record was a cleanup record.
	ResumeCleanup(ctx context.Context, record *models.ExecutorRunning) (bool, error)
}

// nomadCleanupClaimStore is the claim authority every cleanup write is made under.
type nomadCleanupClaimStore interface {
	AcquireTaskEnvironmentRecoveryClaim(context.Context, models.TaskEnvironmentRecoveryClaimRequest) (*models.TaskEnvironmentRecoveryClaim, error)
	ReleaseTaskEnvironmentRecoveryClaim(context.Context, *models.TaskEnvironmentRecoveryClaim) error
}

// DestroyNomadEnvironment runs positive cleanup for every retained Nomad
// inventory row of the environment. It is idempotent: a repeat resumes from the
// persisted drain receipt and an environment without retained rows is a no-op.
func (m *Manager) DestroyNomadEnvironment(ctx context.Context, environmentID string) error {
	if strings.TrimSpace(environmentID) == "" {
		return errors.New("nomad destroy requires the environment identity")
	}
	exec, err := m.nomadExecutorBackend()
	if err != nil {
		return err
	}
	records, err := m.nomadRecordsForEnvironment(ctx, environmentID)
	if err != nil {
		return err
	}
	var failures []error
	for _, record := range records {
		req, err := m.nomadCleanupRequestForRecord(ctx, record)
		if err == nil {
			err = m.withFreshNomadCleanupClaim(ctx, req, func(claimed context.Context, claimID string, fresh *ExecutorCreateRequest) error {
				return exec.destroyNomadOperation(claimed, fresh, claimID)
			})
		}
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}

func (m *Manager) nomadExecutorBackend() (*NomadExecutor, error) {
	if m.executorRegistry == nil {
		return nil, errors.New("nomad destroy: no runtime registry configured")
	}
	backend, err := m.executorRegistry.GetBackend(executor.NameNomad)
	if err != nil {
		return nil, fmt.Errorf("nomad destroy: %w", err)
	}
	exec, ok := backend.(*NomadExecutor)
	if !ok {
		return nil, errors.New("nomad destroy: the registered Nomad runtime is not the built-in executor")
	}
	return exec, nil
}

func (m *Manager) nomadRecordsForEnvironment(ctx context.Context, environmentID string) ([]*models.ExecutorRunning, error) {
	rows, err := m.ListLiveNomadExecutorsRunning(ctx)
	if err != nil {
		return nil, fmt.Errorf("nomad destroy: read retained inventory: %w", err)
	}
	var matches []*models.ExecutorRunning
	for _, row := range rows {
		if row == nil {
			continue
		}
		state, err := nomad.EnvironmentStateFromMetadata(row.Metadata)
		if err != nil {
			return nil, fmt.Errorf("nomad destroy: decode retained inventory row: %w", err)
		}
		if state.EnvironmentID == environmentID && state.Phase != nomadPhaseOperatorReleased {
			matches = append(matches, row)
		}
	}
	return matches, nil
}

// bindNomadCleanup installs this Manager as the built-in Nomad executor's
// cleanup coordinator.
func (m *Manager) bindNomadCleanup() {
	exec, err := m.nomadExecutorBackend()
	if err != nil {
		return
	}
	exec.setCleanupCoordinator(&managerNomadCleanup{manager: m, executor: exec})
}

type managerNomadCleanup struct {
	manager  *Manager
	executor *NomadExecutor
}

func (c *managerNomadCleanup) DestroyInstance(ctx context.Context, instance *ExecutorInstance) error {
	req, err := c.manager.nomadCleanupRequest(ctx, instance)
	if err != nil {
		return err
	}
	return c.manager.withFreshNomadCleanupClaim(ctx, req, func(claimed context.Context, claimID string, fresh *ExecutorCreateRequest) error {
		return c.executor.destroyNomadOperation(claimed, fresh, claimID)
	})
}

func (c *managerNomadCleanup) DestroyLaunch(ctx context.Context, req *ExecutorCreateRequest) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), nomadLaunchRollbackBudget)
	defer cancel()
	if req.ReleaseRuntimeInventory == nil {
		c.manager.wireNomadCleanupRelease(req)
	}
	return c.destroy(ctx, req)
}

// destroy runs the launch rollback under the cleanup claim. The inventory row is
// re-read once the claim is held, so a rollback that waited behind an archive
// cleanup resumes from that cleanup's receipt and absence instead of draining
// again from the launch's own stale state. A launch with no matching row yet
// keeps its own request.
func (c *managerNomadCleanup) destroy(ctx context.Context, req *ExecutorCreateRequest) error {
	return c.manager.withNomadCleanupClaim(ctx, req, func(claimed context.Context, claimID string) error {
		current := req
		fresh, err := c.manager.claimedNomadCleanupRequest(claimed, req)
		switch {
		case err == nil:
			if fresh.AuthToken == "" {
				fresh.AuthToken = req.AuthToken
			}
			current = fresh
		case !errors.Is(err, models.ErrExecutionRotated):
			return err
		}
		return c.executor.destroyNomadOperation(claimed, current, claimID)
	})
}

func (c *managerNomadCleanup) ResumeCleanup(ctx context.Context, record *models.ExecutorRunning) (bool, error) {
	if record == nil {
		return false, nil
	}
	state, err := nomad.EnvironmentStateFromMetadata(record.Metadata)
	if err != nil || !nomadCleanupResumable(state) {
		return false, nil
	}
	req, err := c.manager.nomadCleanupRequestForRecord(ctx, record)
	if err != nil {
		return true, err
	}
	return true, c.manager.withFreshNomadCleanupClaim(ctx, req, func(claimed context.Context, claimID string, fresh *ExecutorCreateRequest) error {
		return c.executor.resumeNomadCleanup(claimed, fresh, claimID)
	})
}

func nomadCleanupResumable(state nomad.EnvironmentState) bool {
	if state.Phase == nomadPhaseOperatorReleased {
		return false
	}
	return state.QuarantineState == nomad.QuarantinePending ||
		state.Phase == nomadPhaseCleanupPending || state.Phase == nomadPhaseAbsent || unplacedPoolLaunch(state)
}

// unplacedPoolLaunch reports a pool launch row that never recorded a node.
// Startup recovery owns it: no live launch holds such a row after a restart.
func unplacedPoolLaunch(state nomad.EnvironmentState) bool {
	return state.PlacementMode == nomad.ModePool && state.NodeID == "" &&
		(state.Phase == nomadPhaseIntent || state.Phase == nomadPhaseRegistered)
}

// nomadCleanupRequest builds the cleanup request for a connected instance from
// the exact running inventory row. Callers must hold the cleanup claim.
func (m *Manager) nomadCleanupRequest(ctx context.Context, instance *ExecutorInstance) (*ExecutorCreateRequest, error) {
	if instance == nil || instance.InstanceID == "" || instance.SessionID == "" {
		return nil, errors.New("nomad cleanup requires the exact instance identity")
	}
	reader, ok := m.runningWriter.(executorRunningReader)
	if !ok {
		return nil, errors.New("nomad cleanup: running inventory is unavailable")
	}
	record, err := reader.GetExecutorRunningBySessionID(ctx, instance.SessionID)
	if err != nil {
		return nil, fmt.Errorf("nomad cleanup: read running inventory: %w", err)
	}
	if record == nil || record.AgentExecutionID != instance.InstanceID || record.TaskID != instance.TaskID {
		return nil, fmt.Errorf("nomad cleanup: %w", models.ErrExecutionRotated)
	}
	return m.nomadCleanupRequestForRecord(ctx, record)
}

func (m *Manager) nomadCleanupRequestForRecord(ctx context.Context, record *models.ExecutorRunning) (*ExecutorCreateRequest, error) {
	if record == nil || record.Runtime != agentruntime.RuntimeNomad || record.Metadata == nil || record.AgentExecutionID == "" {
		return nil, errors.New("nomad cleanup requires the exact running inventory row")
	}
	state, err := nomad.EnvironmentStateFromMetadata(record.Metadata)
	if err != nil || state.ExecutionID != record.AgentExecutionID || state.EnvironmentID == "" {
		return nil, errors.New("nomad cleanup requires the exact typed environment state")
	}
	hydrated := cloneRemoteRecoveryRecord(record)
	if hydrated.TransientAuthToken == "" {
		hydrated.TransientAuthToken = m.revealRuntimeSecret(ctx, hydrated.Metadata, MetadataKeyAuthTokenSecret)
	}
	req := nomadRecoveryRequest(hydrated, state)
	m.wireNomadInventoryPersistence(req, string(models.ExecutorTypeNomad))
	m.wireNomadCleanupRelease(req)
	return req, nil
}

// wireNomadCleanupRelease attaches the receipt-checked inventory release to a
// cleanup request regardless of how the request was created.
func (m *Manager) wireNomadCleanupRelease(req *ExecutorCreateRequest) {
	req.ReleaseRuntimeInventory = func(ctx context.Context) error {
		return m.releaseNomadRuntimeInventory(ctx, req)
	}
}

// withNomadCleanupClaim runs fn under the environment's cleanup claim. The
// claim identity is deterministic per operation, so a retry presents the same
// claim and a different holder is rejected before any external call.
func (m *Manager) withNomadCleanupClaim(ctx context.Context, req *ExecutorCreateRequest, fn func(context.Context, string) error) error {
	state, err := nomad.EnvironmentStateFromMetadata(req.Metadata)
	if err != nil {
		return errors.New("nomad cleanup requires the exact typed environment state")
	}
	if held := recoveryclaim.ClaimFromContext(ctx); held != nil && held.TaskEnvironmentID == state.EnvironmentID {
		return fn(ctx, held.OperationID)
	}
	store, ok := m.runningWriter.(nomadCleanupClaimStore)
	if !ok {
		return errors.New("nomad cleanup: environment claim authority is unavailable")
	}
	if _, active := m.nomadCleanupEnvs.LoadOrStore(state.EnvironmentID, struct{}{}); active {
		return errors.New("nomad cleanup is already running for this environment")
	}
	defer m.nomadCleanupEnvs.Delete(state.EnvironmentID)
	claimRequest := models.TaskEnvironmentRecoveryClaimRequest{
		TaskEnvironmentID: state.EnvironmentID, OwnerTaskID: req.TaskID, OwnershipGeneration: state.Generation,
		SessionID: req.SessionID, OperationID: state.OperationID + nomadCleanupClaimSuffix,
		ExecutorType: string(models.ExecutorTypeNomad), AllowCurrentSessionRuntime: true,
	}
	if job, ok := recoveryclaim.TaskCleanupJobFromContext(ctx); ok && job.TaskID == req.TaskID {
		claimRequest.CleanupJobID = job.ID
	}
	claim, err := store.AcquireTaskEnvironmentRecoveryClaim(ctx, claimRequest)
	if err != nil {
		return fmt.Errorf("nomad cleanup: environment claim unavailable: %w", err)
	}
	defer m.releaseNomadCleanupClaim(store, claim)
	return fn(recoveryclaim.WithClaim(ctx, claim), claim.OperationID)
}

// withFreshNomadCleanupClaim runs fn under the cleanup claim with the request
// rebuilt from the inventory row as it stands once the claim is held. A cleanup
// that waited on a competing holder therefore resumes from that holder's
// receipt and absence instead of the state read before the wait.
func (m *Manager) withFreshNomadCleanupClaim(
	ctx context.Context, req *ExecutorCreateRequest, fn func(context.Context, string, *ExecutorCreateRequest) error,
) error {
	return m.withNomadCleanupClaim(ctx, req, func(claimed context.Context, claimID string) error {
		fresh, err := m.claimedNomadCleanupRequest(claimed, req)
		if err != nil {
			return err
		}
		return fn(claimed, claimID, fresh)
	})
}

func (m *Manager) claimedNomadCleanupRequest(ctx context.Context, req *ExecutorCreateRequest) (*ExecutorCreateRequest, error) {
	reader, ok := m.runningWriter.(executorRunningReader)
	if !ok {
		return nil, errors.New("nomad cleanup: running inventory is unavailable")
	}
	record, err := reader.GetExecutorRunningBySessionID(ctx, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("nomad cleanup: re-read running inventory: %w", err)
	}
	if record == nil || record.AgentExecutionID != req.InstanceID {
		return nil, fmt.Errorf("nomad cleanup: %w", models.ErrExecutionRotated)
	}
	return m.nomadCleanupRequestForRecord(ctx, record)
}

func (m *Manager) releaseNomadCleanupClaim(store nomadCleanupClaimStore, claim *models.TaskEnvironmentRecoveryClaim) {
	ctx, cancel := context.WithTimeout(context.Background(), nomadClaimReleaseTimeout)
	defer cancel()
	if err := store.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim); err != nil {
		m.logger.Warn("nomad cleanup claim release failed", zap.String("operation_id", claim.OperationID))
	}
}
