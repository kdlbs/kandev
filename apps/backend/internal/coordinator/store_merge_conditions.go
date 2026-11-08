package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
	"github.com/kandev/kandev/internal/db/dialect"
)

const mergeConditionsSQL = `
	CREATE TABLE IF NOT EXISTS coordinator_change_conditions (
		workspace_id TEXT NOT NULL,
		change_key TEXT NOT NULL,
		kind TEXT NOT NULL,
		active INTEGER NOT NULL,
		generation INTEGER NOT NULL,
		head_sha TEXT NOT NULL DEFAULT '',
		baseline INTEGER NOT NULL DEFAULT 0,
		last_observed_at {{timestamp}} NOT NULL,
		PRIMARY KEY (workspace_id, change_key, kind)
	);
`

// migrateMerge applies the additive merge stewardship schema on SQLite and
// PostgreSQL.
func (s *Store) migrateMerge() error {
	if _, err := s.db.Exec(dialect.MustRenderSchema(s.db.DriverName(), mergeConditionsSQL)); err != nil {
		return fmt.Errorf("coordinator merge schema: %w", err)
	}
	if err := s.migrateMergePolicy(); err != nil {
		return err
	}
	return s.migrateMergeProposals()
}

// WakeTarget is one coordinator and the watched task a change wake is for.
type WakeTarget struct {
	CoordinatorID string
	TaskID        string
}

// ChangeObservationInput is one successful read of a change and the watchers
// that a resulting wake is recorded for. Targets are resolved by the caller
// from the coordinators' Watches.
type ChangeObservationInput struct {
	WorkspaceID string
	ChangeKey   string
	Obs         mergestate.Observation
	Targets     []WakeTarget
}

// ChangeWake is the outcome of recording one wake.
type ChangeWake struct {
	Target  WakeTarget
	Kind    WakeKind
	Outcome WakeOutcome
}

// ChangeObservationResult reports each condition's transition and the wakes
// recorded or refused with it.
type ChangeObservationResult struct {
	Transitions map[mergestate.Kind]mergestate.Result
	Wakes       []ChangeWake
}

var mergeConditionKinds = []mergestate.Kind{mergestate.KindRed, mergestate.KindConflict, mergestate.KindThreads}

func changeWakeKind(k mergestate.Kind) WakeKind { return WakeKind("change_" + string(k)) }

