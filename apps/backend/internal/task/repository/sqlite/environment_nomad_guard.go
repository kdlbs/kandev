package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/nomad"
	agentruntime "github.com/kandev/kandev/internal/agentruntime"
)

// ErrNomadInventoryRetained reports that a live, quarantined, or
// cleanup-pending Nomad allocation inventory still references the environment
// or task, so its owner rows must be kept.
var ErrNomadInventoryRetained = errors.New("nomad environment inventory is retained")

// nomadInventoryRetained reports whether a Nomad executor row decodes to a
// phase other than absent or operator_released. An unreadable state is retained: absence is never
// inferred from missing evidence.
func nomadInventoryRetained(raw string) bool {
	metadata := map[string]interface{}{}
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return true
	}
	state, err := nomad.EnvironmentStateFromMetadata(metadata)
	if err != nil {
		return true
	}
	return state.Phase != nomadPhaseAbsent && state.Phase != nomadPhaseOperatorReleased
}

func (r *Repository) anyNomadInventoryRetained(ctx context.Context, tx *sqlx.Tx, query string, id string) (bool, error) {
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(query), agentruntime.RuntimeNomad, id)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return false, err
		}
		if nomadInventoryRetained(raw) {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (r *Repository) ensureNomadInventoryAbsent(ctx context.Context, tx *sqlx.Tx, environmentID string) error {
	retained, err := r.anyNomadInventoryRetained(ctx, tx, `
		SELECT COALESCE(er.metadata, '') FROM executors_running er
		JOIN task_sessions ts ON ts.id = er.session_id
		WHERE er.runtime = ? AND ts.task_environment_id = ?`, environmentID)
	if err != nil {
		return err
	}
	if retained {
		return fmt.Errorf("%w: environment %s", ErrNomadInventoryRetained, environmentID)
	}
	return nil
}

func (r *Repository) ensureNomadInventoryAbsentForTask(ctx context.Context, tx *sqlx.Tx, taskID string) error {
	retained, err := r.anyNomadInventoryRetained(ctx, tx, `
		SELECT COALESCE(metadata, '') FROM executors_running WHERE runtime = ? AND task_id = ?`, taskID)
	if err != nil {
		return err
	}
	if retained {
		return fmt.Errorf("%w: task %s", ErrNomadInventoryRetained, taskID)
	}
	return nil
}

// HasNomadEnvironmentInventory reports whether a non-absent Nomad inventory
// row is bound to the environment.
func (r *Repository) HasNomadEnvironmentInventory(ctx context.Context, environmentID string) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	err = r.ensureNomadInventoryAbsent(ctx, tx, environmentID)
	if errors.Is(err, ErrNomadInventoryRetained) {
		return true, nil
	}
	return false, err
}

// ensureNomadRunningRowReleasable refuses to delete the executors_running row
// of a session whose Nomad inventory is still retained.
func (r *Repository) ensureNomadRunningRowReleasable(ctx context.Context, tx *sqlx.Tx, sessionID string) error {
	retained, err := r.anyNomadInventoryRetained(ctx, tx, `
		SELECT COALESCE(metadata, '') FROM executors_running WHERE runtime = ? AND session_id = ?`, sessionID)
	if err != nil {
		return err
	}
	if retained {
		return fmt.Errorf("%w: session %s", ErrNomadInventoryRetained, sessionID)
	}
	return nil
}
