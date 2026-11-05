package sqlite

import (
	"context"
	"errors"
	"fmt"
	"github.com/kandev/kandev/internal/db/dialect"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	pluginStateReadabilityLockNamespace = "plugin_state_readability:"
	pluginExecutorEnvelopeKey           = "plugin_executor"
)

// PluginStateWritableChecker reports whether the activated manifest of an
// installation can read a provider state version. It runs inside the admission
// write, under the installation's readability lock.
type PluginStateWritableChecker func(ctx context.Context, installationID, providerKey string, stateVersion uint32) error

// SetPluginStateWritableChecker installs the check applied to every non-terminal
// plugin executor checkpoint that records a state version.
func (r *Repository) SetPluginStateWritableChecker(check PluginStateWritableChecker) {
	r.stateWritableMu.Lock()
	defer r.stateWritableMu.Unlock()
	r.stateWritable = check
}

func (r *Repository) pluginStateWritableChecker() PluginStateWritableChecker {
	r.stateWritableMu.RLock()
	defer r.stateWritableMu.RUnlock()
	return r.stateWritable
}

// WithPluginStateReadabilityLock runs fn while holding the installation's
// readability lock. An admission write that records a state version and an
// activation that counts recorded versions take this same lock, so whichever
// commits first is the one the other observes. On SQLite the transaction is the
// immediate-mode writer; on PostgreSQL it is a transaction-scoped advisory lock.
func (r *Repository) WithPluginStateReadabilityLock(ctx context.Context, installationID string, fn func(context.Context) error) error {
	if installationID == "" {
		return errors.New("plugin state readability lock requires an installation id")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockPluginStateReadability(ctx, tx, r.db.DriverName(), installationID); err != nil {
		return err
	}
	if err := fn(ctx); err != nil {
		return err
	}
	return tx.Commit()
}

func lockPluginStateReadability(ctx context.Context, tx *sqlx.Tx, driverName, installationID string) error {
	if !dialect.IsPostgres(driverName) {
		return nil
	}
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, pluginStateReadabilityLockNamespace+installationID)
	if err != nil {
		return fmt.Errorf("lock plugin state readability: %w", err)
	}
	return nil
}

// checkPluginStateWritable gates a checkpoint that records a provider state
// version. Terminal phases are exempt so cleanup of an unreadable record is
// never blocked.
func (r *Repository) checkPluginStateWritable(ctx context.Context, tx *sqlx.Tx, running *models.ExecutorRunning) error {
	view := decodePluginEnvelopeView(running.Metadata)
	if view.InstallationID == "" || view.StateVersion == 0 || pluginExecutorPhaseTerminal(view.Phase) {
		return nil
	}
	check := r.pluginStateWritableChecker()
	if check == nil {
		return nil
	}
	if err := lockPluginStateReadability(ctx, tx, r.db.DriverName(), view.InstallationID); err != nil {
		return err
	}
	return check(ctx, view.InstallationID, view.ProviderKey, view.StateVersion)
}

const (
	pluginPhaseAbsent          = "absent"
	pluginPhaseArtifactStaging = "artifact_staging"
	pluginPhaseBootstrapping   = "bootstrapping"
	pluginPhaseProvisioned     = "provisioned"
	pluginPhaseReady           = "ready"
	pluginPhaseAllocating      = "allocating"
	pluginPhaseCleanupPending  = "cleanup_pending"
	pluginPhaseExpired         = "expired"
)

func pluginExecutorPhaseTerminal(phase string) bool {
	switch phase {
	case pluginPhaseAbsent, pluginPhaseCleanupPending, pluginPhaseExpired:
		return true
	}
	return false
}
