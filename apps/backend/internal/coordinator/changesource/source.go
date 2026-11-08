// Package changesource reads pull and merge requests from the GitHub and
// GitLab status-sync tables for the coordinator's change observer.
package changesource

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
)

// ErrNotLinked means no live task of the workspace links the change.
var ErrNotLinked = errors.New("change is not linked to a task of this workspace")

const (
	statePending   = "pending"
	providerGitHub = "github"
	providerGitLab = "gitlab"
)

// Source reads synced change state. Synced rows that were never refreshed
// read as unavailable, never as passing.
type Source struct {
	db *sqlx.DB
}

// New builds a Source over the database holding the sync tables and tasks.
func New(db *sqlx.DB) *Source { return &Source{db: db} }

// normalizeHost reduces a host or base URL to a lowercase bare host.
func normalizeHost(h string) string {
	h = strings.TrimSpace(h)
	if u, err := url.Parse(h); err == nil && u.Host != "" {
		h = u.Host
	}
	return strings.TrimSuffix(strings.ToLower(h), ".")
}

// TasksLinkedToChange returns the IDs of the workspace's unarchived tasks
// linked to the change key, sorted ascending.
func (s *Source) TasksLinkedToChange(ctx context.Context, workspaceID, changeKey string) ([]string, error) {
	key, ok := mergestate.ParseChangeKey(changeKey)
	if !ok {
		return nil, nil
	}
	switch key.Provider {
	case providerGitHub:
		rows, err := s.githubRows(ctx, workspaceID, key)
		if err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.TaskID)
		}
		return ids, nil
	case providerGitLab:
		rows, err := s.gitlabRows(ctx, workspaceID, key)
		if err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.TaskID)
		}
		return ids, nil
	}
	return nil, nil
}

// ReadChange maps the freshest synced row of the change to a snapshot.
func (s *Source) ReadChange(ctx context.Context, workspaceID string, key mergestate.ChangeKey) (mergestate.Snapshot, error) {
	switch key.Provider {
	case providerGitHub:
		rows, err := s.githubRows(ctx, workspaceID, key)
		if err != nil {
			return mergestate.Snapshot{}, err
		}
		if len(rows) == 0 {
			return mergestate.Snapshot{}, ErrNotLinked
		}
		return githubSnapshot(key, freshestGitHub(rows)), nil
	case providerGitLab:
		rows, err := s.gitlabRows(ctx, workspaceID, key)
		if err != nil {
			return mergestate.Snapshot{}, err
		}
		if len(rows) == 0 {
			return mergestate.Snapshot{}, ErrNotLinked
		}
		return gitlabSnapshot(key, freshestGitLab(rows)), nil
	}
	return mergestate.Snapshot{}, fmt.Errorf("unsupported provider %q", key.Provider)
}

type githubRow struct {
	TaskID         string       `db:"task_id"`
	Host           string       `db:"github_host"`
	State          string       `db:"state"`
	HeadSHA        string       `db:"head_sha"`
	ChecksState    string       `db:"checks_state"`
	ChecksTotal    int          `db:"checks_total"`
	MergeableState string       `db:"mergeable_state"`
	HasConflicts   sql.NullBool `db:"has_merge_conflicts"`
	ReviewState    string       `db:"review_state"`
	Threads        int          `db:"unresolved_review_threads"`
	BaseBranch     string       `db:"base_branch"`
	LastSyncedAt   sql.NullTime `db:"last_synced_at"`
}

func (s *Source) githubRows(ctx context.Context, workspaceID string, key mergestate.ChangeKey) ([]githubRow, error) {
	var rows []githubRow
	err := s.db.SelectContext(ctx, &rows, `
		SELECT gtp.task_id, gtp.github_host, gtp.state, gtp.head_sha, gtp.checks_state, gtp.checks_total,
		       gtp.mergeable_state, gtp.has_merge_conflicts, gtp.review_state, gtp.unresolved_review_threads,
		       gtp.base_branch, gtp.last_synced_at
		FROM github_task_prs gtp JOIN tasks t ON t.id = gtp.task_id
		WHERE t.workspace_id = ? AND t.archived_at IS NULL AND gtp.repository_id = ? AND gtp.pr_number = ?
		ORDER BY gtp.task_id`, workspaceID, key.RepoID, key.Number)
	if err != nil {
		return nil, fmt.Errorf("read github change rows: %w", err)
	}
	out := rows[:0]
	for _, r := range rows {
		if normalizeHost(r.Host) == normalizeHost(key.Host) {
			out = append(out, r)
		}
	}
	return out, nil
}

