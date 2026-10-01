package coordinator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// Dream statuses.
const (
	DreamRunning = "running"
	DreamOK      = "ok"
	DreamClean   = "clean"
	DreamPartial = "partial"
	DreamFailed  = "failed"
	DreamSkipped = "skipped"
)

// DreamRetention is how long dream reports are kept.
const DreamRetention = 400 * 24 * time.Hour

// ErrDreamNotFound is returned when a dream, item or rating target does not
// exist for the coordinator named.
var ErrDreamNotFound = errors.New("coordinator: dream not found")

// Dream is the review-result row of one dream.
type Dream struct {
	ID               string
	CoordinatorID    string
	Status           string
	Reason           string
	WindowStart      time.Time
	WindowEnd        time.Time
	InputHash        string
	TurnIDs          []string
	Considered       []string
	Model            string
	CostSubcents     *int64
	EpisodeTaskID    string
	EpisodeSessionID string
	EpisodeArchived  bool
	StartedAt        time.Time
	RefreshedAt      time.Time
	FinishedAt       *time.Time
}

// DreamItem is one suggestion of a report with its gate and replay result.
type DreamItem struct {
	ID           string
	DreamID      string
	Position     int
	Kind         string
	Text         string
	TargetID     string
	CitedTurnIDs []string
	Gate         string
	ReplayID     string
	Verdict      string
	ReplayReason string
	// Rating is the item's effective rating, empty when unrated.
	Rating string
}

type dreamRow struct {
	ID               string         `db:"id"`
	CoordinatorID    string         `db:"coordinator_id"`
	Status           string         `db:"status"`
	Reason           string         `db:"reason"`
	WindowStart      time.Time      `db:"window_start"`
	WindowEnd        time.Time      `db:"window_end"`
	InputHash        string         `db:"input_hash"`
	TurnIDs          string         `db:"turn_ids"`
	Considered       string         `db:"considered"`
	Model            string         `db:"model"`
	CostSubcents     sql.NullInt64  `db:"cost_subcents"`
	EpisodeTaskID    sql.NullString `db:"episode_task_id"`
	EpisodeSessionID sql.NullString `db:"episode_session_id"`
	EpisodeArchived  sql.NullTime   `db:"episode_archived_at"`
	StartedAt        time.Time      `db:"started_at"`
	RefreshedAt      time.Time      `db:"refreshed_at"`
	FinishedAt       sql.NullTime   `db:"finished_at"`
}

const dreamColumns = `id, coordinator_id, status, reason, window_start, window_end, input_hash, turn_ids, considered, model,
	cost_subcents, episode_task_id, episode_session_id, episode_archived_at, started_at, refreshed_at, finished_at`

func (r dreamRow) dream() Dream {
	d := Dream{
		ID: r.ID, CoordinatorID: r.CoordinatorID, Status: r.Status, Reason: r.Reason,
		WindowStart: r.WindowStart.UTC(), WindowEnd: r.WindowEnd.UTC(), InputHash: r.InputHash, Model: r.Model,
		EpisodeTaskID: r.EpisodeTaskID.String, EpisodeSessionID: r.EpisodeSessionID.String,
		EpisodeArchived: r.EpisodeArchived.Valid,
		StartedAt:       r.StartedAt.UTC(), RefreshedAt: r.RefreshedAt.UTC(),
	}
	_ = json.Unmarshal([]byte(r.TurnIDs), &d.TurnIDs)
	_ = json.Unmarshal([]byte(r.Considered), &d.Considered)
	if r.CostSubcents.Valid {
		v := r.CostSubcents.Int64
		d.CostSubcents = &v
	}
	if r.FinishedAt.Valid {
		t := r.FinishedAt.Time.UTC()
		d.FinishedAt = &t
	}
	return d
}

