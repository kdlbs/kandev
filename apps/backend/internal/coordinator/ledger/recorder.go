package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

const maxWakeKinds = 20

// Subscribe observes turn.started and turn.completed. The returned function
// unsubscribes; the recorder never publishes.
func (l *Ledger) Subscribe(eventBus bus.EventBus) (func(), error) {
	var subs []bus.Subscription
	for _, s := range []struct {
		subject string
		handle  func(context.Context, *bus.Event)
		stage   string
	}{
		{events.TurnStarted, l.OnTurnStarted, StageStart},
		{events.TurnCompleted, l.OnTurnCompleted, StageComplete},
	} {
		handle, stage := s.handle, s.stage
		sub, err := eventBus.Subscribe(s.subject, func(_ context.Context, e *bus.Event) error {
			l.enqueueEvent(turnJob{handle: handle, event: e, stage: stage})
			return nil
		})
		if err != nil {
			for _, done := range subs {
				_ = done.Unsubscribe()
			}
			return nil, fmt.Errorf("ledger subscribe %s: %w", s.subject, err)
		}
		subs = append(subs, sub)
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			for _, sub := range subs {
				_ = sub.Unsubscribe()
			}
		})
	}, nil
}

type turnEvent struct {
	sessionTurnID string
	sessionID     string
	taskID        string
	startedAt     time.Time
	completedAt   time.Time
}

func parseTurnEvent(event *bus.Event) (turnEvent, bool) {
	if event == nil {
		return turnEvent{}, false
	}
	data, ok := event.Data.(map[string]any)
	if !ok {
		return turnEvent{}, false
	}
	t := turnEvent{
		sessionTurnID: stringField(data, "id"),
		sessionID:     stringField(data, "session_id"),
		taskID:        stringField(data, "task_id"),
		startedAt:     timeField(data, "started_at"),
		completedAt:   timeField(data, "completed_at"),
	}
	return t, t.sessionTurnID != "" && t.sessionID != "" && t.taskID != ""
}

func stringField(data map[string]any, key string) string {
	s, _ := data[key].(string)
	return s
}

