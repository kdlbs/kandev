package sqlite

import (
	"strings"
	"time"

	"go.uber.org/zap"
)

const commentTimestampLayout = "2006-01-02 15:04:05.999999999-07:00"

type commentTimestampUpdate struct {
	id        string
	createdAt string
}

// migrateTaskCommentTimestamps converts legacy offset timestamps to one UTC
// representation. The comment window orders the stored text, so mixed offsets
// can otherwise select an older instant as the newest row. The update is
// idempotent and preserves nanoseconds for values the time parser accepts.
func (r *Repository) migrateTaskCommentTimestamps() {
	updates, unparseable, err := r.collectCommentTimestampUpdates()
	if err != nil {
		if r.log != nil {
			r.log.Warn("task comment timestamp migration scan failed", zap.Error(err))
		}
		return
	}
	if len(updates) > 0 {
		if err := r.applyCommentTimestampUpdates(updates); err != nil {
			if r.log != nil {
				r.log.Warn("task comment timestamp migration update failed", zap.Error(err))
			}
			return
		}
	}
	if r.log != nil && (len(updates) > 0 || unparseable > 0) {
		r.log.Info("task comment timestamps normalized",
			zap.Int("updated", len(updates)), zap.Int("unparseable", unparseable))
	}
}

func (r *Repository) collectCommentTimestampUpdates() ([]commentTimestampUpdate, int, error) {
	rows, err := r.db.Queryx(`SELECT id, CAST(created_at AS TEXT) FROM task_comments`)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	updates := make([]commentTimestampUpdate, 0)
	unparseable := 0
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			unparseable++
			continue
		}
		parsed, ok := parseCommentTimestamp(raw)
		if !ok {
			unparseable++
			continue
		}
		canonical := parsed.UTC().Format(commentTimestampLayout)
		if raw != canonical {
			updates = append(updates, commentTimestampUpdate{id: id, createdAt: canonical})
		}
	}
	return updates, unparseable, rows.Err()
}

func (r *Repository) applyCommentTimestampUpdates(updates []commentTimestampUpdate) error {
	tx, err := r.db.Beginx()
	if err != nil {
		return err
	}
	for _, item := range updates {
		if _, err := tx.Exec(tx.Rebind(
			`UPDATE task_comments SET created_at = ? WHERE id = ?`), item.createdAt, item.id); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func parseCommentTimestamp(raw string) (time.Time, bool) {
	value := strings.TrimSpace(raw)
	for _, layout := range []string{
		time.RFC3339Nano,
		commentTimestampLayout,
		"2006-01-02T15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02T15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}
