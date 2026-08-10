// Package workflowsync keeps workspace workflows in sync with definition
// files stored in a configured GitHub repository. A background poller fetches
// the configured directory on an interval and applies changes through the
// workflow service; users can also force a sync from the settings UI. Synced
// workflows coexist with manually-created ones — see workflows.source.
package workflowsync

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/securityutil"
)

// Defaults for optional config fields.
const (
	DefaultBranch          = "main"
	DefaultPath            = ".kandev/workflows"
	DefaultIntervalSeconds = 300
	MinIntervalSeconds     = 60
	// MaxIntervalSeconds caps the poll interval at 30 days — far beyond any
	// sensible cadence, and safely below time.Duration overflow territory.
	MaxIntervalSeconds = 30 * 24 * 60 * 60
)

// Supported sync sources. A workspace has exactly one.
const (
	ProviderGitHub = "github"
	ProviderGitLab = "gitlab"
)

// ErrInvalidConfig marks validation failures on SetConfigRequest so handlers
// can map them to 400.
var ErrInvalidConfig = errors.New("invalid workflow sync config")

// ErrNotConfigured is returned when a sync is requested for a workspace that
// has no workflow sync config.
var ErrNotConfigured = errors.New("workflow sync is not configured for this workspace")

// Config is the per-workspace workflow sync configuration plus the status of
// the most recent sync attempt (written by the poller and force syncs).
type Config struct {
	WorkspaceID string `json:"workspace_id"`
	// Provider selects the sync source: ProviderGitHub uses RepoOwner and
	// RepoName, ProviderGitLab uses ProjectPath.
	Provider        string     `json:"provider"`
	RepoOwner       string     `json:"repo_owner"`
	RepoName        string     `json:"repo_name"`
	ProjectPath     string     `json:"project_path"`
	Branch          string     `json:"branch"`
	Path            string     `json:"path"`
	IntervalSeconds int        `json:"interval_seconds"`
	PollEnabled     bool       `json:"poll_enabled"`
	LastSyncedAt    *time.Time `json:"last_synced_at,omitempty"`
	LastOk          bool       `json:"last_ok"`
	LastError       string     `json:"last_error,omitempty"`
	LastWarnings    []string   `json:"last_warnings,omitempty"`
	LastHash        string     `json:"-"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// SetConfigRequest is the payload for creating or updating a workspace's
// workflow sync config. Branch, Path, and IntervalSeconds fall back to
// defaults when empty/zero.
type SetConfigRequest struct {
	// Provider is optional; an empty value means ProviderGitHub, which keeps
	// pre-GitLab API clients working unchanged.
	Provider        string `json:"provider"`
	RepoOwner       string `json:"repo_owner"`
	RepoName        string `json:"repo_name"`
	ProjectPath     string `json:"project_path"`
	Branch          string `json:"branch"`
	Path            string `json:"path"`
	IntervalSeconds int    `json:"interval_seconds"`
	// PollEnabled controls the background polling loop; nil defaults to
	// true. When false the workspace only syncs via "Sync now".
	PollEnabled *bool `json:"poll_enabled"`
}

// normalizeTarget validates the repository coordinates for the selected
// provider and clears the other provider's fields, so a switch cannot leave
// stale values behind.
func (r *SetConfigRequest) normalizeTarget() error {
	r.Provider = strings.TrimSpace(r.Provider)
	if r.Provider == "" {
		r.Provider = ProviderGitHub
	}
	r.RepoOwner = strings.TrimSpace(r.RepoOwner)
	r.RepoName = strings.TrimSpace(r.RepoName)
	r.ProjectPath = strings.TrimSpace(r.ProjectPath)

	switch r.Provider {
	case ProviderGitHub:
		return r.normalizeGitHubTarget()
	case ProviderGitLab:
		return r.normalizeGitLabTarget()
	default:
		return fmt.Errorf("%w: provider must be %q or %q", ErrInvalidConfig, ProviderGitHub, ProviderGitLab)
	}
}

func (r *SetConfigRequest) normalizeGitHubTarget() error {
	if r.ProjectPath != "" {
		return fmt.Errorf("%w: project_path is only valid for the %q provider", ErrInvalidConfig, ProviderGitLab)
	}
	if r.RepoOwner == "" || strings.ContainsAny(r.RepoOwner, "/ ") {
		return fmt.Errorf("%w: repo_owner is required and cannot contain slashes or spaces", ErrInvalidConfig)
	}
	if r.RepoName == "" || strings.ContainsAny(r.RepoName, "/ ") {
		return fmt.Errorf("%w: repo_name is required and cannot contain slashes or spaces", ErrInvalidConfig)
	}
	return nil
}

// normalizeGitLabTarget requires a namespace path. GitLab projects live at
// arbitrarily nested paths ("group/subgroup/project"), so the value is
// validated as a whole rather than split into owner and name.
func (r *SetConfigRequest) normalizeGitLabTarget() error {
	if r.RepoOwner != "" || r.RepoName != "" {
		return fmt.Errorf(
			"%w: repo_owner and repo_name are only valid for the %q provider", ErrInvalidConfig, ProviderGitHub)
	}
	if r.ProjectPath == "" {
		return fmt.Errorf("%w: project_path is required", ErrInvalidConfig)
	}
	if strings.Contains(r.ProjectPath, " ") {
		return fmt.Errorf("%w: project_path cannot contain spaces", ErrInvalidConfig)
	}
	segments := strings.Split(r.ProjectPath, "/")
	if len(segments) < 2 {
		return fmt.Errorf(
			"%w: project_path must include the namespace, e.g. \"group/project\"", ErrInvalidConfig)
	}
	for _, segment := range segments {
		if segment == "" {
			return fmt.Errorf(
				"%w: project_path cannot have empty segments or leading/trailing slashes", ErrInvalidConfig)
		}
		if segment == "." || segment == ".." {
			return fmt.Errorf("%w: project_path cannot contain \".\" or \"..\" segments", ErrInvalidConfig)
		}
	}
	return nil
}

// Normalize validates the request and fills defaults. It returns a wrapped
// ErrInvalidConfig on bad input.
func (r *SetConfigRequest) Normalize() error {
	r.Branch = strings.TrimSpace(r.Branch)
	r.Path = strings.Trim(strings.TrimSpace(r.Path), "/")
	if err := r.normalizeTarget(); err != nil {
		return err
	}
	if r.Branch == "" {
		r.Branch = DefaultBranch
	}
	if !securityutil.IsValidBranchName(r.Branch) {
		return fmt.Errorf("%w: branch is not a valid git branch name", ErrInvalidConfig)
	}
	if r.Path == "" {
		r.Path = DefaultPath
	}
	for _, segment := range strings.Split(r.Path, "/") {
		if segment == ".." {
			return fmt.Errorf("%w: path cannot contain \"..\"", ErrInvalidConfig)
		}
	}
	if r.IntervalSeconds == 0 {
		r.IntervalSeconds = DefaultIntervalSeconds
	}
	if r.IntervalSeconds < MinIntervalSeconds {
		return fmt.Errorf("%w: interval_seconds must be at least %d", ErrInvalidConfig, MinIntervalSeconds)
	}
	if r.IntervalSeconds > MaxIntervalSeconds {
		return fmt.Errorf("%w: interval_seconds must be at most %d", ErrInvalidConfig, MaxIntervalSeconds)
	}
	if r.PollEnabled == nil {
		enabled := true
		r.PollEnabled = &enabled
	}
	return nil
}

// SyncResult reports the outcome of one sync run for a workspace.
type SyncResult struct {
	Created   []string `json:"created"`
	Updated   []string `json:"updated"`
	Deleted   []string `json:"deleted"`
	Warnings  []string `json:"warnings"`
	Unchanged bool     `json:"unchanged"`
}
