// Package models defines the durable descriptors for Quick Terminal tabs.
package models

import "time"

const (
	StatusConnecting = "connecting"
	StatusRunning    = "running"
	StatusExited     = "exited"
	StatusError      = "error"
)

// Tab is the server-owned identity and lifecycle snapshot for one Quick
// Terminal tab. The PTY itself remains owned by loginpty and is intentionally
// not persisted here.
type Tab struct {
	TabID       string    `db:"tab_id" json:"tabId"`
	UserID      string    `db:"user_id" json:"-"`
	WorkspaceID string    `db:"workspace_id" json:"workspaceId"`
	SessionID   *string   `db:"session_id" json:"sessionId"`
	Sequence    int       `db:"sequence" json:"sequence"`
	Status      string    `db:"status" json:"status"`
	ExitCode    *int      `db:"exit_code" json:"exitCode,omitempty"`
	Error       string    `db:"error" json:"error,omitempty"`
	CreatedAt   time.Time `db:"created_at" json:"-"`
	UpdatedAt   time.Time `db:"updated_at" json:"-"`
}
