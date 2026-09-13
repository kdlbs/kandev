package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// clarificationInboxSidecarSchemaDDL creates the Needs-you Inbox per-user
// dismiss/snooze sidecar (needs-you-inbox design, "Persistence"). Primary key
// (user_id, pending_id) makes repeated dismiss/snooze an upsert by
// construction, and the sidecar never touches task_session_messages: the
// underlying clarification record is unchanged by a dismiss or a snooze.
const clarificationInboxSidecarSchemaDDL = `
	CREATE TABLE IF NOT EXISTS clarification_inbox_sidecar (
		user_id TEXT NOT NULL,
		pending_id TEXT NOT NULL,
		state TEXT NOT NULL,
		snooze_until TIMESTAMP,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		PRIMARY KEY (user_id, pending_id)
	);
`

func (r *Repository) initClarificationInboxSidecarSchema() error {
	_, err := r.db.ExecContext(r.migrationContext(), clarificationInboxSidecarSchemaDDL)
	return err
}

const upsertClarificationInboxSidecarSQL = `
	INSERT INTO clarification_inbox_sidecar (user_id, pending_id, state, snooze_until, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?)
	ON CONFLICT (user_id, pending_id) DO UPDATE SET
		state = excluded.state,
		snooze_until = excluded.snooze_until,
		updated_at = excluded.updated_at
`

// UpsertClarificationInboxSidecar dismisses or snoozes one bundle for one
// operator. Idempotent by construction: repeating the same call leaves the
// single row the primary key allows. Re-snoozing REPLACES
// snooze_until with a fresh absolute instant rather than extending the old
// one, matching the design's "no accumulation" rule.
func (r *Repository) UpsertClarificationInboxSidecar(
	ctx context.Context,
	userID, pendingID string,
	state models.ClarificationSidecarState,
	snoozeUntil *time.Time,
	now time.Time,
) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(upsertClarificationInboxSidecarSQL),
		userID, pendingID, string(state), snoozeUntil, now, now,
	)
	return err
}

// DeleteClarificationInboxSidecar restores a bundle: the sidecar row is
// deleted, which is the whole of restore (the underlying record was never
// touched). Deleting an absent row is a no-op success, which is what makes
// this endpoint idempotent.
func (r *Repository) DeleteClarificationInboxSidecar(ctx context.Context, userID, pendingID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM clarification_inbox_sidecar WHERE user_id = ? AND pending_id = ?
	`), userID, pendingID)
	return err
}

// CountHiddenClarificationBundles answers the workspace-wide hidden_count and
// next_snooze_expiry the main read and the hidden-bundles read both need
// (needs-you-inbox design, "Data and contracts"). opts.Sidecar must carry
// Only=true; every other opts field narrows the same way it does for
// ListUnresolvedClarificationBundles, so this can never disagree with the
// page query about what counts as answerable.
func (r *Repository) CountHiddenClarificationBundles(
	ctx context.Context,
	opts models.ListClarificationBundlesOptions,
) (models.ClarificationInboxHiddenSummary, error) {
	drv := r.ro.DriverName()
	joinExtra, joinArgs := clarificationSidecarJoin(opts.Sidecar)
	whereExtra, whereArgs := clarificationBundleWhereClause(opts)
	args := append(append([]interface{}{}, joinArgs...), whereArgs...)
	query := clarificationBundleCountQuery(drv, joinExtra, whereExtra)

	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(query), args...)
	var count int
	if dialect.IsPostgres(drv) {
		var nextExpiry sql.NullTime
		if err := row.Scan(&count, &nextExpiry); err != nil {
			return models.ClarificationInboxHiddenSummary{}, err
		}
		return summaryFromScan(count, nextExpiry.Valid, nextExpiry.Time), nil
	}
	var nextExpiryRaw sql.NullString
	if err := row.Scan(&count, &nextExpiryRaw); err != nil {
		return models.ClarificationInboxHiddenSummary{}, err
	}
	if !nextExpiryRaw.Valid || nextExpiryRaw.String == "" {
		return summaryFromScan(count, false, time.Time{}), nil
	}
	return summaryFromScan(count, true, parseLegacyTimestamp(nextExpiryRaw.String)), nil
}

func summaryFromScan(count int, hasExpiry bool, expiry time.Time) models.ClarificationInboxHiddenSummary {
	summary := models.ClarificationInboxHiddenSummary{HiddenCount: count}
	if hasExpiry {
		summary.NextSnoozeExpiry = &expiry
	}
	return summary
}

// GetClarificationInboxSidecarStates resolves each addressed pending_id's
// hidden state and snooze expiry for one operator, used to enrich the hidden-
// bundles enumeration after ListUnresolvedClarificationBundles has already
// produced the bundle identities with the Only=true sidecar filter.
// A pending_id with no sidecar row (should not happen given the caller's own
// filter, but the map lookup degrades safely) is simply absent from the map.
func (r *Repository) GetClarificationInboxSidecarStates(
	ctx context.Context,
	userID string,
	pendingIDs []string,
) (map[string]models.ClarificationInboxHiddenBundle, error) {
	out := make(map[string]models.ClarificationInboxHiddenBundle, len(pendingIDs))
	if len(pendingIDs) == 0 {
		return out, nil
	}

	placeholders := make([]string, len(pendingIDs))
	args := make([]interface{}, 0, len(pendingIDs)+1)
	args = append(args, userID)
	for i, id := range pendingIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}

	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT pending_id, state, snooze_until
		FROM clarification_inbox_sidecar
		WHERE user_id = ? AND pending_id IN (`+strings.Join(placeholders, ", ")+`)
	`), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var pendingID, state string
		var snoozeUntil sql.NullTime
		if err := rows.Scan(&pendingID, &state, &snoozeUntil); err != nil {
			return nil, err
		}
		entry := models.ClarificationInboxHiddenBundle{
			PendingID: pendingID,
			State:     models.ClarificationSidecarState(state),
		}
		if snoozeUntil.Valid {
			t := snoozeUntil.Time
			entry.SnoozeUntil = &t
		}
		out[pendingID] = entry
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
