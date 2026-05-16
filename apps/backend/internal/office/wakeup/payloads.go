// Package wakeup defines the typed payload structs for
// agent_wakeup_requests rows. The DB column is intentionally
// free-form JSON so adding a new wakeup source requires no schema
// migration; type safety lives here, with one struct per Source enum.
//
// The dispatcher unmarshals per-source on claim:
//
//	switch req.Source {
//	case wakeup.SourceComment:
//	    var p wakeup.CommentPayload
//	    _ = wakeup.UnmarshalPayload(req.Payload, &p)
//	}
//
// Adding a new source = add a struct + a switch case; no schema dance.
package wakeup

import (
	"encoding/json"
	"strings"
)

// Source enum — what produced the wakeup request. Stored verbatim in
// agent_wakeup_requests.source.
//
// "heartbeat" is intentionally absent. The agent-level heartbeat cron
// was retired in favour of the coordinator-heartbeat routine, so every
// scheduled wake now flows through SourceRoutine.
const (
	SourceComment    = "comment"
	SourceAgentError = "agent_error"
	SourceRoutine    = "routine"
	SourceSelf       = "self"
	SourceUser       = "user"
)

// CommentPayload is the wakeup-request payload for new task-comment
// events. Carries the originating task + comment IDs so the dispatcher
// can attach them to the run's context_snapshot for the agent.
type CommentPayload struct {
	TaskID    string `json:"task_id"`
	CommentID string `json:"comment_id"`
}

// AgentErrorPayload is the wakeup-request payload for an agent-error
// escalation: the failing agent + the run that failed + the error
// message. The escalating agent (typically the coordinator) reads
// these via the run's context_snapshot.
type AgentErrorPayload struct {
	AgentProfileID string `json:"agent_profile_id"`
	RunID          string `json:"run_id"`
	Error          string `json:"error"`
}

// RoutinePayload is the wakeup-request payload for cron-fired
// routines. Variables carries the routine's interpolation variables
// (free-form JSON) so the dispatcher / scriptengine can render them
// into the prompt at run-build time.
//
// MissedTicks is set by the routines cron tick when the catch-up cap
// collapsed N missed fires into one. Zero on the happy path; non-zero
// signals "you missed N ticks since the last fire" — the prompt
// builder surfaces it in the wake context so the agent can decide
// whether the gap matters.
type RoutinePayload struct {
	RoutineID   string         `json:"routine_id"`
	Variables   map[string]any `json:"variables,omitempty"`
	MissedTicks int            `json:"missed_ticks,omitempty"`
}

// SelfPayload is the wakeup-request payload for an agent-initiated
// self-wake (e.g. via a "schedule_followup" tool call). The agent
// records why it wanted to be woken; the next fire surfaces this in
// the wake context.
type SelfPayload struct {
	Reason string `json:"reason,omitempty"`
	Note   string `json:"note,omitempty"`
}

// UserPayload is the wakeup-request payload for a user-initiated wake
// (e.g. an explicit "wake the coordinator" UI button). Carries the
// optional message the user attached.
type UserPayload struct {
	UserID  string `json:"user_id,omitempty"`
	Message string `json:"message,omitempty"`
}

// MarshalPayload renders v to a JSON string suitable for the
// agent_wakeup_requests.payload column. Returns "{}" for nil so the
// caller never has to remember the empty-payload sentinel.
func MarshalPayload(v any) (string, error) {
	if v == nil {
		return "{}", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	out := string(b)
	if strings.TrimSpace(out) == "" {
		return "{}", nil
	}
	return out, nil
}

// UnmarshalPayload populates v from a JSON-encoded payload string.
// Empty / "{}" decode cleanly into v (no fields set). Errors propagate
// verbatim so callers can decide how to react to corrupt rows.
func UnmarshalPayload(raw string, v any) error {
	if v == nil {
		return nil
	}
	if strings.TrimSpace(raw) == "" || raw == "{}" {
		return nil
	}
	return json.Unmarshal([]byte(raw), v)
}
