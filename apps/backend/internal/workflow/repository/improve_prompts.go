package repository

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	workflowcfg "github.com/kandev/kandev/config/workflows"
	"github.com/kandev/kandev/internal/workflow/models"
)

//go:embed testdata/improve-kandev-before-lifecycle.yml
var improveKandevBeforeLifecycleYAML []byte

const improveKandevTemplateID = "improve-kandev"

type promptRevision struct {
	name     string
	current  string
	previous string
}

type storedImproveStep struct {
	id     string
	name   string
	prompt sql.NullString
}

func (r *Repository) upgradeImproveKandevPrompts() error {
	revisions, err := loadImprovePromptRevisions()
	if err != nil {
		return err
	}

	var unknownTemplatePrompts, unknownStepPrompts, concurrentEdits int
	if unknown, conflict, err := r.upgradeImproveTemplate(revisions); err != nil {
		return err
	} else {
		unknownTemplatePrompts += unknown
		concurrentEdits += conflict
	}
	unknown, conflict, err := r.upgradeImproveWorkflowSteps(revisions)
	if err != nil {
		return err
	}
	unknownStepPrompts += unknown
	concurrentEdits += conflict

	if r.log != nil && (unknownTemplatePrompts > 0 || unknownStepPrompts > 0 || concurrentEdits > 0) {
		r.log.Warn("Improve Kandev prompt upgrade preserved unknown or concurrently edited content",
			zap.String("template_id", improveKandevTemplateID),
			zap.Int("unknown_template_prompts", unknownTemplatePrompts),
			zap.Int("unknown_workflow_prompts", unknownStepPrompts),
			zap.Int("concurrent_edits", concurrentEdits),
		)
	}
	return nil
}

func loadImprovePromptRevisions() (map[string]promptRevision, error) {
	previous, err := workflowcfg.LoadTemplateYAML(improveKandevBeforeLifecycleYAML)
	if err != nil {
		return nil, fmt.Errorf("load preceding Improve Kandev prompt revision: %w", err)
	}
	if previous.ID != improveKandevTemplateID {
		return nil, fmt.Errorf("preceding Improve Kandev template ID = %q", previous.ID)
	}
	currentTemplates, err := workflowcfg.LoadTemplates()
	if err != nil {
		return nil, fmt.Errorf("load current workflow templates: %w", err)
	}
	var current *models.WorkflowTemplate
	for _, template := range currentTemplates {
		if template.ID == improveKandevTemplateID {
			current = template
			break
		}
	}
	if current == nil {
		return nil, fmt.Errorf("current Improve Kandev template is missing")
	}
	previousByID := make(map[string]models.StepDefinition, len(previous.Steps))
	for _, step := range previous.Steps {
		previousByID[step.ID] = step
	}
	revisions := make(map[string]promptRevision, len(previousByID))
	for _, step := range current.Steps {
		previousStep, ok := previousByID[step.ID]
		if !ok {
			continue
		}
		revisions[step.ID] = promptRevision{
			name:     step.Name,
			current:  step.Prompt,
			previous: previousStep.Prompt,
		}
	}
	if len(revisions) != len(previousByID) || len(revisions) == 0 {
		return nil, fmt.Errorf("current and preceding Improve Kandev prompt revisions do not match")
	}
	return revisions, nil
}

func (r *Repository) upgradeImproveTemplate(revisions map[string]promptRevision) (int, int, error) {
	var before string
	err := r.db.QueryRowContext(context.Background(), r.db.Rebind(`
		SELECT steps FROM workflow_templates WHERE id = ?
	`), improveKandevTemplateID).Scan(&before)
	if err != nil {
		return 0, 0, fmt.Errorf("read stored Improve Kandev template: %w", err)
	}
	var steps []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(before), &steps); err != nil {
		return 0, 0, fmt.Errorf("decode stored Improve Kandev template steps: %w", err)
	}
	changed := false
	unknown := 0
	for _, step := range steps {
		var id string
		if err := json.Unmarshal(step["id"], &id); err != nil {
			continue
		}
		revision, ok := revisions[id]
		if !ok {
			continue
		}
		var prompt string
		if err := json.Unmarshal(step["prompt"], &prompt); err != nil {
			unknown++
			continue
		}
		switch prompt {
		case revision.current:
		case revision.previous:
			encoded, err := json.Marshal(revision.current)
			if err != nil {
				return 0, 0, err
			}
			step["prompt"] = encoded
			changed = true
		default:
			unknown++
		}
	}
	if !changed {
		return unknown, 0, nil
	}
	after, err := json.Marshal(steps)
	if err != nil {
		return 0, 0, fmt.Errorf("encode upgraded Improve Kandev template steps: %w", err)
	}
	updated, err := r.updateImproveTemplateStepsIfUnchanged(before, string(after))
	if err != nil {
		return 0, 0, fmt.Errorf("write upgraded Improve Kandev template: %w", err)
	}
	if !updated {
		return unknown, 1, nil
	}
	return unknown, 0, nil
}

func (r *Repository) updateImproveTemplateStepsIfUnchanged(before, after string) (bool, error) {
	result, err := r.db.ExecContext(context.Background(), r.db.Rebind(`
		UPDATE workflow_templates SET steps = ?, updated_at = ?
		WHERE id = ? AND steps = ?
	`), after, time.Now().UTC(), improveKandevTemplateID, before)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

func (r *Repository) upgradeImproveWorkflowSteps(revisions map[string]promptRevision) (int, int, error) {
	byName := make(map[string]promptRevision, len(revisions))
	for _, revision := range revisions {
		byName[revision.name] = revision
	}
	rows, err := r.db.QueryContext(context.Background(), r.db.Rebind(`
		SELECT s.id, s.name, s.prompt
		FROM workflow_steps AS s
		JOIN workflows AS w ON w.id = s.workflow_id
		WHERE w.workflow_template_id = ?
			AND s.name IN (?, ?)
	`), improveKandevTemplateID, revisions["improve"].name, revisions["test"].name)
	if err != nil {
		return 0, 0, fmt.Errorf("read stored Improve Kandev workflow steps: %w", err)
	}
	var stored []storedImproveStep
	for rows.Next() {
		var step storedImproveStep
		if err := rows.Scan(&step.id, &step.name, &step.prompt); err != nil {
			_ = rows.Close()
			return 0, 0, fmt.Errorf("scan stored Improve Kandev workflow step: %w", err)
		}
		stored = append(stored, step)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, 0, fmt.Errorf("read stored Improve Kandev workflow steps: %w", err)
	}
	if err := rows.Close(); err != nil {
		return 0, 0, fmt.Errorf("close stored Improve Kandev workflow step rows: %w", err)
	}

	unknown, conflicts := 0, 0
	for _, step := range stored {
		revision, ok := byName[step.name]
		if !ok || !step.prompt.Valid {
			unknown++
			continue
		}
		switch step.prompt.String {
		case revision.current:
		case revision.previous:
			updated, err := r.updateImproveStepPromptIfUnchanged(step.id, step.prompt.String, revision.current)
			if err != nil {
				return 0, 0, fmt.Errorf("write upgraded Improve Kandev workflow step %s: %w", step.id, err)
			}
			if !updated {
				conflicts++
			}
		default:
			unknown++
		}
	}
	return unknown, conflicts, nil
}

func (r *Repository) updateImproveStepPromptIfUnchanged(id, before, after string) (bool, error) {
	result, err := r.db.ExecContext(context.Background(), r.db.Rebind(`
		UPDATE workflow_steps SET prompt = ?, updated_at = ?
		WHERE id = ? AND prompt = ?
	`), after, time.Now().UTC(), id, before)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}
