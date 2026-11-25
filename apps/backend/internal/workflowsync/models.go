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

	"github.com/kandev/kandev/internal/common/authcircuit"
	"github.com/kandev/kandev/internal/reposync"
)

// Defaults for optional config fields.
const (
	DefaultBranch          = reposync.DefaultBranch
	DefaultPath            = ".kandev/workflows"
	DefaultIntervalSeconds = reposync.DefaultIntervalSeconds
	MinIntervalSeconds     = reposync.MinIntervalSeconds
	MaxIntervalSeconds     = reposync.MaxIntervalSeconds
)

// Supported sync sources. A workspace has exactly one.
const (
	ProviderGitHub = reposync.ProviderGitHub
	ProviderGitLab = reposync.ProviderGitLab
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
	GitHubHost      string     `json:"github_host"`
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

	// FailureClass, ConsecutiveFailures, and NextRetryAt persist the
	// generation-aware circuit-breaker state for this config (see
	// internal/common/authcircuit). Exposed read-only to operators so a
	// permanently-failing config is visibly distinguishable from one that is
	// simply between polls.
	FailureClass        authcircuit.FailureClass `json:"failure_class,omitempty"`
	ConsecutiveFailures int                      `json:"consecutive_failures,omitempty"`
	NextRetryAt         *time.Time               `json:"next_retry_at,omitempty"`
	// The rate-specific class and suspension reason complement the shared
	// circuit state. NextAttemptAt mirrors NextRetryAt for sync clients.
	NextAttemptAt        *time.Time `json:"next_attempt_at,omitempty"`
	LastErrorClass       string     `json:"last_error_class,omitempty"`
	PollSuspended        bool       `json:"poll_suspended"`
	PollSuspensionReason string     `json:"poll_suspension_reason,omitempty"`
	// ConfigFingerprint and CredentialFingerprint are internal-only circuit
	// identity keys (never secrets — see authcircuit.State.Fingerprint) and
	// are not surfaced over the API.
	ConfigFingerprint     string `json:"-"`
	CredentialFingerprint string `json:"-"`
}

// circuitState extracts the persisted authcircuit.State embedded in this
// config's flat columns.
func (c *Config) circuitState() authcircuit.State {
	return authcircuit.State{
		FailureClass:        c.FailureClass,
		ConsecutiveFailures: c.ConsecutiveFailures,
		NextRetryAt:         c.NextRetryAt,
		Fingerprint:         c.CredentialFingerprint,
	}
}

// applyCircuitState writes an updated authcircuit.State back onto the
// config's flat columns (the in-memory mirror of what Store persists).
func (c *Config) applyCircuitState(state authcircuit.State) {
	c.FailureClass = state.FailureClass
	c.ConsecutiveFailures = state.ConsecutiveFailures
	c.NextRetryAt = state.NextRetryAt
	c.NextAttemptAt = state.NextRetryAt
	c.CredentialFingerprint = state.Fingerprint
}

// circuitOpen reports whether this config's circuit is currently open (skip
// syncing) at the given time.
func (c *Config) circuitOpen(now time.Time) bool {
	return c.circuitState().Open(now)
}

// fingerprint is a stable, non-secret digest of the fields that define
// "what this config points at". Changing any of these fields (a deliberate
// SetConfig call) always resets the circuit — see Store.UpsertConfigForWorkspace.
func (r *SetConfigRequest) fingerprint() string {
	return strings.Join([]string{
		r.Provider, r.GitHubHost, r.RepoOwner, r.RepoName, r.ProjectPath, r.Branch, r.Path,
		fmt.Sprintf("%d", r.IntervalSeconds),
	}, "\x1f")
}

// SetConfigRequest is the payload for creating or updating a workspace's
// workflow sync config. Branch, Path, and IntervalSeconds fall back to
// defaults when empty/zero.
type SetConfigRequest struct {
	// Provider is optional; an empty value means ProviderGitHub, which keeps
	// pre-GitLab API clients working unchanged.
	GitHubHost      string `json:"github_host"`
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

// Normalize validates the request and fills defaults. It returns a wrapped
// ErrInvalidConfig on bad input.
func (r *SetConfigRequest) Normalize() error {
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
