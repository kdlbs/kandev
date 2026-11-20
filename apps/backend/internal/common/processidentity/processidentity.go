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
	// AgentGroupID and AgentBirthToken optionally name the agent process group
	// hosted by this runtime when a durable prompt was bound.
	AgentGroupID    int    `json:"agent_group_id,omitempty"`
	AgentBirthToken string `json:"agent_birth_token,omitempty"`
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

// WithAgentGroup records the process group of the agent child that this
// runtime hosted. Only a group leader with a birth token is recorded, so an
// unverifiable agent leaves the runtime-only identity unchanged.
func (identity Identity) WithAgentGroup(agent Identity) Identity {
	if agent.Validate() != nil || agent.GroupID != agent.PID {
		return identity
	}
	identity.AgentGroupID = agent.GroupID
	identity.AgentBirthToken = agent.BirthToken
	return identity
}

// OriginalOwnerTerminated proves that the recorded owner stopped writing: the
// runtime's owned OS session ended, or every process in the recorded agent
// group exited. A shared runtime can outlive one session's agent, so either
// proof is sufficient. Unverifiable group evidence never proves termination.
func OriginalOwnerTerminated(identity Identity) (bool, error) {
	terminated, err := OwnedSessionTerminated(identity)
	if terminated || identity.AgentGroupID <= 0 || identity.AgentBirthToken == "" {
		return terminated, err
	}
	groupTerminated, groupErr := OwnedGroupTerminated(Identity{
		PID: identity.AgentGroupID, GroupID: identity.AgentGroupID, BirthToken: identity.AgentBirthToken,
	})
	if groupErr == nil && groupTerminated {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if errors.Is(groupErr, ErrUnverifiableIdentity) {
		return false, nil
	}
	return false, groupErr
}
