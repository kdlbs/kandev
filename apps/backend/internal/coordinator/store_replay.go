package coordinator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/coordinator/replay"
)

// replayBaselineScanRows bounds how many finished rows a baseline lookup reads;
// a case not found in them is simply run again.
const replayBaselineScanRows = 100

// ReplayResults is the replay harness's result-row store, over the
// coordinator's store.
type ReplayResults struct{ store *Store }

// NewReplayResults returns the result-row store of the replay harness.
func NewReplayResults(s *Store) *ReplayResults { return &ReplayResults{store: s} }

var _ replay.Results = (*ReplayResults)(nil)

type replayRow struct {
	ID                    string         `db:"id"`
	Status                string         `db:"status"`
	CandidateHash         string         `db:"candidate_hash"`
	BaselineHash          string         `db:"baseline_hash"`
	Model                 string         `db:"model"`
	Cases                 string         `db:"cases"`
	CandidateScore        sql.NullInt64  `db:"candidate_score"`
	BaselineScore         sql.NullInt64  `db:"baseline_score"`
	HeldOutCandidateScore sql.NullInt64  `db:"heldout_candidate_score"`
	HeldOutBaselineScore  sql.NullInt64  `db:"heldout_baseline_score"`
	CasesRanCandidate     int            `db:"cases_ran_candidate"`
	CasesRanBaseline      int            `db:"cases_ran_baseline"`
	CasesCompared         int            `db:"cases_compared"`
	HeldOutCompared       int            `db:"heldout_compared"`
	CitedTurnIDs          string         `db:"cited_turn_ids"`
	Flips                 string         `db:"flips"`
	UnmatchedCandidate    int            `db:"unmatched_candidate"`
	UnmatchedBaseline     int            `db:"unmatched_baseline"`
	Guard                 string         `db:"guard"`
	Verdict               string         `db:"verdict"`
	Reason                string         `db:"reason"`
	CostSubcents          int64          `db:"cost_subcents"`
	DreamID               sql.NullString `db:"dream_id"`
}

const replayColumns = `id, status, candidate_hash, baseline_hash, model, cases, candidate_score, baseline_score,
	heldout_candidate_score, heldout_baseline_score, cases_ran_candidate, cases_ran_baseline, cases_compared,
	heldout_compared, cited_turn_ids, flips, unmatched_candidate, unmatched_baseline, guard, verdict, reason,
	cost_subcents, dream_id`

func nullInt(p *int64) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *p, Valid: true}
}

func intPtr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

func nullString(v string) sql.NullString { return sql.NullString{String: v, Valid: v != ""} }

func (r replayRow) stored() (replay.Stored, error) {
	res := replay.Result{
		RowID: r.ID, Guard: r.Guard, Verdict: r.Verdict, Reason: r.Reason,
		CandidateHash: r.CandidateHash, BaselineHash: r.BaselineHash, Model: r.Model,
		CandidateScore: intPtr(r.CandidateScore), BaselineScore: intPtr(r.BaselineScore),
		HeldOutCandidateScore: intPtr(r.HeldOutCandidateScore), HeldOutBaselineScore: intPtr(r.HeldOutBaselineScore),
		CasesRanCandidate: r.CasesRanCandidate, CasesRanBaseline: r.CasesRanBaseline,
		CasesCompared: r.CasesCompared, HeldOutCompared: r.HeldOutCompared,
		UnmatchedCandidate: r.UnmatchedCandidate, UnmatchedBaseline: r.UnmatchedBaseline,
		CostSubcents: r.CostSubcents,
	}
	for _, f := range []struct {
		raw  string
		into any
	}{{r.Cases, &res.Cases}, {r.CitedTurnIDs, &res.CitedTurnIDs}, {r.Flips, &res.Flips}} {
		if err := json.Unmarshal([]byte(f.raw), f.into); err != nil {
			return replay.Stored{}, fmt.Errorf("decode replay row %s: %w", r.ID, err)
		}
	}
	for _, c := range res.Cases {
		if c.Skip != "" {
			res.Skipped = append(res.Skipped, replay.Skip{TurnID: c.TurnID, Reason: c.Skip})
		}
	}
	return replay.Stored{ID: r.ID, Status: r.Status, Result: res}, nil
}

