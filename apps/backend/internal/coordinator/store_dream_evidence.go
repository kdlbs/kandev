package coordinator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// DreamTrigger is the ledger trigger of a dream episode's own turn.
const DreamTrigger = "dream"

// DreamTurn is one completed non-dream turn of a window as the opening
// message projects it.
type DreamTurn struct {
	ID        string
	Trigger   string
	Verdict   string
	StartedAt time.Time
	Calls     map[string]int
}

// DreamDecision is one decided proposal of a window as the opening message
// projects it.
type DreamDecision struct {
	ProposalID   string
	Kind         string
	Decision     string
	EditedFields string
	ReasonCode   string
	TaskResult   string
	CostSubcents *int64
	Title        string
}

// CountDreamTurns counts the completed non-dream turns started in
// [start, end).
func (s *Store) CountDreamTurns(ctx context.Context, coordinatorID string, start, end time.Time) (int, error) {
	var n int
	err := s.ro.GetContext(ctx, &n, s.ro.Rebind(`SELECT COUNT(*) FROM coordinator_turns
		WHERE coordinator_id = ? AND "trigger" <> ? AND finished_at IS NOT NULL AND started_at >= ? AND started_at < ?`),
		coordinatorID, DreamTrigger, start.UTC(), end.UTC())
	if err != nil {
		return 0, fmt.Errorf("count dream window turns: %w", err)
	}
	return n, nil
}

// CountDreamDecisions counts the decided proposals and overrides decided in
// [start, end).
func (s *Store) CountDreamDecisions(ctx context.Context, coordinatorID string, start, end time.Time) (int, error) {
	var n int
	err := s.ro.GetContext(ctx, &n, s.ro.Rebind(`SELECT
		(SELECT COUNT(*) FROM coordinator_outcomes WHERE coordinator_id = ? AND decided_at >= ? AND decided_at < ?) +
		(SELECT COUNT(*) FROM coordinator_feedback WHERE coordinator_id = ? AND created_at >= ? AND created_at < ?)`),
		coordinatorID, start.UTC(), end.UTC(), coordinatorID, start.UTC(), end.UTC())
	if err != nil {
		return 0, fmt.Errorf("count dream window decisions: %w", err)
	}
	return n, nil
}

// FirstLedgerTurnAt returns the start of the coordinator's oldest turn, nil
// when it has none.
func (s *Store) FirstLedgerTurnAt(ctx context.Context, coordinatorID string) (*time.Time, error) {
	var t sql.NullTime
	err := s.ro.GetContext(ctx, &t, s.ro.Rebind(`SELECT MIN(started_at) FROM coordinator_turns WHERE coordinator_id = ?`), coordinatorID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("read first ledger turn: %w", err)
	}
	if !t.Valid {
		return nil, nil
	}
	u := t.Time.UTC()
	return &u, nil
}

// NthCompletedTurnFinish returns the finish time of the n-th completed
// non-dream turn started at or after from, nil when fewer exist.
func (s *Store) NthCompletedTurnFinish(ctx context.Context, coordinatorID string, from time.Time, n int) (*time.Time, error) {
	var t sql.NullTime
	err := s.ro.GetContext(ctx, &t, s.ro.Rebind(`SELECT finished_at FROM coordinator_turns
		WHERE coordinator_id = ? AND "trigger" <> ? AND finished_at IS NOT NULL AND started_at >= ?
		ORDER BY started_at, id LIMIT 1 OFFSET ?`), coordinatorID, DreamTrigger, from.UTC(), n-1)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read nth completed turn: %w", err)
	}
	u := t.Time.UTC()
	return &u, nil
}

