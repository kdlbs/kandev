package usage

import (
	"math"
	"testing"
)

// TestValidateStage_AllChecksPass_ReturnsEmptyReason pins the happy path:
// a fully-formed payload passes validate with no drop reason.
func TestValidateStage_AllChecksPass_ReturnsEmptyReason(t *testing.T) {
	p := &usageEventPayload{
		UsageEventID: "evt-1",
		TaskID:       "task-1",
		Usage:        &promptUsagePayload{InputTokens: 100},
	}
	if reason := validateStage(p); reason != "" {
		t.Errorf("validateStage = %q, want empty (valid)", reason)
	}
}

// TestValidateStage_MissingUsageEventID_ReturnsInvalid pins sub-check (1):
// checked first, regardless of what else is wrong with the payload.
func TestValidateStage_MissingUsageEventID_ReturnsInvalid(t *testing.T) {
	p := &usageEventPayload{
		UsageEventID: "",
		TaskID:       "",
		Usage:        &promptUsagePayload{InputTokens: -1},
	}
	if reason := validateStage(p); reason != dropReasonInvalid {
		t.Errorf("validateStage = %q, want %q (usage_event_id missing wins over every other failure)", reason, dropReasonInvalid)
	}
}

// TestValidateStage_MissingTaskID_ReturnsUnattributable pins sub-check (2).
func TestValidateStage_MissingTaskID_ReturnsUnattributable(t *testing.T) {
	p := &usageEventPayload{
		UsageEventID: "evt-1",
		TaskID:       "",
		Usage:        &promptUsagePayload{InputTokens: 10},
	}
	if reason := validateStage(p); reason != dropReasonUnattributable {
		t.Errorf("validateStage = %q, want %q", reason, dropReasonUnattributable)
	}
}

// TestValidateStage_MissingTaskIDAndNegativeValue_ReturnsUnattributable pins
// the specific (2)-vs-(3) ordering AC-27 calls out by name: a payload
// missing task_id AND carrying a negative token value counts
// unattributable, never reaching the negative-value check.
func TestValidateStage_MissingTaskIDAndNegativeValue_ReturnsUnattributable(t *testing.T) {
	p := &usageEventPayload{
		UsageEventID: "evt-1",
		TaskID:       "",
		Usage:        &promptUsagePayload{InputTokens: -5},
	}
	if reason := validateStage(p); reason != dropReasonUnattributable {
		t.Errorf("validateStage = %q, want %q (task_id-missing must win over negative-value)", reason, dropReasonUnattributable)
	}
}

// TestValidateStage_NegativeValue_ReturnsInvalid pins sub-check (3) across
// every field it covers, including the provider-reported cost.
func TestValidateStage_NegativeValue_ReturnsInvalid(t *testing.T) {
	tests := map[string]*promptUsagePayload{
		"negative input tokens":           {InputTokens: -1},
		"negative output tokens":          {OutputTokens: -1},
		"negative cached read tokens":     {CachedReadTokens: -1},
		"negative cached write tokens":    {CachedWriteTokens: -1},
		"negative thought tokens":         {ThoughtTokens: -1},
		"negative provider-reported cost": {ProviderReportedCostPresent: true, ProviderReportedCostSubcents: -1},
	}
	for name, usage := range tests {
		t.Run(name, func(t *testing.T) {
			p := &usageEventPayload{UsageEventID: "evt-1", TaskID: "task-1", Usage: usage}
			if reason := validateStage(p); reason != dropReasonInvalid {
				t.Errorf("validateStage = %q, want %q", reason, dropReasonInvalid)
			}
		})
	}
}

// TestValidateStage_NegativeProviderCostButNotPresent_PassesValidation pins
// that an unset provider-reported cost is never inspected for sign, since
// ProviderReportedCostPresent=false means the field simply wasn't sent.
func TestValidateStage_NegativeProviderCostButNotPresent_PassesValidation(t *testing.T) {
	p := &usageEventPayload{
		UsageEventID: "evt-1",
		TaskID:       "task-1",
		Usage:        &promptUsagePayload{ProviderReportedCostPresent: false, ProviderReportedCostSubcents: -1},
	}
	if reason := validateStage(p); reason != "" {
		t.Errorf("validateStage = %q, want empty (unset provider cost is not sign-checked)", reason)
	}
}

// TestValidateStage_NilUsage_PassesValidation pins the defensive-only nil
// guard: publishPromptUsage never publishes a nil Usage (AC-24), but the
// decode stage doesn't reject one either, so validate must not panic.
func TestValidateStage_NilUsage_PassesValidation(t *testing.T) {
	p := &usageEventPayload{UsageEventID: "evt-1", TaskID: "task-1", Usage: nil}
	if reason := validateStage(p); reason != "" {
		t.Errorf("validateStage = %q, want empty", reason)
	}
}

func TestValidateStage_TokenTotalOverflow_ReturnsOverflow(t *testing.T) {
	p := &usageEventPayload{
		UsageEventID: "evt-1",
		TaskID:       "task-1",
		Usage:        &promptUsagePayload{InputTokens: math.MaxInt64, CachedReadTokens: 1},
	}
	if reason := validateStage(p); reason != dropReasonOverflow {
		t.Errorf("validateStage = %q, want %q for an overflowing token total", reason, dropReasonOverflow)
	}
}
