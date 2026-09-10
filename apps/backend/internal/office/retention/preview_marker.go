package retention

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	systemsettings "github.com/kandev/kandev/internal/system/settings"
)

// previewMarkerKey is the settings-store key holding the per-swept-table
// preview completion marker, a JSON object keyed by table name.
const previewMarkerKey = "office_run_retention_preview_completed"

// PreviewMarker records, per swept table, the timestamp its preview
// completed. A table absent from the map has never completed a preview.
type PreviewMarker map[TableName]time.Time

// PreviewMarkerStore persists and reads the preview marker.
type PreviewMarkerStore struct {
	store *systemsettings.Store
}

// NewPreviewMarkerStore wraps the shared key/value settings store.
func NewPreviewMarkerStore(store *systemsettings.Store) *PreviewMarkerStore {
	return &PreviewMarkerStore{store: store}
}

// Get reads the marker. readable is false only when a document is present
// but cannot be read or parsed (AC-OFFICE-RUN-HISTORY-RETENTION-003.10); a
// document that was never written is the ordinary fresh-install state and
// reports readable=true with an empty marker, not an error. Either way, a
// table absent from the returned marker has not completed a preview.
func (s *PreviewMarkerStore) Get(ctx context.Context) (PreviewMarker, bool) {
	raw, found, err := s.store.Get(ctx, previewMarkerKey)
	if err != nil {
		return PreviewMarker{}, false
	}
	if !found {
		return PreviewMarker{}, true
	}
	var doc PreviewMarker
	if err := json.Unmarshal(raw, &doc); err != nil {
		return PreviewMarker{}, false
	}
	if doc == nil {
		doc = PreviewMarker{}
	}
	return doc, true
}

// MarkCompleted records that table's preview as completed at the given
// time. It is only ever called after that table's preview evaluation
// completed successfully, is never cleared by a settings change or
// restart, and never touches another table's entry. If the stored document
// was corrupt, this write replaces it with a fresh document carrying only
// this table's entry — the safe direction, since a spurious re-preview of
// another table costs one sweep and deletes nothing.
func (s *PreviewMarkerStore) MarkCompleted(ctx context.Context, table TableName, at time.Time) error {
	marker, readable := s.Get(ctx)
	if !readable {
		marker = PreviewMarker{}
	}
	marker[table] = at
	raw, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	return s.store.Save(ctx, previewMarkerKey, raw)
}

// GetWith is Get against an explicit connection instead of the shared
// settings pool. Required mid-sweep on PostgreSQL: every statement in a
// sweep must run on the session holding the advisory lock (see sweep.go
// and lock.go), and going through the pool here would request a second
// connection, which deadlocks under a maxOpenConns=1 pool (the mandated
// Postgres-gated test harness). This bypasses systemsettings.Store and
// reads the key/value pair directly, so it depends on that package's
// `settings` table keeping its `key`/`value` column names.
func (s *PreviewMarkerStore) GetWith(ctx context.Context, q queryer) (PreviewMarker, bool) {
	var raw string
	err := q.GetContext(ctx, &raw, q.Rebind(`SELECT value FROM settings WHERE key = ?`), previewMarkerKey)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PreviewMarker{}, true
		}
		return PreviewMarker{}, false
	}
	var doc PreviewMarker
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return PreviewMarker{}, false
	}
	if doc == nil {
		doc = PreviewMarker{}
	}
	return doc, true
}

// MarkCompletedWith is MarkCompleted against an explicit connection; see
// GetWith.
func (s *PreviewMarkerStore) MarkCompletedWith(ctx context.Context, q queryer, table TableName, at time.Time) error {
	marker, readable := s.GetWith(ctx, q)
	if !readable {
		marker = PreviewMarker{}
	}
	marker[table] = at
	raw, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, q.Rebind(`
		INSERT INTO settings (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`), previewMarkerKey, string(raw), time.Now().UTC())
	return err
}
