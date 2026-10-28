package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/kandev/kandev/internal/db/dialect"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/plugins/workspacedescriptor"
	"github.com/kandev/kandev/internal/task/models"
)

const pluginWorkspaceDescriptorLockNamespace = "plugin_workspace_descriptor:"

// pluginEnvelopeView is the part of a plugin_executor envelope the descriptor
// checks read.
type pluginEnvelopeView struct {
	Phase          string                          `json:"phase"`
	InstallationID string                          `json:"installation_id"`
	ProviderKey    string                          `json:"provider_key"`
	StateVersion   uint32                          `json:"state_version"`
	EnvironmentID  string                          `json:"environment_id"`
	Platform       string                          `json:"platform"`
	Descriptor     *workspacedescriptor.Descriptor `json:"workspace_descriptor"`
}

func decodePluginEnvelopeView(metadata map[string]interface{}) pluginEnvelopeView {
	var view pluginEnvelopeView
	if encoded, err := json.Marshal(metadata[pluginExecutorEnvelopeKey]); err == nil {
		_ = json.Unmarshal(encoded, &view)
	}
	if view.Descriptor != nil && *view.Descriptor == (workspacedescriptor.Descriptor{}) {
		view.Descriptor = nil
	}
	return view
}

// checkPluginWorkspaceImmutable refuses a non-terminal write whose descriptor
// differs from the one recorded for the environment generation. It runs inside
// the envelope-revision CAS, so a refused write changes nothing.
func checkPluginWorkspaceImmutable(current, incoming map[string]interface{}) error {
	recorded, next := decodePluginEnvelopeView(current), decodePluginEnvelopeView(incoming)
	if recorded.Descriptor == nil || pluginExecutorPhaseTerminal(next.Phase) {
		return nil
	}
	if next.Descriptor == nil || *next.Descriptor != *recorded.Descriptor {
		return fmt.Errorf("%w: the recorded workspace descriptor is immutable for an environment generation", models.ErrExecutorConflict)
	}
	return nil
}

// checkPluginWorkspaceUnique refuses a non-terminal write that records a path
// or identity another non-terminal environment of the same installation already
// holds. The scan and the write share one transaction; on PostgreSQL an
// installation-scoped advisory lock orders concurrent writers so the second
// committer sees the first.
func (r *Repository) checkPluginWorkspaceUnique(ctx context.Context, tx *sqlx.Tx, running *models.ExecutorRunning) error {
	next := decodePluginEnvelopeView(running.Metadata)
	if next.Descriptor == nil || next.InstallationID == "" || pluginExecutorPhaseTerminal(next.Phase) {
		return nil
	}
	if err := lockPluginWorkspaceDescriptors(ctx, tx, r.db.DriverName(), next.InstallationID); err != nil {
		return err
	}
	rows, err := tx.QueryxContext(ctx, tx.Rebind(`SELECT session_id, metadata FROM executors_running WHERE runtime = ? AND session_id <> ?`),
		string(agentruntime.RuntimePluginRemote), running.SessionID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var sessionID, metadataJSON string
		if err := rows.Scan(&sessionID, &metadataJSON); err != nil {
			return err
		}
		var metadata map[string]interface{}
		if json.Unmarshal([]byte(metadataJSON), &metadata) != nil {
			continue
		}
		other := decodePluginEnvelopeView(metadata)
		if other.InstallationID != next.InstallationID || other.EnvironmentID == next.EnvironmentID ||
			other.Descriptor == nil || pluginExecutorPhaseTerminal(other.Phase) {
			continue
		}
		if workspacedescriptor.Conflicts(next.Platform, *next.Descriptor, *other.Descriptor) {
			return fmt.Errorf("%w: another environment of this installation holds the workspace path or identity", models.ErrExecutorConflict)
		}
	}
	return rows.Err()
}

func lockPluginWorkspaceDescriptors(ctx context.Context, tx *sqlx.Tx, driverName, installationID string) error {
	if !dialect.IsPostgres(driverName) {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, pluginWorkspaceDescriptorLockNamespace+installationID); err != nil {
		return fmt.Errorf("lock plugin workspace descriptors: %w", err)
	}
	return nil
}