func timeField(data map[string]any, key string) time.Time {
	switch v := data[key].(type) {
	case time.Time:
		return v.UTC()
	case *time.Time:
		if v != nil {
			return v.UTC()
		}
	case string:
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// OnTurnStarted records the opening of a coordinator conversation turn.
func (l *Ledger) OnTurnStarted(ctx context.Context, event *bus.Event) {
	l.safe(StageStart, func() error {
		te, ok := parseTurnEvent(event)
		if !ok {
			return nil
		}
		coordinatorID, isCoordinator, err := l.deps.CoordinatorForTask(ctx, te.taskID)
		if err != nil {
			return fmt.Errorf("coordinator for task: %w", err)
		}
		if !isCoordinator {
			return nil
		}
		if te.startedAt.IsZero() {
			te.startedAt = l.now()
		}
		return l.start(ctx, coordinatorID, te)
	})
}

type turnRow struct {
	ID            string         `db:"id"`
	CoordinatorID string         `db:"coordinator_id"`
	SessionID     string         `db:"session_id"`
	SessionTurnID string         `db:"session_turn_id"`
	Trigger       string         `db:"trigger"`
	WakeKinds     string         `db:"wake_kinds"`
	StartedAt     time.Time      `db:"started_at"`
	FinishedAt    sql.NullTime   `db:"finished_at"`
	Outcome       sql.NullString `db:"outcome"`
	Truncated     bool           `db:"calls_truncated"`
}

func (l *Ledger) turnBySessionTurn(ctx context.Context, sessionID, sessionTurnID string) (*turnRow, error) {
	var r turnRow
	err := l.deps.DB.GetContext(ctx, &r, l.deps.DB.Rebind(`
		SELECT id, coordinator_id, session_id, session_turn_id, "trigger", wake_kinds, started_at, finished_at, outcome, calls_truncated
		FROM coordinator_turns WHERE session_id = ? AND session_turn_id = ?`), sessionID, sessionTurnID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read ledger turn: %w", err)
	}
	return &r, nil
}

func (l *Ledger) start(ctx context.Context, coordinatorID string, te turnEvent) error {
	if cur, ok := l.activeEntryFor(te.sessionID); ok && cur.sessionTurnID == te.sessionTurnID {
		return nil
	}
	entry := activeEntry{rowID: uuid.NewString(), sessionTurnID: te.sessionTurnID, startedAt: te.startedAt}
	l.setActive(te.sessionID, entry)
	row, snap, err := l.buildRow(ctx, coordinatorID, te, entry.rowID, true)
	if err != nil {
		l.dropActive(te.sessionID, entry.rowID, te.sessionTurnID)
		return err
	}
	created, err := l.insertRow(ctx, row, snap)
	if err != nil && snap != nil {
		countFailure(StageSnapshot)
		l.log.Warn("coordinator ledger: snapshot store failed", zap.Error(err))
		row.snapshotHash = ""
		created, err = l.insertRow(ctx, row, nil)
	}
	if err != nil {
		l.dropActive(te.sessionID, entry.rowID, te.sessionTurnID)
		return err
	}
	if !created {
		return l.reconcileExisting(ctx, te, entry)
	}
	l.linkUnattended(ctx, row.id, row.unattendedID)
	return nil
}

// reconcileExisting repairs the active entry after an insert found its row
// already there.
func (l *Ledger) reconcileExisting(ctx context.Context, te turnEvent, entry activeEntry) error {
	existing, err := l.turnBySessionTurn(ctx, te.sessionID, te.sessionTurnID)
	if err != nil {
		l.dropActive(te.sessionID, entry.rowID, te.sessionTurnID)
		return err
	}
	if existing == nil || existing.FinishedAt.Valid {
		l.dropActive(te.sessionID, entry.rowID, te.sessionTurnID)
		return nil
	}
	l.swapActive(te.sessionID, entry, activeEntry{rowID: existing.ID, sessionTurnID: te.sessionTurnID, startedAt: te.startedAt})
	return nil
}

type newRow struct {
	id, coordinatorID, sessionID, sessionTurnID, trigger string
	wakeKinds                                            []string
	agentProfileID                                       string
	configRevision, policyRevision                       int64
	promptHash, snapshotHash                             string
	watchScope, watchIDs                                 string
	startedAt                                            time.Time
	unattendedID                                         string
}

type coordinatorStamp struct {
	WorkspaceID    string `db:"workspace_id"`
	AgentProfileID string `db:"agent_profile_id"`
	ConfigRevision int64  `db:"config_revision"`
	PolicyRevision int64  `db:"policy_revision"`
}

// buildRow derives the stamp, trigger, watch scope and (for a live start) the
// board snapshot. A failed snapshot leaves its hash empty.
func (l *Ledger) buildRow(ctx context.Context, coordinatorID string, te turnEvent, rowID string, withSnapshot bool) (*newRow, *Snapshot, error) {
	var cs coordinatorStamp
	if err := l.deps.DB.GetContext(ctx, &cs, l.deps.DB.Rebind(
		`SELECT workspace_id, agent_profile_id, config_revision, policy_revision FROM coordinators WHERE id = ?`), coordinatorID); err != nil {
		return nil, nil, fmt.Errorf("read coordinator stamp: %w", err)
	}
	row := &newRow{
		id: rowID, coordinatorID: coordinatorID, sessionID: te.sessionID, sessionTurnID: te.sessionTurnID,
		trigger: TriggerMessage, agentProfileID: cs.AgentProfileID,
		configRevision: cs.ConfigRevision, policyRevision: cs.PolicyRevision, startedAt: te.startedAt,
		watchScope: "all", watchIDs: "[]",
	}
	if l.deps.PromptHash != nil {
		row.promptHash = l.deps.PromptHash(ctx, coordinatorID)
	}
	if l.deps.IsDreamTask != nil {
		dream, err := l.deps.IsDreamTask(ctx, te.taskID)
		if err != nil {
			return nil, nil, fmt.Errorf("dream task check: %w", err)
		}
		if dream {
			row.trigger = TriggerDream
		}
	}
	var snap *Snapshot
	if l.deps.WatchSet != nil {
		ws, err := l.deps.WatchSet(ctx, coordinatorID)
		if err != nil {
			countFailure(StageSnapshot)
			l.log.Warn("coordinator ledger: watch set read failed", zap.Error(err))
		} else {
			row.watchScope, row.watchIDs = encodeWatchSet(ws)
			if withSnapshot {
				snap = l.snapshotOrNil(ctx, coordinatorID, cs.WorkspaceID, ws, te.startedAt)
			}
		}
	}
	if snap != nil {
		row.snapshotHash = snap.Hash
	}
	if err := l.matchWake(ctx, te, row); err != nil {
		return nil, nil, err
	}
	return row, snap, nil
}

func (l *Ledger) snapshotOrNil(ctx context.Context, coordinatorID, workspaceID string, ws coordinator.WatchSet, startedAt time.Time) *Snapshot {
	snap, err := BuildSnapshot(ctx, l.deps.RO, coordinatorID, workspaceID, ws, startedAt)
	if err != nil {
		countFailure(StageSnapshot)
		l.log.Warn("coordinator ledger: snapshot build failed", zap.Error(err))
		return nil
	}
	return snap
}

func encodeWatchSet(ws coordinator.WatchSet) (scope, ids string) {
	if ws.All {
		return "all", "[]"
	}
	list := slices.Clone(ws.WorkflowIDs)
	slices.Sort(list)
	list = slices.Compact(list)
	if list == nil {
		list = []string{}
	}
	raw, _ := json.Marshal(list)
	return "workflows", string(raw)
}

type unattendedMatch struct {
	ID            string         `db:"id"`
	SessionTurnID sql.NullString `db:"session_turn_id"`
	Outcome       sql.NullString `db:"outcome"`
}

// findUnattended is the wake match: the session's unattended row bound to or
// reserved for this session turn, never a send_failed row and never one settled
// while unbound. The newest by (started_at, id) wins.
func (l *Ledger) findUnattended(ctx context.Context, sessionID, sessionTurnID string) (*unattendedMatch, error) {
	var m unattendedMatch
	err := l.deps.DB.GetContext(ctx, &m, l.deps.DB.Rebind(`
		SELECT id, session_turn_id, outcome FROM coordinator_unattended_turns
		WHERE session_id = ? AND (session_turn_id = ? OR reserved_turn_id = ?)
		  AND (outcome IS NULL OR outcome <> 'send_failed')
		  AND (session_turn_id IS NOT NULL OR outcome IS NULL)
		ORDER BY started_at DESC, id DESC LIMIT 1`), sessionID, sessionTurnID, sessionTurnID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("match unattended turn: %w", err)
	}
	return &m, nil
}

func (l *Ledger) matchWake(ctx context.Context, te turnEvent, row *newRow) error {
	m, err := l.findUnattended(ctx, te.sessionID, te.sessionTurnID)
	if err != nil {
		return err
	}
	if m == nil {
		return nil
	}
	kinds, err := l.wakeKinds(ctx, m.ID)
	if err != nil {
		return err
	}
	row.trigger, row.wakeKinds, row.unattendedID = TriggerWake, kinds, m.ID
	return nil
}

func (l *Ledger) wakeKinds(ctx context.Context, unattendedID string) ([]string, error) {
	var kinds []string
	if err := l.deps.DB.SelectContext(ctx, &kinds, l.deps.DB.Rebind(
		`SELECT kind FROM coordinator_wakes WHERE turn_id = ? ORDER BY kind, id LIMIT ?`), unattendedID, maxWakeKinds); err != nil {
		return nil, fmt.Errorf("read wake kinds: %w", err)
	}
	return kinds, nil
}

func encodeKinds(kinds []string) string {
	if kinds == nil {
		kinds = []string{}
	}
	raw, _ := json.Marshal(kinds)
	return string(raw)
}

func decodeKinds(raw string) []string {
	var kinds []string
	if err := json.Unmarshal([]byte(raw), &kinds); err != nil {
		return nil
	}
	return kinds
}

// insertRow stores the snapshot and the row in one transaction. created is
// false when the row already existed.
func (l *Ledger) insertRow(ctx context.Context, r *newRow, snap *Snapshot) (bool, error) {
	db := l.deps.DB
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin ledger insert: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if snap != nil {
		if err := l.lockSnapshots(ctx, tx); err != nil {
			return false, err
		}
		if _, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO coordinator_turn_snapshots (hash, body, created_at) VALUES (?, ?, ?) ON CONFLICT (hash) DO NOTHING`),
			snap.Hash, string(snap.Body), l.now()); err != nil {
			return false, fmt.Errorf("insert snapshot: %w", err)
		}
	}
	res, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO coordinator_turns (id, coordinator_id, session_id, session_turn_id, "trigger", wake_kinds, agent_profile_id,
			model, harness, config_revision, policy_revision, prompt_hash, snapshot_hash, watch_scope, watch_ids, started_at, calls_truncated)
		VALUES (?, ?, ?, ?, ?, ?, ?, '', '', ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (session_id, session_turn_id) DO NOTHING`),
		r.id, r.coordinatorID, r.sessionID, r.sessionTurnID, r.trigger, encodeKinds(r.wakeKinds), r.agentProfileID,
		r.configRevision, r.policyRevision, r.promptHash, r.snapshotHash, r.watchScope, r.watchIDs, r.startedAt.UTC(), false)
	if err != nil {
		return false, fmt.Errorf("insert ledger turn: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("ledger insert rows: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit ledger insert: %w", err)
	}
	return n == 1, nil
}

// linkUnattended sets the unattended row's ledger_turn_id once; a second link
// fails on the unique index and is counted and logged.
func (l *Ledger) linkUnattended(ctx context.Context, ledgerID, unattendedID string) {
	if unattendedID == "" {
		return
	}
	l.safe(StageLink, func() error {
		_, err := l.deps.DB.ExecContext(ctx, l.deps.DB.Rebind(
			`UPDATE coordinator_unattended_turns SET ledger_turn_id = ? WHERE id = ? AND ledger_turn_id IS NULL`), ledgerID, unattendedID)
		return err
	})
}
