package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/workflow/models"
	"sort"
	"time"
)

func (r *Repository) ApplyPromptSources(ctx context.Context, plans []models.PromptSourcePlan) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	workspaces := make([]string, 0, len(plans))
	for _, plan := range plans {
		workspaces = append(workspaces, plan.Record.WorkspaceID)
	}
	if err := db.LockTaskHierarchy(ctx, tx, r.db.DriverName(), tx.Rebind, workspaces, nil); err != nil {
		return err
	}
	if err := r.lockSourceTemplates(ctx, tx, plans); err != nil {
		return err
	}
	for i := range plans {
		if err := r.ensureSourceTemplate(ctx, tx, plans[i].Record.WorkspaceID, plans[i].Template); err != nil {
			return err
		}
		if err := r.fenceSourcePlan(ctx, tx, plans[i]); err != nil {
			return err
		}
		if len(plans[i].ChangedFields) == 0 {
			continue
		}
		now := time.Now().UTC()
		if err := r.writeSourceWorkflow(ctx, tx, &plans[i], now); err != nil {
			return err
		}
		if err := r.writeSourceSteps(ctx, tx, plans[i], now); err != nil {
			return err
		}
		if err := r.writeSourceRecord(ctx, tx, &plans[i], now); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (r *Repository) lockSourceTemplates(ctx context.Context, tx *sqlx.Tx, plans []models.PromptSourcePlan) error {
	if !dialect.IsPostgres(r.db.DriverName()) {
		return nil
	}
	ids := make([]string, 0, len(plans))
	for _, plan := range plans {
		ids = append(ids, plan.Record.WorkspaceID+":"+plan.Template.ID)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('prompt-source-template:' || $1, 0))`, id); err != nil {
			return err
		}
	}
	return nil
}
func canonicalSourceTemplate(template models.PromptSourceTemplate) (string, error) {
	raw, err := json.Marshal(template)
	if err != nil {
		return "", err
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	raw, err = json.Marshal(value)
	return string(raw), err
}
func (r *Repository) ensureSourceTemplate(ctx context.Context, tx *sqlx.Tx, workspaceID string, template models.PromptSourceTemplate) error {
	desired, err := canonicalSourceTemplate(template)
	if err != nil {
		return err
	}
	var existing string
	err = tx.QueryRowContext(ctx, tx.Rebind(`SELECT definition FROM prompt_source_templates WHERE workspace_id = ? AND id = ? AND revision = ?`), workspaceID, template.ID, template.Revision).Scan(&existing)
	if err == nil {
		if existing != desired {
			return fmt.Errorf("%w: template revision is immutable", models.ErrPromptSourceConflict)
		}
		return nil
	}
	if err != sql.ErrNoRows {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO prompt_source_templates (workspace_id, id, revision, definition, created_at) VALUES (?, ?, ?, ?, ?)`), workspaceID, template.ID, template.Revision, desired, time.Now().UTC())
	return err
}
func (r *Repository) fenceSourcePlan(ctx context.Context, tx *sqlx.Tx, plan models.PromptSourcePlan) error {
	if err := r.fenceSourceInstance(ctx, tx, plan); err != nil {
		return err
	}
	var workflowVersion time.Time
	query := `SELECT updated_at FROM workflows WHERE id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += sourceRowForUpdate
	}
	err := tx.QueryRowContext(ctx, tx.Rebind(query), plan.Workflow.ID).Scan(&workflowVersion)
	if plan.ExpectedWorkflow == nil {
		if err != sql.ErrNoRows {
			return models.ErrPromptSourceConflict
		}
		return nil
	}
	if err != nil || !workflowVersion.Equal(plan.ExpectedWorkflow.UpdatedAt) {
		return models.ErrPromptSourceConflict
	}
	var count int
	if err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT COUNT(*) FROM workflow_steps WHERE workflow_id = ?`), plan.Workflow.ID).Scan(&count); err != nil {
		return err
	}
	if count != len(plan.ExistingSteps) {
		return models.ErrPromptSourceConflict
	}
	for _, step := range plan.ExistingSteps {
		var version time.Time
		if err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT updated_at FROM workflow_steps WHERE id = ? AND workflow_id = ?`), step.ID, plan.Workflow.ID).Scan(&version); err != nil || !version.Equal(step.UpdatedAt) {
			return models.ErrPromptSourceConflict
		}
	}
	return nil
}
func (r *Repository) writeSourceRecord(ctx context.Context, tx *sqlx.Tx, plan *models.PromptSourcePlan, now time.Time) error {
	variables, err := json.Marshal(plan.Record.Variables)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO prompt_source_instances (id, template_id, workspace_id, workflow_id, applied_revision, source_locked, variables, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (workspace_id, template_id) DO UPDATE SET applied_revision = excluded.applied_revision, source_locked = excluded.source_locked, variables = excluded.variables, updated_at = excluded.updated_at`), plan.Record.ID, plan.Record.TemplateID, plan.Record.WorkspaceID, plan.Record.WorkflowID, plan.Record.AppliedRevision, dialect.BoolToInt(plan.Record.SourceLocked), string(variables), now)
	if err == nil {
		plan.Record.UpdatedAt = now
	}
	return err
}

func (r *Repository) fenceSourceInstance(ctx context.Context, tx *sqlx.Tx, plan models.PromptSourcePlan) error {
	observed, err := scanSourceRecord(tx.QueryRowContext(ctx, tx.Rebind(`SELECT `+sourceInstanceColumns+` FROM prompt_source_instances WHERE workspace_id = ? AND template_id = ?`), plan.Record.WorkspaceID, plan.Record.TemplateID))
	if plan.ExpectedRecord == nil {
		if err != sql.ErrNoRows {
			return models.ErrPromptSourceConflict
		}
	} else if err != nil || observed.ID != plan.ExpectedRecord.ID || !observed.UpdatedAt.Equal(plan.ExpectedRecord.UpdatedAt) {
		return models.ErrPromptSourceConflict
	}
	return nil
}
