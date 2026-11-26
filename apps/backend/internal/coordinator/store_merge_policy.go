package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
	"github.com/kandev/kandev/internal/db/dialect"
)

const mergePolicySQL = `
	CREATE TABLE IF NOT EXISTS coordinator_merge_repos (
		coordinator_id TEXT NOT NULL,
		repository_id TEXT NOT NULL,
		policy TEXT NOT NULL,
		selected_by TEXT NOT NULL DEFAULT '',
		automatic_since_revision INTEGER NOT NULL DEFAULT 0,
		updated_at {{timestamp}} NOT NULL,
		PRIMARY KEY (coordinator_id, repository_id)
	);
	CREATE TABLE IF NOT EXISTS coordinator_merge_deps (
		coordinator_id TEXT NOT NULL,
		task_id TEXT NOT NULL,
		revision INTEGER NOT NULL,
		refs_json TEXT NOT NULL,
		updated_at {{timestamp}} NOT NULL,
		PRIMARY KEY (coordinator_id, task_id)
	);
	CREATE TABLE IF NOT EXISTS coordinator_merge_protection (
		workspace_id TEXT NOT NULL,
		repository_id TEXT NOT NULL,
		branch TEXT NOT NULL,
		pipeline TEXT NOT NULL,
		approvals TEXT NOT NULL,
		threads TEXT NOT NULL,
		observed_at {{timestamp}} NOT NULL,
		PRIMARY KEY (workspace_id, repository_id, branch)
	);
	CREATE TABLE IF NOT EXISTS coordinator_merge_audit (
		id TEXT PRIMARY KEY,
		coordinator_id TEXT NOT NULL,
		kind TEXT NOT NULL,
		actor TEXT NOT NULL DEFAULT '',
		subject TEXT NOT NULL,
		before_value TEXT NOT NULL DEFAULT '',
		after_value TEXT NOT NULL DEFAULT '',
		revision INTEGER NOT NULL,
		created_at {{timestamp}} NOT NULL
	);
`

func (s *Store) migrateMergePolicy() error {
	if _, err := s.db.Exec(dialect.MustRenderSchema(s.db.DriverName(), mergePolicySQL)); err != nil {
		return fmt.Errorf("coordinator merge policy schema: %w", err)
	}
	return nil
}

