package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/workflow/models"
)

const holdStageTypeBackfillMetaKey = "workflow_step_hold_stage_type_backfill_v1"

// holdStepNames are the step names a workflow used before a step could declare
// itself a hold. This list is read only by the one-time backfill below: the
// overview reads the stored stage_type, never the name. It is kept here, at the
// transition, rather than at runtime so the names it converts cannot start
// deciding anything again.
var holdStepNames = map[string]bool{
	"보류":      true,
	"보류중":     true,
	"hold":    true,
	"on hold": true,
	"blocked": true,
	"paused":  true,
}

// backfillHoldStageType converts the steps a workflow named like a hold, once,
// so their stored nature says what the name used to say. It runs before system
// templates and default steps are seeded, exactly like the completion-policy
// backfill, and the marker is committed with the data changes so a failed
// startup retries rather than half-converts.
//
// Only a step whose stored nature is the default - empty or "custom" - is
// converted. A step that already declares work, review, approval, or hold
// carries a nature a consumer reads (a review or approval step keeps its
// decision prompt, a work step keeps its rail variant), so converting it would
// lose that nature. A step whose name is not on the list is never guessed at -
// a workflow that spells its hold step differently is marked through the step
// settings instead.
func (r *Repository) backfillHoldStageType() error {
	if _, err := r.db.Exec(`
		CREATE TABLE IF NOT EXISTS kandev_meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		)`); err != nil {
		return fmt.Errorf("create metadata table: %w", err)
	}

	tx, err := r.db.Beginx()
	if err != nil {
		return fmt.Errorf("begin hold stage type backfill: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var marker string
	err = tx.QueryRowx(tx.Rebind(`
		SELECT value
		FROM kandev_meta
		WHERE key = ?
	`), holdStageTypeBackfillMetaKey).Scan(&marker)
	if err == nil {
		return tx.Commit()
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("read hold stage type backfill marker: %w", err)
	}

	updateHoldRows := tx.Rebind(fmt.Sprintf(`
		UPDATE workflow_steps
		SET stage_type = ?
		WHERE COALESCE(stage_type, '') IN ('', ?)
		  AND LOWER(TRIM(name)) IN (%s)
	`, holdStageTypeNameList()))
	if _, err := tx.Exec(updateHoldRows, string(models.StageTypeHold), string(models.StageTypeCustom)); err != nil {
		return fmt.Errorf("backfill workflow step hold stage type: %w", err)
	}
	if err := backfillTemplateHoldStageType(tx); err != nil {
		return err
	}

	if _, err := tx.Exec(tx.Rebind(`
		INSERT INTO kandev_meta (key, value)
		VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`), holdStageTypeBackfillMetaKey, "completed"); err != nil {
		return fmt.Errorf("write hold stage type backfill marker: %w", err)
	}
	return tx.Commit()
}

// holdStageTypeNameList renders the hold names as a quoted, sorted SQL literal
// list so the same Go set drives the row update and the template rewrite.
func holdStageTypeNameList() string {
	names := make([]string, 0, len(holdStepNames))
	for name := range holdStepNames {
		names = append(names, "'"+strings.ReplaceAll(name, "'", "''")+"'")
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// backfillTemplateHoldStageType gives a template step the same hold nature, so
// a workflow seeded from that template after the transition is a hold from the
// start rather than losing the meaning its name carried.
func backfillTemplateHoldStageType(tx *sqlx.Tx) error {
	rows, err := tx.Queryx(tx.Rebind(`SELECT id, steps FROM workflow_templates`))
	if err != nil {
		return fmt.Errorf("read workflow template steps: %w", err)
	}
	templates, err := scanTemplateSteps(rows)
	if err != nil {
		return err
	}
	for _, template := range templates {
		normalized, changed, err := normalizeTemplateHoldStageType([]byte(template.stepsJSON))
		if err != nil {
			return fmt.Errorf("normalize workflow template %q: %w", template.id, err)
		}
		if !changed {
			continue
		}
		if _, err := tx.Exec(tx.Rebind(`
			UPDATE workflow_templates
			SET steps = ?
			WHERE id = ?
		`), string(normalized), template.id); err != nil {
			return fmt.Errorf("update workflow template %q: %w", template.id, err)
		}
	}
	return nil
}

type templateStepRow struct {
	id        string
	stepsJSON string
}

func scanTemplateSteps(rows *sqlx.Rows) ([]templateStepRow, error) {
	defer func() { _ = rows.Close() }()
	templates := make([]templateStepRow, 0)
	for rows.Next() {
		var template templateStepRow
		if err := rows.Scan(&template.id, &template.stepsJSON); err != nil {
			return nil, fmt.Errorf("scan workflow template %q: %w", template.id, err)
		}
		templates = append(templates, template)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read workflow template steps: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close workflow template steps: %w", err)
	}
	return templates, nil
}

// normalizeTemplateHoldStageType stamps "hold" on the template steps whose name
// is a hold name and whose stored nature is the default (empty or "custom"), so
// a workflow seeded from that template marks the step as a hold from the start.
// A step that already states work, review, approval, or hold keeps it, and a
// step whose name is not on the list keeps whatever it states.
func normalizeTemplateHoldStageType(data []byte) ([]byte, bool, error) {
	var steps []map[string]json.RawMessage
	if err := json.Unmarshal(data, &steps); err != nil {
		return nil, false, err
	}
	changed := false
	for _, step := range steps {
		if !templateStageTypeIsDefault(step) {
			continue
		}
		name, err := templateStepName(step)
		if err != nil {
			return nil, false, err
		}
		if !holdStepNames[strings.ToLower(strings.TrimSpace(name))] {
			continue
		}
		encoded, err := json.Marshal(string(models.StageTypeHold))
		if err != nil {
			return nil, false, err
		}
		step["stage_type"] = encoded
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

// templateStageTypeIsDefault reports whether a template step states no nature or
// the default "custom". A JSON null or a value that is not a string states
// something, so it is left alone rather than overwritten.
func templateStageTypeIsDefault(step map[string]json.RawMessage) bool {
	raw, ok := step["stage_type"]
	if !ok {
		return true
	}
	var stageType *string
	if err := json.Unmarshal(raw, &stageType); err != nil || stageType == nil {
		return false
	}
	return *stageType == "" || *stageType == string(models.StageTypeCustom)
}