// DreamWindowTurns returns up to limit completed non-dream turns started in
// [start, end), newest first by (started_at, id), with call counts by action.
func (s *Store) DreamWindowTurns(ctx context.Context, coordinatorID string, start, end time.Time, limit int) ([]DreamTurn, error) {
	rows, err := s.ro.QueryContext(ctx, s.ro.Rebind(`SELECT id, "trigger", COALESCE(verdict, ''), started_at FROM coordinator_turns
		WHERE coordinator_id = ? AND "trigger" <> ? AND finished_at IS NOT NULL AND started_at >= ? AND started_at < ?
		ORDER BY started_at DESC, id DESC LIMIT ?`), coordinatorID, DreamTrigger, start.UTC(), end.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("read dream window turns: %w", err)
	}
	var turns []DreamTurn
	for rows.Next() {
		var t DreamTurn
		if err := rows.Scan(&t.ID, &t.Trigger, &t.Verdict, &t.StartedAt); err != nil {
			_ = rows.Close()
			return nil, err
		}
		t.StartedAt = t.StartedAt.UTC()
		t.Calls = map[string]int{}
		turns = append(turns, t)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()
	for i := range turns {
		crows, err := s.ro.QueryContext(ctx, s.ro.Rebind(`SELECT action, COUNT(*) FROM coordinator_turn_calls WHERE turn_id = ? GROUP BY action`), turns[i].ID)
		if err != nil {
			return nil, fmt.Errorf("read dream turn calls: %w", err)
		}
		for crows.Next() {
			var a string
			var n int
			if err := crows.Scan(&a, &n); err != nil {
				_ = crows.Close()
				return nil, err
			}
			turns[i].Calls[a] = n
		}
		if err := crows.Err(); err != nil {
			_ = crows.Close()
			return nil, err
		}
		_ = crows.Close()
	}
	return turns, nil
}

// DreamWindowDecisions returns the proposals decided in [start, end) by
// (decided_at, proposal id), with the proposal's title.
func (s *Store) DreamWindowDecisions(ctx context.Context, coordinatorID string, start, end time.Time, limit int) ([]DreamDecision, error) {
	rows, err := s.ro.QueryContext(ctx, s.ro.Rebind(`SELECT o.proposal_id, o.kind, o.decision, o.edited_fields, o.reason_code,
		COALESCE(o.task_result, ''), o.cost_subcents, COALESCE(p.spec_json, '') FROM coordinator_outcomes o
		LEFT JOIN coordinator_proposals p ON p.id = o.proposal_id
		WHERE o.coordinator_id = ? AND o.decided_at >= ? AND o.decided_at < ?
		ORDER BY o.decided_at DESC, o.proposal_id DESC LIMIT ?`), coordinatorID, start.UTC(), end.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("read dream window decisions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DreamDecision
	for rows.Next() {
		var d DreamDecision
		var cost sql.NullInt64
		var spec string
		if err := rows.Scan(&d.ProposalID, &d.Kind, &d.Decision, &d.EditedFields, &d.ReasonCode, &d.TaskResult, &cost, &spec); err != nil {
			return nil, err
		}
		var ps ProposalSpec
		if json.Unmarshal([]byte(spec), &ps) == nil {
			d.Title = ps.Title
		}
		if cost.Valid {
			v := cost.Int64
			d.CostSubcents = &v
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DreamAgreement counts the rated items with a measured replay verdict of the
// dreams started in [since, until) and how many agree (improvement with
// useful; blocked or not_an_improvement with not_useful or harmful).
func (s *Store) DreamAgreement(ctx context.Context, coordinatorID string, since, until time.Time) (rated, agreeing int64, err error) {
	rows, err := s.ro.QueryContext(ctx, s.ro.Rebind(`SELECT i.id, i.replay_verdict FROM coordinator_dream_items i
		JOIN coordinator_dreams d ON d.id = i.dream_id
		WHERE d.coordinator_id = ? AND d.started_at >= ? AND d.started_at < ?
		  AND i.replay_verdict IN ('blocked', 'improvement', 'not_an_improvement')`), coordinatorID, since.UTC(), until.UTC())
	if err != nil {
		return 0, 0, fmt.Errorf("read agreement items: %w", err)
	}
	type pair struct{ id, verdict string }
	var items []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.id, &p.verdict); err != nil {
			_ = rows.Close()
			return 0, 0, err
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, 0, err
	}
	_ = rows.Close()
	for _, p := range items {
		rating, err := s.effectiveRating(ctx, p.id)
		if err != nil {
			return 0, 0, err
		}
		if rating == "" {
			continue
		}
		rated++
		if DreamAgrees(p.verdict, rating) {
			agreeing++
		}
	}
	return rated, agreeing, nil
}

// DreamAgrees reports whether a replay verdict and a rating agree.
func DreamAgrees(verdict, rating string) bool {
	if verdict == "improvement" {
		return rating == "useful"
	}
	return rating == "not_useful" || rating == "harmful"
}
