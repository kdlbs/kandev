package sqlite

import (
	"context"
	"fmt"
	"time"
)

// ControlTask is a scoped projection of recorded state, not an inference signal.
type ControlTask struct {
	ID              string `json:"task_id" db:"id"`
	WorkspaceID     string `json:"workspace_id" db:"workspace_id"`
	Title           string `json:"title" db:"title"`
	State           string `json:"state" db:"state"`
	StepID          string `json:"step_id" db:"step_id"`
	StepName        string `json:"step_name" db:"step_name"`
	StageType       string `json:"stage_type" db:"stage_type"`
	SessionID       string `json:"session_id" db:"session_id"`
	SessionState    string `json:"session_state" db:"session_state"`
	Archived        bool   `json:"archived" db:"archived"`
	RunningSessions int    `json:"running_sessions" db:"running_sessions"`
	QueuedMessages  int    `json:"queued_messages" db:"queued_messages"`
	// Nil means the queue policy source could not be read, not that auto-run is off.
	QueuedPausedMessages  *int      `json:"queued_paused_messages,omitempty"`
	QueuedAutoRunMessages *int      `json:"queued_auto_run_messages,omitempty"`
	HoldMode              *string   `json:"hold_mode,omitempty"`
	UpdatedAt             time.Time `json:"updated_at" db:"updated_at"`
}

// ControlTaskList is the recorded per-task projection behind view=control.
type ControlTaskList struct {
	Kind        string        `json:"kind"`
	Filter      string        `json:"filter"`
	Archive     string        `json:"archive"`
	Total       int           `json:"total"`
	Truncated   bool          `json:"truncated"`
	GeneratedAt time.Time     `json:"generated_at"`
	Tasks       []ControlTask `json:"tasks"`
}

const controlRunning = `(SELECT COUNT(*) FROM task_sessions s WHERE s.task_id=t.id AND s.state='RUNNING')`
const controlQueued = `(SELECT COUNT(*) FROM queued_messages q WHERE q.task_id=t.id)`

func controlTaskPredicate(filter, archive string, now time.Time) (string, []any, error) {
	where := controlTaskScope
	switch archive {
	case "active":
		where += " AND t.archived_at IS NULL"
	case "archived":
		where += " AND t.archived_at IS NOT NULL"
	case "all":
	default:
		return "", nil, fmt.Errorf("invalid archive selection")
	}
	condition, args, err := controlTaskFilter(filter, now)
	if err != nil {
		return "", nil, err
	}
	return where + " AND (" + condition + ")", args, nil
}

func controlTaskFilter(filter string, now time.Time) (string, []any, error) {
	switch filter {
	case "all":
		return "1=1", nil, nil
	case "active":
		return "t.state IN ('IN_PROGRESS','REVIEW','SCHEDULING')", nil, nil
	case "sessions":
		return controlRunning + ">0", nil, nil
	case "queued":
		return controlQueued + ">0", nil, nil
	case "hold":
		return "(t.state='BLOCKED' OR COALESCE(w.stage_type,'')='hold')", nil, nil
	case "problems":
		return "(t.state='FAILED' OR EXISTS (SELECT 1 FROM task_sessions s WHERE s.id=" + controlPrimarySession + " AND s.state='FAILED'))", nil, nil
	case "completed":
		return `EXISTS (SELECT 1 FROM task_step_transitions x JOIN workflow_steps dest ON dest.id=x.to_workflow_step_id WHERE x.task_id=t.id AND dest.complete_task_on_enter=1 AND x.occurred_at>=? AND x.occurred_at<=?)`, []any{now.Add(-24 * time.Hour), now}, nil
	default:
		return "", nil, fmt.Errorf("invalid task filter")
	}
}

