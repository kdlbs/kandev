package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	kandevdb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

const (
	pluginInventoryLockNamespace = "plugin_inventory:"

	pluginPhaseCleanupProgress  = "cleanup_progress"
	pluginPhaseRecoveryProgress = "recovery_progress"
)

// pluginClaimState is the claim record kept in the envelope: which claim last
// wrote, and the highest sequence and digest it was accepted at. AcquiredAt
// separates two acquisitions of one claim id.
type pluginClaimState struct {
	Kind        string `json:"kind"`
	OperationID string `json:"operation_id"`
	AcquiredAt  string `json:"acquired_at"`
	Sequence    uint64 `json:"sequence"`
	Digest      string `json:"digest"`
}

func decodePluginClaimState(metadata map[string]interface{}) *pluginClaimState {
	encoded, err := json.Marshal(metadata[pluginExecutorEnvelopeKey])
	if err != nil {
		return nil
	}
	var envelope struct {
		Claim *pluginClaimState `json:"claim"`
	}
	if json.Unmarshal(encoded, &envelope) != nil {
		return nil
	}
	return envelope.Claim
}

func setPluginClaimState(metadata map[string]interface{}, state pluginClaimState) error {
	encoded, err := json.Marshal(metadata[pluginExecutorEnvelopeKey])
	if err != nil {
		return err
	}
	envelope := map[string]interface{}{}
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		return err
	}
	envelope["claim"] = state
	metadata[pluginExecutorEnvelopeKey] = envelope
	return nil
}

// lockPluginInventory serializes every envelope write of one environment on
// PostgreSQL. It is taken after the installation-scoped workspace lock where
// both apply.
func lockPluginInventory(ctx context.Context, tx *sqlx.Tx, driverName, environmentID string) error {
	if !dialect.IsPostgres(driverName) || environmentID == "" {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, pluginInventoryLockNamespace+environmentID); err != nil {
		return fmt.Errorf("lock plugin inventory: %w", err)
	}
	return nil
}

// lockPluginEnvelopeWrite orders the locks of an envelope write as the owner
// task row, the installation descriptors, then the environment inventory, the
// order ready publication takes them in.
func (r *Repository) lockPluginEnvelopeWrite(ctx context.Context, tx *sqlx.Tx, running *models.ExecutorRunning) error {
	if err := kandevdb.LockTaskRowInTx(ctx, tx, r.db.DriverName(), running.TaskID); err != nil && !errors.Is(err, kandevdb.ErrTaskRowNotFound) {
		return err
	}
	view := decodePluginEnvelopeView(running.Metadata)
	if view.Descriptor != nil && view.InstallationID != "" && !pluginExecutorPhaseTerminal(view.Phase) {
		if err := lockPluginWorkspaceDescriptors(ctx, tx, r.db.DriverName(), view.InstallationID); err != nil {
			return err
		}
	}
	return lockPluginInventory(ctx, tx, r.db.DriverName(), view.EnvironmentID)
}

func validatePluginClaimCheckpointShape(running *models.ExecutorRunning, cp models.PluginExecutorClaimCheckpoint) error {
	wantPhase := map[string]string{models.PluginClaimKindCleanup: pluginPhaseCleanupProgress, models.PluginClaimKindRecovery: pluginPhaseRecoveryProgress}[cp.Kind]
	if wantPhase == "" || decodePluginEnvelopeView(running.Metadata).Phase != wantPhase {
		return fmt.Errorf("%w: the checkpoint phase does not belong to a %q claim", models.ErrExecutorInvalidConfig, cp.Kind)
	}
	if cp.Sequence < 1 {
		return fmt.Errorf("%w: the checkpoint sequence must be at least 1", models.ErrExecutorInvalidConfig)
	}
	if cp.Claim == nil || cp.Claim.TaskEnvironmentID == "" || cp.Claim.OperationID == "" {
		return fmt.Errorf("%w: the checkpoint names no claim", models.ErrExecutorClaimNotFound)
	}
	return nil
}

