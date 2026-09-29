package coordinator

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrPolicyUnreadable marks a stored policy value ParsePolicy could not trust.
// The returned Policy denies every action.
var ErrPolicyUnreadable = errors.New("coordinator: stored policy unreadable")

// Setting is what a coordinator may do for one action.
type Setting string

// Policy settings. Automatic parses but Validate refuses it.
const (
	SettingDenied           Setting = "denied"
	SettingRequiresApproval Setting = "requires_approval"
	SettingAutomatic        Setting = "automatic"
)

// Action is a class of thing a coordinator may propose.
type Action string

// The six policy actions and the class recorded for a tool that maps to none.
const (
	ActionCreateTask Action = "create_task"
	ActionStartAgent Action = "start_agent"
	ActionMessage    Action = "message"
	ActionMove       Action = "move"
	ActionResume     Action = "resume"
	ActionStop       Action = "stop"
	ActionUnknown    Action = "unknown"
	// ActionImprovement is an activity-only class; it is not a policy action.
	ActionImprovement Action = "improvement"
)

// AllActions is the fixed action order; ActionUnknown is not a policy action.
var AllActions = []Action{ActionCreateTask, ActionStartAgent, ActionMessage, ActionMove, ActionResume, ActionStop}

const policyVersion = 1

// Policy is the stored per-coordinator permission map.
type Policy struct {
	Version int                `json:"version"`
	Actions map[Action]Setting `json:"actions"`
}

// PolicyFieldError names the action a Validate failure is about.
type PolicyFieldError struct {
	Field string
	Code  string
}

func (e *PolicyFieldError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("policy.actions.%s: %s", e.Field, e.Code)
	}
	return fmt.Sprintf("policy.actions.%s is invalid", e.Field)
}

func deniedActions() map[Action]Setting {
	m := make(map[Action]Setting, len(AllActions))
	for _, a := range AllActions {
		m[a] = SettingDenied
	}
	return m
}

// PhaseOnePolicy is create_task requires_approval with everything else denied.
func PhaseOnePolicy() Policy {
	p := Policy{Version: policyVersion, Actions: deniedActions()}
	p.Actions[ActionCreateTask] = SettingRequiresApproval
	return p
}

// Allows reports whether the action's setting is anything but denied. An
// action outside the six is never allowed.
func (p Policy) Allows(a Action) bool {
	s, ok := p.Actions[a]
	if !ok || !isPolicyAction(a) {
		return false
	}
	return s == SettingRequiresApproval || s == SettingAutomatic
}

func unreadablePolicy(reason string) (Policy, error) {
	return Policy{Version: policyVersion, Actions: deniedActions()}, fmt.Errorf("%w: %s", ErrPolicyUnreadable, reason)
}

// ParsePolicy is pure and never logs. NULL is the phase-1 policy; any value it
// cannot trust yields all six actions denied and ErrPolicyUnreadable; an
// action that is absent or holds an unusable setting is denied.
func ParsePolicy(raw *string) (Policy, error) {
	if raw == nil {
		return PhaseOnePolicy(), nil
	}
	if strings.TrimSpace(*raw) == "" {
		return unreadablePolicy("empty")
	}
	var top struct {
		Version int             `json:"version"`
		Actions json.RawMessage `json:"actions"`
	}
	if err := json.Unmarshal([]byte(*raw), &top); err != nil {
		return unreadablePolicy("invalid json")
	}
	if top.Version != policyVersion {
		return unreadablePolicy("unsupported version")
	}
	var stored map[string]json.RawMessage
	if err := json.Unmarshal(top.Actions, &stored); err != nil || stored == nil {
		return unreadablePolicy("actions is not an object")
	}
	p := Policy{Version: policyVersion, Actions: deniedActions()}
	for _, a := range AllActions {
		var s Setting
		if v, ok := stored[string(a)]; ok && json.Unmarshal(v, &s) == nil && validSetting(s) {
			p.Actions[a] = s
		}
	}
	return p, nil
}

func validSetting(s Setting) bool {
	return s == SettingDenied || s == SettingRequiresApproval || s == SettingAutomatic
}

// Validate refuses a policy a save may not store. The first failing action in
// the fixed action order is the one named; keys outside the six follow.
func Validate(p Policy) error {
	for _, a := range AllActions {
		s, ok := p.Actions[a]
		if !ok {
			s = SettingDenied
		}
		if !validSetting(s) {
			return &PolicyFieldError{Field: string(a)}
		}
		if s == SettingAutomatic {
			return &PolicyFieldError{Field: string(a), Code: "automatic_not_available"}
		}
		if a == ActionStop && s != SettingDenied {
			return &PolicyFieldError{Field: string(a)}
		}
	}
	for a := range p.Actions {
		if !isPolicyAction(a) {
			return &PolicyFieldError{Field: string(a)}
		}
	}
	return nil
}

func isPolicyAction(a Action) bool {
	for _, k := range AllActions {
		if k == a {
			return true
		}
	}
	return false
}
