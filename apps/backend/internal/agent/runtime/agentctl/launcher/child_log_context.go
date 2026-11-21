package launcher

import (
	"encoding/json"
	"math"
	"strings"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/logger"
)

// childLogRecordFields is the fixed reconstruction shape for one recognized
// child record. Optional fields carry only values that passed validation.
type childLogRecordFields struct {
	Level         string                `json:"level"`
	Timestamp     string                `json:"timestamp"`
	Caller        string                `json:"caller"`
	Message       string                `json:"msg"`
	Flow          *logger.ExecutionFlow `json:"flow,omitempty"`
	Component     json.RawMessage       `json:"component,omitempty"`
	TaskID        json.RawMessage       `json:"task_id,omitempty"`
	SessionID     json.RawMessage       `json:"session_id,omitempty"`
	InstanceID    json.RawMessage       `json:"instance_id,omitempty"`
	ExecutionID   json.RawMessage       `json:"execution_id,omitempty"`
	SubmissionID  json.RawMessage       `json:"submission_id,omitempty"`
	RequestID     json.RawMessage       `json:"request_id,omitempty"`
	State         json.RawMessage       `json:"state,omitempty"`
	ErrorClass    json.RawMessage       `json:"error_class,omitempty"`
	Reason        json.RawMessage       `json:"reason,omitempty"`
	Operation     json.RawMessage       `json:"operation,omitempty"`
	Trigger       json.RawMessage       `json:"trigger,omitempty"`
	Runtime       json.RawMessage       `json:"runtime,omitempty"`
	PollMode      json.RawMessage       `json:"poll_mode,omitempty"`
	ExitCode      json.RawMessage       `json:"exit_code,omitempty"`
	ExitReason    json.RawMessage       `json:"process_reason,omitempty"`
	ExitState     json.RawMessage       `json:"exit_disposition,omitempty"`
	Generation    json.RawMessage       `json:"process_generation,omitempty"`
	RetainedBytes json.RawMessage       `json:"retained_bytes,omitempty"`
	LimitBytes    json.RawMessage       `json:"limit_bytes,omitempty"`
}

const childLogComponentField = "component"

var childLogRecordFieldTargets = map[string]func(*childLogRecordFields) *json.RawMessage{
	childLogComponentField: func(f *childLogRecordFields) *json.RawMessage { return &f.Component },
	"task_id":              func(f *childLogRecordFields) *json.RawMessage { return &f.TaskID },
	"session_id":           func(f *childLogRecordFields) *json.RawMessage { return &f.SessionID },
	"instance_id":          func(f *childLogRecordFields) *json.RawMessage { return &f.InstanceID },
	"execution_id":         func(f *childLogRecordFields) *json.RawMessage { return &f.ExecutionID },
	"submission_id":        func(f *childLogRecordFields) *json.RawMessage { return &f.SubmissionID },
	"request_id":           func(f *childLogRecordFields) *json.RawMessage { return &f.RequestID },
	"state":                func(f *childLogRecordFields) *json.RawMessage { return &f.State },
	"error_class":          func(f *childLogRecordFields) *json.RawMessage { return &f.ErrorClass },
	"reason":               func(f *childLogRecordFields) *json.RawMessage { return &f.Reason },
	"operation":            func(f *childLogRecordFields) *json.RawMessage { return &f.Operation },
	"trigger":              func(f *childLogRecordFields) *json.RawMessage { return &f.Trigger },
	"runtime":              func(f *childLogRecordFields) *json.RawMessage { return &f.Runtime },
	"poll_mode":            func(f *childLogRecordFields) *json.RawMessage { return &f.PollMode },
	"exit_code":            func(f *childLogRecordFields) *json.RawMessage { return &f.ExitCode },
	"process_reason":       func(f *childLogRecordFields) *json.RawMessage { return &f.ExitReason },
	"exit_disposition":     func(f *childLogRecordFields) *json.RawMessage { return &f.ExitState },
	"process_generation":   func(f *childLogRecordFields) *json.RawMessage { return &f.Generation },
	"retained_bytes":       func(f *childLogRecordFields) *json.RawMessage { return &f.RetainedBytes },
	"limit_bytes":          func(f *childLogRecordFields) *json.RawMessage { return &f.LimitBytes },
}

func (f *childLogRecordFields) set(key string, raw json.RawMessage) {
	target, ok := childLogRecordFieldTargets[key]
	if !ok {
		return
	}
	*target(f) = raw
}

// childLogContext collects optional child fields while the envelope is
// parsed. Unknown keys are ignored; malformed values never fall back to raw
// line forwarding.
type childLogContext struct {
	seen             map[string]int
	valid            map[string]json.RawMessage
	component        json.RawMessage
	componentInvalid bool
}

