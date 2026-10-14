package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

const completionGateSummaryBatchSize = 100

type completionGateSummaryTask struct {
	updatedAt time.Time
	revision  int64
	criteria  []models.TaskCompletionCriterion
}

type completionGateEvidenceKey struct {
	taskID string
	id     string
}

func (r *Repository) GetTaskCompletionGateSummaries(
	ctx context.Context,
	taskIDs []string,
) (models.TaskCompletionGateSummaryBatch, error) {
	result := models.TaskCompletionGateSummaryBatch{
		ByTaskID:       make(map[string]*models.TaskCompletionGateSummaryObservation, len(taskIDs)),
		MissingTaskIDs: []string{},
	}
	ids := uniqueCompletionGateTaskIDs(taskIDs)
	for _, chunk := range chunkIDs(ids, completionGateSummaryBatchSize) {
		observations, missing, err := r.readTaskCompletionGateSummaryChunk(ctx, chunk)
		if err != nil {
			return models.TaskCompletionGateSummaryBatch{}, err
		}
		for taskID, observation := range observations {
			result.ByTaskID[taskID] = observation
		}
		result.MissingTaskIDs = append(result.MissingTaskIDs, missing...)
	}
	return result, nil
}

func uniqueCompletionGateTaskIDs(taskIDs []string) []string {
	seen := make(map[string]struct{}, len(taskIDs))
	unique := make([]string, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		if _, exists := seen[taskID]; exists {
			continue
		}
		seen[taskID] = struct{}{}
		unique = append(unique, taskID)
	}
	return unique
}

func (r *Repository) readTaskCompletionGateSummaryChunk(
	ctx context.Context,
	taskIDs []string,
) (map[string]*models.TaskCompletionGateSummaryObservation, []string, error) {
	tx, err := r.beginTaskCompletionGateRead(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback() }()

	idsPlaceholder, idArgs := buildInPlaceholders(taskIDs)
	tasks := make(map[string]*completionGateSummaryTask, len(taskIDs))
	identityQuery := `SELECT id, updated_at FROM tasks WHERE id IN (` + idsPlaceholder + `)`
	if err := readCompletionGateSummaryRows(ctx, tx, r.ro.Rebind(identityQuery), idArgs, func(rows *sql.Rows) error {
		var taskID string
		var updatedAt time.Time
		if err := rows.Scan(&taskID, &updatedAt); err != nil {
			return err
		}
		tasks[taskID] = &completionGateSummaryTask{updatedAt: updatedAt}
		return nil
	}); err != nil {
		return nil, nil, fmt.Errorf("load completion-gate task identities: %w", err)
	}

	missing := missingCompletionGateTaskIDs(taskIDs, tasks)
	if len(tasks) == 0 {
		if err := tx.Commit(); err != nil {
			return nil, nil, err
		}
		return map[string]*models.TaskCompletionGateSummaryObservation{}, missing, nil
	}

	placeholders, args := buildInPlaceholders(taskIDs)
	setQuery := `SELECT task_id, revision FROM task_completion_sets WHERE task_id IN (` + placeholders + `)`
	if err := readCompletionGateSummaryRows(ctx, tx, r.ro.Rebind(setQuery), args, func(rows *sql.Rows) error {
		var taskID string
		var revision int64
		if err := rows.Scan(&taskID, &revision); err != nil {
			return err
		}
		if task := tasks[taskID]; task != nil {
			task.revision = revision
		}
		return nil
	}); err != nil {
		return nil, nil, fmt.Errorf("load completion-gate revisions: %w", err)
	}

	criteriaQuery := `
		SELECT task_id, criterion_revision, verified_revision, evidence_kind, evidence_id, evidence_revision
		FROM task_completion_criteria
		WHERE task_id IN (` + placeholders + `)
		ORDER BY task_id, criterion_id`
	if err := readCompletionGateSummaryRows(ctx, tx, r.ro.Rebind(criteriaQuery), args, func(rows *sql.Rows) error {
		return scanCompletionGateBatchCriterion(rows, tasks)
	}); err != nil {
		return nil, nil, fmt.Errorf("load completion-gate criteria: %w", err)
	}

	prHeads, err := r.readCompletionGateBatchPRHeads(ctx, tx, tasks, placeholders, args)
	if err != nil {
		return nil, nil, fmt.Errorf("load completion-gate pull-request evidence: %w", err)
	}
	executions, err := r.readCompletionGateBatchExecutions(ctx, tx, tasks, placeholders, args)
	if err != nil {
		return nil, nil, fmt.Errorf("load completion-gate execution evidence: %w", err)
	}

	observations := completionGateBatchObservations(tasks, prHeads, executions)
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	return observations, missing, nil
}

