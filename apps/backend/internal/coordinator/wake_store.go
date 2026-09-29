package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// WakeKind names the episode a wake records.
type WakeKind string

// Wake kinds, in the order the backstop reads them for one task.
const (
	WakeKindQuestion   WakeKind = "question"
	WakeKindPermission WakeKind = "permission"
	WakeKindStall      WakeKind = "stall"
	WakeKindError      WakeKind = "error"
	WakeKindCompleted  WakeKind = "completed"
)

var wakeKindOrder = []WakeKind{WakeKindQuestion, WakeKindPermission, WakeKindStall, WakeKindError, WakeKindCompleted}

func (k WakeKind) valid() bool {
	for _, known := range wakeKindOrder {
		if k == known {
			return true
		}
	}
	return false
}

// WakeOutcome is the result of one RecordWake.
type WakeOutcome string

// Wake outcomes.
const (
	WakeInserted    WakeOutcome = "inserted"
	WakeExists      WakeOutcome = "exists"
	WakeCapped      WakeOutcome = "capped"
	WakeAutonomyOff WakeOutcome = "autonomy_off"
	WakeNotOwn      WakeOutcome = "not_own"
)

const (
	wakeStatusPending = "pending"
	// pendingWakeCap is the most pending wakes one coordinator holds.
	pendingWakeCap = 200
	// existingWakeKeysChunk bounds the bound parameters of one ExistingWakeKeys query.
	existingWakeKeysChunk = 400
)

// RecordWakeResult is the outcome of a RecordWake and the workspace of the
// coordinator row read under the lock.
type RecordWakeResult struct {
	Outcome     WakeOutcome
	WorkspaceID string
}

// OwnTask is a task a coordinator created through an approved create_task
// proposal.
type OwnTask struct {
	TaskID      string
	WorkspaceID string
	WorkflowID  string
}

// WakeKey identifies one stored wake episode.
type WakeKey struct {
	TaskID     string
	Kind       WakeKind
	EpisodeKey string
}

// ownTaskPredicates selects the tasks a coordinator owns. p is the approved
// create_task proposal, t its task and c the coordinator. The kind predicate
// matters because message, move and resume proposals also record task_id.
const ownTaskPredicates = `p.kind = 'create_task' AND p.status = 'approved' AND p.task_id IS NOT NULL
	AND t.archived_at IS NULL AND t.is_ephemeral = 0
	AND (c.conversation_task_id IS NULL OR t.id <> c.conversation_task_id)`

const ownTaskJoins = `FROM coordinator_proposals p
	JOIN tasks t ON t.id = p.task_id
	JOIN coordinators c ON c.id = p.coordinator_id`

