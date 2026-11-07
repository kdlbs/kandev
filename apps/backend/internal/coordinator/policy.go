package coordinator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrPolicyUnreadable marks a stored policy value ParsePolicy could not trust.
// The returned Policy denies every action.
var ErrPolicyUnreadable = errors.New("coordinator: stored policy unreadable")

// Setting is what a coordinator may do for one action.
type Setting string

// Policy settings. Validate accepts Automatic only for a raisable class.
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

const (
	policyVersion     = 1
	eligibilityPrefix = "eligibility."
)

// Policy is the stored per-coordinator permission map.
type Policy struct {
	Version int                `json:"version"`
	Actions map[Action]Setting `json:"actions"`
	// Eligibility is the stored eligibility object; nil means the defaults.
	Eligibility *EligibilitySettings `json:"eligibility,omitempty"`
}

// EligibilitySettings are the five bounded gates that earn a raise to
// automatic, stored beside the actions.
type EligibilitySettings struct {
	EvidenceWindowHours int `json:"evidence_window_hours"`
	MinDecisions        int `json:"min_decisions"`
	UneditedFloorPct    int `json:"unedited_floor_percent"`
	ReviewMaxAgeHours   int `json:"review_max_age_hours"`
	DailyLimit          int `json:"daily_limit"`
}

// eligibilityField is one eligibility setting: its JSON name, bounds and the
// accessor the bounds check and the parser share.
type eligibilityField struct {
	name     string
	def      int
	min, max int
	ptr      func(*EligibilitySettings) *int
}

// eligibilityFields is the field table; its order is the order errors are
// reported in.
var eligibilityFields = []eligibilityField{
	{"evidence_window_hours", 720, 24, 2160, func(e *EligibilitySettings) *int { return &e.EvidenceWindowHours }},
	{"min_decisions", 20, 3, 500, func(e *EligibilitySettings) *int { return &e.MinDecisions }},
	{"unedited_floor_percent", 90, 50, 100, func(e *EligibilitySettings) *int { return &e.UneditedFloorPct }},
	{"review_max_age_hours", 168, 1, 720, func(e *EligibilitySettings) *int { return &e.ReviewMaxAgeHours }},
	{"daily_limit", 10, 1, 100, func(e *EligibilitySettings) *int { return &e.DailyLimit }},
}

// DefaultEligibility holds the values the constant gates used before they
// became settings.
func DefaultEligibility() EligibilitySettings {
	var e EligibilitySettings
	for _, f := range eligibilityFields {
		*f.ptr(&e) = f.def
	}
	return e
}

// EffectiveEligibility is the stored object, or the defaults when absent.
func (p Policy) EffectiveEligibility() EligibilitySettings {
	if p.Eligibility == nil {
		return DefaultEligibility()
	}
	return *p.Eligibility
}

func (e EligibilitySettings) evidenceWindow() time.Duration {
	return time.Duration(e.EvidenceWindowHours) * time.Hour
}

func (e EligibilitySettings) reviewMaxAge() time.Duration {
	return time.Duration(e.ReviewMaxAgeHours) * time.Hour
}

// eligibilityFromStored reads the stored eligibility member. A field that is
// missing, not an integer or outside its bounds takes its default alone; a
// member that is not an object takes every default.
func eligibilityFromStored(raw json.RawMessage) *EligibilitySettings {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var member map[string]json.RawMessage
	if err := json.Unmarshal(raw, &member); err != nil || member == nil {
		return nil
	}
	e := DefaultEligibility()
	for _, f := range eligibilityFields {
		var n int
		if v, ok := member[f.name]; ok && json.Unmarshal(v, &n) == nil && n >= f.min && n <= f.max {
			*f.ptr(&e) = n
		}
	}
	return &e
}

// PolicyFieldError names the action a Validate failure is about.
type PolicyFieldError struct {
	Field string
	Code  string
}

func (e *PolicyFieldError) Error() string {
	if strings.HasPrefix(e.Field, eligibilityPrefix) {
		return fmt.Sprintf("policy.%s is out of bounds", e.Field)
	}
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
		Version     int             `json:"version"`
		Actions     json.RawMessage `json:"actions"`
		Eligibility json.RawMessage `json:"eligibility"`
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
	p := Policy{Version: policyVersion, Actions: deniedActions(), Eligibility: eligibilityFromStored(top.Eligibility)}
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
// Automatic is accepted only for a raisable class, and only while phase 3 is
// effective.
func Validate(p Policy, phase3 bool) error {
	for _, a := range AllActions {
		s, ok := p.Actions[a]
		if !ok {
			s = SettingDenied
		}
		if !validSetting(s) {
			return &PolicyFieldError{Field: string(a)}
		}
		if s == SettingAutomatic && (!phase3 || !raisableClasses[a]) {
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
	return validateEligibility(p.Eligibility)
}

// validateEligibility checks the bounds of a present eligibility object. The
// structural checks (shape, missing, unknown and non-integer fields) belong to
// the settings parser, which sees the raw body.
func validateEligibility(e *EligibilitySettings) error {
	if e == nil {
		return nil
	}
	for _, f := range eligibilityFields {
		if n := *f.ptr(e); n < f.min || n > f.max {
			return &PolicyFieldError{Field: eligibilityPrefix + f.name, Code: codeInvalidSetting}
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
