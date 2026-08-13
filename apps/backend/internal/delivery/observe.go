package delivery

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	dbutil "github.com/kandev/kandev/internal/db"
)

// CandidatePair is a (task, repository) pair eligible for evaluation.
type CandidatePair struct {
	TaskID       string
	RepositoryID string
}

// CandidateResult is the outcome of one candidacy pass: the resolvable
// pairs plus the two exclusion gauges spec "Candidate pairs" requires —
// each pair is excluded and counted, never silently dropped.
type CandidateResult struct {
	Pairs             []CandidatePair
	MissingRepository int
	MissingTask       int
}

type pairKey struct{ taskID, repositoryID string }

// candidateSourceQueries enumerates every table condition 1 of "Candidate
// pairs" names as a candidacy source. Each is queried independently and
// tolerantly of a missing table (dbutil.IsMissingTableError) so an
// integration whose store never initialized cannot break candidacy for
// every other source.
var candidateSourceQueries = []string{
	`SELECT task_id, repository_id FROM task_repositories WHERE repository_id != ''`,
	`SELECT task_id, repository_id FROM task_sessions WHERE repository_id != ''`,
	`SELECT task_id, repository_id FROM github_task_prs WHERE repository_id != ''`,
	`SELECT task_id, repository_id FROM gitlab_task_mrs WHERE repository_id != ''`,
	`SELECT task_id, repository_id FROM azure_devops_task_prs WHERE repository_id != ''`,
}

// Candidates computes candidacy per spec "Candidate pairs": a pair is a
// candidate only when its repository_id resolves to a repositories row
// AND its task_id resolves to a tasks row. This stage never reads
// task_delivery_ledger, so a missing ledger table cannot suppress it (see
// "Sweep selection predicate § When the ledger itself cannot be read").
func (r *Repository) Candidates(ctx context.Context) (CandidateResult, error) {
	raw, err := r.discoverCandidatePairs(ctx)
	if err != nil {
		return CandidateResult{}, err
	}
	repoIDs, err := r.existingIDs(ctx, "repositories")
	if err != nil {
		return CandidateResult{}, err
	}
	taskIDs, err := r.existingIDs(ctx, "tasks")
	if err != nil {
		return CandidateResult{}, err
	}

	var result CandidateResult
	for k := range raw {
		if _, ok := repoIDs[k.repositoryID]; !ok {
			result.MissingRepository++
			continue
		}
		if _, ok := taskIDs[k.taskID]; !ok {
			result.MissingTask++
			continue
		}
		result.Pairs = append(result.Pairs, CandidatePair{TaskID: k.taskID, RepositoryID: k.repositoryID})
	}
	return result, nil
}

func (r *Repository) discoverCandidatePairs(ctx context.Context) (map[pairKey]struct{}, error) {
	out := map[pairKey]struct{}{}
	for _, q := range candidateSourceQueries {
		if err := r.collectPairs(ctx, q, out); err != nil {
			if dbutil.IsMissingTableError(err) {
				continue
			}
			return nil, err
		}
	}
	return out, nil
}

func (r *Repository) collectPairs(ctx context.Context, query string, out map[pairKey]struct{}) error {
	rows, err := r.ro.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var k pairKey
		if err := rows.Scan(&k.taskID, &k.repositoryID); err != nil {
			return err
		}
		out[k] = struct{}{}
	}
	return rows.Err()
}

// existingIDs is a caller-controlled table-name lookup (never
// user-supplied), used to resolve candidacy conditions 2 and 3.
func (r *Repository) existingIDs(ctx context.Context, table string) (map[string]struct{}, error) {
	rows, err := r.ro.QueryContext(ctx, fmt.Sprintf("SELECT id FROM %s", table)) //nolint:gosec // table is a package-internal constant, never user input
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]struct{}{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = struct{}{}
	}
	return out, rows.Err()
}

// RepositoryInfo is the subset of a repositories row the evaluator and
// sweep need.
type RepositoryInfo struct {
	DefaultBranch string
	LocalPath     string
	DeletedAt     *time.Time
	UpdatedAt     time.Time
}