// Insert writes a running row. When the unique (dream, item) key is already
// held it returns the held row and inserted false.
func (r *ReplayResults) Insert(ctx context.Context, n replay.NewRow) (replay.Stored, bool, error) {
	s := r.store
	id := uuid.New().String()
	res, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO coordinator_replay_results (id, coordinator_id, status, dream_id, item_id, prompt_version, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`),
		id, n.CoordinatorID, replay.StatusRunning, nullString(n.DreamID), nullString(n.ItemID), n.PromptVersion, n.CreatedAt.UTC())
	if err != nil {
		return replay.Stored{}, false, fmt.Errorf("insert replay row: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return replay.Stored{}, false, fmt.Errorf("count inserted replay row: %w", err)
	}
	if affected == 1 {
		return replay.Stored{ID: id, Status: replay.StatusRunning}, true, nil
	}
	var row replayRow
	if err := s.db.GetContext(ctx, &row, s.db.Rebind(`SELECT `+replayColumns+` FROM coordinator_replay_results WHERE dream_id = ? AND item_id = ?`),
		n.DreamID, n.ItemID); err != nil {
		return replay.Stored{}, false, fmt.Errorf("read held replay row: %w", err)
	}
	stored, err := row.stored()
	return stored, false, err
}

// AddCost adds subcents to the row's cost in one statement.
func (r *ReplayResults) AddCost(ctx context.Context, rowID string, subcents int64) error {
	s := r.store
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE coordinator_replay_results SET cost_subcents = cost_subcents + ? WHERE id = ?`), subcents, rowID)
	if err != nil {
		return fmt.Errorf("add replay cost: %w", err)
	}
	return nil
}

