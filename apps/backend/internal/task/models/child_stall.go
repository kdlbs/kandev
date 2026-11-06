package models

import (
	"encoding/json"
	"time"
)

// Turn metadata keys owned by the child-turn stalled signal. The source turn
// row is the durable candidate record; these keys stay compact.
const (
	// TurnMetaKeyChildStallStart is written in the turn-insert transaction for
	// a task that has a parent. It captures the parent, workspace, and workflow
	// entry the turn started in.
	TurnMetaKeyChildStallStart = "child_stall_start"
	// TurnMetaKeyChildStallSettlement records a non-live settlement
	// (abandoned or cancelled). Absent means a live settlement.
	TurnMetaKeyChildStallSettlement = "child_stall_settlement"
	// TurnMetaKeyChildStall holds the producer's classification and delivery
	// state for the turn.
	TurnMetaKeyChildStall = "child_stall"
	// TurnMetaKeyChildStallResolved marks a turn the producer no longer scans.
	TurnMetaKeyChildStallResolved = "child_stall_resolved"
	// TaskMetaKeyChildStallNudge holds the producer's consecutive
	// missing-completion-signal nudge streak for a child task, keyed by the
	// workflow entry the stalls share. It is task-scoped, not session-scoped, so
	// a session substitution does not reset the streak.
	TaskMetaKeyChildStallNudge = "child_stall_nudge"
	// SessionMetaKeyStepCompleteRejection holds the short reason a step_complete
	// call was rejected by the ownership/profile gate. The child reminder reads
	// it to name the real block instead of reporting a missing signal.
	SessionMetaKeyStepCompleteRejection = "step_complete_rejection"
)

// Child-turn settlement kinds stored under TurnMetaKeyChildStallSettlement.
const (
	ChildStallSettlementAbandoned = "abandoned"
	ChildStallSettlementCancelled = "cancelled"
)

// ChildStallStart is the start snapshot stored under TurnMetaKeyChildStallStart.
// A zero TransitionID means the workflow entry was unknown at turn start.
type ChildStallStart struct {
	ParentTaskID string `json:"parent_task_id"`
	WorkspaceID  string `json:"workspace_id"`
	TransitionID int64  `json:"transition_id,omitempty"`
}

// ToMap returns the JSON-compatible metadata value.
func (s ChildStallStart) ToMap() map[string]interface{} {
	value := map[string]interface{}{
		"parent_task_id": s.ParentTaskID,
		"workspace_id":   s.WorkspaceID,
	}
	if s.TransitionID != 0 {
		value["transition_id"] = s.TransitionID
	}
	return value
}

// ChildStallState is the producer state stored under TurnMetaKeyChildStall.
type ChildStallState struct {
	State            string     `json:"state"`
	Cause            string     `json:"cause,omitempty"`
	Reason           string     `json:"reason,omitempty"`
	QuestionID       string     `json:"question_id,omitempty"`
	QueueID          string     `json:"queue_id,omitempty"`
	ParentSessionID  string     `json:"parent_session_id,omitempty"`
	LastError        string     `json:"last_error,omitempty"`
	Attempts         int        `json:"attempts,omitempty"`
	NextAttemptAt    *time.Time `json:"next_attempt_at,omitempty"`
	OperatorNotified bool       `json:"operator_notified,omitempty"`
	FirstSeenAt      *time.Time `json:"first_seen_at,omitempty"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// ToMap returns the JSON-compatible metadata value.
func (s ChildStallState) ToMap() map[string]interface{} {
	encoded, err := json.Marshal(s)
	if err != nil {
		return map[string]interface{}{"state": s.State}
	}
	var value map[string]interface{}
	if err := json.Unmarshal(encoded, &value); err != nil {
		return map[string]interface{}{"state": s.State}
	}
	return value
}

// LoadChildStallStart decodes the start snapshot from turn metadata.
func LoadChildStallStart(metadata map[string]interface{}) (ChildStallStart, bool) {
	var start ChildStallStart
	if !decodeTurnMetadataValue(metadata, TurnMetaKeyChildStallStart, &start) || start.ParentTaskID == "" {
		return ChildStallStart{}, false
	}
	return start, true
}

// LoadChildStallState decodes the producer state from turn metadata.
func LoadChildStallState(metadata map[string]interface{}) (ChildStallState, bool) {
	var state ChildStallState
	if !decodeTurnMetadataValue(metadata, TurnMetaKeyChildStall, &state) {
		return ChildStallState{}, false
	}
	return state, true
}

// ChildStallNudgeStreak is the persisted consecutive missing-signal nudge
// state for one child task. It is keyed by the workflow entry the stalls
// share; a new entry starts a fresh streak.
type ChildStallNudgeStreak struct {
	Cause        string    `json:"cause,omitempty"`
	TransitionID int64     `json:"transition_id,omitempty"`
	Count        int       `json:"count,omitempty"`
	Escalated    bool      `json:"escalated,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ToMap returns the JSON-compatible metadata value.
func (s ChildStallNudgeStreak) ToMap() map[string]interface{} {
	encoded, err := json.Marshal(s)
	if err != nil {
		return map[string]interface{}{"count": s.Count}
	}
	var value map[string]interface{}
	if err := json.Unmarshal(encoded, &value); err != nil {
		return map[string]interface{}{"count": s.Count}
	}
	return value
}

// LoadChildStallNudgeStreak decodes the nudge streak from task metadata.
func LoadChildStallNudgeStreak(metadata map[string]interface{}) (ChildStallNudgeStreak, bool) {
	var streak ChildStallNudgeStreak
	if !decodeTurnMetadataValue(metadata, TaskMetaKeyChildStallNudge, &streak) {
		return ChildStallNudgeStreak{}, false
	}
	return streak, true
}

// StepCompleteRejection is the short reason a step_complete call was refused by
// the ownership/profile gate, recorded on the calling session. StepID scopes it
// to the step that was refused so a later step never reads a stale reason.
type StepCompleteRejection struct {
	StepID string    `json:"step_id"`
	Reason string    `json:"reason"`
	At     time.Time `json:"at"`
}

// ToMap returns the JSON-compatible metadata value.
func (s StepCompleteRejection) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"step_id": s.StepID,
		"reason":  s.Reason,
		"at":      s.At.UTC().Format(time.RFC3339Nano),
	}
}

// LoadStepCompleteRejection decodes a recorded step_complete rejection from
// session metadata.
func LoadStepCompleteRejection(metadata map[string]interface{}) (StepCompleteRejection, bool) {
	var rejection StepCompleteRejection
	if !decodeTurnMetadataValue(metadata, SessionMetaKeyStepCompleteRejection, &rejection) {
		return StepCompleteRejection{}, false
	}
	if rejection.Reason == "" {
		return StepCompleteRejection{}, false
	}
	return rejection, true
}

// ChildStallSettlement returns the recorded non-live settlement kind, or "".
func ChildStallSettlement(metadata map[string]interface{}) string {
	value, _ := metadata[TurnMetaKeyChildStallSettlement].(string)
	return value
}

func decodeTurnMetadataValue(metadata map[string]interface{}, key string, target interface{}) bool {
	raw, ok := metadata[key]
	if !ok || raw == nil {
		return false
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return false
	}
	return json.Unmarshal(encoded, target) == nil
}