// loadRepoPolicies reads the coordinator's stored repository policies.
func (s *Store) loadRepoPolicies(ctx context.Context, q coordinatorExec, coordinatorID string) (map[string]RepoPolicyRow, error) {
	rows, err := q.QueryContext(ctx, s.db.Rebind(`SELECT repository_id, policy, selected_by, automatic_since_revision
		FROM coordinator_merge_repos WHERE coordinator_id = ?`), coordinatorID)
	if err != nil {
		return nil, fmt.Errorf("load repository policies: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]RepoPolicyRow{}
	for rows.Next() {
		var r RepoPolicyRow
		if err := rows.Scan(&r.RepositoryID, &r.Policy, &r.SelectedBy, &r.AutomaticSinceRevision); err != nil {
			return nil, fmt.Errorf("scan repository policy: %w", err)
		}
		out[r.RepositoryID] = r
	}
	return out, rows.Err()
}

// writeRepoPolicyPlan upserts the planned rows and appends their audits in the
// caller's transaction.
func (s *Store) writeRepoPolicyPlan(ctx context.Context, tx coordinatorExec, coordinatorID, actor string, plan policyPlan) error {
	now := s.now().UTC()
	for _, r := range plan.Rows {
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`INSERT INTO coordinator_merge_repos
			(coordinator_id, repository_id, policy, selected_by, automatic_since_revision, updated_at) VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT (coordinator_id, repository_id) DO UPDATE SET policy = excluded.policy,
			selected_by = excluded.selected_by, automatic_since_revision = excluded.automatic_since_revision, updated_at = excluded.updated_at`),
			coordinatorID, r.RepositoryID, string(r.Policy), r.SelectedBy, r.AutomaticSinceRevision, now); err != nil {
			return fmt.Errorf("write repository policy: %w", err)
		}
	}
	for _, a := range plan.Audits {
		if err := s.insertMergeAudit(ctx, tx, coordinatorID, actor, a); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) insertMergeAudit(ctx context.Context, tx coordinatorExec, coordinatorID, actor string, a PolicyAudit) error {
	if _, err := tx.ExecContext(ctx, s.db.Rebind(`INSERT INTO coordinator_merge_audit
		(id, coordinator_id, kind, actor, subject, before_value, after_value, revision, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		uuid.NewString(), coordinatorID, a.Kind, actor, a.Subject, a.Before, a.After, a.Revision, s.now().UTC()); err != nil {
		return fmt.Errorf("insert merge audit: %w", err)
	}
	return nil
}

// RepositoryName is one live workspace repository.
type RepositoryName struct {
	ID   string
	Name string
}

// liveRepositories lists the workspace's repositories that are not deleted,
// ordered by name then ID ascending.
func (s *Store) liveRepositories(ctx context.Context, q coordinatorExec, workspaceID string) ([]RepositoryName, error) {
	rows, err := q.QueryContext(ctx, s.db.Rebind(`SELECT id, name FROM repositories
		WHERE workspace_id = ? AND deleted_at IS NULL ORDER BY name ASC, id ASC`), workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list repositories: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []RepositoryName
	for rows.Next() {
		var r RepositoryName
		if err := rows.Scan(&r.ID, &r.Name); err != nil {
			return nil, fmt.Errorf("scan repository: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// loadDepSets reads every dependency set of the coordinator, keyed by card.
func (s *Store) loadDepSets(ctx context.Context, q coordinatorExec, coordinatorID string) (map[string]DepSet, error) {
	rows, err := q.QueryContext(ctx, s.db.Rebind(`SELECT task_id, revision, refs_json FROM coordinator_merge_deps WHERE coordinator_id = ?`), coordinatorID)
	if err != nil {
		return nil, fmt.Errorf("load dependency sets: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]DepSet{}
	for rows.Next() {
		var task, raw string
		var set DepSet
		if err := rows.Scan(&task, &set.Revision, &raw); err != nil {
			return nil, fmt.Errorf("scan dependency set: %w", err)
		}
		if err := json.Unmarshal([]byte(raw), &set.Refs); err != nil {
			return nil, fmt.Errorf("decode dependency set: %w", err)
		}
		out[task] = set
	}
	return out, rows.Err()
}

// writeDepSet replaces a card's set at the given revision in the caller's
// transaction.
func (s *Store) writeDepSet(ctx context.Context, tx coordinatorExec, coordinatorID, taskID string, set DepSet) error {
	raw, err := json.Marshal(set.Refs)
	if err != nil {
		return fmt.Errorf("encode dependency set: %w", err)
	}
	if _, err := tx.ExecContext(ctx, s.db.Rebind(`INSERT INTO coordinator_merge_deps (coordinator_id, task_id, revision, refs_json, updated_at)
		VALUES (?, ?, ?, ?, ?) ON CONFLICT (coordinator_id, task_id) DO UPDATE SET
		revision = excluded.revision, refs_json = excluded.refs_json, updated_at = excluded.updated_at`),
		coordinatorID, taskID, set.Revision, string(raw), s.now().UTC()); err != nil {
		return fmt.Errorf("write dependency set: %w", err)
	}
	return nil
}

// ProtectionObservation is one recorded protection read of a repository
// branch. Branch is empty for the repository's default branch.
type ProtectionObservation struct {
	RepositoryID string                 `json:"repository_id"`
	Branch       string                 `json:"branch"`
	Pipeline     mergestate.Enforcement `json:"pipeline"`
	Approvals    mergestate.Enforcement `json:"approvals"`
	Threads      mergestate.Enforcement `json:"threads"`
	ObservedAt   time.Time              `json:"observed_at"`
}

func (s *Store) writeProtection(ctx context.Context, o ProtectionObservation, workspaceID string) error {
	if _, err := s.db.ExecContext(ctx, s.db.Rebind(`INSERT INTO coordinator_merge_protection
		(workspace_id, repository_id, branch, pipeline, approvals, threads, observed_at) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (workspace_id, repository_id, branch) DO UPDATE SET pipeline = excluded.pipeline,
		approvals = excluded.approvals, threads = excluded.threads, observed_at = excluded.observed_at`),
		workspaceID, o.RepositoryID, o.Branch, string(o.Pipeline), string(o.Approvals), string(o.Threads), o.ObservedAt.UTC()); err != nil {
		return fmt.Errorf("write protection observation: %w", err)
	}
	return nil
}

// listProtection returns the workspace's protection observations ordered by
// repository ID then branch ascending.
func (s *Store) listProtection(ctx context.Context, workspaceID string) ([]ProtectionObservation, error) {
	rows, err := s.ro.QueryContext(ctx, s.ro.Rebind(`SELECT repository_id, branch, pipeline, approvals, threads, observed_at
		FROM coordinator_merge_protection WHERE workspace_id = ? ORDER BY repository_id ASC, branch ASC`), workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list protection: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ProtectionObservation
	for rows.Next() {
		var o ProtectionObservation
		if err := rows.Scan(&o.RepositoryID, &o.Branch, &o.Pipeline, &o.Approvals, &o.Threads, &o.ObservedAt); err != nil {
			return nil, fmt.Errorf("scan protection: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
