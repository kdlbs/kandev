package instance

import (
	"sync"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// TurnOutcome pairs a terminal agent event with the identifier assigned to
// the turn it belongs to (AC-EXECUTORS-SURVIVAL-004.1). The identifier is
// scoped to this control server's own process lifetime -- it is not the
// durable, cross-restart Kandev turn identity the rest of the backend
// already tracks (streams.AgentEvent.TurnID). Reusing that value would tie
// this retention mechanism to identifiers that already predate any given
// control-server launch, when the AC explicitly only requires uniqueness
// "for as long as that control server runs".
type TurnOutcome struct {
	TurnID int64              `json:"turn_id"`
	Event  streams.AgentEvent `json:"event"`
}

// turnOutcomeState holds the single retained outcome slot for one instance.
// Only the LAST terminal outcome is kept (design 03: "agentctl therefore
// retains each instance's last terminal turn outcome") -- retaining a
// history is unnecessary because a session can only be in one terminal
// state at re-track time, and design 03 explicitly leaves applying that one
// state as the whole job of AC-EXECUTORS-SURVIVAL-004.2.
type turnOutcomeState struct {
	mu       sync.Mutex
	retained *TurnOutcome
}

// Retain records event as the instance's new last terminal outcome under
// the given turnID, replacing whatever was retained before. Retrieval never
// clears this state (AC-EXECUTORS-SURVIVAL-004.6) -- only Ack does.
func (s *turnOutcomeState) Retain(turnID int64, event streams.AgentEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.retained = &TurnOutcome{TurnID: turnID, Event: event}
}

// Peek returns the retained outcome, if any, without discarding it. Calling
// it repeatedly returns the same outcome and turn identifier until Ack
// removes it (AC-EXECUTORS-SURVIVAL-004.6's repeatable, non-discarding read).
func (s *turnOutcomeState) Peek() (TurnOutcome, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.retained == nil {
		return TurnOutcome{}, false
	}
	return *s.retained, true
}

// Ack discards the retained outcome only when turnID names the one
// currently retained. Naming an identifier that is not (or no longer)
// retained is accepted and changes nothing, so a retried acknowledgement --
// or one that arrives after a newer outcome has already replaced it -- is
// always safe (AC-EXECUTORS-SURVIVAL-004.6).
func (s *turnOutcomeState) Ack(turnID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.retained != nil && s.retained.TurnID == turnID {
		s.retained = nil
	}
}
