package lsp

import (
	"errors"
	"fmt"
	"math"
	"time"
)

type Policy string

const (
	PolicyInherit  Policy = "inherit"
	PolicyKeepWarm Policy = "keep_warm"
	PolicyDisabled Policy = "disabled"
)

type DetectionState string

const (
	DetectionUnknown     DetectionState = "unknown"
	DetectionScanning    DetectionState = "scanning"
	DetectionComplete    DetectionState = "complete"
	DetectionPartial     DetectionState = "partial"
	DetectionUnavailable DetectionState = "unavailable"
)

type Phase string

const (
	PhaseOff            Phase = "off"
	PhaseWaitingForTask Phase = "waiting_for_task"
	PhaseQueued         Phase = "queued"
	PhaseInstalling     Phase = "installing"
	PhaseStarting       Phase = "starting"
	PhaseProcessStarted Phase = "process_started"
	PhaseInitializing   Phase = "initializing"
	PhaseReady          Phase = "ready"
	PhaseStopping       Phase = "stopping"
	PhaseError          Phase = "error"
	PhaseUnsupported    Phase = "unsupported"
)

type Action string

const (
	ActionNone      Action = ""
	ActionStart     Action = "start"
	ActionStop      Action = "stop"
	ActionRestart   Action = "restart"
	ActionSetPolicy Action = "set_policy"
	ActionReconcile Action = "reconcile"
)

type Initiator string

const (
	InitiatorUser      Initiator = "user"
	InitiatorAgent     Initiator = "agent"
	InitiatorAutomatic Initiator = "automatic"
)

var (
	ErrRevisionConflict = errors.New("task LSP language revision conflict")
)

// TaskLanguageState is the durable lifecycle projection for one task/language.
// Runtime transport and execution identifiers deliberately do not appear here.
type TaskLanguageState struct {
	TaskID             string         `json:"task_id"`
	Language           string         `json:"language"`
	Policy             Policy         `json:"policy"`
	Detected           bool           `json:"detected"`
	DetectionState     DetectionState `json:"detection_state"`
	DetectionScannedAt *time.Time     `json:"detection_scanned_at,omitempty"`
	DetectionTruncated bool           `json:"detection_truncated"`
	Phase              Phase          `json:"phase"`
	Generation         uint64         `json:"generation"`
	Revision           uint64         `json:"revision"`
	// Runtime* is internal ordering evidence from the task host. It prevents an
	// older HTTP response or retired host watch from regressing durable state,
	// but is not part of the user-visible task LSP contract.
	RuntimeIncarnation    string     `json:"-"`
	RuntimeStartedAt      *time.Time `json:"-"`
	RuntimeRevision       uint64     `json:"-"`
	ProcessStartedAt      *time.Time `json:"process_started_at,omitempty"`
	InitializeStartedAt   *time.Time `json:"initialize_started_at,omitempty"`
	ReadyAt               *time.Time `json:"ready_at,omitempty"`
	LastTransitionAt      time.Time  `json:"last_transition_at"`
	LastAction            Action     `json:"last_action"`
	LastActionAt          *time.Time `json:"last_action_at,omitempty"`
	LastStopReason        string     `json:"last_stop_reason,omitempty"`
	LastRestartReason     string     `json:"last_restart_reason,omitempty"`
	LastInitiator         Initiator  `json:"last_initiator"`
	RestartRequired       bool       `json:"restart_required"`
	RestartRequiredReason string     `json:"restart_required_reason,omitempty"`
	ErrorCode             string     `json:"error_code,omitempty"`
	ErrorMessage          string     `json:"error_message,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

func DefaultTaskLanguageState(taskID, language string) TaskLanguageState {
	return TaskLanguageState{
		TaskID:         taskID,
		Language:       language,
		Policy:         PolicyInherit,
		DetectionState: DetectionUnknown,
		Phase:          PhaseOff,
		LastInitiator:  InitiatorAutomatic,
	}
}

func (s TaskLanguageState) Validate() error {
	if s.TaskID == "" {
		return fmt.Errorf("task_id is required")
	}
	if s.Language == "" {
		return fmt.Errorf("language is required")
	}
	if !validPolicy(s.Policy) {
		return fmt.Errorf("invalid task LSP policy %q", s.Policy)
	}
	if !validDetectionState(s.DetectionState) {
		return fmt.Errorf("invalid task LSP detection state %q", s.DetectionState)
	}
	if !validPhase(s.Phase) {
		return fmt.Errorf("invalid task LSP phase %q", s.Phase)
	}
	if !validAction(s.LastAction) {
		return fmt.Errorf("invalid task LSP action %q", s.LastAction)
	}
	if !validInitiator(s.LastInitiator) {
		return fmt.Errorf("invalid task LSP initiator %q", s.LastInitiator)
	}
	if s.Generation > math.MaxInt64 || s.Revision > math.MaxInt64 || s.RuntimeRevision > math.MaxInt64 {
		return fmt.Errorf("task LSP generation/revision exceeds database range")
	}
	if s.RestartRequired != (s.RestartRequiredReason != "") {
		return fmt.Errorf("restart_required and restart_required_reason disagree")
	}
	return nil
}

func validPolicy(value Policy) bool {
	return value == PolicyInherit || value == PolicyKeepWarm || value == PolicyDisabled
}

func validDetectionState(value DetectionState) bool {
	switch value {
	case DetectionUnknown, DetectionScanning, DetectionComplete, DetectionPartial, DetectionUnavailable:
		return true
	default:
		return false
	}
}

func validPhase(value Phase) bool {
	switch value {
	case PhaseOff, PhaseWaitingForTask, PhaseQueued, PhaseInstalling, PhaseStarting,
		PhaseProcessStarted, PhaseInitializing, PhaseReady, PhaseStopping, PhaseError,
		PhaseUnsupported:
		return true
	default:
		return false
	}
}

func validAction(value Action) bool {
	switch value {
	case ActionNone, ActionStart, ActionStop, ActionRestart, ActionSetPolicy, ActionReconcile:
		return true
	default:
		return false
	}
}

func validInitiator(value Initiator) bool {
	return value == InitiatorUser || value == InitiatorAgent || value == InitiatorAutomatic
}
