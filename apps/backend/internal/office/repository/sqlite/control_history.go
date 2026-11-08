package sqlite

import (
	"context"
	"sort"
	"time"
)

type ControlEvent struct {
	ID          string    `json:"id" db:"id"`
	Kind        string    `json:"kind" db:"kind"`
	TaskID      string    `json:"task_id,omitempty" db:"task_id"`
	SessionID   string    `json:"session_id,omitempty" db:"session_id"`
	Title       string    `json:"title,omitempty" db:"title"`
	State       string    `json:"state,omitempty" db:"state"`
	OccurredAt  time.Time `json:"occurred_at" db:"occurred_at"`
	SourceTotal int       `json:"-" db:"source_total"`
	Provider    string    `json:"provider,omitempty" db:"provider"`
	Repository  string    `json:"repository,omitempty" db:"repository"`
	Number      int64     `json:"number,omitempty" db:"number"`
}

type ControlCoverage struct {
	State     string `json:"state"`
	Reason    string `json:"reason,omitempty"`
	Total     *int   `json:"total"`
	Returned  int    `json:"returned"`
	Truncated bool   `json:"truncated"`
}

type ControlHistory struct {
	WorkspaceID string                     `json:"workspace_id"`
	GeneratedAt time.Time                  `json:"generated_at"`
	Since       time.Time                  `json:"since"`
	Coverage    string                     `json:"coverage"`
	Truncated   bool                       `json:"truncated"`
	Sources     map[string]ControlCoverage `json:"sources"`
	Events      []ControlEvent             `json:"events"`
}

type controlHistorySource struct{ name, columns, from, predicate, timeColumn string }

func controlHistorySources() []controlHistorySource {
	return []controlHistorySource{
		{"task_creation", `'task:'||t.id AS id,'task_created' AS kind,t.id AS task_id,'' AS session_id,t.title,t.created_at AS occurred_at`, "tasks t", controlTaskScope, "t.created_at"},
		{"task_transition", `'transition:'||CAST(x.id AS TEXT) AS id,CASE WHEN COALESCE(w.complete_task_on_enter,0)=1 THEN 'task_completed' ELSE 'task_moved' END AS kind,t.id AS task_id,COALESCE(x.session_id,'') AS session_id,t.title,x.occurred_at`, "task_step_transitions x JOIN tasks t ON t.id=x.task_id LEFT JOIN workflow_steps w ON w.id=x.to_workflow_step_id", controlTaskScope, "x.occurred_at"},
		{"session_start", `'session:'||s.id AS id,'session_started' AS kind,t.id AS task_id,s.id AS session_id,t.title,s.started_at AS occurred_at`, "task_sessions s JOIN tasks t ON t.id=s.task_id", controlTaskScope, "s.started_at"},
		{"session_finish", `'session-end:'||s.id AS id,'session_finished' AS kind,t.id AS task_id,s.id AS session_id,t.title,s.completed_at AS occurred_at`, "task_sessions s JOIN tasks t ON t.id=s.task_id", controlTaskScope, "s.completed_at"},
		{"office_activity", `'activity:'||a.id AS id,a.action AS kind,CASE WHEN a.target_type='task' THEN a.target_id ELSE '' END AS task_id,COALESCE(a.session_id,'') AS session_id,'' AS title,a.created_at AS occurred_at`, "office_activity_log a", "a.workspace_id=?", "a.created_at"},
		{"automation_run", `'automation:'||r.id AS id,'automation_run_created' AS kind,COALESCE(t.id,'') AS task_id,COALESCE(s.id,'') AS session_id,COALESCE(r.display_title,'') AS title,r.status AS state,r.created_at AS occurred_at`, "automation_runs r JOIN automations a ON a.id=r.automation_id LEFT JOIN tasks t ON t.id=r.task_id AND t.workspace_id=a.workspace_id LEFT JOIN task_sessions s ON s.id=r.session_id AND s.task_id=t.id", "a.workspace_id=?", "r.created_at"},
	}
}

// ReadControlHistory reads existing facts in a fixed inclusive 24-hour window.
// Source limits and unsupported families are explicit; a partial empty list is
// never an assertion that the workspace had no activity of any kind.
func (r *Repository) ReadControlHistory(ctx context.Context, wsID string, now time.Time) (*ControlHistory, error) {
	out := &ControlHistory{WorkspaceID: wsID, GeneratedAt: now, Since: now.Add(-24 * time.Hour), Coverage: "partial", Sources: map[string]ControlCoverage{}, Events: []ControlEvent{}}
	for _, source := range controlHistorySources() {
		rows, err := r.controlHistoryRows(ctx, source, wsID, out.Since, now)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			out.Sources[source.name] = ControlCoverage{State: "unavailable", Reason: "source_read_failed"}
			continue
		}
		total := 0
		if len(rows) > 0 {
			total = rows[0].SourceTotal
		}
		out.Sources[source.name] = ControlCoverage{State: "known", Total: &total, Returned: len(rows), Truncated: total > len(rows)}
		out.Events = append(out.Events, rows...)
	}
	// Merged change requests, server starts and circuit transitions are not
	// projected here; naming them keeps an empty list from reading as "none".
	for _, family := range []string{"github_merge", "plugin_merge", "server_start", "circuit_history"} {
		out.Sources[family] = ControlCoverage{State: "unavailable", Reason: "not_projected"}
	}
	sort.Slice(out.Events, func(i, j int) bool {
		if out.Events[i].OccurredAt.Equal(out.Events[j].OccurredAt) {
			return out.Events[i].ID < out.Events[j].ID
		}
		return out.Events[i].OccurredAt.After(out.Events[j].OccurredAt)
	})
	if len(out.Events) > 250 {
		out.Events = out.Events[:250]
		out.Truncated = true
	}
	return out, nil
}

func (r *Repository) controlHistoryRows(ctx context.Context, source controlHistorySource, wsID string, since, now time.Time) ([]ControlEvent, error) {
	query := "SELECT " + source.columns + ",COUNT(*) OVER() AS source_total FROM " + source.from + " WHERE " + source.predicate +
		" AND " + source.timeColumn + ">=? AND " + source.timeColumn + "<=? ORDER BY " + source.timeColumn + " DESC,id ASC LIMIT 50"
	rows := []ControlEvent{}
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(query), wsID, since, now)
	return rows, err
}
