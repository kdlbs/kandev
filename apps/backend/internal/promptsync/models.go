// Package promptsync keeps install-wide saved prompts in sync with Markdown
// files in one repository directory. The repository is read through a chosen
// workspace's existing GitHub or GitLab connection; a background poller and
// "Sync now" reconcile the prompts through the prompt service.
package promptsync

import (
	"errors"
	"time"

	"github.com/kandev/kandev/internal/common/authcircuit"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	"github.com/kandev/kandev/internal/reposync"
)

// DefaultPath is the prompt directory used when the configuration omits one.
const DefaultPath = ".kandev/prompts"

// ErrInvalidConfig marks configuration validation failures (HTTP 400).
var ErrInvalidConfig = errors.New("invalid prompt sync config")

// ErrNotConfigured is returned when a sync runs without a configuration.
var ErrNotConfigured = errors.New("prompt sync is not configured")

// ErrListingIncomplete fails a run whose directory listing may be truncated,
// so files missing from it are never treated as removed.
var ErrListingIncomplete = errors.New("the prompt directory lists 1,000 or more entries, the most the GitHub contents API returns; move prompts to a directory with fewer entries")

// ErrCredentialWorkspaceMissing is returned when the configured credential
// workspace no longer exists.
var ErrCredentialWorkspaceMissing = errors.New("the credential workspace for prompt sync no longer exists; choose another workspace")

// Config is the install-wide prompt sync configuration and its last status.
type Config struct {
	WorkspaceID          string                   `json:"workspace_id"`
	Provider             string                   `json:"provider"`
	GitHubHost           string                   `json:"github_host"`
	RepoOwner            string                   `json:"repo_owner"`
	RepoName             string                   `json:"repo_name"`
	ProjectPath          string                   `json:"project_path"`
	Branch               string                   `json:"branch"`
	Path                 string                   `json:"path"`
	IntervalSeconds      int                      `json:"interval_seconds"`
	PollEnabled          bool                     `json:"poll_enabled"`
	LastSyncedAt         *time.Time               `json:"last_synced_at,omitempty"`
	LastOk               bool                     `json:"last_ok"`
	LastError            string                   `json:"last_error,omitempty"`
	LastWarnings         []string                 `json:"last_warnings,omitempty"`
	FailureClass         authcircuit.FailureClass `json:"failure_class,omitempty"`
	ConsecutiveFailures  int                      `json:"consecutive_failures,omitempty"`
	NextAttemptAt        *time.Time               `json:"next_attempt_at,omitempty"`
	LastErrorClass       string                   `json:"last_error_class,omitempty"`
	PollSuspended        bool                     `json:"poll_suspended"`
	PollSuspensionReason string                   `json:"poll_suspension_reason,omitempty"`
	CreatedAt            time.Time                `json:"created_at"`
	UpdatedAt            time.Time                `json:"updated_at"`

	CredentialFingerprint string `json:"-"`
}

func (c *Config) source() reposync.Source {
	return reposync.Source{
		WorkspaceID: c.WorkspaceID, Provider: c.Provider, RepoOwner: c.RepoOwner, RepoName: c.RepoName,
		ProjectPath: c.ProjectPath, Branch: c.Branch, Path: c.Path,
	}
}

func (c *Config) circuitState() authcircuit.State {
	return authcircuit.State{
		FailureClass:        c.FailureClass,
		ConsecutiveFailures: c.ConsecutiveFailures,
		NextRetryAt:         c.NextAttemptAt,
		Fingerprint:         c.CredentialFingerprint,
	}
}

// due reports whether an automatic sync should run at now.
func (c *Config) due(now time.Time) bool {
	if !c.PollEnabled {
		return false
	}
	if c.NextAttemptAt != nil {
		return !now.Before(*c.NextAttemptAt)
	}
	if c.LastSyncedAt == nil {
		return true
	}
	return now.Sub(*c.LastSyncedAt) >= time.Duration(c.IntervalSeconds)*time.Second
}

// SetConfigRequest creates or replaces the prompt sync configuration.
type SetConfigRequest struct {
	WorkspaceID     string `json:"workspace_id"`
	Provider        string `json:"provider"`
	GitHubHost      string `json:"github_host"`
	RepoOwner       string `json:"repo_owner"`
	RepoName        string `json:"repo_name"`
	ProjectPath     string `json:"project_path"`
	Branch          string `json:"branch"`
	Path            string `json:"path"`
	IntervalSeconds int    `json:"interval_seconds"`
	PollEnabled     *bool  `json:"poll_enabled"`
	// Resolutions are the user's conflict choices for the sync that runs
	// after saving. They are not stored.
	Resolutions map[string]promptservice.ConflictChoice `json:"resolutions,omitempty"`
}

// Normalize validates the request and fills defaults.
func (r *SetConfigRequest) Normalize() error {
	if r.WorkspaceID == "" {
		return errors.Join(ErrInvalidConfig, errors.New("workspace_id is required"))
	}
	target := reposync.Target{
		GitHubHost: r.GitHubHost, Provider: r.Provider, RepoOwner: r.RepoOwner, RepoName: r.RepoName,
		ProjectPath: r.ProjectPath, Branch: r.Branch, Path: r.Path, IntervalSeconds: r.IntervalSeconds,
	}
	err := target.Normalize(DefaultPath, ErrInvalidConfig)
	r.GitHubHost, r.Provider, r.RepoOwner, r.RepoName = target.GitHubHost, target.Provider, target.RepoOwner, target.RepoName
	r.ProjectPath, r.Branch, r.Path, r.IntervalSeconds = target.ProjectPath, target.Branch, target.Path, target.IntervalSeconds
	if err != nil {
		return err
	}
	if err := validateResolutions(r.Resolutions); err != nil {
		return err
	}
	if r.PollEnabled == nil {
		enabled := true
		r.PollEnabled = &enabled
	}
	return nil
}

// validateResolutions rejects a conflict choice other than local or
// repository.
func validateResolutions(resolutions map[string]promptservice.ConflictChoice) error {
	for name, choice := range resolutions {
		if !choice.Valid() {
			return errors.Join(ErrInvalidConfig, errors.New("resolution for "+name+" must be local or repository"))
		}
	}
	return nil
}

// configFromRequest is the unsaved configuration a normalized request
// describes, used to read a candidate source.
func configFromRequest(req *SetConfigRequest) *Config {
	return &Config{
		WorkspaceID: req.WorkspaceID, Provider: req.Provider, GitHubHost: req.GitHubHost,
		RepoOwner: req.RepoOwner, RepoName: req.RepoName, ProjectPath: req.ProjectPath,
		Branch: req.Branch, Path: req.Path,
	}
}

// SyncResult reports the outcome of one prompt sync run. Conflicts lists
// prompts still waiting for the user to choose between the local and
// repository versions; Restored lists synced prompts that left the source
// and became manual again.
type SyncResult struct {
	Created    []string `json:"created"`
	Updated    []string `json:"updated"`
	Deleted    []string `json:"deleted"`
	Restored   []string `json:"restored"`
	Overridden []string `json:"overridden"`
	Conflicts  []string `json:"conflicts"`
	Warnings   []string `json:"warnings"`
	Unchanged  bool     `json:"unchanged"`
}
