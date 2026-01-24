package models

import "time"

// SnapshotType represents the type of git snapshot
type SnapshotType string

const (
	// SnapshotTypeStatusUpdate is for regular git status updates
	SnapshotTypeStatusUpdate SnapshotType = "status_update"
	// SnapshotTypePreCommit is for snapshots taken before a commit
	SnapshotTypePreCommit SnapshotType = "pre_commit"
	// SnapshotTypePostCommit is for snapshots taken after a commit
	SnapshotTypePostCommit SnapshotType = "post_commit"
	// SnapshotTypePreStage is for snapshots taken before staging
	SnapshotTypePreStage SnapshotType = "pre_stage"
	// SnapshotTypePostStage is for snapshots taken after staging
	SnapshotTypePostStage SnapshotType = "post_stage"
)

// GitSnapshot represents a git status snapshot at a specific point in time
type GitSnapshot struct {
	ID           string                 `json:"id"`
	SessionID    string                 `json:"session_id"`
	SnapshotType SnapshotType           `json:"snapshot_type"`
	Branch       string                 `json:"branch"`
	RemoteBranch string                 `json:"remote_branch"`
	HeadCommit   string                 `json:"head_commit"`
	BaseCommit   string                 `json:"base_commit"`
	Ahead        int                    `json:"ahead"`
	Behind       int                    `json:"behind"`
	Files        map[string]interface{} `json:"files"` // FileInfo objects with diff content
	TriggeredBy  string                 `json:"triggered_by"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
}

// SessionCommit represents a commit made during a task session
type SessionCommit struct {
	ID                   string    `json:"id"`
	SessionID            string    `json:"session_id"`
	CommitSHA            string    `json:"commit_sha"`
	ParentSHA            string    `json:"parent_sha"`
	AuthorName           string    `json:"author_name"`
	AuthorEmail          string    `json:"author_email"`
	CommitMessage        string    `json:"commit_message"`
	CommittedAt          time.Time `json:"committed_at"`
	PreCommitSnapshotID  string    `json:"pre_commit_snapshot_id"`
	PostCommitSnapshotID string    `json:"post_commit_snapshot_id"`
	FilesChanged         int       `json:"files_changed"`
	Insertions           int       `json:"insertions"`
	Deletions            int       `json:"deletions"`
	CreatedAt            time.Time `json:"created_at"`
}

// CumulativeDiff represents the cumulative diff from base commit to current HEAD
type CumulativeDiff struct {
	SessionID    string                 `json:"session_id"`
	BaseCommit   string                 `json:"base_commit"`
	HeadCommit   string                 `json:"head_commit"`
	TotalCommits int                    `json:"total_commits"`
	Files        map[string]interface{} `json:"files"` // Cumulative file changes
}

