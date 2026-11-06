package sqlite

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/jmoiron/sqlx"
)

func usageSourceGuardHash(domain, value string) string {
	sum := sha256.Sum256([]byte(domain + "\x00" + value))
	return hex.EncodeToString(sum[:])
}

func (r *Repository) usageSourceScopeUnsafeTx(ctx context.Context, tx *sqlx.Tx, target retainedTarget) (bool, error) {
	var foreign bool
	err := tx.GetContext(ctx, &foreign, r.db.Rebind(`SELECT EXISTS(SELECT 1 FROM usage_subjects WHERE scope_id <> ?
		AND task_id = ? AND task_incarnation_id = ? AND session_id = ? AND session_incarnation_id = ?)
		OR EXISTS(SELECT 1 FROM usage_collection_bindings WHERE external_session_id <> '' AND external_session_id = ? AND scope_id <> ?)`),
		target.ScopeID, target.TaskID, target.TaskIncarnationID, target.SessionID, target.SessionIncarnationID, target.ExternalSessionID, target.ScopeID)
	if err != nil {
		return false, err
	}
	liveForeign, err := r.usageLiveSourceCrossesScopeTx(ctx, tx, target)
	if err != nil {
		return false, err
	}
	return r.guardUsageSourceTx(ctx, tx, target.ScopeID, target.WorkspaceID, target.ExternalSessionID, foreign || liveForeign || target.Forked, false)
}

// Source guards contain only irreversible identity hashes. They outlive erased
// accounting rows so a new session cannot adopt a previously foreign transcript.
func (r *Repository) guardUsageSourceTx(ctx context.Context, tx *sqlx.Tx, scope, workspace, transcript string, mixed, erased bool) (bool, error) {
	if transcript == "" {
		return mixed, nil
	}
	key := usageSourceGuardHash("usage-transcript", transcript)
	owner := usageSourceGuardHash("usage-scope", scope)
	_, err := tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_collection_source_guards (source_hash,scope_hash,mixed,erased)
		VALUES (?, ?, ?, ?) ON CONFLICT(source_hash) DO UPDATE SET
		mixed = CASE WHEN usage_collection_source_guards.mixed = 1 OR usage_collection_source_guards.scope_hash <> excluded.scope_hash OR excluded.mixed = 1 THEN 1 ELSE 0 END,
		erased = CASE WHEN usage_collection_source_guards.erased = 1 OR excluded.erased = 1 THEN 1 ELSE 0 END`),
		key, owner, boolInt(mixed), boolInt(erased))
	if err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_collection_guard_workspaces (workspace_hash,source_hash)
		VALUES (?, ?) ON CONFLICT(workspace_hash,source_hash) DO NOTHING`), usageSourceGuardHash("usage-workspace", workspace), key)
	if err != nil {
		return false, err
	}
	var unsafe bool
	err = tx.GetContext(ctx, &unsafe, r.db.Rebind(`SELECT mixed <> 0 OR erased <> 0 FROM usage_collection_source_guards WHERE source_hash = ?`), key)
	return unsafe, err
}

// preserveErasedUsageSourcePageTx must run before deleting a scope's collection
// bindings. Its sequence cursor and guard mutations share the erasure transaction.
func (r *Repository) preserveErasedUsageSourcePageTx(ctx context.Context, tx *sqlx.Tx, scope string, last int64, limit int) (int64, bool, error) {
	limit = min(max(limit, 1), usageHistoryBatchLimit)
	var rows []struct {
		Sequence  int64  `db:"sequence"`
		Workspace string `db:"workspace_id"`
		Source    string `db:"external_session_id"`
	}
	err := tx.SelectContext(ctx, &rows, r.db.Rebind(`SELECT sequence,workspace_id,external_session_id FROM usage_collection_bindings
		WHERE scope_id = ? AND sequence > ? ORDER BY sequence LIMIT ?`), scope, last, limit)
	if err != nil {
		return last, false, err
	}
	for _, row := range rows {
		if _, err := r.guardUsageSourceTx(ctx, tx, scope, row.Workspace, row.Source, false, true); err != nil {
			return last, false, err
		}
		last = row.Sequence
	}
	if len(rows) == limit {
		return last, false, nil
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_collection_source_guards SET erased = 1 WHERE scope_hash = ?`), usageSourceGuardHash("usage-scope", scope))
	return last, err == nil, err
}

// preserveErasedUsageSourcesTx runs under the erasure fence before any scope
// children are removed. Each read is a bounded scope or binding keyset page.
func (r *Repository) preserveErasedUsageSourcesTx(ctx context.Context, tx *sqlx.Tx, scopeFilter string, args ...any) error {
	last := ""
	for {
		var scopes []string
		queryArgs := append(append([]any{}, args...), last, usageHistoryBatchLimit)
		if err := tx.SelectContext(ctx, &scopes, r.db.Rebind(`SELECT scope_id FROM usage_scopes WHERE `+scopeFilter+` AND scope_id > ? ORDER BY scope_id LIMIT ?`), queryArgs...); err != nil {
			return err
		}
		for _, scope := range scopes {
			if err := r.preserveErasedUsageSourceScopeTx(ctx, tx, scope); err != nil {
				return err
			}
			last = scope
		}
		if len(scopes) < usageHistoryBatchLimit {
			return nil
		}
	}
}
