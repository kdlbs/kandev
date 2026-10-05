package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// MergeTaskMetadata changes only supplied ordinary keys and the modification
// timestamp, under native writer/row serialization across database handles.
func (r *Repository) MergeTaskMetadata(ctx context.Context, id string, requested map[string]interface{}) error {
	overlay, err := ordinaryMetadataMergeOverlay(requested)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, r.hierarchyTxOptions())
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockTaskMetadataMutation(ctx, tx, id); err != nil {
		return err
	}
	current, err := r.currentMetadataForMerge(ctx, tx, id)
	if err != nil {
		return err
	}
	preserveMergeWorkspaceIdentity(current, overlay)
	expression := "?"
	document := overlay
	if strings.TrimSpace(string(current[models.MetaKeyAgentTitlePending])) == "true" {
		expression = pendingTaskMetadataMergeExpression(r.db.DriverName())
	} else {
		maps.Copy(current, overlay)
		document = current
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode merged task metadata: %w", err)
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind("UPDATE tasks SET metadata = "+expression+", updated_at = ? WHERE id = ?"), string(encoded), r.nowUTC(), id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("%w: %s", ErrTaskNotFound, id)
	}
	return tx.Commit()
}

func (r *Repository) lockTaskMetadataMutation(ctx context.Context, tx *sql.Tx, id string) error {
	if dialect.IsPostgres(r.db.DriverName()) {
		_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "task-metadata-merge:"+id)
		return err
	}
	if err := r.lockTaskHierarchy(ctx, tx, nil, nil); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	return nil
}

func ordinaryMetadataMergeOverlay(requested map[string]interface{}) (map[string]json.RawMessage, error) {
	copied := maps.Clone(requested)
	models.StripOfficeCarrierMetadata(copied)
	for _, key := range []string{models.MetaKeyDeferredLaunch, models.MetaKeyStepHandoffCarry, models.MetaKeyHandoffSource, models.MetaKeyHandoffs, models.MetaKeyAgentTitlePending, models.MetaKeyAgentTitleOwnerSessionID} {
		delete(copied, key)
	}
	encoded, err := json.Marshal(copied)
	if err != nil {
		return nil, fmt.Errorf("encode task metadata intent: %w", err)
	}
	return decodeMetadataMergeObject(string(encoded))
}

func (r *Repository) currentMetadataForMerge(ctx context.Context, tx *sql.Tx, id string) (map[string]json.RawMessage, error) {
	query := "SELECT metadata FROM tasks WHERE id = ?"
	if dialect.IsPostgres(r.db.DriverName()) {
		query += forUpdateClause
	}
	var raw sql.NullString
	if err := tx.QueryRowContext(ctx, r.db.Rebind(query), id).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s", ErrTaskNotFound, id)
		}
		return nil, err
	}
	return decodeMetadataMergeObject(raw.String)
}

func decodeMetadataMergeObject(raw string) (map[string]json.RawMessage, error) {
	document := make(map[string]json.RawMessage)
	trimmed := strings.TrimSpace(raw)
	if trimmed != "" && trimmed != jsonNull {
		if err := json.Unmarshal([]byte(trimmed), &document); err != nil {
			return nil, fmt.Errorf("decode task metadata: %w", err)
		}
	}
	return document, nil
}

// Materialized workspace identity survives ordinary metadata intent. Other
// workspace members keep the current operation's value semantics.
func preserveMergeWorkspaceIdentity(current, overlay map[string]json.RawMessage) {
	requested, supplied := overlay["workspace"]
	if !supplied {
		return
	}
	var workspace map[string]json.RawMessage
	if json.Unmarshal(current["workspace"], &workspace) != nil {
		return
	}
	var mode string
	if json.Unmarshal(workspace["mode"], &mode) != nil || mode != taskWorkspaceModeSharedGroup {
		return
	}
	var updated map[string]json.RawMessage
	// Non-object workspace input contributes no ordinary object members.
	if json.Unmarshal(requested, &updated) != nil || updated == nil {
		updated = make(map[string]json.RawMessage)
	}
	updated["mode"] = workspace["mode"]
	if group, exists := workspace["group_id"]; exists {
		updated["group_id"] = group
	} else {
		delete(updated, "group_id")
	}
	overlay["workspace"], _ = json.Marshal(updated)
}
