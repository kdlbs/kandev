package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

func lockTurnChangeRetention(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB) error {
	if !dialect.IsPostgres(db.DriverName()) {
		return nil
	}
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, turnChangeRetentionLock)
	return err
}

func (r *Repository) ApplyTurnChangeRetention(ctx context.Context, policy models.TurnChangeRetentionPolicy, now time.Time) (models.TurnChangeRetentionResult, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if policy.RetainFor <= 0 || policy.TaskBytes < 0 || policy.InstallationBytes < 0 {
		return models.TurnChangeRetentionResult{}, fmt.Errorf("apply turn change retention: invalid policy")
	}
	return r.applyTurnChangeRetention(ctx, policy, now)
}

func (r *Repository) applyTurnChangeRetention(ctx context.Context, policy models.TurnChangeRetentionPolicy, now time.Time) (models.TurnChangeRetentionResult, error) {
	// Retention selection and deletion share one transaction. PostgreSQL writers
	// use a scoped advisory lock so two backend instances cannot over-evict.
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return models.TurnChangeRetentionResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockTurnChangeRetention(ctx, tx, r.db); err != nil {
		return models.TurnChangeRetentionResult{}, err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM turn_change_content_leases WHERE expires_at <= ?`), now); err != nil {
		return models.TurnChangeRetentionResult{}, err
	}
	candidates, err := listTurnChangeRetentionCandidates(ctx, tx, r.db, now)
	if err != nil {
		return models.TurnChangeRetentionResult{}, err
	}
	selected := selectTurnChangeRetentionCandidates(candidates, policy, now)
	result := models.TurnChangeRetentionResult{}
	for _, candidate := range selected {
		if candidate.leased {
			continue
		}
		if err := expireTurnChangeContent(ctx, tx, r.db, candidate, now); err != nil {
			return models.TurnChangeRetentionResult{}, err
		}
		result.ExpiredChangeSets++
	}
	deleted, freed, err := deleteOrphanedTurnChangeContent(ctx, tx, r.db)
	if err != nil {
		return models.TurnChangeRetentionResult{}, err
	}
	result.DeletedContents, result.FreedPayloadBytes = deleted, freed
	if err := tx.Commit(); err != nil {
		return models.TurnChangeRetentionResult{}, err
	}
	return result, nil
}

type turnChangeRetentionCandidate struct {
	id, taskID   string
	createdAt    time.Time
	retainUntil  sql.NullTime
	leased       bool
	bytes        int64
	contentSizes map[string]int64
	expiryReason models.TurnChangeReason
}

func listTurnChangeRetentionCandidates(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, now time.Time) ([]turnChangeRetentionCandidate, error) {
	rows, err := tx.QueryxContext(ctx, db.Rebind(`
		SELECT cs.id, cs.task_id, cs.created_at, cs.retain_until,
			EXISTS(SELECT 1 FROM turn_change_content_leases l WHERE l.change_set_id = cs.id AND l.expires_at > ?),
			c.id, length(c.payload_bytes)
		FROM turn_change_sets cs
		LEFT JOIN turn_repository_changes rc ON rc.change_set_id = cs.id
		LEFT JOIN turn_file_changes f ON f.repository_change_id = rc.id
		LEFT JOIN turn_change_content_links l ON l.file_change_id = f.id
		LEFT JOIN turn_change_contents c ON c.id = l.content_id
		WHERE cs.terminal_at IS NOT NULL AND cs.availability <> 'expired'
		ORDER BY cs.created_at ASC, cs.id ASC
	`), now)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	byID := make(map[string]*turnChangeRetentionCandidate)
	contentSizes := make(map[string]map[string]int64)
	for rows.Next() {
		var candidate turnChangeRetentionCandidate
		var contentID sql.NullString
		var payloadBytes sql.NullInt64
		if err := rows.Scan(&candidate.id, &candidate.taskID, &candidate.createdAt, &candidate.retainUntil, &candidate.leased, &contentID, &payloadBytes); err != nil {
			return nil, err
		}
		stored := byID[candidate.id]
		if stored == nil {
			copy := candidate
			stored = &copy
			byID[candidate.id] = stored
			contentSizes[candidate.id] = make(map[string]int64)
			stored.contentSizes = contentSizes[candidate.id]
		}
		if contentID.Valid && payloadBytes.Valid {
			contentSizes[candidate.id][contentID.String] = payloadBytes.Int64
			stored.contentSizes[contentID.String] = payloadBytes.Int64
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]turnChangeRetentionCandidate, 0, len(byID))
	for id, candidate := range byID {
		for _, size := range contentSizes[id] {
			candidate.bytes += size
		}
		out = append(out, *candidate)
	}
	sortTurnChangeRetentionCandidates(out)
	return out, nil
}

func selectTurnChangeRetentionCandidates(candidates []turnChangeRetentionCandidate, policy models.TurnChangeRetentionPolicy, now time.Time) []turnChangeRetentionCandidate {
	usage := newTurnChangeRetentionUsage(candidates)
	selected := make(map[string]turnChangeRetentionCandidate)
	selectAgeExpiredTurnChanges(candidates, policy, now, usage, selected)
	selectSizeExpiredTurnChanges(candidates, policy, usage, selected)
	out := make([]turnChangeRetentionCandidate, 0, len(selected))
	for _, candidate := range candidates {
		if item, ok := selected[candidate.id]; ok {
			out = append(out, item)
		}
	}
	return out
}

type turnChangeRetentionUsage struct {
	taskSizes         map[string]map[string]int64
	taskRefs          map[string]map[string]int
	taskBytes         map[string]int64
	installSizes      map[string]int64
	installRefs       map[string]int
	installationBytes int64
}

func newTurnChangeRetentionUsage(candidates []turnChangeRetentionCandidate) *turnChangeRetentionUsage {
	usage := &turnChangeRetentionUsage{
		taskSizes: make(map[string]map[string]int64), taskRefs: make(map[string]map[string]int),
		taskBytes: make(map[string]int64), installSizes: make(map[string]int64), installRefs: make(map[string]int),
	}
	for _, candidate := range candidates {
		usage.add(candidate)
	}
	for taskID, sizes := range usage.taskSizes {
		for _, size := range sizes {
			usage.taskBytes[taskID] += size
		}
	}
	for _, size := range usage.installSizes {
		usage.installationBytes += size
	}
	return usage
}

func (usage *turnChangeRetentionUsage) add(candidate turnChangeRetentionCandidate) {
	if usage.taskSizes[candidate.taskID] == nil {
		usage.taskSizes[candidate.taskID] = make(map[string]int64)
		usage.taskRefs[candidate.taskID] = make(map[string]int)
	}
	for contentID, size := range candidate.contentSizes {
		usage.taskSizes[candidate.taskID][contentID] = size
		usage.taskRefs[candidate.taskID][contentID]++
		usage.installSizes[contentID] = size
		usage.installRefs[contentID]++
	}
}

func (usage *turnChangeRetentionUsage) remove(candidate turnChangeRetentionCandidate) {
	for contentID := range candidate.contentSizes {
		usage.taskRefs[candidate.taskID][contentID]--
		if usage.taskRefs[candidate.taskID][contentID] == 0 {
			usage.taskBytes[candidate.taskID] -= usage.taskSizes[candidate.taskID][contentID]
			delete(usage.taskSizes[candidate.taskID], contentID)
		}
		usage.installRefs[contentID]--
		if usage.installRefs[contentID] == 0 {
			usage.installationBytes -= usage.installSizes[contentID]
			delete(usage.installSizes, contentID)
		}
	}
}

func selectAgeExpiredTurnChanges(
	candidates []turnChangeRetentionCandidate,
	policy models.TurnChangeRetentionPolicy,
	now time.Time,
	usage *turnChangeRetentionUsage,
	selected map[string]turnChangeRetentionCandidate,
) {
	for _, candidate := range candidates {
		aged := (!candidate.retainUntil.Valid && candidate.createdAt.Add(policy.RetainFor).Before(now)) ||
			(candidate.retainUntil.Valid && !candidate.retainUntil.Time.After(now))
		if candidate.leased || !aged {
			continue
		}
		candidate.expiryReason = models.TurnChangeReasonExpiredAge
		selected[candidate.id] = candidate
		usage.remove(candidate)
	}
}

func selectSizeExpiredTurnChanges(
	candidates []turnChangeRetentionCandidate,
	policy models.TurnChangeRetentionPolicy,
	usage *turnChangeRetentionUsage,
	selected map[string]turnChangeRetentionCandidate,
) {
	for _, candidate := range candidates {
		if candidate.leased {
			continue
		}
		if _, expired := selected[candidate.id]; expired {
			continue
		}
		if candidate.bytes > 0 && policy.TaskBytes > 0 && usage.taskBytes[candidate.taskID] > policy.TaskBytes {
			candidate.expiryReason = models.TurnChangeReasonExpiredTaskLimit
			selected[candidate.id] = candidate
			usage.remove(candidate)
		} else if candidate.bytes > 0 && policy.InstallationBytes > 0 && usage.installationBytes > policy.InstallationBytes {
			candidate.expiryReason = models.TurnChangeReasonExpiredInstallLimit
			selected[candidate.id] = candidate
			usage.remove(candidate)
		}
	}
}

func sortTurnChangeRetentionCandidates(candidates []turnChangeRetentionCandidate) {
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].createdAt.Equal(candidates[j].createdAt) {
			return candidates[i].id < candidates[j].id
		}
		return candidates[i].createdAt.Before(candidates[j].createdAt)
	})
}

func expireTurnChangeContent(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, candidate turnChangeRetentionCandidate, now time.Time) error {
	reason := candidate.expiryReason
	if reason == "" {
		reason = models.TurnChangeReasonExpiredTaskLimit
	}
	if _, err := tx.ExecContext(ctx, db.Rebind(`
		UPDATE turn_file_changes SET canonical_content_id = '', filtered_content_id = '', old_content_id = '', new_content_id = '',
			content_availability = 'expired', content_reason = ?, content_truncated = TRUE
		WHERE repository_change_id IN (SELECT id FROM turn_repository_changes WHERE change_set_id = ?)
	`), reason, candidate.id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, db.Rebind(`DELETE FROM turn_change_content_links WHERE file_change_id IN (
		SELECT f.id FROM turn_file_changes f JOIN turn_repository_changes r ON r.id = f.repository_change_id WHERE r.change_set_id = ?
	)`), candidate.id); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, db.Rebind(`UPDATE turn_change_sets SET availability = 'expired', reason = ?, content_complete = ?, expiry_reason = ?, content_bytes = 0, revision = revision + 1, updated_at = ? WHERE id = ? AND terminal_at IS NOT NULL`), reason, false, reason, now, candidate.id)
	return err
}

func deleteOrphanedTurnChangeContent(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB) (int64, int64, error) {
	rows, err := tx.QueryxContext(ctx, `SELECT c.id, length(c.payload_bytes) FROM turn_change_contents c WHERE NOT EXISTS (SELECT 1 FROM turn_change_content_links l WHERE l.content_id = c.id)`)
	if err != nil {
		return 0, 0, err
	}
	var ids []string
	var bytesFreed int64
	for rows.Next() {
		var id string
		var size int64
		if err := rows.Scan(&id, &size); err != nil {
			_ = rows.Close()
			return 0, 0, err
		}
		ids = append(ids, id)
		bytesFreed += size
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, 0, err
	}
	_ = rows.Close()
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, db.Rebind(`DELETE FROM turn_change_contents WHERE id = ? AND NOT EXISTS (SELECT 1 FROM turn_change_content_links WHERE content_id = ?)`), id, id); err != nil {
			return 0, 0, err
		}
	}
	return int64(len(ids)), bytesFreed, nil
}
