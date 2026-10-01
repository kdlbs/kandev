package ledger

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/coordinator"
)

// ErrUnavailable is returned when the ledger or the watch set cannot be read;
// no partial page accompanies it.
var ErrUnavailable = errors.New("coordinator turn ledger: unavailable")

const (
	DefaultLimit   = 20
	MaxLimit       = 50
	defaultSince   = 30 * 24 * time.Hour
	maxSince       = 400 * 24 * time.Hour
	maxProposalIDs = 20
)

// ListArgs are the tool arguments after type decoding; ranges are checked by
// List.
type ListArgs struct {
	Since   string
	Verdict string
	Trigger string
	Task    string
	Limit   *int
	Before  string
}

type StampView struct {
	AgentProfileID string `json:"agent_profile_id"`
	Model          string `json:"model"`
	Harness        string `json:"harness"`
	ConfigRevision int64  `json:"config_revision"`
	PolicyRevision int64  `json:"policy_revision"`
	PromptHash     string `json:"prompt_hash"`
	SnapshotHash   string `json:"snapshot_hash"`
}

type CallView struct {
	Action  string  `json:"action"`
	Target  *string `json:"target,omitempty"`
	Allowed bool    `json:"allowed"`
}

type TokensView struct {
	Input  int64 `json:"input"`
	Output int64 `json:"output"`
	Total  int64 `json:"total"`
}

// TurnView is one ledger row as the tool returns it: ids, times, counts and
// digests only, never message text, proposal text or a task title.
type TurnView struct {
	ID             string      `json:"id"`
	Trigger        string      `json:"trigger"`
	WakeKinds      []string    `json:"wake_kinds"`
	StartedAt      time.Time   `json:"started_at"`
	FinishedAt     *time.Time  `json:"finished_at"`
	Outcome        *string     `json:"outcome"`
	Verdict        *string     `json:"verdict"`
	Stamp          StampView   `json:"stamp"`
	ProposalCount  int         `json:"proposal_count"`
	ProposalIDs    []string    `json:"proposal_ids"`
	Calls          []CallView  `json:"calls"`
	CallsTruncated bool        `json:"calls_truncated"`
	Tokens         *TokensView `json:"tokens"`
	CostSubcents   *int64      `json:"cost_subcents"`
}

// Page is one page of turns, newest first.
type Page struct {
	Turns      []TurnView `json:"turns"`
	NextBefore *string    `json:"next_before"`
}

type listQuery struct {
	since   time.Time
	verdict string
	trigger string
	task    string
	limit   int
	before  *cursor
}

type cursor struct {
	startedAt time.Time
	id        string
}

func fieldErr(field, msg string) error { return &coordinator.FieldError{Field: field, Message: msg} }

func encodeCursor(startedAt time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(startedAt.UTC().Format(time.RFC3339Nano) + "|" + id))
}

func decodeCursor(raw string) (*cursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, fieldErr("before", "before is not a valid cursor")
	}
	at, id, ok := strings.Cut(string(b), "|")
	if !ok || id == "" {
		return nil, fieldErr("before", "before is not a valid cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return nil, fieldErr("before", "before is not a valid cursor")
	}
	return &cursor{startedAt: t.UTC(), id: id}, nil
}

var (
	validVerdicts  = []string{VerdictBlocked, VerdictActed, VerdictProposed, VerdictNeedsYou, VerdictNothingNeeded}
	validTriggers  = []string{TriggerMessage, TriggerWake, TriggerDream}
	errSinceFormat = fieldErr("since", "since must be an RFC 3339 time")
)

func (l *Ledger) parseArgs(a ListArgs) (listQuery, error) {
	now := l.now()
	q := listQuery{since: now.Add(-defaultSince), limit: DefaultLimit, verdict: a.Verdict, trigger: a.Trigger, task: a.Task}
	if a.Since != "" {
		t, err := time.Parse(time.RFC3339Nano, a.Since)
		if err != nil {
			return q, errSinceFormat
		}
		if t.Before(now.Add(-maxSince)) {
			return q, fieldErr("since", "since must be at most 400 days ago")
		}
		q.since = t.UTC()
	}
	if a.Limit != nil {
		if *a.Limit < 1 || *a.Limit > MaxLimit {
			return q, fieldErr("limit", fmt.Sprintf("limit must be between 1 and %d", MaxLimit))
		}
		q.limit = *a.Limit
	}
	if a.Verdict != "" && !slices.Contains(validVerdicts, a.Verdict) {
		return q, fieldErr("verdict", "verdict is not a turn verdict")
	}
	if a.Trigger != "" && !slices.Contains(validTriggers, a.Trigger) {
		return q, fieldErr("trigger", "trigger is not a turn trigger")
	}
	if a.Before != "" {
		c, err := decodeCursor(a.Before)
		if err != nil {
			return q, err
		}
		q.before = c
	}
	return q, nil
}

