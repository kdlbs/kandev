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
)

// Defaults for optional config fields.
const (
	DefaultBranch          = "main"
	DefaultPath            = ".kandev/workflows"
	DefaultIntervalSeconds = 300
	MinIntervalSeconds     = 60
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
	WorkspaceID     string     `json:"workspace_id"`
	RepoOwner       string     `json:"repo_owner"`
	RepoName        string     `json:"repo_name"`
	Branch          string     `json:"branch"`
	Path            string     `json:"path"`
	IntervalSeconds int        `json:"interval_seconds"`
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
	RepoOwner       string `json:"repo_owner"`
	RepoName        string `json:"repo_name"`
	Branch          string `json:"branch"`
	Path            string `json:"path"`
	IntervalSeconds int    `json:"interval_seconds"`
}

// Normalize validates the request and fills defaults. It returns a wrapped
// ErrInvalidConfig on bad input.
func (r *SetConfigRequest) Normalize() error {
	r.RepoOwner = strings.TrimSpace(r.RepoOwner)
	r.RepoName = strings.TrimSpace(r.RepoName)
	r.Branch = strings.TrimSpace(r.Branch)
	r.Path = strings.Trim(strings.TrimSpace(r.Path), "/")
	if r.RepoOwner == "" || strings.ContainsAny(r.RepoOwner, "/ ") {
		return fmt.Errorf("%w: repo_owner is required and cannot contain slashes or spaces", ErrInvalidConfig)
	}
	if r.RepoName == "" || strings.ContainsAny(r.RepoName, "/ ") {
		return fmt.Errorf("%w: repo_name is required and cannot contain slashes or spaces", ErrInvalidConfig)
	}
	if r.Branch == "" {
		r.Branch = DefaultBranch
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
