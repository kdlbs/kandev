package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/pkg/api/v1"
)

const (
	maxCompletionCriteria          = 64
	completionGateActorHuman       = "human"
	completionGateActorPlugin      = "plugin"
	completionGateTaskVersionQuery = `SELECT workspace_id, updated_at FROM tasks WHERE id = ?`
)

func (r *Repository) SetTaskCompletionCriteria(ctx context.Context, change models.TaskCompletionCriteriaChange) (*models.TaskCompletionGateSnapshot, error) {
	snapshot, _, err := r.setTaskCompletionCriteria(ctx, change, false)
	return snapshot, err
}

func (r *Repository) SetTaskCompletionCriteriaExact(ctx context.Context, change models.TaskCompletionCriteriaChange) (*models.TaskCompletionGateSnapshot, bool, error) {
	return r.setTaskCompletionCriteria(ctx, change, true)
}

//nolint:cyclop,funlen,gocognit // The transaction replaces the task-owned criteria set with its revision and audit record.
func (r *Repository) setTaskCompletionCriteria(ctx context.Context, change models.TaskCompletionCriteriaChange, exact bool) (*models.TaskCompletionGateSnapshot, bool, error) {
	if err := validateCompletionCriteria(change.Criteria); err != nil {
		return nil, false, err
	}
	if exact && (!validCompletionGateOperation(change.OperationID, change.PayloadDigest, change.ExpectedTaskResourceVersion) ||
		!validExactCompletionGateActor(change.TaskID, change.WorkspaceID, change.ActorKind, change.ActorID, change.ClaimFence)) {
		return nil, false, errors.New("exact completion criteria operation identity is incomplete")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()

	if exact {
		snapshot, found, err := r.replayCompletionGateOperationTx(ctx, tx, change.OperationID, change.WorkspaceID, change.TaskID, "set_criteria", change.PayloadDigest)
		if err != nil {
			return nil, false, err
		}
		if found {
			if err := tx.Commit(); err != nil {
				return nil, false, err
			}
			return snapshot, true, nil
		}
	}
	workspaceID, err := r.lockCompletionTaskForChange(ctx, tx, change.TaskID, change.WorkspaceID, change.ExpectedTaskResourceVersion, change.ClaimFence, exact)
	if err != nil {
		return nil, false, err
	}
	if workspaceID != change.WorkspaceID {
		return nil, false, repoerrors.ErrTaskVersionConflict
	}
	current, err := r.readTaskCompletionGateTx(ctx, tx, change.TaskID, false)
	if err != nil {
		return nil, false, err
	}
	if current.Revision != change.ExpectedRevision {
		return nil, false, repoerrors.ErrTaskCompletionCriteriaConflict
	}
	oldByID := make(map[string]models.TaskCompletionCriterion, len(current.Criteria))
	for _, criterion := range current.Criteria {
		oldByID[criterion.ID] = criterion
	}
	newByID := make(map[string]models.TaskCompletionCriterion, len(change.Criteria))
	for _, criterion := range change.Criteria {
		newByID[criterion.ID] = criterion
	}
	needsHumanConfirmation := false
	for _, previous := range current.Criteria {
		next, retained := newByID[previous.ID]
		if retained && completionCriterionEquivalent(previous, next) {
			continue
		}
		blocked, err := r.completionCriterionBlockedTx(ctx, tx, change.TaskID, previous)
		if err != nil {
			return nil, false, err
		}
		needsHumanConfirmation = needsHumanConfirmation || blocked
	}
	if needsHumanConfirmation && (change.ActorKind != completionGateActorHuman ||
		change.HumanConfirmationRevision != current.Revision || strings.TrimSpace(change.HumanConfirmationReason) == "") {
		return nil, false, repoerrors.ErrTaskCompletionHumanConfirmationRequired
	}

	nextRevision := current.Revision + 1
	now := r.nowUTC()
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_completion_sets (task_id, workspace_id, revision, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(task_id) DO UPDATE SET workspace_id = excluded.workspace_id, revision = excluded.revision, updated_at = excluded.updated_at
	`), change.TaskID, change.WorkspaceID, nextRevision, now); err != nil {
		return nil, false, err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_completion_criteria WHERE task_id = ?`), change.TaskID); err != nil {
		return nil, false, err
	}
	for _, criterion := range change.Criteria {
		criterion.ID = strings.TrimSpace(criterion.ID)
		criterion.Description = strings.TrimSpace(criterion.Description)
		criterion.EvidenceSubject.Kind = strings.TrimSpace(criterion.EvidenceSubject.Kind)
		criterion.EvidenceSubject.ID = strings.TrimSpace(criterion.EvidenceSubject.ID)
		criterionRevision := int64(1)
		if previous, exists := oldByID[criterion.ID]; exists {
			criterionRevision = previous.CriterionRevision
			if !completionCriterionEquivalent(previous, criterion) {
				criterionRevision++
			} else {
				criterion = previous
			}
		}
		criterion.CriterionRevision = criterionRevision
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`
			INSERT INTO task_completion_criteria (
				task_id, criterion_id, description, criterion_revision, subject_kind, subject_id,
				verified_revision, evidence_kind, evidence_id, evidence_revision, evidence_summary,
				evidence_reference, verifier_kind, verifier_id, verified_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`), change.TaskID, criterion.ID, criterion.Description, criterionRevision,
			criterion.EvidenceSubject.Kind, criterion.EvidenceSubject.ID, criterion.VerifiedRevision,
			evidenceField(criterion.Evidence, func(e models.TaskCompletionEvidence) string { return e.Subject.Kind }),
			evidenceField(criterion.Evidence, func(e models.TaskCompletionEvidence) string { return e.Subject.ID }),
			evidenceField(criterion.Evidence, func(e models.TaskCompletionEvidence) string { return e.Subject.Revision }),
			evidenceField(criterion.Evidence, func(e models.TaskCompletionEvidence) string { return e.Summary }),
			evidenceField(criterion.Evidence, func(e models.TaskCompletionEvidence) string { return e.Reference }),
			criterion.VerifierKind, criterion.VerifierID, criterion.VerifiedAt); err != nil {
			return nil, false, err
		}
	}
	details, _ := json.Marshal(change.Criteria)
	reason := strings.TrimSpace(change.HumanConfirmationReason)
	if !needsHumanConfirmation {
		reason = ""
	}
	if err := insertCompletionHistoryTx(ctx, tx, r.db.Rebind, models.TaskCompletionGateHistory{
		ID: uuid.NewString(), TaskID: change.TaskID, WorkspaceID: change.WorkspaceID,
		Revision: nextRevision, Action: "criteria_set", ActorKind: change.ActorKind,
		ActorID: change.ActorID, Reason: reason, Details: string(details), CreatedAt: now,
	}); err != nil {
		return nil, false, err
	}
	snapshot, err := r.readTaskCompletionGateTx(ctx, tx, change.TaskID, false)
	if err != nil {
		return nil, false, err
	}
	if exact {
		if err := r.recordCompletionGateOperationTx(ctx, tx, change.OperationID, change.WorkspaceID, change.TaskID, "set_criteria", change.PayloadDigest, snapshot, now); err != nil {
			return nil, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return snapshot, false, nil
}

func (r *Repository) VerifyTaskCompletionCriterion(ctx context.Context, change models.TaskCompletionEvidenceChange) (*models.TaskCompletionGateSnapshot, error) {
	snapshot, _, err := r.verifyTaskCompletionCriterion(ctx, change, false)
	return snapshot, err
}

func (r *Repository) VerifyTaskCompletionCriterionExact(ctx context.Context, change models.TaskCompletionEvidenceChange) (*models.TaskCompletionGateSnapshot, bool, error) {
	return r.verifyTaskCompletionCriterion(ctx, change, true)
}

//nolint:cyclop,funlen,gocognit // The transaction verifies one typed criterion and appends its evidence record.
func (r *Repository) verifyTaskCompletionCriterion(ctx context.Context, change models.TaskCompletionEvidenceChange, exact bool) (*models.TaskCompletionGateSnapshot, bool, error) {
	if change.CriterionID == "" || !validEvidenceSubject(change.Evidence.Subject) {
		return nil, false, repoerrors.ErrTaskCompletionEvidenceChanged
	}
	if exact && (!validCompletionGateOperation(change.OperationID, change.PayloadDigest, change.ExpectedTaskResourceVersion) ||
		!validExactCompletionGateActor(change.TaskID, change.WorkspaceID, change.ActorKind, change.ActorID, change.ClaimFence)) {
		return nil, false, errors.New("exact completion evidence operation identity is incomplete")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if exact {
		replayed, found, err := r.replayCompletionGateOperationTx(ctx, tx, change.OperationID, change.WorkspaceID, change.TaskID, "verify_evidence", change.PayloadDigest)
		if err != nil {
			return nil, false, err
		}
		if found {
			if err := tx.Commit(); err != nil {
				return nil, false, err
			}
			return replayed, true, nil
		}
	}
	workspaceID, err := r.lockCompletionTaskForChange(ctx, tx, change.TaskID, change.WorkspaceID, change.ExpectedTaskResourceVersion, change.ClaimFence, exact)
	if err != nil {
		return nil, false, err
	}
	if workspaceID != change.WorkspaceID {
		return nil, false, repoerrors.ErrTaskVersionConflict
	}
	snapshot, err := r.readTaskCompletionGateTx(ctx, tx, change.TaskID, false)
	if err != nil {
		return nil, false, err
	}
	if snapshot.Revision != change.ExpectedRevision {
		return nil, false, repoerrors.ErrTaskCompletionCriteriaConflict
	}
	var criterion models.TaskCompletionCriterion
	for _, candidate := range snapshot.Criteria {
		if candidate.ID == change.CriterionID {
			criterion = candidate
			break
		}
	}
	if criterion.ID == "" || criterion.EvidenceSubject.Kind != change.Evidence.Subject.Kind ||
		criterion.EvidenceSubject.ID != change.Evidence.Subject.ID {
		return nil, false, repoerrors.ErrTaskCompletionEvidenceChanged
	}
	current, err := r.evidenceSubjectCurrentTx(ctx, tx, change.TaskID, change.Evidence.Subject, false)
	if err != nil {
		return nil, false, err
	}
	if !current {
		return nil, false, repoerrors.ErrTaskCompletionEvidenceChanged
	}
	verifiedAt := r.nowUTC()
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_completion_criteria SET verified_revision = ?, evidence_kind = ?, evidence_id = ?,
			evidence_revision = ?, evidence_summary = ?, evidence_reference = ?, verifier_kind = ?, verifier_id = ?, verified_at = ?
		WHERE task_id = ? AND criterion_id = ? AND criterion_revision = ?
	`), criterion.CriterionRevision, change.Evidence.Subject.Kind, change.Evidence.Subject.ID,
		change.Evidence.Subject.Revision, strings.TrimSpace(change.Evidence.Summary),
		strings.TrimSpace(change.Evidence.Reference), change.ActorKind, change.ActorID,
		verifiedAt, change.TaskID, change.CriterionID, criterion.CriterionRevision); err != nil {
		return nil, false, err
	}
	details, _ := json.Marshal(change.Evidence)
	if err := insertCompletionHistoryTx(ctx, tx, r.db.Rebind, models.TaskCompletionGateHistory{
		ID: uuid.NewString(), TaskID: change.TaskID, WorkspaceID: change.WorkspaceID,
		Revision: snapshot.Revision, Action: "criterion_verified", ActorKind: change.ActorKind,
		ActorID: change.ActorID, Details: string(details), CreatedAt: verifiedAt,
	}); err != nil {
		return nil, false, err
	}
	result, err := r.readTaskCompletionGateTx(ctx, tx, change.TaskID, false)
	if err != nil {
		return nil, false, err
	}
	if exact {
		if err := r.recordCompletionGateOperationTx(ctx, tx, change.OperationID, change.WorkspaceID, change.TaskID, "verify_evidence", change.PayloadDigest, result, verifiedAt); err != nil {
			return nil, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return result, false, nil
}

func (r *Repository) GetTaskCompletionGate(ctx context.Context, taskID string) (*models.TaskCompletionGateSnapshot, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	snapshot, err := r.readTaskCompletionGateTx(ctx, tx, taskID, false)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (r *Repository) ListTaskCompletionGateHistory(ctx context.Context, taskID string) ([]*models.TaskCompletionGateHistory, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT id, task_id, workspace_id, revision, action, actor_kind, actor_id, reason, details, created_at
		FROM task_completion_gate_history WHERE task_id = ? ORDER BY created_at, id
	`), taskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	history := make([]*models.TaskCompletionGateHistory, 0)
	for rows.Next() {
		item := new(models.TaskCompletionGateHistory)
		if err := rows.Scan(&item.ID, &item.TaskID, &item.WorkspaceID, &item.Revision,
			&item.Action, &item.ActorKind, &item.ActorID, &item.Reason, &item.Details, &item.CreatedAt); err != nil {
			return nil, err
		}
		history = append(history, item)
	}
	return history, rows.Err()
}

//nolint:cyclop // The transaction checks completion gates before allowing a terminal state transition.
func (r *Repository) guardTaskCompletionTransitionTx(ctx context.Context, tx *sql.Tx, taskID string, currentState, nextState v1.TaskState, currentWorkflowID, currentStepID, targetWorkflowID, targetStepID string) error {
	if nextState != v1.TaskStateCompleted || currentState == v1.TaskStateCompleted {
		return nil
	}
	snapshot, err := r.readTaskCompletionGateTx(ctx, tx, taskID, true)
	if err != nil {
		return err
	}
	if len(snapshot.Criteria) == 0 || len(snapshot.Blockers) == 0 {
		return nil
	}
	override, ok := models.TaskCompletionMoveOverrideFromContext(ctx)
	if !ok {
		return repoerrors.ErrTaskCompletionGateBlocked
	}
	if override.TaskID != taskID || override.WorkspaceID != snapshot.WorkspaceID ||
		override.ExpectedRevision != snapshot.Revision ||
		override.SourceWorkflowID != currentWorkflowID || override.SourceStepID != currentStepID ||
		override.TargetWorkflowID != targetWorkflowID || override.TargetStepID != targetStepID ||
		strings.TrimSpace(override.ActorID) == "" || strings.TrimSpace(override.Reason) == "" {
		return repoerrors.ErrTaskCompletionCriteriaConflict
	}
	now := r.nowUTC()
	details, _ := json.Marshal(snapshot.Blockers)
	return insertCompletionHistoryTx(ctx, tx, r.db.Rebind, models.TaskCompletionGateHistory{
		ID: uuid.NewString(), TaskID: taskID, WorkspaceID: snapshot.WorkspaceID,
		Revision: snapshot.Revision, Action: "completion_overridden", ActorKind: completionGateActorHuman,
		ActorID: override.ActorID, Reason: strings.TrimSpace(override.Reason),
		Details: string(details), CreatedAt: now,
	})
}

//nolint:cyclop // The snapshot reads criteria, evidence, and actor-visible state together.
func (r *Repository) readTaskCompletionGateTx(ctx context.Context, tx *sql.Tx, taskID string, lockEvidence bool) (*models.TaskCompletionGateSnapshot, error) {
	snapshot := &models.TaskCompletionGateSnapshot{TaskID: taskID, Criteria: []models.TaskCompletionCriterion{}, Blockers: []models.TaskCompletionBlocker{}}
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT workspace_id FROM tasks WHERE id = ?`), taskID).Scan(&snapshot.WorkspaceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
		}
		return nil, err
	}
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT revision FROM task_completion_sets WHERE task_id = ?`), taskID).Scan(&snapshot.Revision); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, r.db.Rebind(`
		SELECT criterion_id, description, criterion_revision, subject_kind, subject_id,
			verified_revision, evidence_kind, evidence_id, evidence_revision, evidence_summary,
			evidence_reference, verifier_kind, verifier_id, verified_at
		FROM task_completion_criteria WHERE task_id = ? ORDER BY criterion_id
	`), taskID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var item models.TaskCompletionCriterion
		var evidenceKind, evidenceID, evidenceRevision, evidenceSummary, evidenceReference string
		var verifiedAt sql.NullTime
		if err := rows.Scan(&item.ID, &item.Description, &item.CriterionRevision,
			&item.EvidenceSubject.Kind, &item.EvidenceSubject.ID, &item.VerifiedRevision,
			&evidenceKind, &evidenceID, &evidenceRevision, &evidenceSummary, &evidenceReference,
			&item.VerifierKind, &item.VerifierID, &verifiedAt); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if evidenceKind != "" {
			item.Evidence = &models.TaskCompletionEvidence{
				Subject: models.TaskCompletionEvidenceSubject{Kind: evidenceKind, ID: evidenceID, Revision: evidenceRevision},
				Summary: evidenceSummary, Reference: evidenceReference,
			}
		}
		if verifiedAt.Valid {
			item.VerifiedAt = &verifiedAt.Time
		}
		snapshot.Criteria = append(snapshot.Criteria, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for _, criterion := range snapshot.Criteria {
		blocked, err := r.completionCriterionBlockedTx(ctx, tx, taskID, criterion, lockEvidence)
		if err != nil {
			return nil, err
		}
		if blocked {
			var reason string
			if criterion.VerifiedRevision != criterion.CriterionRevision || criterion.Evidence == nil {
				reason = "unverified"
			} else {
				reason = "evidence_stale"
			}
			snapshot.Blockers = append(snapshot.Blockers, models.TaskCompletionBlocker{CriterionID: criterion.ID, Reason: reason})
		}
	}
	snapshot.Blocked = len(snapshot.Criteria) > 0 && len(snapshot.Blockers) > 0
	return snapshot, nil
}

