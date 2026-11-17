package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	workflowcfg "github.com/kandev/kandev/config/workflows"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/workflow/models"
)

//go:embed testdata/improve-kandev-before-lifecycle.yml
var improveKandevBeforeLifecycleYAML []byte

const improveKandevTemplateID = "improve-kandev"

type promptRevision struct {
	name         string
	current      string
	currentHash  [sha256.Size]byte
	previousHash [sha256.Size]byte
}

type storedImproveStep struct {
	id        string
	name      string
	prompt    sql.NullString
	createdAt time.Time
	updatedAt time.Time
}

func (r *Repository) upgradeImproveKandevPrompts() error {
	revisions, err := loadImprovePromptRevisions()
	if err != nil {
		return err
	}

	var unknownTemplatePrompts, unknownStepPrompts, concurrentEdits int
	unknown, conflict, err := r.upgradeImproveTemplate(revisions)
	if err != nil {
		return err
	}
	unknownTemplatePrompts += unknown
	concurrentEdits += conflict
	unknown, conflict, err = r.upgradeImproveWorkflowSteps(revisions)
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
			name:         step.Name,
			current:      step.Prompt,
			currentHash:  normalizedPromptHash(step.Prompt),
			previousHash: normalizedPromptHash(previousStep.Prompt),
		}
	}
	if len(revisions) != len(previousByID) || len(revisions) == 0 {
		return nil, fmt.Errorf("current and preceding Improve Kandev prompt revisions do not match")
	}
	return revisions, nil
}

func normalizedPromptHash(prompt string) [sha256.Size]byte {
	return sha256.Sum256([]byte(strings.TrimSpace(prompt)))
}

func (r *Repository) upgradeImproveTemplate(revisions map[string]promptRevision) (int, int, error) {
	var before string
	var createdAt, updatedAt time.Time
	err := r.db.QueryRowContext(context.Background(), r.db.Rebind(`
		SELECT steps, created_at, updated_at FROM workflow_templates WHERE id = ?
	`), improveKandevTemplateID).Scan(&before, &createdAt, &updatedAt)
	if err != nil {
		return 0, 0, fmt.Errorf("read stored Improve Kandev template: %w", err)
	}
	after, unknown, changed, err := upgradedImproveTemplateSteps(before, revisions)
	if err != nil {
		return 0, 0, err
	}
	if !createdAt.Equal(updatedAt) {
		if changed {
			unknown++
		}
		return unknown, 0, nil
	}
	if !changed {
		return unknown, 0, nil
	}
	updated, err := r.updateImproveTemplateStepsIfUnchanged(before, createdAt, updatedAt, after)
	if err != nil {
		return 0, 0, fmt.Errorf("write upgraded Improve Kandev template: %w", err)
	}
	if updated {
		return unknown, 0, nil
	}

	// A concurrent unrelated template edit may change steps without changing the prompt.
	var current string
	var currentCreatedAt, currentUpdatedAt time.Time
	err = r.db.QueryRowContext(context.Background(), r.db.Rebind(`
		SELECT steps, created_at, updated_at FROM workflow_templates WHERE id = ?
	`), improveKandevTemplateID).Scan(&current, &currentCreatedAt, &currentUpdatedAt)
	if err != nil {
		return 0, 0, fmt.Errorf("reread stored Improve Kandev template after conflict: %w", err)
	}
	if !currentCreatedAt.Equal(currentUpdatedAt) {
		return unknown, 0, nil
	}
	retryAfter, retryUnknown, retryChanged, err := upgradedImproveTemplateSteps(current, revisions)
	if err != nil {
		return 0, 0, err
	}
	if !retryChanged {
		return unknown + retryUnknown, 0, nil
	}
	updated, err = r.updateImproveTemplateStepsIfUnchanged(current, currentCreatedAt, currentUpdatedAt, retryAfter)
	if err != nil {
		return 0, 0, fmt.Errorf("retry upgraded Improve Kandev template write: %w", err)
	}
	if !updated {
		return 0, 0, fmt.Errorf("improve-kandev template changed during prompt upgrade retry")
	}
	return unknown + retryUnknown, 0, nil
}