// List returns the coordinator's own turns. The coordinator comes from the
// caller's principal, never from an argument. The page is built wholly in
// memory, so a failure returns ErrUnavailable and nothing partial.
func (l *Ledger) List(ctx context.Context, coordinatorID string, args ListArgs) (*Page, error) {
	q, err := l.parseArgs(args)
	if err != nil {
		return nil, err
	}
	if l.deps.WatchSet == nil {
		return nil, ErrUnavailable
	}
	ws, err := l.deps.WatchSet(ctx, coordinatorID)
	if err != nil {
		l.log.Warn("coordinator ledger: watch set read failed", zap.Error(err))
		return nil, ErrUnavailable
	}
	scope, err := l.taskScopeFor(ctx, coordinatorID, ws)
	if err != nil {
		l.log.Warn("coordinator ledger: workspace read failed", zap.Error(err))
		return nil, ErrUnavailable
	}
	if q.task != "" {
		visible, err := l.taskVisible(ctx, scope, q.task)
		if err != nil {
			l.log.Warn("coordinator ledger: task read failed", zap.Error(err))
			return nil, ErrUnavailable
		}
		if !visible {
			return nil, coordinator.ErrNotFound
		}
	}
	page, err := l.readPage(ctx, coordinatorID, q, scope)
	if err != nil {
		l.log.Warn("coordinator ledger: read failed", zap.Error(err))
		return nil, ErrUnavailable
	}
	return page, nil
}

// taskScope is what a coordinator may see of a task: the task must sit in the
// coordinator's own workspace and in a watched workflow.
type taskScope struct {
	watch       coordinator.WatchSet
	workspaceID string
}

func (t taskScope) allows(workspaceID, workflowID string) bool {
	return workspaceID == t.workspaceID && t.watch.Contains(workflowID)
}

func (l *Ledger) taskScopeFor(ctx context.Context, coordinatorID string, ws coordinator.WatchSet) (taskScope, error) {
	var workspaceID string
	err := l.deps.RO.GetContext(ctx, &workspaceID, l.deps.RO.Rebind(`SELECT workspace_id FROM coordinators WHERE id = ?`), coordinatorID)
	if err != nil {
		return taskScope{}, fmt.Errorf("read coordinator workspace: %w", err)
	}
	return taskScope{watch: ws, workspaceID: workspaceID}, nil
}

func (l *Ledger) taskVisible(ctx context.Context, scope taskScope, taskID string) (bool, error) {
	var row struct {
		WorkspaceID string `db:"workspace_id"`
		WorkflowID  string `db:"workflow_id"`
	}
	err := l.deps.RO.GetContext(ctx, &row, l.deps.RO.Rebind(`SELECT COALESCE(workspace_id, '') AS workspace_id, COALESCE(workflow_id, '') AS workflow_id FROM tasks WHERE id = ?`), taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read task workflow: %w", err)
	}
	return scope.allows(row.WorkspaceID, row.WorkflowID), nil
}

type listedTurn struct {
	ID             string         `db:"id"`
	Trigger        string         `db:"trigger"`
	WakeKinds      string         `db:"wake_kinds"`
	AgentProfileID string         `db:"agent_profile_id"`
	Model          string         `db:"model"`
	Harness        string         `db:"harness"`
	ConfigRevision int64          `db:"config_revision"`
	PolicyRevision int64          `db:"policy_revision"`
	PromptHash     string         `db:"prompt_hash"`
	SnapshotHash   string         `db:"snapshot_hash"`
	SessionID      string         `db:"session_id"`
	SessionTurnID  string         `db:"session_turn_id"`
	StartedAt      time.Time      `db:"started_at"`
	FinishedAt     sql.NullTime   `db:"finished_at"`
	Outcome        sql.NullString `db:"outcome"`
	Verdict        sql.NullString `db:"verdict"`
	CallsTruncated bool           `db:"calls_truncated"`
}