// RepositoryInfo reads the repository row's evaluation-relevant columns.
// Returns sql.ErrNoRows if the id does not resolve (should not happen for
// a pair Candidates() already returned, since that stage just verified
// it, but a repository can be hard-deleted between candidacy and
// evaluation in a maximally adversarial timing).
func (r *Repository) RepositoryInfo(ctx context.Context, id string) (RepositoryInfo, error) {
	var row struct {
		DefaultBranch sql.NullString `db:"default_branch"`
		LocalPath     sql.NullString `db:"local_path"`
		DeletedAt     sql.NullTime   `db:"deleted_at"`
		UpdatedAt     time.Time      `db:"updated_at"`
	}
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT default_branch, local_path, deleted_at, updated_at FROM repositories WHERE id = ?
	`), id).StructScan(&row)
	if err != nil {
		return RepositoryInfo{}, err
	}
	info := RepositoryInfo{
		DefaultBranch: strings.TrimSpace(row.DefaultBranch.String),
		LocalPath:     row.LocalPath.String,
		UpdatedAt:     row.UpdatedAt,
	}
	if row.DeletedAt.Valid {
		t := row.DeletedAt.Time
		info.DeletedAt = &t
	}
	return info, nil
}

// TaskInfo is the subset of a tasks row the evaluator and sweep ordering
// need.
type TaskInfo struct {
	WorkspaceID string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (r *Repository) TaskInfo(ctx context.Context, id string) (TaskInfo, error) {
	var row struct {
		WorkspaceID string    `db:"workspace_id"`
		CreatedAt   time.Time `db:"created_at"`
		UpdatedAt   time.Time `db:"updated_at"`
	}
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT workspace_id, created_at, updated_at FROM tasks WHERE id = ?
	`), id).StructScan(&row)
	if err != nil {
		return TaskInfo{}, err
	}
	return TaskInfo{WorkspaceID: row.WorkspaceID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, nil
}

// SnapshotsForPair reads every task_session_git_snapshots row for the
// pair's sessions, joined through task_sessions.
func (r *Repository) SnapshotsForPair(ctx context.Context, taskID, repositoryID string) ([]Snapshot, error) {
	var rows []struct {
		SessionID string `db:"session_id"`
		Branch    string `db:"branch"`
		// head_commit is TEXT DEFAULT '', not NOT NULL
		// (base_schema.go), and spec "Classification" normalization
		// explicitly anticipates a real NULL there — scan into
		// sql.NullString rather than a bare string, or a literal NULL
		// row fails the whole query (Review round 1, finding #6).
		HeadCommit sql.NullString `db:"head_commit"`
		Ahead      *int           `db:"ahead"`
		CreatedAt  time.Time      `db:"created_at"`
	}
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT g.session_id, g.branch, g.head_commit, g.ahead, g.created_at
		FROM task_session_git_snapshots g
		JOIN task_sessions s ON s.id = g.session_id
		WHERE s.task_id = ? AND s.repository_id = ?
	`), taskID, repositoryID)
	if err != nil {
		return nil, err
	}
	out := make([]Snapshot, len(rows))
	for i, row := range rows {
		out[i] = Snapshot{
			SessionID: row.SessionID, Branch: row.Branch, HeadCommit: row.HeadCommit.String,
			Ahead: row.Ahead, CreatedAt: row.CreatedAt,
		}
	}
	return out, nil
}

// ProvidersForPair reads every provider pull/merge request row for the
// pair across all three provider tables, normalized to the "Provider
// predicates" table. Tolerant of a missing provider table.
func (r *Repository) ProvidersForPair(ctx context.Context, taskID, repositoryID string) ([]ProviderRequest, error) {
	var all []ProviderRequest
	for _, fetch := range []func(context.Context, string, string) ([]ProviderRequest, error){
		r.githubPRs, r.gitlabMRs, r.azureDevOpsPRs,
	} {
		rows, err := fetch(ctx, taskID, repositoryID)
		if err != nil {
			if dbutil.IsMissingTableError(err) {
				continue
			}
			return nil, err
		}
		all = append(all, rows...)
	}
	return all, nil
}

func (r *Repository) githubPRs(ctx context.Context, taskID, repositoryID string) ([]ProviderRequest, error) {
	var rows []struct {
		MergedAt   sql.NullTime `db:"merged_at"`
		DetachedAt sql.NullTime `db:"detached_at"`
		PRNumber   int          `db:"pr_number"`
		PRURL      string       `db:"pr_url"`
		BaseBranch string       `db:"base_branch"`
		Owner      string       `db:"owner"`
		Repo       string       `db:"repo"`
	}
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT merged_at, detached_at, pr_number, pr_url, base_branch, owner, repo
		FROM github_task_prs WHERE task_id = ? AND repository_id = ?
	`), taskID, repositoryID)
	if err != nil {
		return nil, err
	}
	out := make([]ProviderRequest, len(rows))
	for i, row := range rows {
		out[i] = ProviderRequest{
			Provider: ProviderGitHub, Merged: row.MergedAt.Valid, Detached: row.DetachedAt.Valid,
			MergeInstant: row.MergedAt.Time, RequestNumber: row.PRNumber, URL: row.PRURL,
			BaseBranch: row.BaseBranch, ScopeValue: row.Owner + "/" + row.Repo,
		}
	}
	return out, nil
}