// ApplyChangeObservation transitions the change's condition rows and records
// the wake of each new episode for every target, in one transaction that
// holds the wake lock of each target coordinator. A wake the pending cap
// refuses leaves the open episode stored, so the backstop can derive it.
func (s *Store) ApplyChangeObservation(ctx context.Context, in ChangeObservationInput) (ChangeObservationResult, error) {
	targets := append([]WakeTarget(nil), in.Targets...)
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].CoordinatorID != targets[j].CoordinatorID {
			return targets[i].CoordinatorID < targets[j].CoordinatorID
		}
		return targets[i].TaskID < targets[j].TaskID
	})
	res := ChangeObservationResult{Transitions: map[mergestate.Kind]mergestate.Result{}}
	err := s.withMergeTx(ctx, in.ChangeKey, coordinatorIDsOf(targets), func(tx coordinatorExec) error {
		for _, kind := range mergeConditionKinds {
			if err := s.applyConditionTx(ctx, tx, in, targets, kind, &res); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return ChangeObservationResult{}, err
	}
	return res, nil
}

func (s *Store) applyConditionTx(ctx context.Context, tx coordinatorExec, in ChangeObservationInput, targets []WakeTarget, kind mergestate.Kind, res *ChangeObservationResult) error {
	prev, err := s.readConditionTx(ctx, tx, in.WorkspaceID, in.ChangeKey, kind)
	if err != nil {
		return err
	}
	out := mergestate.Transition(prev, kind, in.Obs)
	res.Transitions[kind] = out
	if !out.Write {
		return nil
	}
	if err := s.writeConditionTx(ctx, tx, in.WorkspaceID, in.ChangeKey, kind, out.Row); err != nil {
		return err
	}
	if !out.Wake {
		return nil
	}
	episode := mergestate.EpisodeKey(in.ChangeKey, out.Row.Generation)
	for _, target := range targets {
		r, err := s.recordWakeCore(ctx, tx, target.CoordinatorID, target.TaskID, changeWakeKind(kind), episode, true)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		res.Wakes = append(res.Wakes, ChangeWake{Target: target, Kind: changeWakeKind(kind), Outcome: r.Outcome})
	}
	return nil
}

func coordinatorIDsOf(targets []WakeTarget) []string {
	seen := map[string]struct{}{}
	var ids []string
	for _, t := range targets {
		if _, ok := seen[t.CoordinatorID]; !ok {
			seen[t.CoordinatorID] = struct{}{}
			ids = append(ids, t.CoordinatorID)
		}
	}
	sort.Strings(ids)
	return ids
}

func (s *Store) readConditionTx(ctx context.Context, tx coordinatorExec, workspaceID, changeKey string, kind mergestate.Kind) (*mergestate.Row, error) {
	var row mergestate.Row
	var active, baseline int
	err := tx.QueryRowContext(ctx, s.db.Rebind(`SELECT active, generation, head_sha, baseline, last_observed_at
		FROM coordinator_change_conditions WHERE workspace_id = ? AND change_key = ? AND kind = ?`),
		workspaceID, changeKey, string(kind)).Scan(&active, &row.Generation, &row.HeadSHA, &baseline, &row.ObservedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read change condition: %w", err)
	}
	row.Active, row.Baseline = active == 1, baseline == 1
	return &row, nil
}

func (s *Store) writeConditionTx(ctx context.Context, tx coordinatorExec, workspaceID, changeKey string, kind mergestate.Kind, row mergestate.Row) error {
	_, err := tx.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO coordinator_change_conditions (workspace_id, change_key, kind, active, generation, head_sha, baseline, last_observed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (workspace_id, change_key, kind) DO UPDATE SET
			active = excluded.active, generation = excluded.generation, head_sha = excluded.head_sha,
			baseline = excluded.baseline, last_observed_at = excluded.last_observed_at`),
		workspaceID, changeKey, string(kind), boolInt(row.Active), row.Generation, row.HeadSHA, boolInt(row.Baseline), row.ObservedAt.UTC())
	if err != nil {
		return fmt.Errorf("write change condition: %w", err)
	}
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// OpenEpisode is an active, non-baseline condition row.
type OpenEpisode struct {
	ChangeKey  string
	Kind       mergestate.Kind
	Generation int
	HeadSHA    string
	ObservedAt time.Time
}

// OpenChangeEpisodes lists the change's active episodes that are not
// baselines, ordered by kind.
func (s *Store) OpenChangeEpisodes(ctx context.Context, workspaceID, changeKey string) ([]OpenEpisode, error) {
	rows, err := s.ro.QueryContext(ctx, s.ro.Rebind(`SELECT change_key, kind, generation, head_sha, last_observed_at
		FROM coordinator_change_conditions WHERE workspace_id = ? AND change_key = ? AND active = 1 AND baseline = 0
		ORDER BY kind`), workspaceID, changeKey)
	if err != nil {
		return nil, fmt.Errorf("list open change episodes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []OpenEpisode{}
	for rows.Next() {
		var e OpenEpisode
		var kind string
		if err := rows.Scan(&e.ChangeKey, &kind, &e.Generation, &e.HeadSHA, &e.ObservedAt); err != nil {
			return nil, fmt.Errorf("scan open change episode: %w", err)
		}
		e.Kind = mergestate.Kind(kind)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ChangeEpisodeOpen reports whether the stored episode named by a merge wake's
// episode key is still the change's active, non-baseline episode of that kind.
func (s *Store) ChangeEpisodeOpen(ctx context.Context, workspaceID string, kind WakeKind, episodeKey string) (OpenEpisode, bool, error) {
	changeKey, generation, ok := mergestate.ParseEpisodeKey(episodeKey)
	if !ok || !kind.IsMerge() {
		return OpenEpisode{}, false, nil
	}
	var e OpenEpisode
	var stored int
	err := s.ro.QueryRowContext(ctx, s.ro.Rebind(`SELECT generation, head_sha, last_observed_at
		FROM coordinator_change_conditions
		WHERE workspace_id = ? AND change_key = ? AND kind = ? AND active = 1 AND baseline = 0`),
		workspaceID, changeKey, string(kind)[len("change_"):]).Scan(&stored, &e.HeadSHA, &e.ObservedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return OpenEpisode{}, false, nil
	}
	if err != nil {
		return OpenEpisode{}, false, fmt.Errorf("read change episode: %w", err)
	}
	e.ChangeKey, e.Kind, e.Generation = changeKey, mergestate.Kind(string(kind)[len("change_"):]), stored
	return e, stored == generation, nil
}

// TaskWorkflows returns the workflow of each live, non-ephemeral task of the
// workspace among taskIDs. Tasks absent from the result are archived, deleted
// or in another workspace.
func (s *Store) TaskWorkflows(ctx context.Context, workspaceID string, taskIDs []string) (map[string]string, error) {
	out := map[string]string{}
	for start := 0; start < len(taskIDs); start += existingWakeKeysChunk {
		chunk := taskIDs[start:min(start+existingWakeKeysChunk, len(taskIDs))]
		args := []any{workspaceID}
		for _, id := range chunk {
			args = append(args, id)
		}
		rows, err := s.ro.QueryContext(ctx, s.ro.Rebind(`SELECT id, COALESCE(workflow_id, '') FROM tasks
			WHERE workspace_id = ? AND archived_at IS NULL AND is_ephemeral = 0 AND id IN (`+placeholders(len(chunk))+`)`), args...)
		if err != nil {
			return nil, fmt.Errorf("read task workflows: %w", err)
		}
		if err := scanTaskWorkflows(rows, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func scanTaskWorkflows(rows *sql.Rows, into map[string]string) error {
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, workflow string
		if err := rows.Scan(&id, &workflow); err != nil {
			return fmt.Errorf("scan task workflow: %w", err)
		}
		into[id] = workflow
	}
	return rows.Err()
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// changeHead returns the head SHA stored with the change's condition row for
// the wake kind, or empty when none is stored or the read fails.
func (s *Store) changeHead(ctx context.Context, workspaceID, changeKey string, kind WakeKind) string {
	var head string
	err := s.ro.QueryRowContext(ctx, s.ro.Rebind(`SELECT head_sha FROM coordinator_change_conditions
		WHERE workspace_id = ? AND change_key = ? AND kind = ?`), workspaceID, changeKey, strings.TrimPrefix(string(kind), "change_")).Scan(&head)
	if err != nil {
		return ""
	}
	return head
}

// ListOpenEpisodes lists every active, non-baseline episode in the workspace,
// ordered by change key then kind.
func (s *Store) ListOpenEpisodes(ctx context.Context, workspaceID string) ([]OpenEpisode, error) {
	rows, err := s.ro.QueryContext(ctx, s.ro.Rebind(`SELECT change_key, kind, generation, head_sha, last_observed_at
		FROM coordinator_change_conditions WHERE workspace_id = ? AND active = 1 AND baseline = 0
		ORDER BY change_key, kind`), workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list open episodes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []OpenEpisode{}
	for rows.Next() {
		var e OpenEpisode
		var kind string
		if err := rows.Scan(&e.ChangeKey, &kind, &e.Generation, &e.HeadSHA, &e.ObservedAt); err != nil {
			return nil, fmt.Errorf("scan open episode: %w", err)
		}
		e.Kind = mergestate.Kind(kind)
		out = append(out, e)
	}
	return out, rows.Err()
}
