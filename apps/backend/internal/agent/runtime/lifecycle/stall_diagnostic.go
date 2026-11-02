package lifecycle

import (
	"unicode"
	"unicode/utf8"
)

// stallDiagnostic contains only bounded identifiers, enums, and scalar evidence.
// Provider text and execution metadata are not diagnostic sources.
// OutputObserved and EffectObserved share conservative turn-content evidence.
type stallDiagnostic struct {
	TaskID             string `json:"task_id,omitempty"`
	SessionID          string `json:"session_id,omitempty"`
	ExecutionID        string `json:"execution_id,omitempty"`
	AgentType          string `json:"agent_type,omitempty"`
	PromptGeneration   uint64 `json:"prompt_generation"`
	HarnessGeneration  uint64 `json:"harness_generation,omitempty"`
	SubmissionID       string `json:"submission_id,omitempty"`
	StreamID           string `json:"stream_id,omitempty"`
	DeliveryMode       string `json:"delivery_mode,omitempty"`
	PromptAccepted     bool   `json:"prompt_accepted"`
	OutputObserved     bool   `json:"output_observed"`
	EffectObserved     bool   `json:"effect_observed"`
	LastProgressMillis int64  `json:"last_progress_unix_ms"`
	ElapsedMillis      int64  `json:"elapsed_ms"`
}

func newStallDiagnostic(execution *AgentExecution, generation uint64, snapshot promptStallSnapshot) stallDiagnostic {
	return stallDiagnostic{
		TaskID:             boundedStallIdentifier(execution.TaskID),
		SessionID:          boundedStallIdentifier(execution.SessionID),
		ExecutionID:        boundedStallIdentifier(execution.ID),
		AgentType:          boundedStallIdentifier(execution.AgentID),
		PromptGeneration:   generation,
		HarnessGeneration:  execution.DeliveryHarnessGeneration,
		SubmissionID:       boundedStallIdentifier(execution.deliverySubmissionIDSnapshot()),
		StreamID:           boundedStallIdentifier(execution.DeliveryStreamID),
		DeliveryMode:       stallDeliveryMode(execution.DeliveryMode),
		PromptAccepted:     snapshot.accepted,
		OutputObserved:     snapshot.outputObserved,
		EffectObserved:     snapshot.outputObserved,
		LastProgressMillis: snapshot.lastActivity.UnixMilli(),
	}
}

func stallDeliveryMode(mode DurableDeliveryMode) string {
	switch mode {
	case DurableDeliveryV1, DurableDeliveryLegacy, DurableDeliveryBlocked:
		return string(mode)
	default:
		return ""
	}
}

func boundedStallIdentifier(value string) string {
	if !utf8.ValidString(value) {
		return ""
	}
	for _, r := range value {
		if !unicode.IsLetter(r) && !unicode.IsNumber(r) &&
			r != '-' && r != '_' && r != '.' && r != ':' {
			return ""
		}
	}
	if len(value) <= 128 {
		return value
	}
	end := 128
	for !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end]
}