func jsonStrings(v []string) string {
	if v == nil {
		v = []string{}
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// ShadowDreamEnabled reads the coordinator's Shadow dream switch.
func (s *Store) ShadowDreamEnabled(ctx context.Context, coordinatorID string) (bool, error) {
	var n int
	err := s.ro.GetContext(ctx, &n, s.ro.Rebind(`SELECT shadow_dream_enabled FROM coordinators WHERE id = ?`), coordinatorID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("read shadow dream switch: %w", err)
	}
	return n != 0, nil
}

// SetShadowDreamEnabled writes the switch only; it changes no revision.
func (s *Store) SetShadowDreamEnabled(ctx context.Context, workspaceID, coordinatorID string, on bool) error {
	v := 0
	if on {
		v = 1
	}
	res, err := s.db.ExecContext(ctx, s.db.Rebind(
		`UPDATE coordinators SET shadow_dream_enabled = ? WHERE id = ? AND workspace_id = ?`), v, coordinatorID, workspaceID)
	if err != nil {
		return fmt.Errorf("set shadow dream switch: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// InsertRunningDream stores the lease row. It reports false, storing nothing,
// when the coordinator already holds a running row.
func (s *Store) InsertRunningDream(ctx context.Context, d Dream) (bool, error) {
	res, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO coordinator_dreams (id, coordinator_id, status, window_start, window_end, input_hash, model, started_at, refreshed_at)
		VALUES (?, ?, 'running', ?, ?, ?, ?, ?, ?)
		ON CONFLICT (coordinator_id) WHERE status = 'running' DO NOTHING`),
		d.ID, d.CoordinatorID, d.WindowStart.UTC(), d.WindowEnd.UTC(), d.InputHash, d.Model, d.StartedAt.UTC(), d.StartedAt.UTC())
	if err != nil {
		return false, fmt.Errorf("insert running dream: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// InsertSkippedDream stores one unchanged skip per input hash; it reports
// false when one is already stored.
func (s *Store) InsertSkippedDream(ctx context.Context, d Dream) (bool, error) {
	res, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO coordinator_dreams (id, coordinator_id, status, reason, window_start, window_end, input_hash, model, started_at, refreshed_at, finished_at)
		VALUES (?, ?, 'skipped', 'unchanged', ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (coordinator_id, input_hash) WHERE status = 'skipped' DO NOTHING`),
		d.ID, d.CoordinatorID, d.WindowStart.UTC(), d.WindowEnd.UTC(), d.InputHash, d.Model,
		d.StartedAt.UTC(), d.StartedAt.UTC(), d.StartedAt.UTC())
	if err != nil {
		return false, fmt.Errorf("insert skipped dream: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// RefreshDream extends the lease; false means the row is no longer running.
func (s *Store) RefreshDream(ctx context.Context, id string, now time.Time) (bool, error) {
	res, err := s.db.ExecContext(ctx, s.db.Rebind(
		`UPDATE coordinator_dreams SET refreshed_at = ? WHERE id = ? AND status = 'running'`), now.UTC(), id)
	return rowsChanged(res, err, "refresh dream")
}

// FailDream settles a running row failed; false means it was not running.
func (s *Store) FailDream(ctx context.Context, id, reason string, now time.Time) (bool, error) {
	res, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE coordinator_dreams SET status = 'failed', reason = ?, finished_at = ? WHERE id = ? AND status = 'running'`),
		reason, now.UTC(), id)
	return rowsChanged(res, err, "fail dream")
}

// ExpireStaleDreams fails every running row last refreshed before cutoff with
// the reason lease_lost and returns the rows it changed.
func (s *Store) ExpireStaleDreams(ctx context.Context, coordinatorID string, cutoff, now time.Time) ([]Dream, error) {
	var rows []dreamRow
	err := s.db.SelectContext(ctx, &rows, s.db.Rebind(`SELECT `+dreamColumns+` FROM coordinator_dreams
		WHERE coordinator_id = ? AND status = 'running' AND refreshed_at < ?`), coordinatorID, cutoff.UTC())
	if err != nil {
		return nil, fmt.Errorf("list stale dreams: %w", err)
	}
	var out []Dream
	for _, r := range rows {
		res, err := s.db.ExecContext(ctx, s.db.Rebind(`
			UPDATE coordinator_dreams SET status = 'failed', reason = 'lease_lost', finished_at = ?
			WHERE id = ? AND status = 'running' AND refreshed_at < ?`), now.UTC(), r.ID, cutoff.UTC())
		changed, err := rowsChanged(res, err, "expire dream")
		if err != nil {
			return out, err
		}
		if changed {
			d := r.dream()
			d.Status, d.Reason = DreamFailed, "lease_lost"
			out = append(out, d)
		}
	}
	return out, nil
}

// SetDreamEpisodeTask records the dream task id before the session exists.
func (s *Store) SetDreamEpisodeTask(ctx context.Context, id, taskID string) (bool, error) {
	res, err := s.db.ExecContext(ctx, s.db.Rebind(
		`UPDATE coordinator_dreams SET episode_task_id = ? WHERE id = ? AND status = 'running'`), taskID, id)
	return rowsChanged(res, err, "set dream task")
}

// SetDreamEpisodeSession records the dream session id.
func (s *Store) SetDreamEpisodeSession(ctx context.Context, id, sessionID string) (bool, error) {
	res, err := s.db.ExecContext(ctx, s.db.Rebind(
		`UPDATE coordinator_dreams SET episode_session_id = ? WHERE id = ? AND status = 'running'`), sessionID, id)
	return rowsChanged(res, err, "set dream session")
}

// MarkDreamEpisodeArchived records that the episode task is archived.
func (s *Store) MarkDreamEpisodeArchived(ctx context.Context, id string, now time.Time) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(
		`UPDATE coordinator_dreams SET episode_archived_at = ? WHERE id = ? AND episode_archived_at IS NULL`), now.UTC(), id)
	if err != nil {
		return fmt.Errorf("mark dream episode archived: %w", err)
	}
	return nil
}

// DreamsWithOpenEpisode lists the coordinator's dream rows that name an
// episode task not yet archived, oldest first.
func (s *Store) DreamsWithOpenEpisode(ctx context.Context, coordinatorID string) ([]Dream, error) {
	var rows []dreamRow
	err := s.ro.SelectContext(ctx, &rows, s.ro.Rebind(`SELECT `+dreamColumns+` FROM coordinator_dreams
		WHERE coordinator_id = ? AND status <> 'running' AND episode_task_id IS NOT NULL AND episode_archived_at IS NULL
		ORDER BY started_at, id`), coordinatorID)
	if err != nil {
		return nil, fmt.Errorf("list dreams with open episode: %w", err)
	}
	return dreamsOf(rows), nil
}

func dreamsOf(rows []dreamRow) []Dream {
	out := make([]Dream, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.dream())
	}
	return out
}

// RunningDream returns the coordinator's running row, nil when none.
func (s *Store) RunningDream(ctx context.Context, coordinatorID string) (*Dream, error) {
	var r dreamRow
	err := s.ro.GetContext(ctx, &r, s.ro.Rebind(`SELECT `+dreamColumns+` FROM coordinator_dreams WHERE coordinator_id = ? AND status = 'running'`), coordinatorID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read running dream: %w", err)
	}
	d := r.dream()
	return &d, nil
}

// LastAcceptedDream returns the newest accepted row (clean, ok or partial).
func (s *Store) LastAcceptedDream(ctx context.Context, coordinatorID string) (*Dream, error) {
	return s.oneDream(ctx, `status IN ('clean','ok','partial')`, coordinatorID)
}

// LastDream returns the newest row of any status but skipped.
func (s *Store) LastDream(ctx context.Context, coordinatorID string) (*Dream, error) {
	return s.oneDream(ctx, `status <> 'skipped'`, coordinatorID)
}

func (s *Store) oneDream(ctx context.Context, cond, coordinatorID string) (*Dream, error) {
	var r dreamRow
	err := s.ro.GetContext(ctx, &r, s.ro.Rebind(`SELECT `+dreamColumns+` FROM coordinator_dreams
		WHERE coordinator_id = ? AND `+cond+` ORDER BY started_at DESC, id DESC LIMIT 1`), coordinatorID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read dream: %w", err)
	}
	d := r.dream()
	return &d, nil
}

// AcceptedDreamWithHash reports whether an accepted row holds the hash.
func (s *Store) AcceptedDreamWithHash(ctx context.Context, coordinatorID, hash string) (bool, error) {
	var n int
	err := s.ro.GetContext(ctx, &n, s.ro.Rebind(`SELECT COUNT(*) FROM coordinator_dreams
		WHERE coordinator_id = ? AND input_hash = ? AND status IN ('clean','ok','partial')`), coordinatorID, hash)
	if err != nil {
		return false, fmt.Errorf("count dreams by hash: %w", err)
	}
	return n > 0, nil
}

// DreamCursor is the list position: rows strictly older than (StartedAt, ID).
type DreamCursor struct {
	StartedAt time.Time
	ID        string
}

// ListDreams returns up to limit reports newest first by (started_at, id),
// skipped rows left out.
func (s *Store) ListDreams(ctx context.Context, coordinatorID string, before *DreamCursor, limit int) ([]Dream, error) {
	q := `SELECT ` + dreamColumns + ` FROM coordinator_dreams WHERE coordinator_id = ? AND status <> 'skipped'`
	args := []any{coordinatorID}
	if before != nil {
		q += ` AND (started_at < ? OR (started_at = ? AND id < ?))`
		args = append(args, before.StartedAt.UTC(), before.StartedAt.UTC(), before.ID)
	}
	q += ` ORDER BY started_at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	var rows []dreamRow
	if err := s.ro.SelectContext(ctx, &rows, s.ro.Rebind(q), args...); err != nil {
		return nil, fmt.Errorf("list dreams: %w", err)
	}
	return dreamsOf(rows), nil
}

// DreamItemCounts returns the item count of each listed dream.
func (s *Store) DreamItemCounts(ctx context.Context, ids []string) (map[string]int, error) {
	out := map[string]int{}
	if len(ids) == 0 {
		return out, nil
	}
	q, args, err := sqlx.In(`SELECT dream_id, COUNT(*) FROM coordinator_dream_items WHERE dream_id IN (?) GROUP BY dream_id`, ids)
	if err != nil {
		return nil, err
	}
	rows, err := s.ro.QueryContext(ctx, s.ro.Rebind(q), args...)
	if err != nil {
		return nil, fmt.Errorf("count dream items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// GetDream returns one dream of the coordinator, ErrDreamNotFound otherwise.
func (s *Store) GetDream(ctx context.Context, coordinatorID, dreamID string) (*Dream, error) {
	var r dreamRow
	err := s.ro.GetContext(ctx, &r, s.ro.Rebind(`SELECT `+dreamColumns+` FROM coordinator_dreams WHERE id = ? AND coordinator_id = ?`), dreamID, coordinatorID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDreamNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read dream: %w", err)
	}
	d := r.dream()
	return &d, nil
}

type dreamItemRow struct {
	ID           string         `db:"id"`
	DreamID      string         `db:"dream_id"`
	Position     int            `db:"position"`
	Kind         string         `db:"kind"`
	Text         string         `db:"text"`
	TargetID     string         `db:"target_id"`
	CitedTurnIDs string         `db:"cited_turn_ids"`
	Gate         string         `db:"gate"`
	ReplayID     sql.NullString `db:"replay_id"`
	Verdict      string         `db:"replay_verdict"`
	ReplayReason string         `db:"replay_reason"`
}

// ListDreamItems returns the dream's items in report order with each item's
// effective rating.
func (s *Store) ListDreamItems(ctx context.Context, dreamID string) ([]DreamItem, error) {
	var rows []dreamItemRow
	err := s.ro.SelectContext(ctx, &rows, s.ro.Rebind(`SELECT id, dream_id, position, kind, text, target_id, cited_turn_ids, gate,
		replay_id, replay_verdict, replay_reason FROM coordinator_dream_items WHERE dream_id = ? ORDER BY position`), dreamID)
	if err != nil {
		return nil, fmt.Errorf("list dream items: %w", err)
	}
	out := make([]DreamItem, 0, len(rows))
	for _, r := range rows {
		it := DreamItem{ID: r.ID, DreamID: r.DreamID, Position: r.Position, Kind: r.Kind, Text: r.Text, TargetID: r.TargetID,
			Gate: r.Gate, ReplayID: r.ReplayID.String, Verdict: r.Verdict, ReplayReason: r.ReplayReason}
		_ = json.Unmarshal([]byte(r.CitedTurnIDs), &it.CitedTurnIDs)
		if it.Rating, err = s.effectiveRating(ctx, it.ID); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, nil
}

// effectiveRating is the newest rating of the item across managers, ties by
// user id ascending.
func (s *Store) effectiveRating(ctx context.Context, itemID string) (string, error) {
	var rating string
	err := s.ro.GetContext(ctx, &rating, s.ro.Rebind(`SELECT rating FROM coordinator_dream_ratings
		WHERE item_id = ? ORDER BY rated_at DESC, user_id ASC LIMIT 1`), itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read rating: %w", err)
	}
	return rating, nil
}

// RateDreamItem upserts the manager's rating of an item of the named dream of
// the coordinator, stamping the commit time.
func (s *Store) RateDreamItem(ctx context.Context, coordinatorID, dreamID, itemID, userID, rating string) error {
	var n int
	err := s.db.GetContext(ctx, &n, s.db.Rebind(`SELECT COUNT(*) FROM coordinator_dream_items i
		JOIN coordinator_dreams d ON d.id = i.dream_id
		WHERE i.id = ? AND i.dream_id = ? AND d.coordinator_id = ?`), itemID, dreamID, coordinatorID)
	if err != nil {
		return fmt.Errorf("read dream item: %w", err)
	}
	if n == 0 {
		return ErrDreamNotFound
	}
	_, err = s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO coordinator_dream_ratings (item_id, user_id, rating, rated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (item_id, user_id) DO UPDATE SET rating = excluded.rating, rated_at = excluded.rated_at`),
		itemID, userID, rating, s.now().UTC())
	if err != nil {
		return fmt.Errorf("rate dream item: %w", err)
	}
	return nil
}

// FinishDream stores the items and the final status in one transaction, only
// while the row is running. It reports false, storing nothing, otherwise.
func (s *Store) FinishDream(ctx context.Context, d Dream, items []DreamItem, now time.Time) (bool, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin finish dream: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var cost any
	if d.CostSubcents != nil {
		cost = *d.CostSubcents
	}
	res, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE coordinator_dreams SET status = ?, reason = ?, turn_ids = ?, considered = ?, model = ?, cost_subcents = ?, finished_at = ?
		WHERE id = ? AND status = 'running'`),
		d.Status, d.Reason, jsonStrings(d.TurnIDs), jsonStrings(d.Considered), d.Model, cost, now.UTC(), d.ID)
	changed, err := rowsChanged(res, err, "finish dream")
	if err != nil || !changed {
		return false, err
	}
	for _, it := range items {
		var replayID any
		if it.ReplayID != "" {
			replayID = it.ReplayID
		}
		if _, err := tx.ExecContext(ctx, tx.Rebind(`
			INSERT INTO coordinator_dream_items (id, dream_id, position, kind, text, target_id, cited_turn_ids, gate, replay_id, replay_verdict, replay_reason)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
			it.ID, d.ID, it.Position, it.Kind, it.Text, it.TargetID, jsonStrings(it.CitedTurnIDs), it.Gate, replayID, it.Verdict, it.ReplayReason); err != nil {
			return false, fmt.Errorf("insert dream item: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit finish dream: %w", err)
	}
	return true, nil
}

// DreamCoordinatorIDs lists the coordinators the backstop visits for the
// dream: Shadow on, a running row, a running replay, or an episode task not
// yet archived.
func (s *Store) DreamCoordinatorIDs(ctx context.Context) ([]string, error) {
	return s.queryIDs(ctx, "dream coordinators", `
		SELECT id FROM coordinators WHERE shadow_dream_enabled = 1
		UNION SELECT coordinator_id FROM coordinator_dreams WHERE status = 'running'
		UNION SELECT coordinator_id FROM coordinator_dreams
			WHERE episode_task_id IS NOT NULL AND episode_archived_at IS NULL
		UNION SELECT coordinator_id FROM coordinator_replay_results WHERE status = 'running'
		ORDER BY 1`)
}

// PruneDreams deletes up to limit dreams started before cutoff with their
// items and ratings.
func (s *Store) PruneDreams(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	var ids []string
	if err := s.db.SelectContext(ctx, &ids, s.db.Rebind(
		`SELECT id FROM coordinator_dreams WHERE started_at < ? AND status <> 'running' ORDER BY started_at, id LIMIT ?`), cutoff.UTC(), limit); err != nil {
		return 0, fmt.Errorf("select dreams to prune: %w", err)
	}
	var total int64
	for _, id := range ids {
		for _, q := range []string{
			`DELETE FROM coordinator_dream_ratings WHERE item_id IN (SELECT id FROM coordinator_dream_items WHERE dream_id = ?)`,
			`DELETE FROM coordinator_dream_items WHERE dream_id = ?`,
			`DELETE FROM coordinator_dreams WHERE id = ?`,
		} {
			if _, err := s.db.ExecContext(ctx, s.db.Rebind(q), id); err != nil {
				return total, fmt.Errorf("prune dream: %w", err)
			}
		}
		total++
	}
	return total, nil
}

func rowsChanged(res sql.Result, err error, what string) (bool, error) {
	if err != nil {
		return false, fmt.Errorf("%s: %w", what, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("%s: %w", what, err)
	}
	return n > 0, nil
}

// NewDreamID returns a fresh dream or item id.
func NewDreamID() string { return uuid.NewString() }