func (l *Ledger) readPage(ctx context.Context, coordinatorID string, q listQuery, scope taskScope) (*Page, error) {
	db := l.deps.RO
	where := `coordinator_id = ? AND started_at >= ?`
	args := []any{coordinatorID, q.since}
	if q.verdict != "" {
		where += ` AND verdict = ?`
		args = append(args, q.verdict)
	}
	if q.trigger != "" {
		where += ` AND "trigger" = ?`
		args = append(args, q.trigger)
	}
	if q.task != "" {
		where += ` AND EXISTS (SELECT 1 FROM coordinator_turn_calls c WHERE c.turn_id = coordinator_turns.id AND c.target_task_id = ?)`
		args = append(args, q.task)
	}
	if q.before != nil {
		where += ` AND (started_at < ? OR (started_at = ? AND id < ?))`
		args = append(args, q.before.startedAt, q.before.startedAt, q.before.id)
	}
	var rows []listedTurn
	if err := db.SelectContext(ctx, &rows, db.Rebind(`
		SELECT id, "trigger", wake_kinds, agent_profile_id, model, harness, config_revision, policy_revision, prompt_hash, snapshot_hash,
		       session_id, session_turn_id, started_at, finished_at, outcome, verdict, calls_truncated
		FROM coordinator_turns WHERE `+where+` ORDER BY started_at DESC, id DESC LIMIT ?`), append(args, q.limit+1)...); err != nil {
		return nil, fmt.Errorf("list ledger turns: %w", err)
	}
	page := &Page{Turns: []TurnView{}}
	if len(rows) > q.limit {
		rows = rows[:q.limit]
		last := rows[len(rows)-1]
		next := encodeCursor(last.StartedAt, last.ID)
		page.NextBefore = &next
	}
	views, err := l.enrich(ctx, db, rows, scope)
	if err != nil {
		return nil, err
	}
	page.Turns = views
	return page, nil
}