// ListOwnTasks returns the coordinator's own tasks ordered by task id.
func (s *Store) ListOwnTasks(ctx context.Context, coordinatorID string) ([]OwnTask, error) {
	rows, err := s.ro.QueryContext(ctx, s.ro.Rebind(`
		SELECT DISTINCT t.id, t.workspace_id, COALESCE(t.workflow_id, '') `+ownTaskJoins+`
		WHERE p.coordinator_id = ? AND `+ownTaskPredicates+`
		ORDER BY t.id`), coordinatorID)
	if err != nil {
		return nil, fmt.Errorf("list own tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []OwnTask{}
	for rows.Next() {
		var o OwnTask
		if err := rows.Scan(&o.TaskID, &o.WorkspaceID, &o.WorkflowID); err != nil {
			return nil, fmt.Errorf("scan own task: %w", err)
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list own tasks: %w", err)
	}
	return out, nil
}

// CoordinatorsOwningTask returns the ids of the autonomous coordinators that
// own taskID, ordered by coordinator id.
func (s *Store) CoordinatorsOwningTask(ctx context.Context, taskID string) ([]string, error) {
	rows, err := s.ro.QueryContext(ctx, s.ro.Rebind(`
		SELECT DISTINCT c.id `+ownTaskJoins+`
		WHERE p.task_id = ? AND c.autonomy_enabled = 1 AND `+ownTaskPredicates+`
		ORDER BY c.id`), taskID)
	if err != nil {
		return nil, fmt.Errorf("list coordinators owning task: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan owning coordinator: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list coordinators owning task: %w", err)
	}
	return out, nil
}

// ExistingWakeKeys returns the (task, kind, episode key) of every stored wake
// of the coordinator, in any status, for the given task ids.
func (s *Store) ExistingWakeKeys(ctx context.Context, coordinatorID string, taskIDs []string) (map[WakeKey]struct{}, error) {
	out := map[WakeKey]struct{}{}
	for start := 0; start < len(taskIDs); start += existingWakeKeysChunk {
		chunk := taskIDs[start:min(start+existingWakeKeysChunk, len(taskIDs))]
		args := make([]any, 0, len(chunk)+1)
		args = append(args, coordinatorID)
		for _, id := range chunk {
			args = append(args, id)
		}
		rows, err := s.ro.QueryContext(ctx, s.ro.Rebind(`
			SELECT task_id, kind, episode_key FROM coordinator_wakes
			WHERE coordinator_id = ? AND task_id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",")+`)`), args...)
		if err != nil {
			return nil, fmt.Errorf("read existing wake keys: %w", err)
		}
		err = scanWakeKeys(rows, out)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func scanWakeKeys(rows *sql.Rows, into map[WakeKey]struct{}) error {
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var k WakeKey
		var kind string
		if err := rows.Scan(&k.TaskID, &kind, &k.EpisodeKey); err != nil {
			return fmt.Errorf("scan wake key: %w", err)
		}
		k.Kind = WakeKind(kind)
		into[k] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read existing wake keys: %w", err)
	}
	return nil
}

// RecordWake stores one pending wake for the episode under the coordinator's
// wake lock. The autonomy read, the own-task check, the existence check, the
// pending count and the insert all run inside that lock, so an autonomy-off
// PATCH that committed first records nothing and concurrent recording never
// passes the pending cap. It returns ErrNotFound when the coordinator row is
// absent, and an error, storing nothing, for an empty id, an unknown kind or
// a blank episode key.
func (s *Store) RecordWake(ctx context.Context, coordinatorID, taskID string, kind WakeKind, episodeKey string) (RecordWakeResult, error) {
	if coordinatorID == "" || taskID == "" || !kind.valid() || strings.TrimSpace(episodeKey) == "" {
		return RecordWakeResult{}, fmt.Errorf("record wake: invalid coordinator, task, kind or episode key")
	}
	var result RecordWakeResult
	err := s.WithWakeLock(ctx, coordinatorID, func(tx coordinatorExec) error {
		var err error
		result, err = s.recordWakeTx(ctx, tx, coordinatorID, taskID, kind, episodeKey)
		return err
	})
	if err != nil {
		return RecordWakeResult{}, err
	}
	return result, nil
}

func (s *Store) recordWakeTx(ctx context.Context, tx coordinatorExec, coordinatorID, taskID string, kind WakeKind, episodeKey string) (RecordWakeResult, error) {
	var workspaceID string
	var autonomy int
	if err := tx.QueryRowContext(ctx, s.db.Rebind(`SELECT workspace_id, autonomy_enabled FROM coordinators WHERE id = ?`), coordinatorID).Scan(&workspaceID, &autonomy); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RecordWakeResult{}, ErrNotFound
		}
		return RecordWakeResult{}, fmt.Errorf("read coordinator for wake: %w", err)
	}
	result := RecordWakeResult{WorkspaceID: workspaceID}
	if autonomy != 1 {
		result.Outcome = WakeAutonomyOff
		return result, nil
	}
	var owned int
	err := tx.QueryRowContext(ctx, s.db.Rebind(`SELECT 1 `+ownTaskJoins+`
		WHERE p.coordinator_id = ? AND p.task_id = ? AND `+ownTaskPredicates+` LIMIT 1`), coordinatorID, taskID).Scan(&owned)
	if errors.Is(err, sql.ErrNoRows) {
		result.Outcome = WakeNotOwn
		return result, nil
	}
	if err != nil {
		return RecordWakeResult{}, fmt.Errorf("check own task for wake: %w", err)
	}
	var stored int
	err = tx.QueryRowContext(ctx, s.db.Rebind(`SELECT 1 FROM coordinator_wakes
		WHERE coordinator_id = ? AND task_id = ? AND kind = ? AND episode_key = ? LIMIT 1`), coordinatorID, taskID, string(kind), episodeKey).Scan(&stored)
	if err == nil {
		result.Outcome = WakeExists
		return result, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return RecordWakeResult{}, fmt.Errorf("check existing wake: %w", err)
	}
	var pending int
	if err := tx.QueryRowContext(ctx, s.db.Rebind(`SELECT COUNT(*) FROM coordinator_wakes WHERE coordinator_id = ? AND status = ?`), coordinatorID, wakeStatusPending).Scan(&pending); err != nil {
		return RecordWakeResult{}, fmt.Errorf("count pending wakes: %w", err)
	}
	if pending >= pendingWakeCap {
		result.Outcome = WakeCapped
		return result, nil
	}
	now := time.Now().UTC()
	res, err := tx.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO coordinator_wakes (id, coordinator_id, workspace_id, task_id, kind, episode_key, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (coordinator_id, task_id, kind, episode_key) DO NOTHING`),
		uuid.NewString(), coordinatorID, workspaceID, taskID, string(kind), episodeKey, wakeStatusPending, now, now)
	if err != nil {
		return RecordWakeResult{}, fmt.Errorf("insert wake: %w", err)
	}
	inserted, err := matchedRow(res)
	if err != nil {
		return RecordWakeResult{}, err
	}
	if inserted {
		result.Outcome = WakeInserted
	} else {
		result.Outcome = WakeExists
	}
	return result, nil
}
