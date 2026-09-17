package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/kandev/kandev/internal/db/dialect"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func (r *Repository) migrateObjectives() error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS orchestration_objectives (
		id TEXT PRIMARY KEY,binding_id TEXT NOT NULL REFERENCES orchestration_assistant_bindings(id) ON DELETE CASCADE,
		workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
		source_comment_id TEXT NOT NULL REFERENCES task_comments(id) ON DELETE CASCADE,
		title TEXT NOT NULL, mode TEXT NOT NULL, status TEXT NOT NULL, revision INTEGER NOT NULL,
		acceptance_revision INTEGER NOT NULL,intent_revision INTEGER NOT NULL,
		acceptance_json TEXT NOT NULL,evidence_json TEXT NOT NULL,created_at TIMESTAMP NOT NULL,updated_at TIMESTAMP NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_orchestration_objectives_binding ON orchestration_objectives(binding_id,id)`,
		`CREATE TABLE IF NOT EXISTS orchestration_objective_tasks (
		objective_id TEXT NOT NULL REFERENCES orchestration_objectives(id) ON DELETE CASCADE,
		task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,session_id TEXT NOT NULL DEFAULT '',
		role TEXT NOT NULL,context_ref TEXT NOT NULL DEFAULT '',operation_id TEXT NOT NULL,
		PRIMARY KEY(objective_id,task_id,session_id,role))`,
	} {
		if _, err := r.db.Exec(dialect.MustRenderSchema(r.db.DriverName(), q)); err != nil {
			return err
		}
	}
	return nil
}

func objectiveJSON(o *models.Objective) error {
	if err := o.Validate(); err != nil {
		return err
	}
	a, err := json.Marshal(o.Acceptance)
	if err != nil {
		return err
	}
	e, err := json.Marshal(o.Evidence)
	if err != nil {
		return err
	}
	if len(a)+len(e) > 24000 {
		return fmt.Errorf("objective evidence budget exceeded")
	}
	o.AcceptanceJSON, o.EvidenceJSON = string(a), string(e)
	return nil
}

func decodeObjective(o *models.Objective) error {
	if err := json.Unmarshal([]byte(o.AcceptanceJSON), &o.Acceptance); err != nil {
		return err
	}
	return json.Unmarshal([]byte(o.EvidenceJSON), &o.Evidence)
}

func (r *Repository) CreateObjective(ctx context.Context, o *models.Objective) error {
	if err := objectiveJSON(o); err != nil {
		return err
	}
	o.ID, o.Revision, o.AcceptanceRevision = uuid.NewString(), 1, 1
	o.CreatedAt, o.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	_, err := r.db.NamedExecContext(ctx, `INSERT INTO orchestration_objectives
	(id,binding_id,workspace_id,source_comment_id,title,mode,status,revision,acceptance_revision,intent_revision,acceptance_json,evidence_json,created_at,updated_at)
	VALUES(:id,:binding_id,:workspace_id,:source_comment_id,:title,:mode,:status,:revision,:acceptance_revision,:intent_revision,:acceptance_json,:evidence_json,:created_at,:updated_at)`, o)
	return err
}

func (r *Repository) Objective(ctx context.Context, binding, id string) (*models.Objective, error) {
	var row models.Objective
	if err := r.db.GetContext(ctx, &row, r.db.Rebind(`SELECT * FROM orchestration_objectives WHERE binding_id=? AND id=?`), binding, id); err != nil {
		return nil, err
	}
	return &row, decodeObjective(&row)
}

func (r *Repository) Objectives(ctx context.Context, binding, after string, limit int) ([]*models.Objective, error) {
	rows := []*models.Objective{}
	if err := r.db.SelectContext(ctx, &rows, r.db.Rebind(`SELECT * FROM orchestration_objectives WHERE binding_id=? AND id>? ORDER BY id LIMIT ?`), binding, after, limit); err != nil {
		return nil, err
	}
	for _, row := range rows {
		if err := decodeObjective(row); err != nil {
			return nil, err
		}
	}
	return rows, nil
}

func (r *Repository) UpdateObjective(ctx context.Context, o *models.Objective, expected int64) error {
	if err := objectiveJSON(o); err != nil {
		return err
	}
	o.UpdatedAt = time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE orchestration_objectives SET title=?,mode=?,status=?,revision=revision+1,
	acceptance_revision=?,intent_revision=?,acceptance_json=?,evidence_json=?,updated_at=? WHERE id=? AND binding_id=? AND revision=?`),
		o.Title, o.Mode, o.Status, o.AcceptanceRevision, o.IntentRevision, o.AcceptanceJSON, o.EvidenceJSON, o.UpdatedAt, o.ID, o.BindingID, expected)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return models.ErrConflict
	}
	o.Revision = expected + 1
	return nil
}

func (r *Repository) LinkObjectiveTask(ctx context.Context, link models.ObjectiveTask) error {
	_, err := r.db.NamedExecContext(ctx, `INSERT INTO orchestration_objective_tasks(objective_id,task_id,session_id,role,context_ref,operation_id)
	VALUES(:objective_id,:task_id,:session_id,:role,:context_ref,:operation_id) ON CONFLICT DO NOTHING`, link)
	return err
}

func (r *Repository) ObjectiveTasks(ctx context.Context, id string) ([]models.ObjectiveTask, error) {
	rows := []models.ObjectiveTask{}
	err := r.db.SelectContext(ctx, &rows, r.db.Rebind(`SELECT * FROM orchestration_objective_tasks WHERE objective_id=? ORDER BY task_id,session_id,role`), id)
	return rows, err
}