func freshestGitHub(rows []githubRow) githubRow {
	best := rows[0]
	for _, r := range rows[1:] {
		if r.LastSyncedAt.Valid && (!best.LastSyncedAt.Valid || r.LastSyncedAt.Time.After(best.LastSyncedAt.Time)) {
			best = r
		}
	}
	return best
}

func changeState(s string) mergestate.ChangeState {
	switch s {
	case "merged":
		return mergestate.StateMerged
	case "closed":
		return mergestate.StateClosed
	}
	return mergestate.StateOpen
}

func triOf(synced, ok bool) mergestate.Tri {
	switch {
	case !synced:
		return mergestate.Unavailable
	case ok:
		return mergestate.Met
	}
	return mergestate.Unmet
}

func githubSnapshot(key mergestate.ChangeKey, r githubRow) mergestate.Snapshot {
	synced := r.LastSyncedAt.Valid
	snap := mergestate.Snapshot{
		Key: key, State: changeState(r.State), TargetBranch: r.BaseBranch, HeadSHA: r.HeadSHA,
		Pipeline: githubPipeline(r, synced), Mergeable: githubMergeable(r.MergeableState),
		UnresolvedThreads: -1, Behind: -1, Draft: triOf(synced, r.MergeableState != "draft"),
		Approvals: githubApprovals(r.ReviewState, synced),
	}
	if !synced {
		snap.Mergeable = mergestate.MergeableReading{Mergeable: mergestate.Unavailable}
	} else {
		snap.UnresolvedThreads = r.Threads
		snap.Behind = behindOf(r.MergeableState)
	}
	in := mergestate.ConflictInput{Provider: providerGitHub, MergeableState: r.MergeableState, Unavailable: !synced}
	if r.HasConflicts.Valid {
		m := !r.HasConflicts.Bool
		in.Mergeable = &m
	}
	snap.Conflict = mergestate.ReadConflict(in)
	return snap
}

func behindOf(mergeableState string) int {
	if mergeableState == "behind" {
		return 1
	}
	return 0
}

func githubPipeline(r githubRow, synced bool) mergestate.Pipeline {
	switch {
	case !synced:
		return mergestate.PipelineUnavailable
	case r.ChecksState == "failure":
		return mergestate.PipelineRed
	case r.ChecksState == statePending:
		return mergestate.PipelinePending
	case r.ChecksState == "success" && r.ChecksTotal > 0:
		return mergestate.PipelineGreen
	case r.ChecksState == "" && r.ChecksTotal == 0:
		return mergestate.PipelineNone
	}
	return mergestate.PipelineUnavailable
}

func githubMergeable(state string) mergestate.MergeableReading {
	switch state {
	case "clean", "has_hooks":
		return mergestate.MergeableReading{Mergeable: mergestate.Met}
	case "draft":
		return mergestate.MergeableReading{Mergeable: mergestate.Unmet, Explains: mergestate.CondDraft}
	case "unstable":
		return mergestate.MergeableReading{Mergeable: mergestate.Unmet, Explains: mergestate.CondPipeline}
	case "dirty", "blocked", "behind":
		return mergestate.MergeableReading{Mergeable: mergestate.Unmet}
	}
	return mergestate.MergeableReading{Mergeable: mergestate.Unavailable}
}

func githubApprovals(review string, synced bool) mergestate.Tri {
	switch {
	case !synced:
		return mergestate.Unavailable
	case review == "approved":
		return mergestate.Met
	case review == "changes_requested" || review == statePending:
		return mergestate.Unmet
	}
	return mergestate.Unavailable
}