func (r *Repository) completionCriterionBlockedTx(ctx context.Context, tx *sql.Tx, taskID string, criterion models.TaskCompletionCriterion, lockEvidence ...bool) (bool, error) {
	if criterion.VerifiedRevision != criterion.CriterionRevision || criterion.Evidence == nil {
		return true, nil
	}
	lock := len(lockEvidence) > 0 && lockEvidence[0]
	current, err := r.evidenceSubjectCurrentTx(ctx, tx, taskID, criterion.Evidence.Subject, lock)
	return !current, err
}

func (r *Repository) evidenceSubjectCurrentTx(ctx context.Context, tx *sql.Tx, taskID string, subject models.TaskCompletionEvidenceSubject, lock bool) (bool, error) {
	if !validEvidenceSubject(subject) {
		return false, nil
	}
	switch subject.Kind {
	case models.TaskCompletionEvidenceTaskRevision:
		var updatedAt time.Time
		if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT updated_at FROM tasks WHERE id = ?`), taskID).Scan(&updatedAt); err != nil {
			return false, err
		}
		observed, err := time.Parse(time.RFC3339Nano, subject.Revision)
		return err == nil && observed.Equal(updatedAt), nil
	case models.TaskCompletionEvidenceGitHubPRHead:
		query := `SELECT head_sha FROM github_task_prs WHERE id = ? AND task_id = ?`
		if lock && dialect.IsPostgres(r.db.DriverName()) {
			query += forUpdateClause
		}
		var head string
		if err := tx.QueryRowContext(ctx, r.db.Rebind(query), subject.ID, taskID).Scan(&head); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return false, nil
			}
			return false, err
		}
		return head == subject.Revision, nil
	case models.TaskCompletionEvidenceExecution:
		var exists int
		err := tx.QueryRowContext(ctx, r.db.Rebind(`
			SELECT 1 FROM task_session_turns WHERE id = ? AND task_id = ? AND completed_at IS NOT NULL
		`), subject.ID, taskID).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return err == nil && subject.Revision == subject.ID, err
	case models.TaskCompletionEvidenceArtifact:
		// Artifact revisions are immutable host-issued identifiers. The exact
		// artifact query that issued one owns its lifetime; evidence stores that
		// revision rather than dereferencing a plugin during completion.
		return subject.Revision != "", nil
	default:
		return false, nil
	}
}

//nolint:nestif // The no-op task update establishes portable row serialization for the gate check.
func (r *Repository) lockCompletionTask(ctx context.Context, tx *sql.Tx, taskID string) (string, error) {
	query := `SELECT workspace_id FROM tasks WHERE id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += forUpdateClause
	} else {
		if result, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE tasks SET updated_at = updated_at WHERE id = ?`), taskID); err != nil {
			return "", err
		} else if rows, err := result.RowsAffected(); err != nil || rows != 1 {
			if err != nil {
				return "", err
			}
			return "", fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
		}
	}
	var workspaceID string
	if err := tx.QueryRowContext(ctx, r.db.Rebind(query), taskID).Scan(&workspaceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
		}
		return "", err
	}
	return workspaceID, nil
}

func (r *Repository) lockCompletionTaskForChange(
	ctx context.Context,
	tx *sql.Tx,
	taskID, workspaceID, expectedTaskVersion string,
	fence models.TaskManagementClaimFence,
	exact bool,
) (string, error) {
	if !exact {
		return r.lockCompletionTask(ctx, tx, taskID)
	}
	expected, err := time.Parse(time.RFC3339Nano, expectedTaskVersion)
	if err != nil {
		return "", repoerrors.ErrTaskVersionConflict
	}
	query := completionGateTaskVersionQuery
	if dialect.IsPostgres(r.db.DriverName()) {
		query += forUpdateClause
	}
	var currentWorkspaceID string
	var currentVersion time.Time
	if err := tx.QueryRowContext(ctx, r.db.Rebind(query), taskID).Scan(&currentWorkspaceID, &currentVersion); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("%w: %s", repoerrors.ErrTaskNotFound, taskID)
		}
		return "", err
	}
	if currentWorkspaceID != workspaceID || !currentVersion.Equal(expected) {
		return "", repoerrors.ErrTaskVersionConflict
	}
	if !dialect.IsPostgres(r.db.DriverName()) {
		result, err := tx.ExecContext(ctx, r.db.Rebind(`
			UPDATE tasks SET updated_at = updated_at
			WHERE id = ? AND workspace_id = ? AND updated_at = ?
		`), taskID, workspaceID, currentVersion)
		if err != nil {
			return "", err
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			if err != nil {
				return "", err
			}
			return "", repoerrors.ErrTaskVersionConflict
		}
	}
	if err := r.checkTaskManagementClaimFence(ctx, tx, taskID, fence); err != nil {
		return "", err
	}
	return currentWorkspaceID, nil
}

func validCompletionGateOperation(operationID, payloadDigest, expectedTaskVersion string) bool {
	if strings.TrimSpace(operationID) == "" || len(payloadDigest) != 71 || !strings.HasPrefix(payloadDigest, "sha256:") {
		return false
	}
	_, err := time.Parse(time.RFC3339Nano, expectedTaskVersion)
	return err == nil
}

func validExactCompletionGateActor(
	taskID, workspaceID, actorKind, actorID string,
	fence models.TaskManagementClaimFence,
) bool {
	return strings.TrimSpace(taskID) != "" && strings.TrimSpace(workspaceID) != "" &&
		actorKind == completionGateActorPlugin && fence.InstallationID != "" && actorID == "plugin:"+fence.InstallationID &&
		fence.Generation >= 0 && (fence.Generation == 0 || strings.TrimSpace(fence.InstanceKey) != "")
}

func (r *Repository) replayCompletionGateOperationTx(
	ctx context.Context,
	tx *sql.Tx,
	operationID, workspaceID, taskID, action, payloadDigest string,
) (*models.TaskCompletionGateSnapshot, bool, error) {
	var storedWorkspaceID, storedTaskID, storedAction, storedDigest, resultJSON string
	err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT workspace_id, task_id, action, payload_digest, result_json
		FROM task_completion_gate_operations WHERE operation_id = ?
	`), operationID).Scan(&storedWorkspaceID, &storedTaskID, &storedAction, &storedDigest, &resultJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if storedWorkspaceID != workspaceID || storedTaskID != taskID || storedAction != action || storedDigest != payloadDigest {
		return nil, false, repoerrors.ErrTaskOperationConflict
	}
	var snapshot models.TaskCompletionGateSnapshot
	if err := json.Unmarshal([]byte(resultJSON), &snapshot); err != nil {
		return nil, false, fmt.Errorf("decode task completion operation result: %w", err)
	}
	return &snapshot, true, nil
}