// Finish writes the rest of the row and marks it done, only while it is
// running, and reports whether a row matched. The cost kept is the larger of
// the stored and the given one.
func (r *ReplayResults) Finish(ctx context.Context, rowID string, res replay.Result, finishedAt time.Time) (bool, error) {
	s := r.store
	cases, flips, cited := mustJSON(res.Cases, "[]"), mustJSON(res.Flips, "[]"), mustJSON(res.CitedTurnIDs, "[]")
	out, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE coordinator_replay_results SET status = ?, candidate_hash = ?, baseline_hash = ?, model = ?, cases = ?,
			candidate_score = ?, baseline_score = ?, heldout_candidate_score = ?, heldout_baseline_score = ?,
			cases_ran_candidate = ?, cases_ran_baseline = ?, cases_compared = ?, heldout_compared = ?, cited_turn_ids = ?,
			flips = ?, unmatched_candidate = ?, unmatched_baseline = ?, guard = ?, verdict = ?, reason = ?,
			cost_subcents = CASE WHEN cost_subcents > ? THEN cost_subcents ELSE ? END, finished_at = ?
		WHERE id = ? AND status = ?`),
		replay.StatusDone, res.CandidateHash, res.BaselineHash, res.Model, cases,
		nullInt(res.CandidateScore), nullInt(res.BaselineScore), nullInt(res.HeldOutCandidateScore), nullInt(res.HeldOutBaselineScore),
		res.CasesRanCandidate, res.CasesRanBaseline, res.CasesCompared, res.HeldOutCompared, cited,
		flips, res.UnmatchedCandidate, res.UnmatchedBaseline, res.Guard, res.Verdict, res.Reason,
		res.CostSubcents, res.CostSubcents, finishedAt.UTC(), rowID, replay.StatusRunning)
	if err != nil {
		return false, fmt.Errorf("finish replay row: %w", err)
	}
	n, err := out.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("count finished replay row: %w", err)
	}
	return n == 1, nil
}

func mustJSON(v any, fallback string) string {
	raw, err := json.Marshal(v)
	if err != nil || string(raw) == "null" {
		return fallback
	}
	return string(raw)
}

// BaselineAttempts returns, per case id, the successful baseline attempts of
// the newest finished row of the coordinator that holds that case under the
// key, ignoring rows settled interrupted or cancelled.
func (r *ReplayResults) BaselineAttempts(ctx context.Context, coordinatorID, baselineHash, model, promptVersion string) (map[string][]replay.Attempt, error) {
	out := map[string][]replay.Attempt{}
	if baselineHash == "" || model == "" {
		return out, nil
	}
	s := r.store
	var raws []string
	err := s.ro.SelectContext(ctx, &raws, s.ro.Rebind(`
		SELECT cases FROM coordinator_replay_results
		WHERE coordinator_id = ? AND status = ? AND baseline_hash = ? AND model = ? AND prompt_version = ?
		  AND reason NOT IN (?, ?)
		ORDER BY created_at DESC, id DESC LIMIT ?`),
		coordinatorID, replay.StatusDone, baselineHash, model, promptVersion,
		replay.ReasonInterrupted, replay.ReasonCancelled, replayBaselineScanRows)
	if err != nil {
		return nil, fmt.Errorf("read baseline attempts: %w", err)
	}
	for _, raw := range raws {
		var cases []replay.CaseRecord
		if err := json.Unmarshal([]byte(raw), &cases); err != nil {
			return nil, fmt.Errorf("decode baseline attempts: %w", err)
		}
		for _, c := range cases {
			if _, held := out[c.TurnID]; held {
				continue
			}
			var ok []replay.Attempt
			for _, a := range c.Baseline {
				if a.OK {
					ok = append(ok, a)
				}
			}
			if len(ok) > 0 {
				out[c.TurnID] = ok
			}
		}
	}
	return out, nil
}

// SettleStaleReplays finishes every replay row still running 30 minutes after
// it was created as interrupted and unmeasured. A late write by the replay
// that owned the row matches nothing.
func (s *Store) SettleStaleReplays(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE coordinator_replay_results SET status = ?, guard = ?, verdict = ?, reason = ?, finished_at = ?
		WHERE status = ? AND created_at < ?`),
		replay.StatusDone, replay.GuardUnmeasured, replay.VerdictUnmeasured, replay.ReasonInterrupted, now.UTC(),
		replay.StatusRunning, now.UTC().Add(-replay.StaleAfter))
	if err != nil {
		return 0, fmt.Errorf("settle stale replays: %w", err)
	}
	return res.RowsAffected()
}

// HasRunningReplay reports whether the coordinator holds a running replay row.
func (s *Store) HasRunningReplay(ctx context.Context, coordinatorID string) (bool, error) {
	var n int
	err := s.ro.GetContext(ctx, &n, s.ro.Rebind(`SELECT COUNT(*) FROM coordinator_replay_results WHERE coordinator_id = ? AND status = ?`),
		coordinatorID, replay.StatusRunning)
	if err != nil {
		return false, fmt.Errorf("count running replays: %w", err)
	}
	return n > 0, nil
}

// ExtraSpend sums the cost of the coordinator's replay rows created in
// [from, to), running rows included.
func (s *Store) ExtraSpend(ctx context.Context, coordinatorID string, from, to time.Time) (int64, error) {
	var total sql.NullInt64
	err := s.ro.GetContext(ctx, &total, s.ro.Rebind(`
		SELECT SUM(cost_subcents) FROM coordinator_replay_results
		WHERE coordinator_id = ? AND created_at >= ? AND created_at < ?`), coordinatorID, from.UTC(), to.UTC())
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("replay spend: %w", err)
	}
	return total.Int64, nil
}

// PruneReplayResults deletes up to limit result rows created before cutoff.
func (s *Store) PruneReplayResults(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	res, err := s.db.ExecContext(ctx, s.db.Rebind(`DELETE FROM coordinator_replay_results WHERE id IN (
		SELECT id FROM coordinator_replay_results WHERE created_at < ? ORDER BY created_at, id LIMIT ?)`), cutoff.UTC(), limit)
	if err != nil {
		return 0, fmt.Errorf("prune replay results: %w", err)
	}
	return res.RowsAffected()
}
