package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agent/nomad"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

const nomadReadmitClaimSuffix = ":readmit"

// ReadmitNomadEnvironment records an operator release of one retained
// cleanup_pending or quarantined row. It only checkpoints the typed phase: it
// never touches Nomad eligibility, the quarantine marker, jobs or the fence,
// and the row is kept.
func (m *Manager) ReadmitNomadEnvironment(ctx context.Context, req nomad.NomadReadmitRequest) error {
	if err := req.Validate(); err != nil {
		return err
	}
	record, err := m.findNomadReadmitRow(ctx, req)
	if err != nil {
		return err
	}
	cleanupReq, err := m.nomadCleanupRequestForRecord(ctx, record)
	if err != nil {
		return fmt.Errorf("nomad readmission: %w", nomad.ErrNomadReadmitNotFound)
	}
	return m.withNomadReadmitClaim(ctx, cleanupReq, func(claimed context.Context) error {
		return m.checkpointNomadReadmit(claimed, cleanupReq, req)
	})
}

// withNomadReadmitClaim runs fn under a claim identity distinct from cleanup's,
// so releasing it can never delete the claim of an in-flight cleanup.
func (m *Manager) withNomadReadmitClaim(ctx context.Context, req *ExecutorCreateRequest, fn func(context.Context) error) error {
	state, err := nomad.EnvironmentStateFromMetadata(req.Metadata)
	if err != nil {
		return errors.New("nomad readmission: exact typed environment state is unavailable")
	}
	store, ok := m.runningWriter.(nomadCleanupClaimStore)
	if !ok {
		return errors.New("nomad readmission: environment claim authority is unavailable")
	}
	claim, err := store.AcquireTaskEnvironmentRecoveryClaim(ctx, models.TaskEnvironmentRecoveryClaimRequest{
		TaskEnvironmentID: state.EnvironmentID, OwnerTaskID: req.TaskID, OwnershipGeneration: state.Generation,
		SessionID: req.SessionID, OperationID: state.OperationID + nomadReadmitClaimSuffix,
		ExecutorType: string(models.ExecutorTypeNomad), AllowCurrentSessionRuntime: true, OperatorReadmission: true,
	})
	if errors.Is(err, recoveryclaim.ErrBusy) {
		return fmt.Errorf("nomad readmission: %w: %w", nomad.ErrNomadReadmitBusy, err)
	}
	if err != nil {
		return fmt.Errorf("nomad readmission: environment claim unavailable: %w", err)
	}
	defer m.releaseNomadCleanupClaim(store, claim)
	return fn(recoveryclaim.WithClaim(ctx, claim))
}

func (m *Manager) findNomadReadmitRow(ctx context.Context, req nomad.NomadReadmitRequest) (*models.ExecutorRunning, error) {
	rows, err := m.ListLiveNomadExecutorsRunning(ctx)
	if err != nil {
		return nil, errors.New("nomad readmission: retained inventory could not be read")
	}
	for _, row := range rows {
		if row == nil {
			continue
		}
		state, err := nomad.EnvironmentStateFromMetadata(row.Metadata)
		if err != nil {
			continue
		}
		if state.NodeID == req.NodeID && state.EnvironmentID == req.EnvironmentID &&
			state.Generation == req.Generation && state.JobID == req.JobID {
			return row, nil
		}
	}
	return nil, nomad.ErrNomadReadmitNotFound
}

// checkpointNomadReadmit re-reads the exact row under the claim so the phase
// decision and the write observe the same state.
func (m *Manager) checkpointNomadReadmit(ctx context.Context, cleanupReq *ExecutorCreateRequest, req nomad.NomadReadmitRequest) error {
	reader, ok := m.runningWriter.(executorRunningReader)
	if !ok {
		return errors.New("nomad readmission: exact retained state is unavailable")
	}
	current, err := reader.GetExecutorRunningBySessionID(ctx, cleanupReq.SessionID)
	if err != nil || current == nil {
		return nomad.ErrNomadReadmitNotFound
	}
	state, err := nomad.EnvironmentStateFromMetadata(current.Metadata)
	if err != nil || state.ExecutionID != cleanupReq.InstanceID || state.NodeID != req.NodeID ||
		state.EnvironmentID != req.EnvironmentID || state.Generation != req.Generation || state.JobID != req.JobID {
		return nomad.ErrNomadReadmitNotFound
	}
	if state.Phase != nomadPhaseCleanupPending && state.Phase != nomadPhaseQuarantined {
		return nomad.ErrNomadReadmitPhase
	}
	cleanupReq.Metadata = clonePluginExecutorMetadata(current.Metadata)
	state.Phase = nomadPhaseOperatorReleased
	state.OperatorAttestationDigest = nomad.OperatorAttestationDigest(state, req.Attestation)
	state.Revision++
	if err := checkpointNomadState(ctx, cleanupReq, &state); err != nil {
		return errors.New("nomad readmission: checkpoint failed")
	}
	return nil
}
