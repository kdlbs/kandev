package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/user/models"
)

var _ AgentProfileRecentUseRepository = (*sqliteRepository)(nil)

const agentProfileRecentUseColumns = `user_id, context, profile_ids, revision, updated_at`

// GetAgentProfileRecentUse reads one user's context history.
func (r *sqliteRepository) GetAgentProfileRecentUse(
	ctx context.Context,
	userID string,
	contextValue models.AgentProfileRecentUseContext,
) (*models.AgentProfileRecentUse, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT `+agentProfileRecentUseColumns+`
		FROM user_agent_profile_recent_use
		WHERE user_id = ? AND context = ?
	`), userID, contextValue)
	return scanAgentProfileRecentUse(row)
}

// ListAgentProfileRecentUse reads all persisted histories for one user.
func (r *sqliteRepository) ListAgentProfileRecentUse(
	ctx context.Context,
	userID string,
) ([]*models.AgentProfileRecentUse, error) {
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT `+agentProfileRecentUseColumns+`
		FROM user_agent_profile_recent_use
		WHERE user_id = ?
		ORDER BY context ASC
	`), userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	records := make([]*models.AgentProfileRecentUse, 0, models.AgentProfileRecentUseMaxContexts)
	for rows.Next() {
		record, scanErr := scanAgentProfileRecentUse(rows)
		if scanErr != nil {
			// A malformed context row must not make the other independent
			// selector histories unavailable. Skip only this row and let the
			// cursor continue to the remaining contexts.
			if r.recentUseLogger != nil {
				fields := []zap.Field{
					zap.String("user_id", userID),
					zap.Error(scanErr),
				}
				if record != nil && record.Context != "" {
					fields = append(fields, zap.String("context", string(record.Context)))
				}
				r.recentUseLogger.Warn("skipping malformed agent profile recent-use row", fields...)
			}
			continue
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// UpsertAgentProfileRecentUse conditionally writes one context history. A
// missing row is created only for revision zero; existing rows update only
// when their current revision equals expectedRevision.
func (r *sqliteRepository) UpsertAgentProfileRecentUse(
	ctx context.Context,
	record *models.AgentProfileRecentUse,
	expectedRevision int64,
) (*models.AgentProfileRecentUse, error) {
	if err := validateAgentProfileRecentUseRecord(record); err != nil {
		return nil, err
	}
	if expectedRevision < 0 || record.Revision != expectedRevision+1 {
		return nil, fmt.Errorf("invalid agent profile recent-use revision")
	}
	profileIDs, err := json.Marshal(record.ProfileIDs)
	if err != nil {
		return nil, fmt.Errorf("marshal agent profile recent-use: %w", err)
	}
	updatedAt := record.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	query := `
		INSERT INTO user_agent_profile_recent_use
			(user_id, context, profile_ids, revision, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (user_id, context) DO UPDATE SET
			profile_ids = excluded.profile_ids,
			revision = excluded.revision,
			updated_at = excluded.updated_at
		WHERE user_agent_profile_recent_use.revision = ?
		RETURNING ` + agentProfileRecentUseColumns
	args := []any{
		record.UserID,
		record.Context,
		string(profileIDs),
		record.Revision,
		updatedAt,
		expectedRevision,
	}
	updated, err := scanAgentProfileRecentUse(r.db.QueryRowContext(ctx, r.db.Rebind(query), args...))
	if err == nil {
		return updated, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAgentProfileRecentUseRevisionConflict
	}
	return nil, err
}

func validateAgentProfileRecentUseRecord(record *models.AgentProfileRecentUse) error {
	if record == nil || record.UserID == "" {
		return errors.New("agent profile recent-use user is required")
	}
	if !models.IsAgentProfileRecentUseContext(record.Context) {
		return fmt.Errorf("unsupported agent profile recent-use context: %q", record.Context)
	}
	if len(record.ProfileIDs) > models.AgentProfileRecentUseMaxProfiles {
		return fmt.Errorf("agent profile recent-use exceeds %d profiles", models.AgentProfileRecentUseMaxProfiles)
	}
	seen := make(map[string]struct{}, len(record.ProfileIDs))
	for _, profileID := range record.ProfileIDs {
		if profileID == "" || len(profileID) > models.AgentProfileRecentUseMaxProfileIDBytes {
			return errors.New("agent profile recent-use profile id is invalid")
		}
		if _, exists := seen[profileID]; exists {
			return errors.New("agent profile recent-use profile ids must be distinct")
		}
		seen[profileID] = struct{}{}
	}
	return nil
}

func scanAgentProfileRecentUse(scanner interface{ Scan(dest ...any) error }) (*models.AgentProfileRecentUse, error) {
	record := &models.AgentProfileRecentUse{}
	var contextValue string
	var profileIDsJSON string
	if err := scanner.Scan(
		&record.UserID,
		&contextValue,
		&profileIDsJSON,
		&record.Revision,
		&record.UpdatedAt,
	); err != nil {
		return nil, err
	}
	record.Context = models.AgentProfileRecentUseContext(contextValue)
	if err := json.Unmarshal([]byte(profileIDsJSON), &record.ProfileIDs); err != nil {
		return record, fmt.Errorf("decode agent profile recent-use: %w", err)
	}
	if record.ProfileIDs == nil {
		record.ProfileIDs = []string{}
	}
	if err := validateAgentProfileRecentUseRecord(record); err != nil {
		return record, err
	}
	return record, nil
}
