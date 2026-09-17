package sqlite

import (
	"context"
	"github.com/kandev/kandev/internal/db/dialect"
)

// ExecutionProfileDirectory intentionally omits credentials and environment values.
func (r *Repository) ExecutionProfileDirectory(ctx context.Context, workspaceID string) ([]map[string]string, error) {
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`SELECT id,name,agent_id FROM agent_profiles WHERE COALESCE(role,'')='' AND enabled=? AND deleted_at IS NULL AND (COALESCE(workspace_id,'')='' OR workspace_id=?) ORDER BY name,id`), true, workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := []map[string]string{}
	for rows.Next() {
		var id, name, agentID string
		if err := rows.Scan(&id, &name, &agentID); err != nil {
			return nil, err
		}
		result = append(result, map[string]string{"id": id, "name": name, "agent_id": agentID})
	}
	return result, rows.Err()
}

// OrchestratedTasks provides bounded navigation data, without transcripts.
func (r *Repository) OrchestratedTasks(ctx context.Context, workspaceID, agentID string) ([]OrchestratedTask, error) {
	tasks := []OrchestratedTask{}
	err := r.ro.SelectContext(ctx, &tasks, r.ro.Rebind("SELECT id,title,state FROM tasks WHERE workspace_id=? AND archived_at IS NULL AND "+dialect.JSONExtract(r.ro.DriverName(), "metadata", "orchestration_chief_id")+"=? ORDER BY updated_at DESC,id LIMIT 100"), workspaceID, agentID)
	return tasks, err
}

type OrchestratedTask struct {
	ID    string `json:"id" db:"id"`
	Title string `json:"title" db:"title"`
	State string `json:"state" db:"state"`
}
