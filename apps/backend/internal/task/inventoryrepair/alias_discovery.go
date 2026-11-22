package inventoryrepair

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
)

// DiscoveryOptions select inventory rows only. No checkout paths are inspected.
type DiscoveryOptions struct {
	Home      string
	Database  string
	TasksRoot string
	After     string
	Limit     int
}

type AliasClaim struct {
	ID            string `json:"id"`
	EnvironmentID string `json:"environment_id"`
	TaskID        string `json:"task_id"`
	RepositoryID  string `json:"repository_id"`
	BranchSlug    string `json:"branch_slug"`
	Path          string `json:"path"`
	Branch        string `json:"branch"`
	ExecutorType  string `json:"executor_type"`
}

type AliasGroup struct {
	WorktreeID string         `json:"worktree_id"`
	Claims     []AliasClaim   `json:"claims,omitempty"`
	Oversized  bool           `json:"oversized,omitempty"`
	Selection  AliasSelection `json:"selection_template"`
}

type DiscoveryReport struct {
	Groups []AliasGroup `json:"groups"`
	Next   string       `json:"next_after,omitempty"`
}

func Discover(ctx context.Context, opts DiscoveryOptions) (*DiscoveryReport, error) {
	if opts.Limit == 0 {
		opts.Limit = 50
	}
	if err := opts.validate(); err != nil {
		return nil, err
	}
	db, err := openDatabase(Plan{Database: opts.Database}, false)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	ids, err := queryRows(ctx, tx, `SELECT worktree_id FROM task_environment_repos WHERE status='active' AND deleted_at IS NULL AND worktree_id > ? GROUP BY worktree_id HAVING count(*) > 1 ORDER BY worktree_id LIMIT ?`, opts.After, opts.Limit+1)
	if err != nil {
		return nil, fmt.Errorf("unsupported discovery inventory: %w", err)
	}
	return discoverPage(ctx, tx, opts, ids)
}

const discoveryByteLimit = 1 << 20

func discoverPage(ctx context.Context, tx queryer, opts DiscoveryOptions, ids []row) (*DiscoveryReport, error) {
	report := &DiscoveryReport{Groups: []AliasGroup{}}
	count := 0
	for i, id := range ids {
		if i == opts.Limit {
			break
		}
		group, err := discoverGroup(ctx, tx, stringValue(id, "worktree_id"))
		if err != nil {
			return nil, err
		}
		next := ""
		if i+1 < len(ids) {
			next = group.WorktreeID
		}
		single := &DiscoveryReport{Groups: []AliasGroup{group}, Next: next}
		fits, err := discoveryReportFits(single)
		if err != nil {
			return nil, err
		}
		if !fits {
			group.Claims = nil
			group.Oversized = true
			single.Groups[0] = group
			fits, err = discoveryReportFits(single)
			if err != nil {
				return nil, err
			}
			if !fits {
				return nil, errors.New("discovery identity exceeds response byte limit")
			}
		}
		candidate := &DiscoveryReport{Groups: append(report.Groups, group), Next: next}
		fits, err = discoveryReportFits(candidate)
		if err != nil {
			return nil, err
		}
		if count+len(group.Claims) > 1000 || !fits {
			break
		}
		report = candidate
		count += len(group.Claims)
	}
	return report, nil
}

// The command uses two-space indentation and a trailing newline.
func discoveryReportFits(report *DiscoveryReport) (bool, error) {
	data, err := json.MarshalIndent(report, "", "  ")
	return len(data)+1 <= discoveryByteLimit, err
}

func discoverGroup(ctx context.Context, db queryer, id string) (AliasGroup, error) {
	group := AliasGroup{WorktreeID: id, Selection: AliasSelection{WorktreeID: id, AliasRowIDs: []string{}}}
	rows, err := queryRows(ctx, db, `SELECT r.*, e.task_id, e.executor_type FROM task_environment_repos r LEFT JOIN task_environments e ON e.id=r.task_environment_id WHERE r.worktree_id=? AND r.status='active' AND r.deleted_at IS NULL ORDER BY r.id LIMIT 257`, id)
	if err != nil {
		return group, err
	}
	if len(rows) > 256 {
		group.Oversized = true
		return group, nil
	}
	for _, r := range rows {
		group.Claims = append(group.Claims, AliasClaim{ID: stringValue(r, "id"), EnvironmentID: stringValue(r, "task_environment_id"), TaskID: stringValue(r, "task_id"), RepositoryID: stringValue(r, "repository_id"), BranchSlug: stringValue(r, "branch_slug"), Path: stringValue(r, "worktree_path"), Branch: stringValue(r, "worktree_branch"), ExecutorType: stringValue(r, "executor_type")})
	}
	return group, nil
}

func (opts DiscoveryOptions) validate() error {
	for _, path := range []string{opts.Home, opts.Database, opts.TasksRoot} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return errors.New("discovery requires canonical absolute installation paths")
		}
	}
	if opts.Limit == 0 {
		opts.Limit = 50
	}
	if opts.Limit < 1 || opts.Limit > 100 || len(opts.After) > 1024 {
		return errors.New("discovery limit must be 1-100 and cursor at most 1024 bytes")
	}
	return nil
}
