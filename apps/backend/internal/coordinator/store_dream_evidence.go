package coordinator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/coordinator/watch"
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

// FirstDreamDecisionAt returns the first proposal decision or override in the
// window. Each query reads a typed timestamp column on both database engines.
func (s *Store) FirstDreamDecisionAt(ctx context.Context, coordinatorID string, start, end time.Time) (*time.Time, error) {
	queries := []string{
		`SELECT decided_at FROM coordinator_outcomes WHERE coordinator_id = ? AND decided_at >= ? AND decided_at < ? ORDER BY decided_at, proposal_id LIMIT 1`,
		`SELECT created_at FROM coordinator_feedback WHERE coordinator_id = ? AND created_at >= ? AND created_at < ? ORDER BY created_at, id LIMIT 1`,
	}
	var first *time.Time
	for _, query := range queries {
		var at sql.NullTime
		err := s.ro.GetContext(ctx, &at, s.ro.Rebind(query), coordinatorID, start.UTC(), end.UTC())
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read first dream decision: %w", err)
		}
		if at.Valid && (first == nil || at.Time.Before(*first)) {
			u := at.Time.UTC()
			first = &u
		}
	}
	return first, nil
}

// FirstLedgerTurnAt returns the start of the coordinator's oldest turn, nil
// when it has none.
func (s *Store) FirstLedgerTurnAt(ctx context.Context, coordinatorID string) (*time.Time, error) {
	var t sql.NullTime
	err := s.ro.GetContext(ctx, &t, s.ro.Rebind(`SELECT started_at FROM coordinator_turns WHERE coordinator_id = ? ORDER BY started_at LIMIT 1`), coordinatorID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("read first ledger turn: %w", err)
	}
	if !t.Valid {
		return nil, nil
	}
	u := t.Time.UTC()
	return &u, nil
}

// NthCompletedTurnFinish returns when n non-dream turns started at or after
// from had finished, nil when fewer exist.
func (s *Store) NthCompletedTurnFinish(ctx context.Context, coordinatorID string, from time.Time, n int) (*time.Time, error) {
	var t sql.NullTime
	err := s.ro.GetContext(ctx, &t, s.ro.Rebind(`SELECT finished_at FROM coordinator_turns
		WHERE coordinator_id = ? AND "trigger" <> ? AND finished_at IS NOT NULL AND started_at >= ?
		ORDER BY finished_at, id LIMIT 1 OFFSET ?`), coordinatorID, DreamTrigger, from.UTC(), n-1)
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

// DreamWindowDecisions returns proposal decisions and manager overrides in
// [start, end), newest first, with the proposal's title.
func (s *Store) DreamWindowDecisions(ctx context.Context, coordinatorID string, start, end time.Time, limit int) ([]DreamDecision, error) {
	where, scopeArgs, err := s.recordingTaskScope(ctx, coordinatorID)
	if err != nil {
		return nil, err
	}
	args := []any{coordinatorID, start.UTC(), end.UTC(), coordinatorID, start.UTC(), end.UTC()}
	args = append(args, scopeArgs...)
	args = append(args, limit)
	rows, err := s.ro.QueryContext(ctx, s.ro.Rebind(`SELECT d.proposal_id, d.kind, d.decision, d.edited_fields, d.reason_code,
		d.task_result, d.cost_subcents, COALESCE(p.spec_json, '') FROM (
		SELECT coordinator_id, proposal_id, kind, decision, edited_fields, reason_code,
		COALESCE(task_result, '') AS task_result, cost_subcents, decided_at AS occurred_at, proposal_id AS event_id
		FROM coordinator_outcomes WHERE coordinator_id = ? AND decided_at >= ? AND decided_at < ?
		UNION ALL
		SELECT f.coordinator_id, f.proposal_id, f.proposal_kind, f.kind, COALESCE(o.edited_fields, '[]'), f.reason_code,
		COALESCE(o.task_result, ''), o.cost_subcents, f.created_at, f.id
		FROM coordinator_feedback f LEFT JOIN coordinator_outcomes o ON o.proposal_id = f.proposal_id AND o.coordinator_id = f.coordinator_id
		WHERE f.coordinator_id = ? AND f.created_at >= ? AND f.created_at < ?
		) d LEFT JOIN coordinator_proposals p ON p.id = d.proposal_id AND p.coordinator_id = d.coordinator_id
		LEFT JOIN tasks t ON t.id = COALESCE(NULLIF(p.target_task_id, ''), NULLIF(p.task_id, ''))
		WHERE COALESCE(NULLIF(p.target_task_id, ''), NULLIF(p.task_id, '')) IS NULL OR (`+where+`)
		ORDER BY d.occurred_at DESC, d.event_id DESC, d.proposal_id DESC LIMIT ?`), args...)
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

func (s *Store) recordingTaskScope(ctx context.Context, coordinatorID string) (string, []any, error) {
	c, err := s.GetCoordinatorByID(ctx, coordinatorID)
	if err != nil {
		return "", nil, err
	}
	set, err := s.EffectiveWatchSet(ctx, s.ro, coordinatorID, c.WorkspaceID)
	if err != nil {
		return "", nil, err
	}
	projects, err := ResolveWatchProjectsSQL(ctx, s.ro, c.WorkspaceID, set.Projects)
	if err != nil {
		return "", nil, err
	}
	projectWhere, projectArgs := watch.TaskSQL(projects)
	where := "t.workspace_id = ? AND " + projectWhere
	args := append([]any{c.WorkspaceID}, projectArgs...)
	if !set.All {
		if len(set.WorkflowIDs) == 0 {
			return "FALSE", nil, nil
		}
		clause, workflowArgs, err := sqlx.In(" AND t.workflow_id IN (?)", set.WorkflowIDs)
		if err != nil {
			return "", nil, err
		}
		where += clause
		args = append(args, workflowArgs...)
	}
	return where, args, nil
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