// ListControlTasks counts matches before clipping. Both reads use the existing
// read pool; generated_at anchors the time window, not an atomic DB snapshot.
func (r *Repository) ListControlTasks(ctx context.Context, wsID, filter, archive string, limit int, now time.Time) (*ControlTaskList, error) {
	where, extra, err := controlTaskPredicate(filter, archive, now)
	if err != nil {
		return nil, err
	}
	args := append([]any{wsID}, extra...)
	out := &ControlTaskList{Kind: "tasks", Filter: filter, Archive: archive, GeneratedAt: now, Tasks: []ControlTask{}}
	if err = r.ro.GetContext(ctx, &out.Total, r.ro.Rebind("SELECT COUNT(*)"+controlTaskFrom+"WHERE "+where), args...); err != nil {
		return nil, err
	}
	query := `SELECT t.id,t.workspace_id,t.title,COALESCE(t.state,'') AS state,COALESCE(w.id,'') AS step_id,COALESCE(w.name,'') AS step_name,COALESCE(w.stage_type,'') AS stage_type,
	 COALESCE(` + controlPrimarySession + `, '') AS session_id,
	 COALESCE((SELECT s.state FROM task_sessions s WHERE s.id=` + controlPrimarySession + `),'') AS session_state,
	 (t.archived_at IS NOT NULL) AS archived,` + controlRunning + ` AS running_sessions,` + controlQueued + ` AS queued_messages,t.updated_at` + controlTaskFrom + `WHERE ` + where + ` ORDER BY t.updated_at DESC,t.id DESC`
	if limit != -1 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	if err = r.ro.SelectContext(ctx, &out.Tasks, r.ro.Rebind(query), args...); err != nil {
		return nil, err
	}
	out.Truncated = out.Total > len(out.Tasks)
	if err = r.appendControlQueuePolicies(ctx, out, wsID); err != nil {
		return nil, err
	}
	if err = r.appendControlHolds(ctx, out, wsID); err != nil {
		return nil, err
	}
	return out, nil
}

// appendControlHolds mirrors the existing GetActiveHoldForMember read, scoped to
// the task workspace. A task with no active hold records an empty mode; a query
// failure surfaces as an error rather than a fabricated empty hold set.
func (r *Repository) appendControlHolds(ctx context.Context, out *ControlTaskList, wsID string) error {
	rows := []struct {
		TaskID string `db:"task_id"`
		Mode   string `db:"mode"`
	}{}
	query := `SELECT m.task_id,h.mode FROM office_task_tree_holds h
	 JOIN office_task_tree_hold_members m ON m.hold_id=h.id JOIN tasks t ON t.id=m.task_id
	 WHERE ` + controlTaskScope + ` AND h.workspace_id=t.workspace_id AND h.released_at IS NULL
	 ORDER BY h.created_at DESC,h.id DESC`
	if err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(query), wsID); err != nil {
		return err
	}
	byTask := make(map[string]string, len(rows))
	for _, row := range rows {
		if _, exists := byTask[row.TaskID]; !exists {
			byTask[row.TaskID] = row.Mode
		}
	}
	for i := range out.Tasks {
		mode := byTask[out.Tasks[i].ID]
		out.Tasks[i].HoldMode = &mode
	}
	return nil
}

// appendControlQueuePolicies reads each queued message's current-incarnation
// session policy, including non-primary sessions. A message whose session or
// matching queue_session_state row cannot be read leaves the task's policy counts
// unknown (nil) rather than defaulting to auto-run. A query failure surfaces as an
// error rather than a fabricated zero.
func (r *Repository) appendControlQueuePolicies(ctx context.Context, out *ControlTaskList, wsID string) error {
	rows := []struct {
		TaskID      string `db:"task_id"`
		Paused      int    `db:"paused"`
		Auto        int    `db:"auto_run"`
		PolicyKnown int    `db:"policy_known"`
	}{}
	query := `SELECT q.task_id,
	 SUM(CASE WHEN p.auto_run=0 THEN 1 ELSE 0 END) AS paused,
	 SUM(CASE WHEN p.auto_run<>0 THEN 1 ELSE 0 END) AS auto_run,
	 MIN(CASE WHEN p.session_id IS NULL THEN 0 ELSE 1 END) AS policy_known
	 FROM queued_messages q JOIN tasks t ON t.id=q.task_id
	 LEFT JOIN task_sessions s ON s.id=q.session_id AND s.task_id=q.task_id
	 LEFT JOIN queue_session_state p ON p.session_id=s.id AND p.session_incarnation_id=s.queue_incarnation_id
	 WHERE ` + controlTaskScope + ` GROUP BY q.task_id`
	if err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(query), wsID); err != nil {
		return err
	}
	byTask := make(map[string][3]int, len(rows))
	for _, row := range rows {
		byTask[row.TaskID] = [3]int{row.Paused, row.Auto, row.PolicyKnown}
	}
	for i := range out.Tasks {
		counts, exists := byTask[out.Tasks[i].ID]
		if exists && counts[2] == 0 {
			continue
		}
		out.Tasks[i].QueuedPausedMessages = &counts[0]
		out.Tasks[i].QueuedAutoRunMessages = &counts[1]
	}
	return nil
}