func readCompletionGateSummaryRows(
	ctx context.Context,
	tx *sql.Tx,
	query string,
	args []interface{},
	visit func(*sql.Rows) error,
) error {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	for rows.Next() {
		if err := visit(rows); err != nil {
			_ = rows.Close()
			return err
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	return rows.Close()
}

func (r *Repository) readCompletionGateBatchPRHeads(
	ctx context.Context,
	tx *sql.Tx,
	tasks map[string]*completionGateSummaryTask,
	placeholders string,
	args []interface{},
) (map[completionGateEvidenceKey]string, error) {
	if !completionGateBatchHasEvidence(tasks, models.TaskCompletionEvidenceGitHubPRHead) {
		return map[completionGateEvidenceKey]string{}, nil
	}
	query := `
		SELECT c.task_id, c.evidence_id, pr.head_sha
		FROM task_completion_criteria c
		JOIN github_task_prs pr ON pr.id = c.evidence_id AND pr.task_id = c.task_id
		WHERE c.task_id IN (` + placeholders + `)
		  AND c.evidence_kind = ?
		  AND c.verified_revision = c.criterion_revision`
	queryArgs := append(append([]interface{}(nil), args...), models.TaskCompletionEvidenceGitHubPRHead)
	observed := make(map[completionGateEvidenceKey]string)
	err := readCompletionGateSummaryRows(ctx, tx, r.ro.Rebind(query), queryArgs, func(rows *sql.Rows) error {
		var key completionGateEvidenceKey
		var head string
		if err := rows.Scan(&key.taskID, &key.id, &head); err != nil {
			return err
		}
		observed[key] = head
		return nil
	})
	return observed, err
}

func (r *Repository) readCompletionGateBatchExecutions(
	ctx context.Context,
	tx *sql.Tx,
	tasks map[string]*completionGateSummaryTask,
	placeholders string,
	args []interface{},
) (map[completionGateEvidenceKey]struct{}, error) {
	if !completionGateBatchHasEvidence(tasks, models.TaskCompletionEvidenceExecution) {
		return map[completionGateEvidenceKey]struct{}{}, nil
	}
	query := `
		SELECT c.task_id, c.evidence_id
		FROM task_completion_criteria c
		JOIN task_session_turns turn
		  ON turn.id = c.evidence_id AND turn.task_id = c.task_id AND turn.completed_at IS NOT NULL
		WHERE c.task_id IN (` + placeholders + `)
		  AND c.evidence_kind = ?
		  AND c.verified_revision = c.criterion_revision`
	queryArgs := append(append([]interface{}(nil), args...), models.TaskCompletionEvidenceExecution)
	observed := make(map[completionGateEvidenceKey]struct{})
	err := readCompletionGateSummaryRows(ctx, tx, r.ro.Rebind(query), queryArgs, func(rows *sql.Rows) error {
		var key completionGateEvidenceKey
		if err := rows.Scan(&key.taskID, &key.id); err != nil {
			return err
		}
		observed[key] = struct{}{}
		return nil
	})
	return observed, err
}

func completionGateBatchHasEvidence(tasks map[string]*completionGateSummaryTask, kind string) bool {
	for _, task := range tasks {
		for _, criterion := range task.criteria {
			if criterion.Evidence != nil && criterion.Evidence.Subject.Kind == kind &&
				!completionCriterionBlocked(criterion, true) {
				return true
			}
		}
	}
	return false
}

func completionGateBatchEvidenceCurrent(
	criterion models.TaskCompletionCriterion,
	taskUpdatedAt time.Time,
	prHeads map[completionGateEvidenceKey]string,
	executions map[completionGateEvidenceKey]struct{},
	taskID string,
) bool {
	if completionCriterionBlocked(criterion, true) {
		return false
	}
	subject := criterion.Evidence.Subject
	key := completionGateEvidenceKey{taskID: taskID, id: subject.ID}
	switch subject.Kind {
	case models.TaskCompletionEvidenceTaskRevision:
		return completionEvidenceSubjectCurrent(subject, &taskUpdatedAt, "", true)
	case models.TaskCompletionEvidenceGitHubPRHead:
		head, exists := prHeads[key]
		return completionEvidenceSubjectCurrent(subject, nil, head, exists)
	case models.TaskCompletionEvidenceExecution:
		_, exists := executions[key]
		return completionEvidenceSubjectCurrent(subject, nil, "", exists)
	case models.TaskCompletionEvidenceArtifact:
		return completionEvidenceSubjectCurrent(subject, nil, "", true)
	default:
		return false
	}
}

func completionGateBatchObservations(tasks map[string]*completionGateSummaryTask, prHeads map[completionGateEvidenceKey]string, executions map[completionGateEvidenceKey]struct{}) map[string]*models.TaskCompletionGateSummaryObservation {
	observations := make(map[string]*models.TaskCompletionGateSummaryObservation, len(tasks))
	for taskID, task := range tasks {
		if task.revision == 0 && len(task.criteria) == 0 {
			observations[taskID] = nil
			continue
		}
		observation := &models.TaskCompletionGateSummaryObservation{
			Revision:      task.revision,
			CriteriaCount: len(task.criteria),
		}
		for _, criterion := range task.criteria {
			evidenceCurrent := completionGateBatchEvidenceCurrent(criterion, task.updatedAt, prHeads, executions, taskID)
			if completionCriterionBlocked(criterion, evidenceCurrent) {
				observation.BlockerCount++
			} else {
				observation.VerifiedCount++
			}
		}
		observation.Blocked = observation.BlockerCount > 0
		observations[taskID] = observation
	}
	return observations
}

func missingCompletionGateTaskIDs(taskIDs []string, tasks map[string]*completionGateSummaryTask) []string {
	missing := make([]string, 0)
	for _, taskID := range taskIDs {
		if tasks[taskID] == nil {
			missing = append(missing, taskID)
		}
	}
	return missing
}

func scanCompletionGateBatchCriterion(rows *sql.Rows, tasks map[string]*completionGateSummaryTask) error {
	var (
		taskID           string
		criterion        models.TaskCompletionCriterion
		evidenceKind     string
		evidenceID       string
		evidenceRevision string
	)
	if err := rows.Scan(&taskID, &criterion.CriterionRevision, &criterion.VerifiedRevision,
		&evidenceKind, &evidenceID, &evidenceRevision); err != nil {
		return err
	}
	if evidenceKind != "" {
		criterion.Evidence = &models.TaskCompletionEvidence{
			Subject: models.TaskCompletionEvidenceSubject{
				Kind: evidenceKind, ID: evidenceID, Revision: evidenceRevision,
			},
		}
	}
	if task := tasks[taskID]; task != nil {
		task.criteria = append(task.criteria, criterion)
	}
	return nil
}
