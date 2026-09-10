// Package configsync keeps an Office workspace's agent/skill/project/routine
// configuration in sync with definition files stored in a configured GitHub
// or GitLab repository. A background poller fetches the configured directory
// on an interval and reconciles it against the workspace's Office config
// tables; users can also force a sync from the settings UI.
//
// This package is deliberately standalone: it mirrors the poll-and-record
// lifecycle established by internal/workflowsync (same field vocabulary,
// same SyncResult shape) but shares no code with it, because the entities it
// reconciles (agents, skills, projects, routines) and their identity rules
// are entirely different from workflow definitions.
package configsync

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"
)

// Defaults for optional config fields.
const (
	DefaultBranch          = "main"
	DefaultIntervalSeconds = 300
	MinIntervalSeconds     = 60
	// MaxIntervalSeconds caps the poll interval at 30 days.
	MaxIntervalSeconds = 30 * 24 * 60 * 60
)

// Supported sync sources. A workspace has exactly one.
const (
	ProviderGitHub = "github"
	ProviderGitLab = "gitlab"
)

// ErrInvalidConfig marks validation failures on SetConfigRequest so handlers
// can map them to 400.
var ErrInvalidConfig = errors.New("invalid config sync config")

// ErrNotConfigured is returned when a sync is requested for a workspace that
// has no config sync configuration.
var ErrNotConfigured = errors.New("config sync is not configured for this workspace")

// ErrWorkspaceGone is returned by SetConfigForWorkspace when the workspace
// finished PurgeForWorkspaceDeletion before this call reached the front of
// the per-workspace lock. Without this check the write proceeds anyway (the
// config row has no foreign key to catch it) and resurrects a config row,
// and the poller a workspace no longer exists for, for a workspace the rest
// of Office has already forgotten.
var ErrWorkspaceGone = errors.New("workspace no longer exists")

// Config is the per-workspace config sync configuration plus the status of
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
// config sync configuration. Branch and IntervalSeconds fall back to
// defaults when empty/zero.
//
// Path is a *string, because Office needs three input states where workflow
// sync has two: nil (field absent) means the default, which for Office IS
// the repository root; "" means the repository root, explicitly; "a/b"
// means that directory (AC-OFFICE-CONFIG-SYNC-001.6). A plain string cannot
// tell an absent field from an empty one, so a settings page that reads a
// root-addressed config and writes it back unchanged would post "" and have
// it collide with a non-root default — this is the departure from
// workflow sync's Normalize, which rewrites both onto a non-empty default
// and so can never address a repository root.
type SetConfigRequest struct {
	// Provider is required and must be ProviderGitHub or ProviderGitLab; an
	// omitted value is rejected rather than defaulted.
	Provider        string  `json:"provider"`
	RepoOwner       string  `json:"repo_owner"`
	RepoName        string  `json:"repo_name"`
	ProjectPath     string  `json:"project_path"`
	Branch          string  `json:"branch"`
	Path            *string `json:"path"`
	IntervalSeconds int     `json:"interval_seconds"`
	// PollEnabled controls the background polling loop; nil defaults to
	// true. When false the workspace only syncs via "Sync now".
	PollEnabled *bool `json:"poll_enabled"`
}

// isValidBranchName is a narrow, local branch-name validator: printable,
// non-empty, no whitespace, no leading '-' (defence against argv injection
// into any future git-based fetch path), and no ".." traversal segment. This
// package fetches files exclusively through the GitHub/GitLab REST/API
// clients (never a raw git subprocess), so this only needs to keep the value
// safe to interpolate into a REST ref parameter — it intentionally does not
// reimplement git's full ref-name grammar.
func isValidBranchName(branch string) bool {
	if branch == "" || strings.TrimSpace(branch) != branch || strings.HasPrefix(branch, "-") {
		return false
	}
	return hasValidBranchSegments(branch) && hasValidBranchRunes(branch)
}