func (r *Repository) recordCompletionGateOperationTx(
	ctx context.Context,
	tx *sql.Tx,
	operationID, workspaceID, taskID, action, payloadDigest string,
	snapshot *models.TaskCompletionGateSnapshot,
	createdAt time.Time,
) error {
	result, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode task completion operation result: %w", err)
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_completion_gate_operations (
			operation_id, workspace_id, task_id, action, payload_digest, result_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`), operationID, workspaceID, taskID, action, payloadDigest, string(result), createdAt)
	return err
}

func insertCompletionHistoryTx(ctx context.Context, tx *sql.Tx, rebind func(string) string, history models.TaskCompletionGateHistory) error {
	_, err := tx.ExecContext(ctx, rebind(`
		INSERT INTO task_completion_gate_history (
			id, task_id, workspace_id, revision, action, actor_kind, actor_id, reason, details, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), history.ID, history.TaskID, history.WorkspaceID, history.Revision, history.Action,
		history.ActorKind, history.ActorID, history.Reason, history.Details, history.CreatedAt)
	return err
}

func validateCompletionCriteria(criteria []models.TaskCompletionCriterion) error {
	if len(criteria) > maxCompletionCriteria {
		return fmt.Errorf("a task can have at most %d completion criteria", maxCompletionCriteria)
	}
	seen := make(map[string]struct{}, len(criteria))
	for _, criterion := range criteria {
		id := strings.TrimSpace(criterion.ID)
		description := strings.TrimSpace(criterion.Description)
		if id == "" || len(id) > 128 || description == "" || len(description) > 2000 || !validRequiredEvidenceSubject(criterion.EvidenceSubject) {
			return fmt.Errorf("completion criterion is invalid")
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("completion criterion IDs must be unique")
		}
		seen[id] = struct{}{}
	}
	return nil
}