func upgradedImproveTemplateSteps(before string, revisions map[string]promptRevision) (string, int, bool, error) {
	var steps []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(before), &steps); err != nil {
		return "", 0, false, fmt.Errorf("decode stored Improve Kandev template steps: %w", err)
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
		switch hash := normalizedPromptHash(prompt); hash {
		case revision.currentHash:
		case revision.previousHash:
			encoded, err := json.Marshal(revision.current)
			if err != nil {
				return "", 0, false, err
			}
			step["prompt"] = encoded
			changed = true
		default:
			unknown++
		}
	}
	if !changed {
		return before, unknown, false, nil
	}
	after, err := json.Marshal(steps)
	if err != nil {
		return "", 0, false, fmt.Errorf("encode upgraded Improve Kandev template steps: %w", err)
	}
	return string(after), unknown, true, nil
}

func (r *Repository) updateImproveTemplateStepsIfUnchanged(before string, createdAt, updatedAt time.Time, after string) (bool, error) {
	result, err := r.db.ExecContext(context.Background(), r.db.Rebind(`
		UPDATE workflow_templates SET steps = ?, updated_at = ?
		WHERE id = ? AND steps = ? AND created_at = ? AND updated_at = ?
	`), after, time.Now().UTC(), improveKandevTemplateID, before, createdAt, updatedAt)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

func (r *Repository) upgradeImproveWorkflowSteps(revisions map[string]promptRevision) (int, int, error) {
	hasTemplateBinding, err := db.ColumnExists(r.db, "workflows", "workflow_template_id")
	if err != nil {
		return 0, 0, fmt.Errorf("inspect workflow template binding column: %w", err)
	}
	if !hasTemplateBinding {
		return 0, 0, nil
	}

	byName := make(map[string]promptRevision, len(revisions))
	for _, revision := range revisions {
		byName[revision.name] = revision
	}
	rows, err := r.db.QueryContext(context.Background(), r.db.Rebind(`
		SELECT s.id, s.name, s.prompt, s.created_at, s.updated_at
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
		if err := rows.Scan(&step.id, &step.name, &step.prompt, &step.createdAt, &step.updatedAt); err != nil {
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
		if !ok {
			unknown++
			continue
		}
		preserved, conflict, err := r.upgradeImproveWorkflowStep(step, revision)
		if err != nil {
			return 0, 0, fmt.Errorf("upgrade Improve Kandev workflow step %s: %w", step.id, err)
		}
		if preserved {
			unknown++
		}
		if conflict {
			conflicts++
		}
	}
	return unknown, conflicts, nil
}

func (r *Repository) upgradeImproveWorkflowStep(step storedImproveStep, revision promptRevision) (bool, bool, error) {
	if !step.prompt.Valid {
		return true, false, nil
	}
	switch normalizedPromptHash(step.prompt.String) {
	case revision.currentHash:
		return false, false, nil
	case revision.previousHash:
		if !step.createdAt.Equal(step.updatedAt) {
			return true, false, nil
		}
		updated, err := r.updateImproveStepPromptIfUnchanged(
			step.id, step.prompt.String, step.createdAt, step.updatedAt, revision.current,
		)
		if err != nil {
			return false, false, err
		}
		return false, !updated, nil
	default:
		return true, false, nil
	}
}

func (r *Repository) updateImproveStepPromptIfUnchanged(id, before string, createdAt, updatedAt time.Time, after string) (bool, error) {
	result, err := r.db.ExecContext(context.Background(), r.db.Rebind(`
		UPDATE workflow_steps SET prompt = ?, updated_at = ?
		WHERE id = ? AND prompt = ? AND created_at = ? AND updated_at = ?
	`), after, time.Now().UTC(), id, before, createdAt, updatedAt)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}
