package gitlab

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

// RegisterTaskMRObservation commits an agent-created association without
// inheriting automation state or replacing an existing association's origin.
func (s *Store) RegisterTaskMRObservation(
	ctx context.Context,
	registration *TaskMR,
	restoreDetached bool,
	expectedRevision int64,
) (*TaskMR, bool, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	registered, changed, err := s.RegisterTaskMRObservationTx(ctx, tx, registration, restoreDetached, expectedRevision)
	if err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return registered, changed, nil
}

// RestoreDetachedTaskMR reactivates a manually linked detached association
// while preserving its source, origin, and automation metadata.
func (s *Store) RestoreDetachedTaskMR(
	ctx context.Context, registration *TaskMR, expectedRevision int64,
) (*TaskMR, error) {
	if err := validateDetachedTaskMRRestoreRequest(registration, expectedRevision); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	existing, err := getTaskMRByIDTx(ctx, tx, registration.ID)
	if err != nil {
		return nil, err
	}
	if err := validateDetachedTaskMRRestoreTarget(existing, registration, expectedRevision); err != nil {
		return nil, err
	}
	if err := restoreTaskMRObservationRowTx(ctx, tx, registration, existing.ID, expectedRevision); err != nil {
		return nil, err
	}
	restored, err := getTaskMRByIDTx(ctx, tx, existing.ID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return restored, nil
}

func validateDetachedTaskMRRestoreRequest(registration *TaskMR, expectedRevision int64) error {
	if registration == nil || registration.ID == "" || expectedRevision <= 0 {
		return ErrTaskMRRegistrationRevisionChanged
	}
	return nil
}

func validateDetachedTaskMRRestoreTarget(
	existing, registration *TaskMR, expectedRevision int64,
) error {
	if existing == nil || existing.DetachedAt == nil || existing.AssociationRevision != expectedRevision {
		return ErrTaskMRRegistrationRevisionChanged
	}
	if !sameDetachedTaskMRIdentity(existing, registration) {
		return ErrTaskMRRepositoryMismatch
	}
	return nil
}

func sameDetachedTaskMRIdentity(existing, registration *TaskMR) bool {
	return existing.TaskID == registration.TaskID && existing.RepositoryID == registration.RepositoryID &&
		existing.ProjectPath == registration.ProjectPath && existing.MRIID == registration.MRIID &&
		existing.Host != "" && sameConfiguredOrigin(existing.Host, registration.Host)
}

// RegisterTaskMRObservationTx commits through a caller-owned transaction so a
// durable registration intent can share the association's revision fence.
func (s *Store) RegisterTaskMRObservationTx(
	ctx context.Context,
	tx *sqlx.Tx,
	registration *TaskMR,
	restoreDetached bool,
	expectedRevision int64,
) (*TaskMR, bool, error) {
	if !validTaskMRObservation(registration) {
		return nil, false, ErrTaskMRRegistrationIdentityUnresolved
	}
	if tx == nil {
		return nil, false, errors.New("GitLab registration transaction is required")
	}
	if err := validateTaskMRRegistrationIdentity(ctx, tx, registration); err != nil {
		return nil, false, err
	}
	existing, err := getTaskMRByProviderIdentity(ctx, tx, registration)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		return s.registerExistingTaskMRObservationTx(ctx, tx, registration, existing, restoreDetached, expectedRevision)
	}
	return s.insertTaskMRObservationTx(ctx, tx, registration)
}

func validTaskMRObservation(registration *TaskMR) bool {
	return registration != nil && registration.TaskID != "" && registration.RepositoryID != "" &&
		registration.Host != "" && registration.ProjectPath != "" && registration.MRIID > 0 &&
		registration.ProviderRepositoryID != "" && registration.ConnectionScope != "" &&
		(registration.Source == TaskMRSourceAgentRegistration || registration.Source == TaskMRSourceSelectedMention) &&
		registration.ComparisonPolicy == TaskMRComparisonPolicyPreserve
}

func (s *Store) registerExistingTaskMRObservationTx(
	ctx context.Context,
	tx *sqlx.Tx,
	registration, existing *TaskMR,
	restoreDetached bool,
	expectedRevision int64,
) (*TaskMR, bool, error) {
	if existing.ProviderRepositoryID == "" || existing.ConnectionScope == "" ||
		existing.ProviderRepositoryID != registration.ProviderRepositoryID ||
		existing.ConnectionScope != registration.ConnectionScope {
		return nil, false, ErrTaskMRRegistrationIdentityUnresolved
	}
	if existing.DetachedAt == nil {
		updated, err := getTaskMRByIDTx(ctx, tx, existing.ID)
		return updated, false, err
	}
	if !restoreDetached {
		return nil, false, ErrTaskMRDetached
	}
	if expectedRevision <= 0 || existing.AssociationRevision != expectedRevision {
		return nil, false, ErrTaskMRRegistrationRevisionChanged
	}
	return s.restoreTaskMRObservationTx(ctx, tx, registration, existing.ID, expectedRevision)
}

