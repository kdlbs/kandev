package github

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

var (
	ErrTaskPRRegistrationIdentityUnresolved = errors.New("GitHub PR association identity is unresolved")
	ErrTaskPRRegistrationIdentityConflict   = errors.New("GitHub PR association identity conflicts with the current repository")
	ErrTaskPRRegistrationDetached           = errors.New("GitHub PR association is detached; explicit restoration is required")
	ErrTaskPRRegistrationRevisionChanged    = errors.New("GitHub PR association revision changed")
)

// GetTaskPRByRegistrationIdentityIncludingDetached finds the canonical task
// association independently of a local repository locator.
func (s *Store) GetTaskPRByRegistrationIdentityIncludingDetached(
	ctx context.Context, taskID, providerHost, connectionScope, providerRepositoryID string, prNumber int,
) (*TaskPR, error) {
	var existing TaskPR
	err := s.ro.GetContext(ctx, &existing, s.ro.Rebind(`
		SELECT `+taskPRColumns+` FROM github_task_prs
		WHERE task_id = ? AND provider_host = ? AND connection_scope = ?
		  AND provider_repository_id = ? AND pr_number = ? LIMIT 1`),
		taskID, providerHost, connectionScope, providerRepositoryID, prNumber)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err == nil {
		err = hydrateTaskPRWorkflowAttention(&existing)
	}
	return &existing, err
}

// RegisterTaskPRObservation stores a comparison-preserving agent registration.
// expectedRevision is zero when the provider read began without an association.
func (s *Store) RegisterTaskPRObservation(
	ctx context.Context, registration *TaskPR, restoreDetached bool, expectedRevision int64,
) (*TaskPR, bool, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	registered, changed, err := s.RegisterTaskPRObservationTx(ctx, tx, registration, restoreDetached, expectedRevision)
	if err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return registered, changed, nil
}

// RegisterTaskPRObservationTx commits through a caller-owned transaction so a
// durable registration intent can share the association's revision fence.
func (s *Store) RegisterTaskPRObservationTx(
	ctx context.Context, tx *sqlx.Tx, registration *TaskPR, restoreDetached bool, expectedRevision int64,
) (*TaskPR, bool, error) {
	if err := validateTaskPRRegistration(registration); err != nil {
		return nil, false, err
	}
	if tx == nil {
		return nil, false, errors.New("GitHub registration transaction is required")
	}

	existing, err := getTaskPRForRegistration(ctx, tx, registration)
	if errors.Is(err, sql.ErrNoRows) {
		if expectedRevision != 0 {
			return nil, false, ErrTaskPRRegistrationRevisionChanged
		}
		return s.insertTaskPRObservationTx(ctx, tx, registration)
	}
	if err != nil {
		return nil, false, fmt.Errorf("read current GitHub PR association: %w", err)
	}
	return s.updateTaskPRObservationTx(ctx, tx, registration, existing, restoreDetached, expectedRevision)
}

func (s *Store) insertTaskPRObservationTx(
	ctx context.Context,
	tx *sqlx.Tx,
	registration *TaskPR,
) (*TaskPR, bool, error) {
	registration.ID = uuid.NewString()
	registration.CreatedAt = time.Now().UTC()
	registration.UpdatedAt = registration.CreatedAt
	if registration.Source == "" {
		registration.Source = TaskPRSourceAgentRegistration
	}
	if registration.Source != TaskPRSourceAgentRegistration && registration.Source != TaskPRSourceSelectedMention {
		return nil, false, errors.New("GitHub observation source is invalid")
	}
	registration.ComparisonPolicy = TaskPRComparisonPolicyPreserve
	registration.AssociationRevision = 1
	registration.WorkflowAttentionJSON = marshalWorkflowAttention(registration.WorkflowAttention)
	if err := insertTaskPR(ctx, tx, registration); err != nil {
		return nil, false, fmt.Errorf("insert observation-only GitHub PR association: %w", err)
	}
	if err := ensureTaskPRRegistrationAutomationDisabled(ctx, tx, registration); err != nil {
		return nil, false, err
	}
	return registration, true, nil
}

func (s *Store) updateTaskPRObservationTx(
	ctx context.Context,
	tx *sqlx.Tx,
	registration, existing *TaskPR,
	restoreDetached bool,
	expectedRevision int64,
) (*TaskPR, bool, error) {
	if strings.TrimSpace(existing.ProviderRepositoryID) == "" || strings.TrimSpace(existing.ConnectionScope) == "" {
		return nil, false, ErrTaskPRRegistrationIdentityUnresolved
	}
	if !sameTaskPRRegistrationIdentity(existing, registration) {
		return nil, false, ErrTaskPRRegistrationIdentityConflict
	}
	if existing.DetachedAt == nil {
		return existing, false, nil
	}
	if !restoreDetached {
		return existing, false, ErrTaskPRRegistrationDetached
	}
	if existing.AssociationRevision != expectedRevision {
		return existing, false, ErrTaskPRRegistrationRevisionChanged
	}
	return s.restoreTaskPRObservationTx(ctx, tx, registration, existing, expectedRevision)
}

