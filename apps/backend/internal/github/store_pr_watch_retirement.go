package github

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
)

// RetirePRWatch records explicit retirement without changing task-owned PR
// history. Repeating the operation preserves the first retirement timestamp.
func (s *Store) RetirePRWatch(ctx context.Context, id string, retiredAt time.Time) (*PRWatch, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin PR watch retirement: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockPRWatchID(ctx, tx, s.db.DriverName(), id); err != nil {
		return nil, err
	}
	watch, err := getPRWatchInTransaction(ctx, tx, id)
	if err != nil || watch == nil {
		return watch, commitPRWatchTransaction(tx, err)
	}
	if err := lockPRWatchIdentity(ctx, tx, s.db.DriverName(), watch); err != nil {
		return nil, err
	}
	if watch.RetiredAt == nil {
		if retiredAt.IsZero() {
			retiredAt = time.Now().UTC()
		}
		if _, err := tx.ExecContext(ctx, tx.Rebind(`
			UPDATE github_pr_watches SET retired_at = ?, updated_at = ?
			WHERE id = ? AND retired_at IS NULL`), retiredAt, retiredAt, id); err != nil {
			return nil, fmt.Errorf("retire PR watch: %w", err)
		}
		watch.RetiredAt = &retiredAt
		watch.UpdatedAt = retiredAt
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit PR watch retirement: %w", err)
	}
	return watch, nil
}

// ResumePRWatch replaces a retired row with a new active lifetime. A live
// watch is returned unchanged, and an unknown ID returns nil.
func (s *Store) ResumePRWatch(ctx context.Context, id string, resumedAt time.Time) (*PRWatch, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin PR watch resumption: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockPRWatchID(ctx, tx, s.db.DriverName(), id); err != nil {
		return nil, err
	}
	watch, err := getPRWatchInTransaction(ctx, tx, id)
	if err != nil || watch == nil {
		return watch, commitPRWatchTransaction(tx, err)
	}
	if watch.RetiredAt == nil {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit unchanged PR watch resumption: %w", err)
		}
		return watch, nil
	}
	if err := lockPRWatchIdentity(ctx, tx, s.db.DriverName(), watch); err != nil {
		return nil, err
	}
	if resumedAt.IsZero() {
		resumedAt = time.Now().UTC()
	}
	resumed := *watch
	resumed.ID = uuid.NewString()
	resumed.RetiredAt = nil
	resumed.LastCheckedAt = nil
	resumed.LastCommentAt = nil
	resumed.CreatedAt = resumedAt
	resumed.UpdatedAt = resumedAt
	if _, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM github_pr_watches WHERE id = ? AND retired_at IS NOT NULL`), id); err != nil {
		return nil, fmt.Errorf("remove retired PR watch lifetime: %w", err)
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO github_pr_watches (
			github_host, id, workspace_id, session_id, task_id, repository_id, owner, repo,
			pr_number, branch, last_checked_at, last_comment_at,
			last_check_status, last_review_state, retired_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		resumed.GitHubHost, resumed.ID, resumed.WorkspaceID, resumed.SessionID, resumed.TaskID,
		resumed.RepositoryID,
		resumed.Owner, resumed.Repo, resumed.PRNumber, resumed.Branch, resumed.LastCheckedAt,
		resumed.LastCommentAt, resumed.LastCheckStatus, resumed.LastReviewState, nil,
		resumed.CreatedAt, resumed.UpdatedAt); err != nil {
		return nil, fmt.Errorf("create resumed PR watch lifetime: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit PR watch resumption: %w", err)
	}
	return &resumed, nil
}

// ApplyPRWatchResult admits a provider result only while the same watch ID is
// present and active. The callback must perform database work through tx only;
// provider requests must finish before this method is called.
func (s *Store) ApplyPRWatchResult(
	ctx context.Context,
	id string,
	apply func(tx *sqlx.Tx, current *PRWatch) error,
) (bool, error) {
	if apply == nil {
		return false, errors.New("apply PR watch result callback is required")
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin PR watch result: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockPRWatchID(ctx, tx, s.db.DriverName(), id); err != nil {
		return false, err
	}
	watch, err := getPRWatchInTransaction(ctx, tx, id)
	if err != nil || watch == nil {
		return false, commitPRWatchTransaction(tx, err)
	}
	if err := lockPRWatchIdentity(ctx, tx, s.db.DriverName(), watch); err != nil {
		return false, err
	}
	if watch.RetiredAt != nil {
		if err := tx.Commit(); err != nil {
			return false, fmt.Errorf("commit rejected PR watch result: %w", err)
		}
		return false, nil
	}
	retiredBranch, err := hasRetiredPRWatchByTaskRepoBranchTx(
		ctx, tx, watch.TaskID, watch.RepositoryID, watch.Branch, watch.ID,
	)
	if err != nil {
		return false, err
	}
	if retiredBranch {
		if err := tx.Commit(); err != nil {
			return false, fmt.Errorf("commit blocked PR watch result: %w", err)
		}
		return false, nil
	}
	if err := apply(tx, watch); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit PR watch result: %w", err)
	}
	return true, nil
}

func hasRetiredPRWatchByTaskRepoBranchTx(
	ctx context.Context, tx *sqlx.Tx, taskID, repositoryID, branch, excludeID string,
) (bool, error) {
	var retired bool
	err := tx.GetContext(ctx, &retired, tx.Rebind(`
		SELECT EXISTS(SELECT 1 FROM github_pr_watches
			WHERE task_id = ? AND repository_id = ? AND branch = ? AND retired_at IS NOT NULL AND id <> ?)`),
		taskID, repositoryID, branch, excludeID)
	return retired, err
}

// DeleteActivePRWatchesByTaskID removes watches discarded by task archival
// while preserving explicit retirement rows for a later unarchive.
func (s *Store) DeleteActivePRWatchesByTaskID(ctx context.Context, taskID string) (int64, error) {
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		DELETE FROM github_pr_watches WHERE task_id = ? AND retired_at IS NULL`), taskID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func lockPRWatchID(ctx context.Context, tx *sqlx.Tx, driver, id string) error {
	if !dialect.IsPostgres(driver) {
		if _, err := tx.ExecContext(ctx,
			tx.Rebind(`UPDATE github_pr_watches SET retired_at = retired_at WHERE id = ?`), id); err != nil {
			return fmt.Errorf("acquire SQLite PR watch writer transaction: %w", err)
		}
		return nil
	}
	return acquirePRWatchAdvisoryLock(ctx, tx, "github-pr-watch-id:"+id)
}