func (s *Store) restoreTaskMRObservationTx(
	ctx context.Context,
	tx *sqlx.Tx,
	registration *TaskMR,
	existingID string,
	expectedRevision int64,
) (*TaskMR, bool, error) {
	if err := restoreTaskMRObservationRowTx(ctx, tx, registration, existingID, expectedRevision); err != nil {
		return nil, false, err
	}
	if err := ensureTaskMRAutomationOff(ctx, tx, registration); err != nil {
		return nil, false, err
	}
	updated, err := getTaskMRByIDTx(ctx, tx, existingID)
	return updated, true, err
}

func restoreTaskMRObservationRowTx(
	ctx context.Context,
	tx *sqlx.Tx,
	registration *TaskMR,
	existingID string,
	expectedRevision int64,
) error {
	result, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE gitlab_task_mrs SET detached_at = NULL, association_revision = association_revision + 1,
			repository_id = ?, host = ?, project_path = ?, repository_path = ?, mr_url = ?, mr_title = ?,
			head_branch = ?, head_sha = ?, base_branch = ?, base_sha = ?, author_username = ?, state = ?,
			approval_state = ?, pipeline_state = ?, merge_status = ?, draft = CASE WHEN ? THEN 1 ELSE 0 END, approval_count = ?,
			required_approvals = ?, pipeline_jobs_total = ?, pipeline_jobs_pass = ?, detailed_merge_status = ?,
		reviewer_count = ?, unapproved_reviewers = ?, created_at = ?, merged_at = ?, closed_at = ?,
		last_synced_at = ?, updated_at = ?
		WHERE id = ? AND detached_at IS NOT NULL AND association_revision = ?`),
		registration.RepositoryID, registration.Host, registration.ProjectPath, registration.RepositoryPath,
		registration.MRURL, registration.MRTitle, registration.HeadBranch, registration.HeadSHA,
		registration.BaseBranch, registration.BaseSHA, registration.AuthorUsername, registration.State,
		registration.ApprovalState, registration.PipelineState, registration.MergeStatus, registration.Draft,
		registration.ApprovalCount, registration.RequiredApprovals, registration.PipelineJobsTotal,
		registration.PipelineJobsPass, registration.DetailedMergeStatus, registration.ReviewerCount,
		registration.UnapprovedReviewers, registration.CreatedAt, registration.MergedAt, registration.ClosedAt,
		registration.LastSyncedAt, time.Now().UTC(), existingID, expectedRevision)
	if err != nil {
		return fmt.Errorf("restore detached GitLab association: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read restored GitLab association result: %w", err)
	}
	if rowsAffected != 1 {
		return ErrTaskMRRegistrationRevisionChanged
	}
	return nil
}

func (s *Store) insertTaskMRObservationTx(
	ctx context.Context, tx *sqlx.Tx, registration *TaskMR,
) (*TaskMR, bool, error) {
	if registration.ID == "" {
		registration.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if registration.CreatedAt.IsZero() {
		registration.CreatedAt = now
	}
	registration.UpdatedAt = now
	registration.AssociationRevision = 1
	if _, err := tx.NamedExecContext(ctx, `
		INSERT INTO gitlab_task_mrs (
			id, task_id, repository_id, host, project_path, mr_iid, mr_url, mr_title, head_branch, head_sha,
			base_branch, base_sha, author_username, state, approval_state, pipeline_state, merge_status, draft,
			approval_count, required_approvals, pipeline_jobs_total, pipeline_jobs_pass, detailed_merge_status,
			reviewer_count, unapproved_reviewers, unresolved_discussions, created_at, merged_at, closed_at,
			last_synced_at, updated_at, provider_repository_id, connection_scope, repository_path, source,
			comparison_policy, association_revision, first_origin_kind, first_origin_id, first_origin_revision, detached_at
		) VALUES (
			:id, :task_id, :repository_id, :host, :project_path, :mr_iid, :mr_url, :mr_title, :head_branch, :head_sha,
			:base_branch, :base_sha, :author_username, :state, :approval_state, :pipeline_state, :merge_status, CASE WHEN :draft THEN 1 ELSE 0 END,
			:approval_count, :required_approvals, :pipeline_jobs_total, :pipeline_jobs_pass, :detailed_merge_status,
			:reviewer_count, :unapproved_reviewers, :unresolved_discussions, :created_at, :merged_at, :closed_at,
			:last_synced_at, :updated_at, :provider_repository_id, :connection_scope, :repository_path, :source,
			:comparison_policy, :association_revision, :first_origin_kind, :first_origin_id, :first_origin_revision, NULL
		)`, registration); err != nil {
		return nil, false, fmt.Errorf("insert GitLab observation: %w", err)
	}
	if err := ensureTaskMRAutomationOff(ctx, tx, registration); err != nil {
		return nil, false, err
	}
	created, err := getTaskMRByIDTx(ctx, tx, registration.ID)
	if err != nil {
		return nil, false, err
	}
	return created, true, nil
}

func validateTaskMRRegistrationIdentity(ctx context.Context, tx *sqlx.Tx, registration *TaskMR) error {
	var identity taskMRRepositoryIdentityRow
	err := tx.GetContext(ctx, &identity, tx.Rebind(`
		SELECT COALESCE(r.provider, '') AS provider,
			COALESCE(r.provider_repo_id, '') AS provider_repo_id,
			COALESCE(r.provider_host, '') AS provider_host,
			COALESCE(r.provider_owner, '') AS provider_owner,
			COALESCE(r.provider_name, '') AS provider_name,
			COALESCE(r.remote_url, '') AS remote_url,
			COALESCE(r.local_path, '') AS local_path
		FROM task_repositories tr
		JOIN repositories r ON r.id = tr.repository_id
		JOIN tasks t ON t.id = tr.task_id
		WHERE tr.task_id = ? AND tr.repository_id = ? AND t.workspace_id = ? AND r.workspace_id = ?
		LIMIT 1`), registration.TaskID, registration.RepositoryID, strings.TrimPrefix(registration.ConnectionScope, "builtin:"),
		strings.TrimPrefix(registration.ConnectionScope, "builtin:"))
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTaskMRNotFound
	}
	if err != nil {
		return fmt.Errorf("load GitLab registration repository identity: %w", err)
	}
	if identity.ProviderRepoID == "" || identity.ProviderRepoID != registration.ProviderRepositoryID {
		return ErrTaskMRRegistrationIdentityUnresolved
	}
	wantHost, err := normalizeHostOrigin(registration.Host)
	if err != nil {
		return ErrTaskMRRegistrationIdentityUnresolved
	}
	wantProject := strings.Trim(registration.RepositoryPath, "/")
	if wantProject == "" {
		wantProject = strings.Trim(registration.ProjectPath, "/")
	}
	var candidates []taskMRIdentityCandidate
	if strings.EqualFold(identity.Provider, "gitlab") {
		if host, hostErr := normalizeHostOrigin(identity.Host); hostErr == nil {
			candidates = append(candidates, taskMRIdentityCandidate{host: host, project: strings.Trim(identity.Owner+"/"+identity.Name, "/")})
		}
	}
	if host, project := parseGitLabRemoteURLIdentity(identity.RemoteURL); host != "" {
		if normalized, hostErr := normalizeHostOrigin(host); hostErr == nil {
			candidates = append(candidates, taskMRIdentityCandidate{host: normalized, project: strings.Trim(project, "/")})
		}
	}
	for _, candidate := range candidates {
		if sameGitLabHost(candidate.host, wantHost) && strings.EqualFold(candidate.project, wantProject) &&
			strings.EqualFold(wantProject, strings.Trim(registration.ProjectPath, "/")) {
			return nil
		}
	}
	return ErrTaskMRRepositoryMismatch
}

func getTaskMRByProviderIdentity(ctx context.Context, tx *sqlx.Tx, registration *TaskMR) (*TaskMR, error) {
	var mr TaskMR
	err := tx.GetContext(ctx, &mr, tx.Rebind(`
		SELECT `+taskMRSelectCols+` FROM gitlab_task_mrs
		WHERE task_id = ? AND connection_scope = ? AND provider_repository_id = ? AND lower(host) = lower(?) AND mr_iid = ?
		LIMIT 1`), registration.TaskID, registration.ConnectionScope, registration.ProviderRepositoryID, registration.Host, registration.MRIID)
	if errors.Is(err, sql.ErrNoRows) {
		var legacy TaskMR
		err = tx.GetContext(ctx, &legacy, tx.Rebind(`
			SELECT `+taskMRSelectCols+` FROM gitlab_task_mrs
			WHERE task_id = ? AND repository_id = ? AND project_path = ? AND mr_iid = ? LIMIT 1`),
			registration.TaskID, registration.RepositoryID, registration.ProjectPath, registration.MRIID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if legacy.ProviderRepositoryID == "" || legacy.ConnectionScope == "" {
			return &legacy, nil
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &mr, nil
}

func ensureTaskMRAutomationOff(ctx context.Context, tx *sqlx.Tx, registration *TaskMR) error {
	now := time.Now().UTC()
	_, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO gitlab_task_mr_automation_options (
			task_id, repository_id, project_path, mr_iid, auto_fix_enabled, auto_merge_enabled,
			prompt_on_review_requested, prompt_on_merged, prompt_on_closed, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, repository_id, project_path, mr_iid) DO NOTHING`),
		registration.TaskID, registration.RepositoryID, registration.ProjectPath, registration.MRIID,
		false, false, false, false, false, now, now)
	if err != nil {
		return fmt.Errorf("initialize GitLab registration automation: %w", err)
	}
	return nil
}

func getTaskMRByIDTx(ctx context.Context, tx *sqlx.Tx, id string) (*TaskMR, error) {
	var mr TaskMR
	if err := tx.GetContext(ctx, &mr, tx.Rebind(`SELECT `+taskMRSelectCols+` FROM gitlab_task_mrs WHERE id = ?`), id); err != nil {
		return nil, err
	}
	return &mr, nil
}
