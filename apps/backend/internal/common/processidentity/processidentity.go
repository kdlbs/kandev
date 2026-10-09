// Package processidentity records and verifies host process ownership without
// treating a PID or port alone as proof of identity.
package processidentity

import (
	"errors"
	"fmt"
)

var ErrUnverifiableIdentity = errors.New("process identity is incomplete or unsupported")

type State string

const (
	StateAlive   State = "alive"
	StateExited  State = "exited"
	StateReused  State = "reused"
	StateUnknown State = "unknown"
)

// Identity binds a process ID and its containment group to an OS-specific
// birth token. Empty birth tokens represent legacy or unsupported evidence.
type Identity struct {
	PID        int    `json:"pid"`
	GroupID    int    `json:"group_id,omitempty"`
	SessionID  int    `json:"session_id,omitempty"`
	BirthToken string `json:"birth_token,omitempty"`
}

func (identity Identity) Validate() error {
	if identity.PID <= 0 || identity.BirthToken == "" {
		return ErrUnverifiableIdentity
	}
	return nil
}

func (identity Identity) String() string {
	if identity.PID <= 0 {
		return "unknown"
	}
	return fmt.Sprintf("pid=%d group=%d session=%d", identity.PID, identity.GroupID, identity.SessionID)
}
