package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/workflow/models"
)

const sourceInstanceColumns = `id, template_id, workspace_id, workflow_id, applied_revision, source_locked, variables, updated_at`

type sourceRowScanner interface{ Scan(...any) error }

func scanSourceRecord(row sourceRowScanner) (*models.PromptSourceRecord, error) {
	var record models.PromptSourceRecord
	var locked int
	var variables string
	err := row.Scan(&record.ID, &record.TemplateID, &record.WorkspaceID, &record.WorkflowID, &record.AppliedRevision, &locked, &variables, &record.UpdatedAt)
	if err != nil {
		return nil, err
	}
	record.SourceLocked = locked != 0
	if err := json.Unmarshal([]byte(variables), &record.Variables); err != nil {
		return nil, err
	}
	return &record, nil
}
func (r *Repository) GetPromptSourceInstance(ctx context.Context, workspaceID, templateID string) (*models.PromptSourceRecord, error) {
	record, err := scanSourceRecord(r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT `+sourceInstanceColumns+` FROM prompt_source_instances WHERE workspace_id = ? AND template_id = ?`), workspaceID, templateID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return record, err
}
func (r *Repository) ListPromptSourceInstances(ctx context.Context) ([]models.PromptSourceRecord, error) {
	return r.listSourceRecords(ctx, `SELECT `+sourceInstanceColumns+` FROM prompt_source_instances ORDER BY workspace_id, template_id`)
}
func (r *Repository) ListPromptSourceInstancesByWorkspace(ctx context.Context, ids []string) ([]models.PromptSourceRecord, error) {
	if len(ids) == 0 {
		return []models.PromptSourceRecord{}, nil
	}
	query, args, err := sqlx.In(`SELECT `+sourceInstanceColumns+` FROM prompt_source_instances WHERE workspace_id IN (?) ORDER BY workspace_id, template_id`, ids)
	if err != nil {
		return nil, err
	}
	return r.listSourceRecords(ctx, r.ro.Rebind(query), args...)
}
func (r *Repository) listSourceRecords(ctx context.Context, query string, args ...any) ([]models.PromptSourceRecord, error) {
	rows, err := r.ro.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := []models.PromptSourceRecord{}
	for rows.Next() {
		record, err := scanSourceRecord(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *record)
	}
	return result, rows.Err()
}

func (r *Repository) GetPromptSourceTemplate(ctx context.Context, workspaceID, id string, revision int) (models.PromptSourceTemplate, error) {
	var raw string
	var template models.PromptSourceTemplate
	if err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT definition FROM prompt_source_templates WHERE workspace_id = ? AND id = ? AND revision = ?`), workspaceID, id, revision).Scan(&raw); err != nil {
		return template, err
	}
	err := json.Unmarshal([]byte(raw), &template)
	return template, err
}

func (r *Repository) CheckPromptSourceTemplate(ctx context.Context, workspaceID string, template models.PromptSourceTemplate) error {
	var existing string
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT definition FROM prompt_source_templates WHERE workspace_id = ? AND id = ? AND revision = ?`), workspaceID, template.ID, template.Revision).Scan(&existing)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	desired, err := canonicalSourceTemplate(template)
	if err != nil {
		return err
	}
	if existing != desired {
		return models.ErrPromptSourceConflict
	}
	return nil
}

func (r *Repository) GetPromptSourceWorkflowOwner(ctx context.Context, workflowID string) (string, error) {
	var id string
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT id FROM prompt_source_instances WHERE workflow_id = ?`), workflowID).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}