func lockPRWatchIdentity(ctx context.Context, tx *sqlx.Tx, driver string, watch *PRWatch) error {
	return lockPRWatchIdentities(ctx, tx, driver, watch)
}

func lockPRWatchIdentities(ctx context.Context, tx *sqlx.Tx, driver string, watches ...*PRWatch) error {
	if !dialect.IsPostgres(driver) {
		return nil
	}
	keySet := make(map[string]struct{}, len(watches)*2)
	for _, watch := range watches {
		if watch == nil {
			continue
		}
		keySet[fmt.Sprintf("github-pr-watch-branch:%q:%q:%q", watch.TaskID, watch.RepositoryID, watch.Branch)] = struct{}{}
		if watch.PRNumber != 0 {
			keySet[fmt.Sprintf("github-pr-watch-pr:%q:%q:%d", watch.TaskID, watch.RepositoryID, watch.PRNumber)] = struct{}{}
		}
	}
	keys := make([]string, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := acquirePRWatchAdvisoryLock(ctx, tx, key); err != nil {
			return err
		}
	}
	return nil
}

func acquirePRWatchAdvisoryLock(ctx context.Context, tx *sqlx.Tx, key string) error {
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key); err != nil {
		return fmt.Errorf("acquire PR watch identity lock: %w", err)
	}
	return nil
}

func getPRWatchInTransaction(ctx context.Context, tx *sqlx.Tx, id string) (*PRWatch, error) {
	var watch PRWatch
	err := tx.GetContext(ctx, &watch, tx.Rebind(`SELECT * FROM github_pr_watches WHERE id = ?`), id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load PR watch %q: %w", id, err)
	}
	return &watch, nil
}

func commitPRWatchTransaction(tx *sqlx.Tx, operationErr error) error {
	if operationErr != nil {
		return operationErr
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit empty PR watch transaction: %w", err)
	}
	return nil
}
