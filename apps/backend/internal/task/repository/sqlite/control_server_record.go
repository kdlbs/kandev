package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// GetControlServerRecord reads the single installation-scoped control-server
// record. Startup step 4 reads this before any control-server contact: with
// no record (models.ErrControlServerRecordNotFound), the caller spawns a
// fresh server and reports no recovered instances.
func (r *Repository) GetControlServerRecord(ctx context.Context) (*models.ControlServerRecord, error) {
	record := &models.ControlServerRecord{}
	var capabilitiesJSON string

	err := r.ro.QueryRowContext(ctx, `
		SELECT endpoint, server_identity, credential_secret_id, capabilities, diagnostic_log_path, created_at, updated_at
		FROM control_server_records WHERE id = 1
	`).Scan(
		&record.Endpoint, &record.ServerIdentity, &record.CredentialSecretID, &capabilitiesJSON,
		&record.DiagnosticLogPath, &record.CreatedAt, &record.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, models.ErrControlServerRecordNotFound
	}
	if err != nil {
		return nil, err
	}

	if capabilitiesJSON != "" {
		if err := json.Unmarshal([]byte(capabilitiesJSON), &record.Capabilities); err != nil {
			return nil, fmt.Errorf("failed to deserialize control server record capabilities: %w", err)
		}
	}
	return record, nil
}

// UpsertControlServerRecord writes the single installation-scoped
// control-server record, replacing whatever record was there before. A
// server started after a refused or failed adoption rewrites the record to
// name the new server: the record cannot keep pointing at a server that is
// about to reap itself. CreatedAt is preserved across a rewrite.
func (r *Repository) UpsertControlServerRecord(ctx context.Context, record *models.ControlServerRecord) error {
	if record == nil {
		return fmt.Errorf("control server record is nil")
	}

	now := time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	record.UpdatedAt = now

	capabilities := record.Capabilities
	if capabilities == nil {
		capabilities = []string{}
	}
	capabilitiesJSON, err := json.Marshal(capabilities)
	if err != nil {
		return fmt.Errorf("failed to serialize control server record capabilities: %w", err)
	}

	_, err = r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO control_server_records (
			id, endpoint, server_identity, credential_secret_id, capabilities, diagnostic_log_path, created_at, updated_at
		) VALUES (1, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			endpoint = excluded.endpoint,
			server_identity = excluded.server_identity,
			credential_secret_id = excluded.credential_secret_id,
			capabilities = excluded.capabilities,
			diagnostic_log_path = excluded.diagnostic_log_path,
			updated_at = excluded.updated_at
	`),
		record.Endpoint,
		record.ServerIdentity,
		record.CredentialSecretID,
		string(capabilitiesJSON),
		record.DiagnosticLogPath,
		record.CreatedAt,
		record.UpdatedAt,
	)
	// created_at is deliberately absent from the UPDATE SET list above, so a
	// rewrite after a prior record's row keeps the original creation time in
	// the database even though the in-memory record above was stamped with
	// this call's now.
	return err
}