type gitlabRow struct {
	TaskID            string       `db:"task_id"`
	Host              string       `db:"host"`
	State             string       `db:"state"`
	HeadSHA           string       `db:"head_sha"`
	BaseBranch        string       `db:"base_branch"`
	PipelineState     string       `db:"pipeline_state"`
	PipelineJobsTotal int          `db:"pipeline_jobs_total"`
	MergeStatus       string       `db:"merge_status"`
	DetailedStatus    string       `db:"detailed_merge_status"`
	Draft             bool         `db:"draft"`
	ApprovalState     string       `db:"approval_state"`
	RequiredApprovals int          `db:"required_approvals"`
	Unresolved        int          `db:"unresolved_discussions"`
	LastSyncedAt      sql.NullTime `db:"last_synced_at"`
}

func (s *Source) gitlabRows(ctx context.Context, workspaceID string, key mergestate.ChangeKey) ([]gitlabRow, error) {
	var rows []gitlabRow
	err := s.db.SelectContext(ctx, &rows, `
		SELECT m.task_id, m.host, m.state, m.head_sha, m.base_branch, m.pipeline_state, m.pipeline_jobs_total,
		       m.merge_status, m.detailed_merge_status, m.draft, m.approval_state, m.required_approvals,
		       m.unresolved_discussions, m.last_synced_at
		FROM gitlab_task_mrs m JOIN tasks t ON t.id = m.task_id
		WHERE t.workspace_id = ? AND t.archived_at IS NULL AND m.repository_id = ? AND m.mr_iid = ?
		ORDER BY m.task_id`, workspaceID, key.RepoID, key.Number)
	if err != nil {
		return nil, fmt.Errorf("read gitlab change rows: %w", err)
	}
	out := rows[:0]
	for _, r := range rows {
		if normalizeHost(r.Host) == normalizeHost(key.Host) {
			out = append(out, r)
		}
	}
	return out, nil
}

func freshestGitLab(rows []gitlabRow) gitlabRow {
	best := rows[0]
	for _, r := range rows[1:] {
		if r.LastSyncedAt.Valid && (!best.LastSyncedAt.Valid || r.LastSyncedAt.Time.After(best.LastSyncedAt.Time)) {
			best = r
		}
	}
	return best
}

func gitlabSnapshot(key mergestate.ChangeKey, r gitlabRow) mergestate.Snapshot {
	synced := r.LastSyncedAt.Valid
	snap := mergestate.Snapshot{
		Key: key, State: changeState(r.State), TargetBranch: r.BaseBranch, HeadSHA: r.HeadSHA,
		Pipeline: gitlabPipeline(r, synced), UnresolvedThreads: -1, Behind: -1,
		Draft: triOf(synced, !r.Draft), InProgress: r.State == "locked" || r.DetailedStatus == "locked",
		Mergeable: mergestate.GitLabMergeable(mergestate.GitLabMR{DetailedMergeStatus: r.DetailedStatus, MergeStatus: r.MergeStatus}),
	}
	if !synced {
		snap.Mergeable = mergestate.MergeableReading{Mergeable: mergestate.Unavailable}
		snap.Approvals = mergestate.Unavailable
		return snap
	}
	snap.UnresolvedThreads = r.Unresolved
	snap.Approvals = gitlabApprovals(r)
	in := mergestate.ConflictInput{Provider: providerGitLab, DetailedMergeStatus: r.DetailedStatus}
	if r.DetailedStatus == "mergeable" {
		no := false
		in.HasConflicts = &no
	}
	snap.Conflict = mergestate.ReadConflict(in)
	return snap
}

func gitlabPipeline(r gitlabRow, synced bool) mergestate.Pipeline {
	switch {
	case !synced:
		return mergestate.PipelineUnavailable
	case r.PipelineState == "failure":
		return mergestate.PipelineRed
	case r.PipelineState == statePending:
		return mergestate.PipelinePending
	case r.PipelineState == "success":
		return mergestate.PipelineGreen
	case r.PipelineState == "" && r.PipelineJobsTotal == 0:
		return mergestate.PipelineNone
	}
	return mergestate.PipelineUnavailable
}

