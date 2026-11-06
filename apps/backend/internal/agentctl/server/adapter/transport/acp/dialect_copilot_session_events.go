package acp

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// copilotSessionEventMethod is the exact vendor notification method Copilot
// emits for its native session event stream. It has no leading underscore, so
// it only reaches the extension callback when the connection explicitly allows
// it.
const copilotSessionEventMethod = "github.com/copilot/sessionEvent"

const (
	copilotNativeTypeCompactionStart    = "session.compaction_start"
	copilotNativeTypeCompactionComplete = "session.compaction_complete"
	copilotNativeTypeUsage              = "session.usage_info"

	// copilotNativeMaxPayloadBytes bounds one decoded vendor notification before
	// normalization. Copilot itself caps event data at 32 KiB; the extra room
	// covers the envelope without accepting unbounded provider payloads.
	copilotNativeMaxPayloadBytes = 64 << 10

	// copilotReportingReasonOmitted marks a recognized root completion whose
	// data was withheld by the provider, so counting it would be a guess.
	copilotReportingReasonOmitted = "completion_data_omitted"
	// copilotReportingReasonUnusable marks a recognized root completion whose
	// typed outcome fields were missing or malformed.
	copilotReportingReasonUnusable = "completion_unusable"
)

// decodedCopilotNativeEvent is a bounded, attribution-checked vendor envelope.
// Root means the top-level agentId property was absent. Child and malformed
// attribution never carry counting authority.
type decodedCopilotNativeEvent struct {
	SessionID   string
	Type        string
	Timestamp   string
	Root        bool
	Child       bool
	Malformed   bool
	DataOmitted bool
	Data        json.RawMessage
}

// copilotCapabilityMetaKey is the vendor capability namespace Copilot reads to
// opt into its native session-event stream.
const copilotCapabilityMetaKey = "github.com/copilot"

// decodeCopilotNativeEvent validates the common Copilot envelope. It returns
// ok=false for a foreign method, an unsupported event type, a non-object
// payload, or an oversized payload.
func decodeCopilotNativeEvent(method string, params json.RawMessage) (decodedCopilotNativeEvent, bool) {
	var decoded decodedCopilotNativeEvent
	envelope, ok := decodeCopilotEnvelope(method, params)
	if !ok {
		return decoded, false
	}
	decoded.SessionID = copilotEnvelopeString(envelope["sessionId"])
	decoded.Type = copilotEnvelopeString(envelope["type"])
	if decoded.SessionID == "" || !knownCopilotNativeType(decoded.Type) {
		return decoded, false
	}
	decoded.Timestamp = copilotEnvelopeString(envelope["timestamp"])
	decoded.Root, decoded.Child, decoded.Malformed = copilotAgentAttribution(envelope)
	data, omitted, malformed := copilotEnvelopeData(envelope)
	decoded.Data = data
	decoded.DataOmitted = omitted
	decoded.Malformed = decoded.Malformed || malformed
	return decoded, true
}

func decodeCopilotEnvelope(method string, params json.RawMessage) (map[string]json.RawMessage, bool) {
	if method != copilotSessionEventMethod || len(params) == 0 || len(params) > copilotNativeMaxPayloadBytes {
		return nil, false
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(params, &envelope); err != nil || envelope == nil {
		return nil, false
	}
	return envelope, true
}

func copilotEnvelopeString(raw json.RawMessage) string {
	var value string
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return ""
	}
	return value
}

func knownCopilotNativeType(eventType string) bool {
	switch eventType {
	case copilotNativeTypeCompactionStart, copilotNativeTypeCompactionComplete, copilotNativeTypeUsage:
		return true
	default:
		return false
	}
}

func copilotAgentAttribution(envelope map[string]json.RawMessage) (root, child, malformed bool) {
	raw, present := envelope["agentId"]
	if !present {
		return true, false, false
	}
	if isJSONNull(raw) {
		return false, false, true
	}
	agentID := copilotEnvelopeString(raw)
	if strings.TrimSpace(agentID) == "" {
		return false, false, true
	}
	return false, true, false
}

func copilotEnvelopeData(envelope map[string]json.RawMessage) (data json.RawMessage, omitted, malformed bool) {
	if raw := envelope["dataOmitted"]; len(raw) > 0 && !isJSONNull(raw) {
		omitted = true
	}
	data = envelope["data"]
	if len(data) == 0 || isJSONNull(data) {
		return data, true, false
	}
	var dataObject map[string]json.RawMessage
	if err := json.Unmarshal(data, &dataObject); err != nil || dataObject == nil {
		return data, omitted, true
	}
	if raw := dataObject["omitted"]; len(raw) > 0 && !isJSONNull(raw) {
		omitted = true
	}
	return data, omitted, false
}

func isJSONNull(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "null"
}

type copilotNativeCompactionData struct {
	CompactionAttemptID  string `json:"compactionAttemptId"`
	Success              *bool  `json:"success"`
	Outcome              string `json:"outcome"`
	Trigger              string `json:"trigger"`
	PreCompactionTokens  *int64 `json:"preCompactionTokens"`
	PostCompactionTokens *int64 `json:"postCompactionTokens"`
	FailureReason        string `json:"failureReason"`
	Error                string `json:"error"`
}

