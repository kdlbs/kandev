package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) BeginManagedAgentTermination(ctx context.Context, bindingID string, at time.Time) error {
	if bindingID == "" {
		return fmt.Errorf("managed agent binding ID is required")
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin managed agent termination intent: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	query := "SELECT lifecycle FROM managed_agent_bindings WHERE id = ?" + managedAgentLockSuffix(r.db.DriverName())
	var lifecycle models.ManagedAgentBindingLifecycle
	if err := tx.QueryRowContext(ctx, r.db.Rebind(query), bindingID).Scan(&lifecycle); err != nil {
		if err == sql.ErrNoRows {
			return ErrManagedAgentBindingNotFound
		}
		return fmt.Errorf("load managed agent binding for termination: %w", err)
	}
	if lifecycle != models.ManagedAgentBindingArchived && lifecycle != models.ManagedAgentBindingTerminationPending {
		_, err := tx.ExecContext(ctx, r.db.Rebind(
			"UPDATE managed_agent_bindings SET lifecycle = ?, revision = revision + 1, updated_at = ? WHERE id = ?",
		), models.ManagedAgentBindingTerminationPending, at, bindingID)
		if err != nil {
			return fmt.Errorf("persist managed agent termination intent: %w", err)
		}
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(
		"UPDATE managed_agent_tool_grants SET revoked_at = ? WHERE binding_id = ? AND revoked_at IS NULL",
	), at, bindingID)
	if err != nil {
		return fmt.Errorf("revoke managed agent grants for termination: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit managed agent termination intent: %w", err)
	}
	return nil
}

func (r *Repository) ResolveManagedAgentTermination(
	ctx context.Context,
	bindingID string,
	lifecycle models.ManagedAgentBindingLifecycle,
	at time.Time,
) error {
	if bindingID == "" || lifecycle != models.ManagedAgentBindingReady && lifecycle != models.ManagedAgentBindingArchived {
		return fmt.Errorf("managed agent termination resolution is invalid")
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin managed agent termination resolution: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	bindingQuery := "SELECT " + managedAgentBindingColumns + " FROM managed_agent_bindings WHERE id = ?" + managedAgentLockSuffix(r.db.DriverName())
	binding, err := scanManagedAgentBinding(tx.QueryRowContext(ctx, r.db.Rebind(bindingQuery), bindingID))
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrManagedAgentBindingNotFound
		}
		return fmt.Errorf("load managed agent binding for termination resolution: %w", err)
	}
	operationQuery := "SELECT submission_state FROM managed_agent_operations WHERE binding_id = ? ORDER BY dispatch_generation DESC LIMIT 1" + managedAgentLockSuffix(r.db.DriverName())
	var state models.ManagedAgentSubmissionState
	if err := tx.QueryRowContext(ctx, r.db.Rebind(operationQuery), bindingID).Scan(&state); err != nil {
		return fmt.Errorf("load managed agent operation for termination resolution: %w", err)
	}
	if managedAgentOperationActive(state) {
		return ErrManagedAgentActiveOperation
	}
	if binding.Lifecycle != lifecycle {
		_, err := tx.ExecContext(ctx, r.db.Rebind(
			"UPDATE managed_agent_bindings SET lifecycle = ?, revision = revision + 1, updated_at = ? WHERE id = ?",
		), lifecycle, at, bindingID)
		if err != nil {
			return fmt.Errorf("resolve managed agent termination: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit managed agent termination resolution: %w", err)
	}
	return nil
}