func (l *Ledger) enrich(ctx context.Context, db *sqlx.DB, rows []listedTurn, scope taskScope) ([]TurnView, error) {
	if len(rows) == 0 {
		return []TurnView{}, nil
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	proposals, err := proposalIDsByTurn(ctx, db, ids)
	if err != nil {
		return nil, err
	}
	calls, err := l.callsByTurn(ctx, db, ids, scope)
	if err != nil {
		return nil, err
	}
	out := make([]TurnView, 0, len(rows))
	for _, r := range rows {
		v := TurnView{
			ID: r.ID, Trigger: r.Trigger, WakeKinds: decodeKinds(r.WakeKinds), StartedAt: r.StartedAt.UTC(),
			Stamp: StampView{AgentProfileID: r.AgentProfileID, Model: r.Model, Harness: r.Harness, ConfigRevision: r.ConfigRevision,
				PolicyRevision: r.PolicyRevision, PromptHash: r.PromptHash, SnapshotHash: r.SnapshotHash},
			ProposalCount: len(proposals[r.ID]), ProposalIDs: proposals[r.ID][:min(len(proposals[r.ID]), maxProposalIDs)],
			Calls: calls[r.ID], CallsTruncated: r.CallsTruncated,
		}
		if v.WakeKinds == nil {
			v.WakeKinds = []string{}
		}
		if v.Calls == nil {
			v.Calls = []CallView{}
		}
		if r.FinishedAt.Valid {
			t := r.FinishedAt.Time.UTC()
			v.FinishedAt = &t
		}
		if r.Outcome.Valid {
			v.Outcome = &r.Outcome.String
		}
		if r.Verdict.Valid {
			v.Verdict = &r.Verdict.String
		}
		if v.Tokens, v.CostSubcents, err = usageOf(ctx, db, r.SessionID, r.SessionTurnID); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func proposalIDsByTurn(ctx context.Context, db *sqlx.DB, turnIDs []string) (map[string][]string, error) {
	query, args, err := sqlx.In(`SELECT turn_id, id FROM coordinator_proposals WHERE turn_id IN (?) ORDER BY created_at, id`, turnIDs)
	if err != nil {
		return nil, fmt.Errorf("proposal ids: %w", err)
	}
	var rows []struct {
		TurnID string `db:"turn_id"`
		ID     string `db:"id"`
	}
	if err := db.SelectContext(ctx, &rows, db.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("proposal ids: %w", err)
	}
	out := map[string][]string{}
	for _, r := range rows {
		out[r.TurnID] = append(out[r.TurnID], r.ID)
	}
	return out, nil
}

// callsByTurn reads the digests and drops the target of any entry outside the
// watch set.
func (l *Ledger) callsByTurn(ctx context.Context, db *sqlx.DB, turnIDs []string, scope taskScope) (map[string][]CallView, error) {
	query, args, err := sqlx.In(`SELECT turn_id, action, target_task_id, allowed FROM coordinator_turn_calls WHERE turn_id IN (?) ORDER BY id`, turnIDs)
	if err != nil {
		return nil, fmt.Errorf("call digests: %w", err)
	}
	var rows []struct {
		TurnID  string         `db:"turn_id"`
		Action  string         `db:"action"`
		Target  sql.NullString `db:"target_task_id"`
		Allowed bool           `db:"allowed"`
	}
	if err := db.SelectContext(ctx, &rows, db.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("call digests: %w", err)
	}
	var targets []string
	for _, r := range rows {
		if r.Target.Valid && r.Target.String != "" && !slices.Contains(targets, r.Target.String) {
			targets = append(targets, r.Target.String)
		}
	}
	visible, err := visibleTasks(ctx, db, targets, scope)
	if err != nil {
		return nil, err
	}
	out := map[string][]CallView{}
	for _, r := range rows {
		c := CallView{Action: r.Action, Allowed: r.Allowed}
		if r.Target.Valid && visible[r.Target.String] {
			t := r.Target.String
			c.Target = &t
		}
		out[r.TurnID] = append(out[r.TurnID], c)
	}
	return out, nil
}

func visibleTasks(ctx context.Context, db *sqlx.DB, taskIDs []string, scope taskScope) (map[string]bool, error) {
	visible := map[string]bool{}
	if len(taskIDs) == 0 {
		return visible, nil
	}
	query, args, err := sqlx.In(`SELECT id, COALESCE(workspace_id, '') AS workspace_id, COALESCE(workflow_id, '') AS workflow_id FROM tasks WHERE id IN (?)`, taskIDs)
	if err != nil {
		return nil, fmt.Errorf("digest targets: %w", err)
	}
	var rows []struct {
		ID          string `db:"id"`
		WorkspaceID string `db:"workspace_id"`
		WorkflowID  string `db:"workflow_id"`
	}
	if err := db.SelectContext(ctx, &rows, db.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("digest targets: %w", err)
	}
	for _, r := range rows {
		visible[r.ID] = scope.allows(r.WorkspaceID, r.WorkflowID)
	}
	return visible, nil
}

// usageOf joins tokens and cost from the usage ledger by session turn. Cost is
// nil, never zero, when the turn has no usage rows or any row is unpriced.
func usageOf(ctx context.Context, db *sqlx.DB, sessionID, sessionTurnID string) (*TokensView, *int64, error) {
	var u struct {
		Rows     int64 `db:"n"`
		Unpriced int64 `db:"unpriced"`
		In       int64 `db:"tin"`
		Out      int64 `db:"tout"`
		Total    int64 `db:"ttotal"`
		Cost     int64 `db:"cost"`
	}
	if err := db.GetContext(ctx, &u, db.Rebind(`
		SELECT COUNT(*) AS n,
		       COALESCE(SUM(CASE WHEN cost_source = 'unpriced' THEN 1 ELSE 0 END), 0) AS unpriced,
		       COALESCE(SUM(tokens_in), 0) AS tin, COALESCE(SUM(COALESCE(tokens_out, 0)), 0) AS tout,
		       COALESCE(SUM(tokens_total), 0) AS ttotal, COALESCE(SUM(cost_subcents), 0) AS cost
		FROM task_usage_events WHERE session_id = ? AND turn_id = ?`), sessionID, sessionTurnID); err != nil {
		return nil, nil, fmt.Errorf("turn usage: %w", err)
	}
	if u.Rows == 0 {
		return nil, nil, nil
	}
	tokens := &TokensView{Input: u.In, Output: u.Out, Total: u.Total}
	if u.Unpriced > 0 {
		return tokens, nil, nil
	}
	return tokens, &u.Cost, nil
}
