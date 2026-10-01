package ledger

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/db/dialect"
)

const (
	snapshotTaskCap     = 200
	snapshotProposalCap = 50
)

// Struct fields are declared in sorted JSON key order so the encoding is
// canonical.
type snapshotBody struct {
	Proposals      []snapshotProposal `json:"proposals"`
	ProposalsTotal int                `json:"proposals_total"`
	Tasks          []snapshotTask     `json:"tasks"`
	TasksTotal     int                `json:"tasks_total"`
	Truncated      bool               `json:"truncated,omitempty"`
}

type snapshotTask struct {
	Kinds     []string `json:"kinds"`
	State     string   `json:"state"`
	StepID    string   `json:"step_id"`
	TaskID    string   `json:"task_id"`
	UpdatedAt *string  `json:"updated_at"`
}

type snapshotProposal struct {
	ID           string  `json:"id"`
	Kind         string  `json:"kind"`
	Status       string  `json:"status"`
	TargetTaskID *string `json:"target_task_id"`
}

// Snapshot is the frozen board a turn started against: canonical JSON and its
// SHA-256.
type Snapshot struct {
	Hash string
	Body []byte
}

// BuildSnapshot reads the coordinator's board server-side. It holds ids,
// states, kinds and times only, never a title or text. Proposals created at or
// after startedAt are left out.
func BuildSnapshot(ctx context.Context, db *sqlx.DB, coordinatorID, workspaceID string, ws coordinator.WatchSet, startedAt time.Time) (*Snapshot, error) {
	tasks, total, err := snapshotTasks(ctx, db, workspaceID, ws)
	if err != nil {
		return nil, err
	}
	proposals, ptotal, err := snapshotProposals(ctx, db, coordinatorID, startedAt)
	if err != nil {
		return nil, err
	}
	kinds, err := pendingKindsByTask(ctx, db, coordinatorID, startedAt, tasks)
	if err != nil {
		return nil, err
	}
	body := snapshotBody{Tasks: tasks, TasksTotal: total, Proposals: proposals, ProposalsTotal: ptotal,
		Truncated: total > len(tasks) || ptotal > len(proposals)}
	for i := range body.Tasks {
		k := kinds[body.Tasks[i].TaskID]
		slices.Sort(k)
		if k == nil {
			k = []string{}
		}
		body.Tasks[i].Kinds = k
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode snapshot: %w", err)
	}
	sum := sha256.Sum256(raw)
	return &Snapshot{Hash: hex.EncodeToString(sum[:]), Body: raw}, nil
}

func snapshotTasks(ctx context.Context, db *sqlx.DB, workspaceID string, ws coordinator.WatchSet) ([]snapshotTask, int, error) {
	if !ws.All && len(ws.WorkflowIDs) == 0 {
		return []snapshotTask{}, 0, nil
	}
	where := `t.workspace_id = ? AND t.archived_at IS NULL AND NOT EXISTS (
		SELECT 1 FROM workflow_steps s WHERE s.id = t.workflow_step_id AND s.complete_task_on_enter = ?)`
	args := []any{workspaceID, dialect.BoolToInt(true)}
	if !ws.All {
		ids := slices.Clone(ws.WorkflowIDs)
		slices.Sort(ids)
		clause, inArgs, err := sqlx.In(" AND t.workflow_id IN (?)", ids)
		if err != nil {
			return nil, 0, fmt.Errorf("snapshot watch set: %w", err)
		}
		where += clause
		args = append(args, inArgs...)
	}
	var total int
	if err := db.GetContext(ctx, &total, db.Rebind(`SELECT COUNT(*) FROM tasks t WHERE `+where), args...); err != nil {
		return nil, 0, fmt.Errorf("snapshot task count: %w", err)
	}
	var rows []struct {
		ID        string       `db:"id"`
		StepID    string       `db:"workflow_step_id"`
		State     string       `db:"state"`
		UpdatedAt sql.NullTime `db:"updated_at"`
	}
	q := `SELECT t.id, COALESCE(t.workflow_step_id, '') AS workflow_step_id, t.state, t.updated_at FROM tasks t WHERE ` + where +
		fmt.Sprintf(` ORDER BY CASE WHEN t.updated_at IS NULL THEN 1 ELSE 0 END, t.updated_at DESC, t.id LIMIT %d`, snapshotTaskCap)
	if err := db.SelectContext(ctx, &rows, db.Rebind(q), args...); err != nil {
		return nil, 0, fmt.Errorf("snapshot tasks: %w", err)
	}
	out := make([]snapshotTask, 0, len(rows))
	for _, r := range rows {
		t := snapshotTask{TaskID: r.ID, StepID: r.StepID, State: r.State, Kinds: []string{}}
		if r.UpdatedAt.Valid {
			s := r.UpdatedAt.Time.UTC().Format(time.RFC3339Nano)
			t.UpdatedAt = &s
		}
		out = append(out, t)
	}
	return out, total, nil
}

func snapshotProposals(ctx context.Context, db *sqlx.DB, coordinatorID string, startedAt time.Time) ([]snapshotProposal, int, error) {
	const where = `coordinator_id = ? AND status IN ('pending', 'approving', 'failed') AND created_at < ?`
	var total int
	if err := db.GetContext(ctx, &total, db.Rebind(`SELECT COUNT(*) FROM coordinator_proposals WHERE `+where), coordinatorID, startedAt); err != nil {
		return nil, 0, fmt.Errorf("snapshot proposal count: %w", err)
	}
	var rows []struct {
		ID     string         `db:"id"`
		Kind   string         `db:"kind"`
		Status string         `db:"status"`
		Target sql.NullString `db:"target_task_id"`
	}
	q := `SELECT id, kind, status, target_task_id FROM coordinator_proposals WHERE ` + where +
		fmt.Sprintf(` ORDER BY created_at, id LIMIT %d`, snapshotProposalCap)
	if err := db.SelectContext(ctx, &rows, db.Rebind(q), coordinatorID, startedAt); err != nil {
		return nil, 0, fmt.Errorf("snapshot proposals: %w", err)
	}
	out := make([]snapshotProposal, 0, len(rows))
	for _, r := range rows {
		p := snapshotProposal{ID: r.ID, Kind: r.Kind, Status: r.Status}
		if r.Target.Valid && r.Target.String != "" {
			s := r.Target.String
			p.TargetTaskID = &s
		}
		out = append(out, p)
	}
	return out, total, nil
}

// pendingKindsByTask reads the kinds of the coordinator's pending proposals that
// target each captured task, independent of the capped proposal list.
func pendingKindsByTask(ctx context.Context, db *sqlx.DB, coordinatorID string, startedAt time.Time, tasks []snapshotTask) (map[string][]string, error) {
	kinds := map[string][]string{}
	if len(tasks) == 0 {
		return kinds, nil
	}
	ids := make([]string, len(tasks))
	for i, t := range tasks {
		ids[i] = t.TaskID
	}
	query, args, err := sqlx.In(`SELECT DISTINCT target_task_id, kind FROM coordinator_proposals
		WHERE coordinator_id = ? AND status = 'pending' AND created_at < ? AND target_task_id IN (?)`, coordinatorID, startedAt, ids)
	if err != nil {
		return nil, fmt.Errorf("snapshot kinds: %w", err)
	}
	var rows []struct {
		Target string `db:"target_task_id"`
		Kind   string `db:"kind"`
	}
	if err := db.SelectContext(ctx, &rows, db.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("snapshot kinds: %w", err)
	}
	for _, r := range rows {
		kinds[r.Target] = append(kinds[r.Target], r.Kind)
	}
	return kinds, nil
}
