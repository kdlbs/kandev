package sqlite

import (
	"context"
	"time"
)

const controlTaskFrom = ` FROM tasks t LEFT JOIN workflow_steps w ON w.id=t.workflow_step_id `
const controlTaskVisibility = `t.is_ephemeral=0 AND ` + notAutomationOriginT
const controlTaskScope = `t.workspace_id=? AND ` + controlTaskVisibility
const controlPrimarySession = `(SELECT s.id FROM task_sessions s WHERE s.task_id=t.id AND s.is_primary=1 ORDER BY s.started_at DESC,s.id DESC LIMIT 1)`

// controlLiveTask keeps finished tasks out of figures that describe pending work.
const controlLiveTask = `COALESCE(t.state,'') NOT IN ('COMPLETED','CANCELLED')`

// ControlTaskStage separates workflow classification from recorded task state.
// Custom/unclassified steps keep their identity without guessing from their names.
type ControlTaskStage struct {
	State     string `json:"state" db:"state"`
	StepID    string `json:"step_id" db:"step_id"`
	StepName  string `json:"step_name" db:"step_name"`
	StageType string `json:"stage_type" db:"stage_type"`
	Count     int    `json:"count" db:"count"`
}

// ControlMetrics reports recorded state. Sessions counts every session per state;
// WaitingInputTasks counts unfinished tasks whose primary session waits for input.
type ControlMetrics struct {
	WorkspaceID       string             `json:"workspace_id"`
	GeneratedAt       time.Time          `json:"generated_at"`
	Since             time.Time          `json:"since"`
	Basis             string             `json:"basis"`
	InferenceState    string             `json:"inference_state"`
	Tasks             map[string]int     `json:"tasks"`
	TaskStages        []ControlTaskStage `json:"task_stages,omitempty"`
	Sessions          map[string]int     `json:"sessions"`
	WaitingInputTasks int                `json:"waiting_input_tasks"`
	QueuedMessages    int                `json:"queued_messages"`
	Transitions24h    int                `json:"transitions_24h"`
	CompletedTasks24h int                `json:"completed_tasks_24h"`
}

func (r *Repository) ReadControlMetrics(ctx context.Context, wsID string, now time.Time) (*ControlMetrics, error) {
	out := &ControlMetrics{WorkspaceID: wsID, GeneratedAt: now, Since: now.Add(-24 * time.Hour), Basis: "recorded_task_session_queue_transition_state", InferenceState: "unknown"}
	var err error
	out.Tasks, err = r.controlStateCounts(ctx, "SELECT COALESCE(t.state,'') AS state,COUNT(*) AS count FROM tasks t WHERE "+controlTaskScope+" AND t.archived_at IS NULL GROUP BY t.state", wsID)
	if err != nil {
		return nil, err
	}
	stages := []ControlTaskStage{}
	stageQuery := `SELECT COALESCE(t.state,'') AS state,COALESCE(w.id,'') AS step_id,
	 COALESCE(w.name,'') AS step_name,COALESCE(w.stage_type,'') AS stage_type,COUNT(*) AS count` +
		controlTaskFrom + `WHERE ` + controlTaskScope + ` AND t.archived_at IS NULL
	 GROUP BY t.state,w.id,w.name,w.stage_type ORDER BY w.id,t.state`
	if stageErr := r.ro.SelectContext(ctx, &stages, r.ro.Rebind(stageQuery), wsID); stageErr == nil {
		out.TaskStages = stages
	} else if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	out.Sessions, err = r.controlStateCounts(ctx, "SELECT s.state,COUNT(*) AS count FROM task_sessions s JOIN tasks t ON t.id=s.task_id WHERE "+controlTaskScope+" AND t.archived_at IS NULL GROUP BY s.state", wsID)
	if err != nil {
		return nil, err
	}
	queries := []struct {
		dest *int
		sql  string
		args []any
	}{
		{&out.WaitingInputTasks, "SELECT COUNT(*) FROM tasks t WHERE " + controlTaskScope + " AND t.archived_at IS NULL AND " + controlLiveTask + " AND EXISTS (SELECT 1 FROM task_sessions p WHERE p.id=" + controlPrimarySession + " AND p.state='WAITING_FOR_INPUT')", []any{wsID}},
		{&out.QueuedMessages, "SELECT COUNT(*) FROM queued_messages q JOIN tasks t ON t.id=q.task_id WHERE " + controlTaskScope + " AND t.archived_at IS NULL", []any{wsID}},
		{&out.Transitions24h, "SELECT COUNT(*) FROM task_step_transitions x JOIN tasks t ON t.id=x.task_id WHERE " + controlTaskScope + " AND x.occurred_at>=? AND x.occurred_at<=?", []any{wsID, out.Since, now}},
		{&out.CompletedTasks24h, "SELECT COUNT(DISTINCT x.task_id) FROM task_step_transitions x JOIN tasks t ON t.id=x.task_id JOIN workflow_steps w ON w.id=x.to_workflow_step_id WHERE " + controlTaskScope + " AND w.complete_task_on_enter=1 AND x.occurred_at>=? AND x.occurred_at<=?", []any{wsID, out.Since, now}},
	}
	for _, q := range queries {
		if err = r.ro.GetContext(ctx, q.dest, r.ro.Rebind(q.sql), q.args...); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (r *Repository) controlStateCounts(ctx context.Context, query, wsID string) (map[string]int, error) {
	rows := []struct {
		State string `db:"state"`
		Count int    `db:"count"`
	}{}
	if err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(query), wsID); err != nil {
		return nil, err
	}
	out := make(map[string]int, len(rows))
	for _, row := range rows {
		out[row.State] = row.Count
	}
	return out, nil
}
