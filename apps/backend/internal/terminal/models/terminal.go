// Package models defines persistent types for first-class user terminals.
//
// Ordinary user terminals (the ones created via the `+` button in the UI) have
// a DB row that survives backend restarts and page refreshes. The terminal id
// equals the agentctl PTY id; the row carries the per-task sequence number
// (stable across deletes, gaps preserved), the user's custom name override,
// and the open/parked state.
//
// The hardcoded `bottom-panel` terminal and script terminals (id prefix
// `script-`) are NOT persisted here — they pass straight through to agentctl.
// Guards live in the terminal service, not in this package.
package models

import (
	"fmt"
	"time"
)

// TerminalState is the user-facing visibility state for an ordinary terminal.
type TerminalState string

const (
	// StateOpen — terminal tab visible in the panel strip.
	StateOpen TerminalState = "open"
	// StateParked — tab hidden; PTY may still be running. User can resume
	// to bring it back.
	StateParked TerminalState = "parked"
)

// Terminal is a persisted ordinary user terminal.
type Terminal struct {
	ID             string        `db:"id"`
	TaskID         string        `db:"task_id"`
	EnvironmentID  string        `db:"environment_id"`
	Seq            int           `db:"seq"`
	CustomName     *string       `db:"custom_name"`
	State          TerminalState `db:"state"`
	InitialCommand string        `db:"initial_command"`
	CreatedAt      time.Time     `db:"created_at"`
}

// DisplayName returns the user-facing label: custom_name if set, else the
// derived "Terminal N" form.
func (t *Terminal) DisplayName() string {
	if t.CustomName != nil && *t.CustomName != "" {
		return *t.CustomName
	}
	return fmt.Sprintf("Terminal %d", t.Seq)
}
