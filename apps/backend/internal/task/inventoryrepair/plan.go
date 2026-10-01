// Package inventoryrepair provides explicit, offline repair of selected legacy
// task worktree inventory. Normal launch and cleanup never invoke it.
package inventoryrepair

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
)

type Plan struct {
	Version      int                 `json:"version"`
	OperationID  string              `json:"operation_id"`
	Home         string              `json:"home"`
	Database     string              `json:"database"`
	Driver       string              `json:"driver"`
	TasksRoot    string              `json:"tasks_root"`
	Repairs      []Repair            `json:"repairs"`
	Workspaces   []WorkspaceRepair   `json:"workspaces,omitempty"`
	CleanupJobs  []string            `json:"cleanup_jobs,omitempty"`
	ExpectedRows map[string]string   `json:"expected_rows,omitempty"`
	ExpectedGit  map[string]GitState `json:"expected_git,omitempty"`
}

type Repair struct {
	WorktreeID       string `json:"worktree_id"`
	EnvironmentID    string `json:"environment_id"`
	RepositoryID     string `json:"repository_id"`
	RepositoryPath   string `json:"repository_path"`
	SourcePath       string `json:"source_path"`
	Path             string `json:"path"`
	Branch           string `json:"branch"`
	HeadOID          string `json:"head_oid"`
	SourceRootTaskID string `json:"source_root_task_id"`
}

type WorkspaceRepair struct {
	EnvironmentID string   `json:"environment_id"`
	Path          string   `json:"path"`
	SessionIDs    []string `json:"session_ids"`
}

type GitState struct {
	Head          string `json:"head"`
	Branch        string `json:"branch"`
	CommonDir     string `json:"common_dir"`
	GitDir        string `json:"git_dir"`
	IndexSHA256   string `json:"index_sha256"`
	ContentSHA256 string `json:"content_sha256"`
	RefsSHA256    string `json:"refs_sha256"`
}

type Change struct {
	WorktreeID      string `json:"worktree_id"`
	OldRepositoryID string `json:"old_repository_id"`
	OldBranch       string `json:"old_branch"`
	OldPath         string `json:"old_path"`
	Repair          Repair `json:"replacement"`
}

type Report struct {
	Plan     Plan     `json:"plan"`
	Changes  []Change `json:"changes"`
	Blockers []string `json:"blockers,omitempty"`
}

var operationPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,79}$`)

func Decode(r io.Reader) (Plan, error) {
	var p Plan
	decoder := json.NewDecoder(io.LimitReader(r, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&p); err != nil {
		return p, err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return p, errors.New("repair plan must contain exactly one JSON value")
	}
	return p, p.validate()
}

func (p Plan) validate() error {
	if p.Version != 1 || p.Driver != "sqlite" || !operationPattern.MatchString(p.OperationID) {
		return errors.New("repair requires version 1, sqlite, and a bounded operation ID")
	}
	if len(p.Repairs) == 0 || len(p.Repairs) > 16 || len(p.Workspaces) > 16 || len(p.CleanupJobs) > 16 {
		return errors.New("repair must select 1-16 worktrees and at most 16 workspaces/jobs")
	}
	for _, path := range []string{p.Home, p.Database, p.TasksRoot} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return fmt.Errorf("repair path must be canonical and absolute: %q", path)
		}
	}
	seen := map[string]bool{}
	for _, r := range p.Repairs {
		if seen[r.WorktreeID] || r.incomplete() {
			return errors.New("repair worktree identity is incomplete or duplicated")
		}
		seen[r.WorktreeID] = true
		if err := r.validatePaths(); err != nil {
			return err
		}
	}
	return p.validateLocations()
}

func (r Repair) validatePaths() error {
	for _, path := range []string{r.RepositoryPath, r.SourcePath, r.Path} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return fmt.Errorf("invalid worktree repair path %q", path)
		}
	}
	return nil
}

func (r Repair) incomplete() bool {
	for _, value := range []string{r.WorktreeID, r.EnvironmentID, r.RepositoryID, r.Branch, r.HeadOID, r.SourceRootTaskID} {
		if value == "" {
			return true
		}
	}
	return false
}

func (p Plan) validateLocations() error {
	seen := map[string]bool{}
	for _, r := range p.Repairs {
		for _, key := range []string{"source:" + r.SourcePath, "destination:" + r.Path} {
			if seen[key] {
				return errors.New("selected worktrees have duplicate source or destination paths")
			}
			seen[key] = true
		}
	}
	return nil
}
