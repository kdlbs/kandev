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

func TestValidateStage_NativeUsageObservation(t *testing.T) {
	reasoning := int64(80)
	cacheWrite := int64(4)
	valid := nativeUsageObservationPayload{
		SchemaVersion: 1, Source: "response", ProviderThreadID: "thread-1",
		ProviderTurnID: "provider-turn-1", ProviderResponseID: "response-1",
		Scope: "child", Completeness: "exact", ReasoningOutputTokens: &reasoning,
		ReportedCacheWriteTokens: &cacheWrite, PriceSuppressed: true,
	}
	tests := map[string]struct {
		observation nativeUsageObservationPayload
		turnID      string
		usage       *promptUsagePayload
	}{
		"accept valid response detail": {valid, "turn-1", &promptUsagePayload{OutputTokens: 200}},
		"reject unknown schema":        {func() nativeUsageObservationPayload { o := valid; o.SchemaVersion = 2; return o }(), "turn-1", &promptUsagePayload{OutputTokens: 200}},
		"reject missing native thread": {func() nativeUsageObservationPayload { o := valid; o.ProviderThreadID = ""; return o }(), "turn-1", &promptUsagePayload{OutputTokens: 200}},
		"reject reasoning beyond output": {func() nativeUsageObservationPayload {
			o := valid
			tooMany := int64(201)
			o.ReasoningOutputTokens = &tooMany
			return o
		}(), "turn-1", &promptUsagePayload{OutputTokens: 200}},
		"reject cache write priced as normal input": {func() nativeUsageObservationPayload { o := valid; o.PriceSuppressed = false; return o }(), "turn-1", &promptUsagePayload{OutputTokens: 200}},
		"reject missing Kandev turn":                {valid, "", &promptUsagePayload{OutputTokens: 200}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			p := &usageEventPayload{
				UsageEventID: "event-1", TaskID: "task-1", TurnID: tt.turnID, Usage: tt.usage,
				UsageObservation: &tt.observation,
			}
			got := validateStage(p)
			if name == "accept valid response detail" {
				if got != "" {
					t.Fatalf("validateStage = %q, want valid", got)
				}
			} else if got != dropReasonInvalid {
				t.Fatalf("validateStage = %q, want %q", got, dropReasonInvalid)
			}
		})
	}
}

func TestValidateStage_NativeFallbackRequiresEstimatedAndNoResponseID(t *testing.T) {
	observation := &nativeUsageObservationPayload{
		SchemaVersion: 1, Source: "turn_fallback", ProviderThreadID: "thread-1",
		ProviderTurnID: "provider-turn-1", Scope: "direct", Completeness: "estimated",
	}
	for name, usage := range map[string]*promptUsagePayload{
		"estimated fallback":     {InputTokens: 12, Estimated: true},
		"non-estimated fallback": {InputTokens: 12},
	} {
		t.Run(name, func(t *testing.T) {
			p := &usageEventPayload{UsageEventID: "event-1", TaskID: "task-1", TurnID: "turn-1", Usage: usage, UsageObservation: observation}
			want := ""
			if !usage.Estimated {
				want = dropReasonInvalid
			}
			if got := validateStage(p); got != want {
				t.Fatalf("validateStage = %q, want %q", got, want)
			}
		})
	}
	response := *observation
	response.ProviderResponseID = "response-1"
	if got := validateStage(&usageEventPayload{
		UsageEventID: "event-2", TaskID: "task-1", TurnID: "turn-1",
		Usage: &promptUsagePayload{InputTokens: 12, Estimated: true}, UsageObservation: &response,
	}); got != dropReasonInvalid {
		t.Fatalf("response-bearing fallback validateStage = %q, want %q", got, dropReasonInvalid)
	}
}