func validRequiredEvidenceSubject(subject models.TaskCompletionEvidenceSubject) bool {
	return validEvidenceSubject(models.TaskCompletionEvidenceSubject{Kind: subject.Kind, ID: subject.ID, Revision: "immutable"})
}

func validEvidenceSubject(subject models.TaskCompletionEvidenceSubject) bool {
	if strings.TrimSpace(subject.ID) == "" || len(subject.ID) > 256 || len(subject.Revision) > 256 {
		return false
	}
	switch subject.Kind {
	case models.TaskCompletionEvidenceTaskRevision, models.TaskCompletionEvidenceExecution,
		models.TaskCompletionEvidenceArtifact, models.TaskCompletionEvidenceGitHubPRHead:
		return true
	default:
		return false
	}
}

func completionCriterionEquivalent(previous, next models.TaskCompletionCriterion) bool {
	return previous.Description == strings.TrimSpace(next.Description) &&
		previous.EvidenceSubject.Kind == strings.TrimSpace(next.EvidenceSubject.Kind) &&
		previous.EvidenceSubject.ID == strings.TrimSpace(next.EvidenceSubject.ID)
}

func evidenceField(evidence *models.TaskCompletionEvidence, selectValue func(models.TaskCompletionEvidence) string) string {
	if evidence == nil {
		return ""
	}
	return selectValue(*evidence)
}