func gitlabApprovals(r gitlabRow) mergestate.Tri {
	switch {
	case r.ApprovalState == "approved":
		return mergestate.Met
	case r.ApprovalState == statePending:
		return mergestate.Unmet
	case r.ApprovalState == "" && r.RequiredApprovals == 0:
		return mergestate.Met
	}
	return mergestate.Unavailable
}

// OpenChanges returns the canonical keys of the workspace's open changes that
// a live task links, sorted ascending.
func (s *Source) OpenChanges(ctx context.Context, workspaceID string) ([]string, error) {
	var gh []struct {
		Host   string `db:"github_host"`
		RepoID string `db:"repository_id"`
		Number int    `db:"pr_number"`
	}
	if err := s.db.SelectContext(ctx, &gh, `
		SELECT DISTINCT gtp.github_host, gtp.repository_id, gtp.pr_number
		FROM github_task_prs gtp JOIN tasks t ON t.id = gtp.task_id
		WHERE t.workspace_id = ? AND t.archived_at IS NULL AND gtp.state = 'open' AND gtp.repository_id <> ''`, workspaceID); err != nil {
		return nil, fmt.Errorf("list open github changes: %w", err)
	}
	var gl []struct {
		Host   string `db:"host"`
		RepoID string `db:"repository_id"`
		Number int    `db:"mr_iid"`
	}
	if err := s.db.SelectContext(ctx, &gl, `
		SELECT DISTINCT m.host, m.repository_id, m.mr_iid
		FROM gitlab_task_mrs m JOIN tasks t ON t.id = m.task_id
		WHERE t.workspace_id = ? AND t.archived_at IS NULL AND m.state = 'open' AND m.repository_id <> ''`, workspaceID); err != nil {
		return nil, fmt.Errorf("list open gitlab changes: %w", err)
	}
	seen := map[string]struct{}{}
	for _, r := range gh {
		seen[mergestate.ChangeKey{Provider: providerGitHub, Host: normalizeHost(r.Host), RepoID: r.RepoID, Number: r.Number}.String()] = struct{}{}
	}
	for _, r := range gl {
		seen[mergestate.ChangeKey{Provider: providerGitLab, Host: normalizeHost(r.Host), RepoID: r.RepoID, Number: r.Number}.String()] = struct{}{}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}

// ChangesLinkedToTask returns the canonical keys of every change a live task
// links, whatever the change state.
func (s *Source) ChangesLinkedToTask(ctx context.Context, workspaceID, taskID string) ([]string, error) {
	var gh []struct {
		Host   string `db:"github_host"`
		RepoID string `db:"repository_id"`
		Number int    `db:"pr_number"`
	}
	if err := s.db.SelectContext(ctx, &gh, `
		SELECT gtp.github_host, gtp.repository_id, gtp.pr_number
		FROM github_task_prs gtp JOIN tasks t ON t.id = gtp.task_id
		WHERE t.workspace_id = ? AND t.archived_at IS NULL AND gtp.task_id = ? AND gtp.repository_id <> ''`, workspaceID, taskID); err != nil {
		return nil, fmt.Errorf("list task github changes: %w", err)
	}
	var gl []struct {
		Host   string `db:"host"`
		RepoID string `db:"repository_id"`
		Number int    `db:"mr_iid"`
	}
	if err := s.db.SelectContext(ctx, &gl, `
		SELECT m.host, m.repository_id, m.mr_iid
		FROM gitlab_task_mrs m JOIN tasks t ON t.id = m.task_id
		WHERE t.workspace_id = ? AND t.archived_at IS NULL AND m.task_id = ? AND m.repository_id <> ''`, workspaceID, taskID); err != nil {
		return nil, fmt.Errorf("list task gitlab changes: %w", err)
	}
	keys := make([]string, 0, len(gh)+len(gl))
	for _, r := range gh {
		keys = append(keys, mergestate.ChangeKey{Provider: providerGitHub, Host: normalizeHost(r.Host), RepoID: r.RepoID, Number: r.Number}.String())
	}
	for _, r := range gl {
		keys = append(keys, mergestate.ChangeKey{Provider: providerGitLab, Host: normalizeHost(r.Host), RepoID: r.RepoID, Number: r.Number}.String())
	}
	sort.Strings(keys)
	return keys, nil
}