func (s *Store) restoreTaskPRObservationTx(
	ctx context.Context,
	tx *sqlx.Tx,
	registration, existing *TaskPR,
	expectedRevision int64,
) (*TaskPR, bool, error) {
	registration.ID = existing.ID
	registration.CreatedAt = existing.CreatedAt
	registration.UpdatedAt = time.Now().UTC()
	registration.Source = existing.Source
	registration.FirstOriginKind = existing.FirstOriginKind
	registration.FirstOriginID = existing.FirstOriginID
	registration.FirstOriginRevision = existing.FirstOriginRevision
	registration.ComparisonPolicy = TaskPRComparisonPolicyPreserve
	registration.AssociationRevision = existing.AssociationRevision + 1
	registration.WorkflowAttentionJSON = marshalWorkflowAttention(registration.WorkflowAttention)
	result, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE github_task_prs SET owner = ?, repo = ?, pr_url = ?, pr_title = ?, head_branch = ?, base_branch = ?,
			head_sha = ?, author_login = ?, state = ?, mergeable_state = ?, has_merge_conflicts = ?,
			additions = ?, deletions = ?, merged_at = ?, closed_at = ?, last_synced_at = ?, detached_at = NULL,
			comparison_policy = ?, association_revision = ?, updated_at = ?, workflow_attention = ?
		WHERE id = ? AND detached_at IS NOT NULL AND association_revision = ?`),
		registration.Owner, registration.Repo, registration.PRURL, registration.PRTitle, registration.HeadBranch,
		registration.BaseBranch, registration.HeadSHA, registration.AuthorLogin, registration.State,
		registration.MergeableState, registration.HasMergeConflicts, registration.Additions, registration.Deletions,
		registration.MergedAt, registration.ClosedAt, registration.LastSyncedAt, registration.ComparisonPolicy,
		registration.AssociationRevision, registration.UpdatedAt, registration.WorkflowAttentionJSON,
		registration.ID, expectedRevision)
	if err != nil {
		return nil, false, fmt.Errorf("restore observation-only GitHub PR association: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, false, err
	}
	if rows != 1 {
		return existing, false, ErrTaskPRRegistrationRevisionChanged
	}
	if err := ensureTaskPRRegistrationAutomationDisabled(ctx, tx, registration); err != nil {
		return nil, false, err
	}
	return registration, true, nil
}

func getTaskPRForRegistration(ctx context.Context, tx *sqlx.Tx, registration *TaskPR) (*TaskPR, error) {
	var existing TaskPR
	if strings.TrimSpace(registration.ProviderRepositoryID) != "" {
		err := tx.GetContext(ctx, &existing, tx.Rebind(`
			SELECT `+taskPRColumns+` FROM github_task_prs
			WHERE task_id = ? AND provider_host = ? AND connection_scope = ?
			  AND provider_repository_id = ? AND pr_number = ? LIMIT 1`),
			registration.TaskID, registration.ProviderHost, registration.ConnectionScope,
			registration.ProviderRepositoryID, registration.PRNumber)
		if err == nil || !errors.Is(err, sql.ErrNoRows) {
			return &existing, err
		}
	}
	// A local locator can identify a legacy row that has no immutable provider
	// identity. It is returned only so registration can fail closed; it never
	// supplies identity for an automatic repair.
	err := tx.GetContext(ctx, &existing, tx.Rebind(
		`SELECT `+taskPRColumns+` FROM github_task_prs WHERE task_id = ? AND repository_id = ? AND pr_number = ? LIMIT 1`),
		registration.TaskID, registration.RepositoryID, registration.PRNumber)
	return &existing, err
}

func validateTaskPRRegistration(registration *TaskPR) error {
	if registration == nil || strings.TrimSpace(registration.TaskID) == "" || strings.TrimSpace(registration.RepositoryID) == "" ||
		registration.PRNumber <= 0 || strings.TrimSpace(registration.ProviderHost) == "" ||
		strings.TrimSpace(registration.ConnectionScope) == "" || strings.TrimSpace(registration.ProviderRepositoryID) == "" ||
		strings.TrimSpace(registration.RepositoryPath) == "" {
		return errors.New("GitHub PR registration requires exact repository identity")
	}
	return nil
}

func sameTaskPRRegistrationIdentity(existing, requested *TaskPR) bool {
	return existing.TaskID == requested.TaskID && existing.PRNumber == requested.PRNumber && strings.EqualFold(existing.ProviderHost, requested.ProviderHost) &&
		existing.ConnectionScope == requested.ConnectionScope &&
		existing.ProviderRepositoryID == requested.ProviderRepositoryID
}

func ensureTaskPRRegistrationAutomationDisabled(ctx context.Context, tx *sqlx.Tx, registration *TaskPR) error {
	now := time.Now().UTC()
	_, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO github_task_pr_automation_options (
			task_id, repository_id, pr_number, auto_fix_enabled, auto_merge_enabled,
			prompt_on_review_requested, prompt_on_merged, prompt_on_closed, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, repository_id, pr_number) DO NOTHING`),
		registration.TaskID, registration.RepositoryID, registration.PRNumber, false, false, false, false, false, now, now)
	if err != nil {
		return fmt.Errorf("persist disabled GitHub PR registration automation: %w", err)
	}
	return nil
}