// copilotCompactionCompletion returns the countable observation and whether a
// recognized root completion must be reported as partial evidence. A known
// failure or non-applied outcome is complete evidence, not missing evidence;
// malformed or missing outcome/identity fields fail closed as partial.
func copilotCompactionCompletion(event decodedCopilotNativeEvent) (*streams.ContextCompactionObservation, bool) {
	if event.Type != copilotNativeTypeCompactionComplete {
		return nil, false
	}
	if event.DataOmitted {
		return nil, true
	}
	if !event.Root {
		return nil, false
	}
	if event.Malformed {
		return nil, true
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(event.Data, &raw); err != nil || raw == nil {
		return nil, true
	}
	attemptID := strings.TrimSpace(copilotRawString(raw["compactionAttemptId"]))
	success, successOK := copilotRawBool(raw["success"])
	outcome, outcomeOK := copilotRawStringValue(raw["outcome"])
	if attemptID == "" || !successOK || !outcomeOK {
		return nil, true
	}
	if !success {
		return nil, false
	}
	if outcome != "applied" {
		return nil, !copilotKnownNonAppliedOutcome(outcome)
	}
	trigger, _ := copilotRawStringValue(raw["trigger"])
	return &streams.ContextCompactionObservation{
		Kind:            streams.ContextCompactionObservationKindCompletion,
		Provider:        copilotAgentID,
		NativeSessionID: event.SessionID,
		AttemptID:       attemptID,
		Success:         true,
		Outcome:         outcome,
		Trigger:         trigger,
	}, false
}

// copilotKnownNonAppliedOutcome distinguishes an explicit non-application
// from an unrecognized future outcome. Only the former is complete evidence.
func copilotKnownNonAppliedOutcome(outcome string) bool {
	switch outcome {
	case "failed", toolStatusCancelled, "canceled", "skipped", "not_applied", "no_op", "empty":
		return true
	default:
		return false
	}
}

func copilotRawString(raw json.RawMessage) string {
	value, ok := copilotRawStringValue(raw)
	if !ok {
		return ""
	}
	return value
}

func copilotRawStringValue(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || isJSONNull(raw) {
		return "", false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return value, true
}

func copilotRawBool(raw json.RawMessage) (bool, bool) {
	if len(raw) == 0 || isJSONNull(raw) {
		return false, false
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, false
	}
	return value, true
}

func copilotOmittedCompletionReporting(event decodedCopilotNativeEvent) *streams.ContextCompactionObservation {
	if event.Type != copilotNativeTypeCompactionComplete || !event.Root {
		return nil
	}
	reason := copilotReportingReasonUnusable
	switch {
	case event.DataOmitted:
		reason = copilotReportingReasonOmitted
	case event.Malformed:
		reason = "completion_malformed"
	}
	return &streams.ContextCompactionObservation{
		Kind:            streams.ContextCompactionObservationKindReporting,
		Provider:        copilotAgentID,
		NativeSessionID: event.SessionID,
		Coverage:        streams.ContextCompactionCoveragePartial,
		Reason:          reason,
	}
}

type copilotNativeUsageData struct {
	CurrentTokens *int64 `json:"currentTokens"`
	TokenLimit    *int64 `json:"tokenLimit"`
}

// copilotRootUsage returns bounded root context usage. Child and malformed
// attribution yield no gauge update.
func copilotRootUsage(event decodedCopilotNativeEvent) (used, size int64, ok bool) {
	if event.Type != copilotNativeTypeUsage || event.DataOmitted || event.Malformed || !event.Root {
		return 0, 0, false
	}
	var data copilotNativeUsageData
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return 0, 0, false
	}
	if data.CurrentTokens == nil || data.TokenLimit == nil {
		return 0, 0, false
	}
	if *data.CurrentTokens < 0 || *data.TokenLimit <= 0 {
		return 0, 0, false
	}
	return *data.CurrentTokens, *data.TokenLimit, true
}

// copilotRuntimeQualified reports whether the effective Copilot CLI version has
// the captured native compaction contract. 1.0.88 lacks attempt identity and
// outcome, so it cannot be treated as available merely because session RPCs
// succeeded. Unknown or unparsable versions fail closed.
func copilotRuntimeQualified(version string) bool {
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(version, "GitHub Copilot CLI ")
	version = strings.TrimPrefix(version, "v")
	core := version
	if index := strings.IndexAny(core, "-+"); index >= 0 {
		core = core[:index]
	}
	parts := strings.Split(core, ".")
	if len(parts) < 3 {
		return false
	}
	numbers := make([]int, 3)
	for i := 0; i < 3; i++ {
		parsed, err := strconv.Atoi(strings.TrimSuffix(parts[i], "."))
		if err != nil || parsed < 0 {
			return false
		}
		numbers[i] = parsed
	}
	if numbers[0] != 1 {
		return numbers[0] > 1
	}
	if numbers[1] != 0 {
		return numbers[1] > 0
	}
	return numbers[2] >= 103
}
