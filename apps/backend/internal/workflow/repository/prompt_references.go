package repository

import "context"

// PromptReferenceText is one non-empty workflow or workflow-step prompt.
type PromptReferenceText struct {
	WorkflowID string
	Text       string
}

// ListPromptReferenceTexts returns every non-empty workflow prompt and step
// prompt in the installation, across all workspaces. Saved prompts are
// install-wide, so any of these texts can reference one.
func (r *Repository) ListPromptReferenceTexts(ctx context.Context) ([]PromptReferenceText, error) {
	rows, err := r.ro.QueryContext(ctx, `
		SELECT id, prompt FROM workflows WHERE prompt <> ''
		UNION ALL
		SELECT workflow_id, prompt FROM workflow_steps WHERE prompt IS NOT NULL AND prompt <> ''
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var texts []PromptReferenceText
	for rows.Next() {
		var text PromptReferenceText
		if err := rows.Scan(&text.WorkflowID, &text.Text); err != nil {
			return nil, err
		}
		texts = append(texts, text)
	}
	return texts, rows.Err()
}