// hasValidBranchSegments rejects an empty, "." or ".." path segment.
func hasValidBranchSegments(branch string) bool {
	for _, seg := range strings.Split(branch, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

// hasValidBranchRunes rejects control characters and the git ref-name
// special characters that would be unsafe to interpolate into a REST ref
// parameter.
func hasValidBranchRunes(branch string) bool {
	const disallowed = " ~^:?*[\\"
	for _, r := range branch {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(disallowed, r) {
			return false
		}
	}
	return true
}

func (r *SetConfigRequest) normalizeTarget() error {
	r.Provider = strings.TrimSpace(r.Provider)
	if r.Provider == "" {
		return fmt.Errorf("%w: provider is required and must be %q or %q", ErrInvalidConfig, ProviderGitHub, ProviderGitLab)
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
	if err := r.normalizeTarget(); err != nil {
		return err
	}
	if r.Branch == "" {
		r.Branch = DefaultBranch
	}
	if !isValidBranchName(r.Branch) {
		return fmt.Errorf("%w: branch is not a valid git branch name", ErrInvalidConfig)
	}
	if err := r.normalizePath(); err != nil {
		return err
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

// normalizePath implements AC-OFFICE-CONFIG-SYNC-001.6 and
// AC-OFFICE-CONFIG-SYNC-001.7: a nil or empty Path means the repository
// root, and every other value is validated as-is rather than trimmed into
// validity, so what is stored is exactly what the walk uses. The one
// transform applied is Unicode NFC.
func (r *SetConfigRequest) normalizePath() error {
	if r.Path == nil {
		root := ""
		r.Path = &root
		return nil
	}
	p := norm.NFC.String(*r.Path)
	if err := validateConfigPath(p); err != nil {
		return err
	}
	r.Path = &p
	return nil
}

// validateConfigPath rejects everything AC-OFFICE-CONFIG-SYNC-001.7 lists: a
// ".." or "." segment, an empty segment from a repeated slash, a leading
// slash, a backslash, a NUL byte, and (since the empty string is itself a
// meaningful "repository root" value) a whitespace-only value or a trailing
// slash.
func validateConfigPath(p string) error {
	if strings.ContainsRune(p, 0) {
		return fmt.Errorf("%w: path cannot contain a NUL byte", ErrInvalidConfig)
	}
	if strings.Contains(p, "\\") {
		return fmt.Errorf("%w: path cannot contain a backslash", ErrInvalidConfig)
	}
	if strings.HasPrefix(p, "/") {
		return fmt.Errorf("%w: path cannot start with a slash", ErrInvalidConfig)
	}
	if p == "" {
		return nil
	}
	if strings.TrimSpace(p) != p {
		return fmt.Errorf("%w: path cannot have leading or trailing whitespace", ErrInvalidConfig)
	}
	if strings.HasSuffix(p, "/") {
		return fmt.Errorf("%w: path cannot end with a slash", ErrInvalidConfig)
	}
	for _, segment := range strings.Split(p, "/") {
		switch segment {
		case "":
			return fmt.Errorf("%w: path cannot contain an empty segment (repeated slash)", ErrInvalidConfig)
		case ".", "..":
			return fmt.Errorf("%w: path cannot contain \".\" or \"..\" segments", ErrInvalidConfig)
		}
	}
	return nil
}

// normalizePathFrame is the single normalization applied to every directory
// path this package handles: the configured root path, and (in walk.go) the
// path recorded against each fetched entity in the ownership manifest. Using
// one function on both sides is the fix for R5-F4 (manifest point-reads
// comparing the configured root against a freshly-listed provider path in
// two different frames): trim surrounding slashes/space and collapse to
// forward slashes, so "/foo/bar/", "foo/bar", and "foo\\bar" (an unlikely but
// possible GitLab path separator quirk) all normalize identically.
func normalizePathFrame(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), "\\", "/")
	return strings.Trim(p, "/")
}

// SyncResult reports the outcome of one sync run for a workspace.
//
// Unlike internal/workflowsync's SyncResult.Unchanged (which requires zero
// warnings too), Office's formula only looks at whether any row actually
// changed: a sync that produced warnings (e.g. a Foreign-identity skip, or a
// parse error on one file) but left every row untouched is still
// "Unchanged" for the purposes of this signal, since warnings here are
// per-entity outcomes surfaced independently in Warnings, not a marker that
// the reconciliation itself is incomplete.
type SyncResult struct {
	Created   []string `json:"created"`
	Updated   []string `json:"updated"`
	Deleted   []string `json:"deleted"`
	Warnings  []string `json:"warnings"`
	Unchanged bool     `json:"unchanged"`
}