func newChildLogContext() *childLogContext {
	return &childLogContext{
		seen:  make(map[string]int, len(childLogContextValidators)),
		valid: make(map[string]json.RawMessage, len(childLogContextValidators)),
	}
}

func (c *childLogContext) observe(key string, raw json.RawMessage) {
	validator, ok := childLogContextValidators[key]
	if !ok {
		return
	}
	c.seen[key]++
	if !validator(raw) {
		// A component may be repeated by nested loggers; one unsupported
		// occurrence omits the whole field.
		if key == childLogComponentField {
			c.componentInvalid = true
			c.component = nil
		}
		return
	}
	if key == childLogComponentField {
		c.component = raw
		return
	}
	if c.seen[key] == 1 {
		c.valid[key] = raw
	} else {
		delete(c.valid, key)
	}
}

func (c *childLogContext) apply(fields *childLogRecordFields) {
	for key := range childLogContextValidators {
		if key == childLogComponentField || c.seen[key] != 1 {
			continue
		}
		if raw, ok := c.valid[key]; ok {
			fields.set(key, raw)
		}
	}
	if c.component != nil && !c.componentInvalid {
		fields.Component = c.component
	}
}

var childLogComponents = stringSet(
	"process-manager", "workspace-tracker", "instance-manager", "control-server",
	"api-server", "process-runner", "interactive-runner", "shell", "shell-manager",
	"git-operator", "status-tracker", "workspace-preview", "vscode-manager",
	"wakeup-scheduler",
)

var childLogStates = stringSet(
	"healthy", "pressure", "guarded", "cancelling", "interrupted",
	"cancellation_failed", "storage_error", "delivery_failed",
)

var childLogErrorClasses = stringSet(
	"permission_denied", "not_found", "context_canceled", "deadline_exceeded",
	"git_start_failed", "git_command_failed", "repository_context_unavailable",
	"io_error", "unknown", "agent_running", "managed_runtime_npm_resolution",
	"managed_runtime_npm_policy", "unsupported_profile_context", "authentication_required",
	"evidence_changed",
)

var childLogOperations = stringSet(
	"read_directory", "read_file", "stat", "git_inspection", "submodule_discovery",
	"agent_configure", "agent_probe",
)

var childLogTriggers = stringSet("manual_refresh", "user_select", "turn_complete", "startup_grace", "poll")

var childLogProcessReasons = stringSet("mise_config_untrusted", "unclassified")

var childLogExitDispositions = stringSet(
	"ordinary_exit", "intentional_stop", "signal_exit", "unknown",
)

var childLogContextValidators = map[string]func(json.RawMessage) bool{
	childLogComponentField: childLogAllowedString(childLogComponents, 64),
	"task_id":              childLogUUID,
	"session_id":           childLogUUID,
	"instance_id":          childLogUUID,
	"execution_id":         childLogUUID,
	"submission_id":        childLogUUID,
	"request_id":           childLogUUID,
	"state":                childLogAllowedString(childLogStates, 32),
	"error_class":          childLogAllowedString(childLogErrorClasses, 40),
	"reason":               childLogAllowedString(childLogErrorClasses, 40),
	"operation":            childLogAllowedString(childLogOperations, 32),
	"trigger":              childLogAllowedString(childLogTriggers, 32),
	"runtime":              childLogAllowedString(stringSet("desktop", "server"), 16),
	"poll_mode":            childLogAllowedString(stringSet("fast", "slow", "paused"), 16),
	"exit_code":            childLogInteger(-1, 4294967295),
	"process_reason":       childLogAllowedString(childLogProcessReasons, 40),
	"exit_disposition":     childLogAllowedString(childLogExitDispositions, 32),
	"process_generation":   childLogInteger(1, math.MaxInt64),
	"retained_bytes":       childLogInteger(0, math.MaxInt64),
	"limit_bytes":          childLogInteger(0, math.MaxInt64),
}

func stringSet(values ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func childLogAllowedString(allowed map[string]struct{}, maxBytes int) func(json.RawMessage) bool {
	return func(raw json.RawMessage) bool {
		value, ok := childJSONString(raw)
		if !ok || len(value) > maxBytes {
			return false
		}
		_, allowedValue := allowed[value]
		return allowedValue
	}
}

func childLogUUID(raw json.RawMessage) bool {
	value, ok := childJSONString(raw)
	if !ok || len(value) != 36 || strings.ToLower(value) != value {
		return false
	}
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.String() == value
}

func childLogInteger(minimum, maximum int64) func(json.RawMessage) bool {
	return func(raw json.RawMessage) bool {
		var value *int64
		if err := json.Unmarshal(raw, &value); err != nil || value == nil {
			return false
		}
		return *value >= minimum && *value <= maximum
	}
}