// CheckpointPluginExecutorClaimInventory writes a provider checkpoint that was
// issued under an active cleanup or recovery claim. Order of refusal: shape
// (invalid_config), claim identity (not_found), sequence, then the envelope
// revision CAS (conflict). The claim row, the sequence and the revision are
// read and written in one transaction, so a released claim, a successor claim
// or a competing checkpoint changes nothing. applied is false for an equal
// sequence with an equal digest, which succeeds without a write.
func (r *Repository) CheckpointPluginExecutorClaimInventory(ctx context.Context, running *models.ExecutorRunning, cp models.PluginExecutorClaimCheckpoint) (applied bool, err error) {
	if err := validatePluginClaimCheckpointShape(running, cp); err != nil {
		return false, err
	}
	now := time.Now().UTC()
	metadataJSON, observedRevision, err := preparePluginExecutorInventoryCheckpoint(running, now)
	if err != nil {
		return false, err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockPluginEnvelopeWrite(ctx, tx, running); err != nil {
		return false, err
	}
	claimedCtx := recoveryclaim.WithClaim(ctx, cp.Claim)
	if err := r.validatePluginExecutorCheckpointOwner(claimedCtx, tx, running, metadataJSON); err != nil {
		return false, mapPluginClaimError(err)
	}
	if err := recoveryclaim.ValidateAcquisitionTx(ctx, r.db, tx, cp.Claim); err != nil {
		return false, mapPluginClaimError(err)
	}
	proceed, err := r.checkPluginClaimSequence(ctx, tx, running, cp, observedRevision)
	if err != nil || !proceed {
		return false, err
	}
	if err := r.checkPluginStateWritable(ctx, tx, running); err != nil {
		return false, err
	}
	if err := r.checkPluginWorkspaceUnique(ctx, tx, running); err != nil {
		return false, err
	}
	if err := setPluginClaimState(running.Metadata, pluginClaimState{Kind: cp.Kind, OperationID: cp.Claim.OperationID, AcquiredAt: claimAcquiredAt(cp.Claim), Sequence: cp.Sequence, Digest: cp.Digest}); err != nil {
		return false, err
	}
	if err := checkpointPluginExecutorInventoryRow(ctx, tx, r.db, running, observedRevision, now); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	running.UpdatedAt = now
	running.ExpectedPluginExecutorRevision, _ = pluginExecutorInventoryRevision(running.Metadata)
	return true, nil
}

func mapPluginClaimError(err error) error {
	if errors.Is(err, recoveryclaim.ErrClaimNotFound) || errors.Is(err, recoveryclaim.ErrClaimMismatch) {
		return fmt.Errorf("%w: %v", models.ErrExecutorClaimNotFound, err)
	}
	return err
}

func claimAcquiredAt(claim *models.TaskEnvironmentRecoveryClaim) string {
	return claim.CreatedAt.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano)
}

// checkPluginClaimSequence applies the per-claim sequence rule against the
// stored envelope and reports whether the write should proceed. A claim other
// than the acquisition that last wrote starts a fresh sequence.
func (r *Repository) checkPluginClaimSequence(ctx context.Context, tx *sqlx.Tx, running *models.ExecutorRunning, cp models.PluginExecutorClaimCheckpoint, observedRevision uint64) (bool, error) {
	executionID, current, _, err := readPluginExecutorInventoryForUpdate(ctx, tx, running.SessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, models.ErrExecutionRotated
	}
	if err != nil {
		return false, err
	}
	if executionID != running.AgentExecutionID {
		return false, models.ErrExecutionRotated
	}
	if last := decodePluginClaimState(current); last != nil && last.OperationID == cp.Claim.OperationID && last.AcquiredAt == claimAcquiredAt(cp.Claim) {
		switch {
		case cp.Sequence < last.Sequence:
			return false, fmt.Errorf("%w: sequence %d is older than the accepted %d", models.ErrExecutorConflict, cp.Sequence, last.Sequence)
		case cp.Sequence == last.Sequence && cp.Digest == last.Digest:
			return false, nil
		case cp.Sequence == last.Sequence:
			return false, fmt.Errorf("%w: sequence %d was accepted with a different payload", models.ErrExecutorConflict, cp.Sequence)
		}
	}
	currentRevision, err := pluginExecutorInventoryRevision(current)
	if err != nil {
		return false, err
	}
	if currentRevision != observedRevision {
		return false, fmt.Errorf("%w: the envelope moved from revision %d to %d", models.ErrExecutorConflict, observedRevision, currentRevision)
	}
	return true, nil
}