func (r *Repository) gitlabMRs(ctx context.Context, taskID, repositoryID string) ([]ProviderRequest, error) {
	var rows []struct {
		MergedAt    sql.NullTime `db:"merged_at"`
		MRIID       int          `db:"mr_iid"`
		MRURL       string       `db:"mr_url"`
		BaseBranch  string       `db:"base_branch"`
		ProjectPath string       `db:"project_path"`
	}
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT merged_at, mr_iid, mr_url, base_branch, project_path
		FROM gitlab_task_mrs WHERE task_id = ? AND repository_id = ?
	`), taskID, repositoryID)
	if err != nil {
		return nil, err
	}
	out := make([]ProviderRequest, len(rows))
	for i, row := range rows {
		out[i] = ProviderRequest{
			// gitlab_task_mrs has no detached_at column: a GitLab detach
			// deletes the row, so Detached is always false here.
			Provider: ProviderGitLab, Merged: row.MergedAt.Valid, Detached: false,
			MergeInstant: row.MergedAt.Time, RequestNumber: row.MRIID, URL: row.MRURL,
			BaseBranch: row.BaseBranch, ScopeValue: row.ProjectPath,
		}
	}
	return out, nil
}

func (r *Repository) azureDevOpsPRs(ctx context.Context, taskID, repositoryID string) ([]ProviderRequest, error) {
	var rows []struct {
		Status            string    `db:"status"`
		PullRequestID     int       `db:"pull_request_id"`
		PullRequestURL    string    `db:"pull_request_url"`
		TargetBranch      string    `db:"target_branch"`
		AzureRepositoryID string    `db:"azure_repository_id"`
		UpdatedAt         time.Time `db:"updated_at"`
	}
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT status, pull_request_id, pull_request_url, target_branch, azure_repository_id, updated_at
		FROM azure_devops_task_prs WHERE task_id = ? AND repository_id = ?
	`), taskID, repositoryID)
	if err != nil {
		return nil, err
	}
	out := make([]ProviderRequest, len(rows))
	for i, row := range rows {
		out[i] = ProviderRequest{
			// azure_devops_task_prs has no merged_at: updated_at is the
			// merge instant used for ordering only, per spec "Provider
			// predicates" — it is never written to reached_default_at.
			Provider: ProviderAzureDevOps, Merged: isAzureMergedStatus(row.Status), Detached: false,
			MergeInstant: row.UpdatedAt, RequestNumber: row.PullRequestID, URL: row.PullRequestURL,
			BaseBranch: row.TargetBranch, ScopeValue: row.AzureRepositoryID,
		}
	}
	return out, nil
}

// isAzureMergedStatus implements spec "Provider predicates": "completed"
// and "merged" are the merged states; every other value — including an
// unrecognised one — is not-merged, and is never guessed at.
func isAzureMergedStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "merged":
		return true
	default:
		return false
	}
}
