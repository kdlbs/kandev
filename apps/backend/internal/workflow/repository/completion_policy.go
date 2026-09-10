package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/workflow/models"
)

const completionPolicyBackfillMetaKey = "workflow_complete_task_on_enter_backfill_v1"

// backfillCompletionPolicy converts the legacy name-based terminal convention
// once, before system templates and default workflow steps are seeded. The
// marker and all data changes are committed together so a failed startup can
// safely retry the migration.
func (r *Repository) backfillCompletionPolicy() error {
	if _, err := r.db.Exec(`
		CREATE TABLE IF NOT EXISTS kandev_meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		)`); err != nil {
		return fmt.Errorf("create metadata table: %w", err)
	}

	tx, err := r.db.Beginx()
	if err != nil {
		return fmt.Errorf("begin completion policy backfill: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var marker string
	err = tx.QueryRowx(tx.Rebind(`
		SELECT value
		FROM kandev_meta
		WHERE key = ?
	`), completionPolicyBackfillMetaKey).Scan(&marker)
	if err == nil {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit completed completion policy backfill: %w", err)
		}
		return nil
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("read completion policy backfill marker: %w", err)
	}

	if _, err := tx.Exec(tx.Rebind(`
		UPDATE workflow_steps
		SET complete_task_on_enter = 1
		WHERE complete_task_on_enter = 0
		  AND LOWER(TRIM(workflow_steps.name)) IN ('done', 'complete', 'completed', 'approved')
		  AND NOT EXISTS (
			SELECT 1
			FROM workflow_steps AS successor
			WHERE successor.workflow_id = workflow_steps.workflow_id
			  AND successor.position > workflow_steps.position
		  )
	`)); err != nil {
		return fmt.Errorf("backfill workflow step completion policy: %w", err)
	}

	if err := r.backfillTemplateCompletionPolicy(tx); err != nil {
		return err
	}

	if _, err := tx.Exec(tx.Rebind(`
		INSERT INTO kandev_meta (key, value)
		VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`), completionPolicyBackfillMetaKey, "completed"); err != nil {
		return fmt.Errorf("write completion policy backfill marker: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit completion policy backfill: %w", err)
	}
	return nil
}

func (r *Repository) backfillTemplateCompletionPolicy(tx *sqlx.Tx) error {
	rows, err := tx.Queryx(tx.Rebind(`
		SELECT id, steps
		FROM workflow_templates
	`))
	if err != nil {
		return fmt.Errorf("read workflow template steps: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var id, stepsJSON string
		if err := rows.Scan(&id, &stepsJSON); err != nil {
			return fmt.Errorf("scan workflow template %q: %w", id, err)
		}

		normalized, changed, err := normalizeTemplateCompletionPolicy([]byte(stepsJSON))
		if err != nil {
			return fmt.Errorf("normalize workflow template %q: %w", id, err)
		}
		if !changed {
			continue
		}
		if _, err := tx.Exec(tx.Rebind(`
			UPDATE workflow_templates
			SET steps = ?
			WHERE id = ?
		`), string(normalized), id); err != nil {
			return fmt.Errorf("update workflow template %q: %w", id, err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read workflow template steps: %w", err)
	}
	return nil
}

func normalizeTemplateCompletionPolicy(data []byte) ([]byte, bool, error) {
	var steps []map[string]json.RawMessage
	if err := json.Unmarshal(data, &steps); err != nil {
		return nil, false, err
	}
	if len(steps) == 0 {
		return data, false, nil
	}

	positions := make([]int, len(steps))
	maxPosition := 0
	for i, step := range steps {
		position, err := templateStepPosition(step)
		if err != nil {
			return nil, false, err
		}
		positions[i] = position
		if i == 0 || position > maxPosition {
			maxPosition = position
		}
	}

	changed := false
	for i, step := range steps {
		if _, exists := step["complete_task_on_enter"]; exists {
			continue
		}
		name, err := templateStepName(step)
		if err != nil {
			return nil, false, err
		}
		enabled := positions[i] == maxPosition && models.IsTerminalStepName(name)
		encoded, err := json.Marshal(enabled)
		if err != nil {
			return nil, false, err
		}
		step["complete_task_on_enter"] = encoded
		changed = true
	}
	if !changed {
		return data, false, nil
	}

	normalized, err := json.Marshal(steps)
	if err != nil {
		return nil, false, err
	}
	return normalized, true, nil
}

func templateStepPosition(step map[string]json.RawMessage) (int, error) {
	raw, ok := step["position"]
	if !ok {
		return 0, fmt.Errorf("step is missing position")
	}
	var position int
	if err := json.Unmarshal(raw, &position); err != nil {
		return 0, fmt.Errorf("decode step position: %w", err)
	}
	return position, nil
}

func templateStepName(step map[string]json.RawMessage) (string, error) {
	raw, ok := step["name"]
	if !ok {
		return "", fmt.Errorf("step is missing name")
	}
	var name string
	if err := json.Unmarshal(raw, &name); err != nil {
		return "", fmt.Errorf("decode step name: %w", err)
	}
	return name, nil
}
