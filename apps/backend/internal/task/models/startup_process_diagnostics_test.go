package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNormalizeAgentErrorCauseProcessExited(t *testing.T) {
	exitCode := 1
	negative := -1
	overflow := int(^uint32(0)) + 1
	cause := AgentErrorCause{
		Operation: AgentErrorCauseOperationResume,
		Code:      AgentErrorCauseCodeProcessExited,
		Reason:    AgentErrorCauseReasonMiseConfigUntrusted,
		Detail:    "raw /private/workspace/mise.toml",
		ExitCode:  &exitCode,
	}
	normalized := NormalizeAgentErrorCauses([]AgentErrorCause{cause})
	if len(normalized) != 1 {
		t.Fatalf("normalized causes = %#v", normalized)
	}
	if normalized[0].Detail != AgentErrorCauseProcessExitedDetail ||
		normalized[0].Reason != AgentErrorCauseReasonMiseConfigUntrusted ||
		normalized[0].ExitCode == nil || *normalized[0].ExitCode != 1 {
		t.Fatalf("normalized cause = %#v", normalized[0])
	}

	unknownReason := cause
	unknownReason.Reason = "private_reason"
	unknownReason.ExitCode = &exitCode
	normalized = NormalizeAgentErrorCauses([]AgentErrorCause{unknownReason})
	if len(normalized) != 1 || normalized[0].Reason != AgentErrorCauseReasonUnclassified {
		t.Fatalf("unknown reason cause = %#v, want unclassified", normalized)
	}

	negativeCause := cause
	negativeCause.ExitCode = &negative
	normalized = NormalizeAgentErrorCauses([]AgentErrorCause{negativeCause})
	if len(normalized) != 1 || normalized[0].ExitCode != nil {
		t.Fatalf("negative exit cause = %#v, want status omitted", normalized)
	}

	overflowCause := cause
	overflowCause.ExitCode = &overflow
	normalized = NormalizeAgentErrorCauses([]AgentErrorCause{overflowCause})
	if len(normalized) != 1 || normalized[0].ExitCode != nil {
		t.Fatalf("overflow exit cause = %#v, want status omitted", normalized)
	}

	unrelated := AgentErrorCause{
		Operation: AgentErrorCauseOperationStart,
		Code:      AgentErrorCauseCodeUnknown,
		ExitCode:  &exitCode,
	}
	normalized = NormalizeAgentErrorCauses([]AgentErrorCause{unrelated})
	if len(normalized) != 1 || normalized[0].ExitCode != nil {
		t.Fatalf("unrelated cause = %#v, want status cleared", normalized)
	}
}

func TestAgentErrorCausesEqualDistinguishesExitStatus(t *testing.T) {
	zero, one := 0, 1
	base := AgentErrorCause{
		Operation: AgentErrorCauseOperationResume,
		Code:      AgentErrorCauseCodeProcessExited,
		Reason:    AgentErrorCauseReasonMiseConfigUntrusted,
		Detail:    AgentErrorCauseProcessExitedDetail,
		ExitCode:  &zero,
	}
	successor := base
	successor.ExitCode = &one
	if AgentErrorCausesEqual([]AgentErrorCause{base}, []AgentErrorCause{successor}) {
		t.Fatal("exit status changes must affect semantic equality")
	}
	missing := base
	missing.ExitCode = nil
	if AgentErrorCausesEqual([]AgentErrorCause{base}, []AgentErrorCause{missing}) {
		t.Fatal("missing exit status must remain distinct from status zero")
	}
}

func TestNormalizeAgentErrorCauseClearsExitCodeFromSelectionCauses(t *testing.T) {
	exitCode := 1
	causes := NormalizeAgentErrorCauses([]AgentErrorCause{
		{
			Operation: AgentErrorCauseOperationStart, Code: AgentErrorCauseCodeModelUnavailable,
			Reason: AgentErrorCauseReasonRequestedNotAdvertised, RequestedModel: "model-a", ExitCode: &exitCode,
		},
		{
			Operation: AgentErrorCauseOperationResume, Code: AgentErrorCauseCodePermissionModeMismatch,
			Reason: AgentErrorCauseReasonEffectiveMismatch, RequestedMode: "default", EffectiveMode: "plan",
			ExitCode: &exitCode,
		},
	})
	if len(causes) != 2 {
		t.Fatalf("causes = %#v", causes)
	}
	for _, cause := range causes {
		if cause.ExitCode != nil {
			t.Fatalf("selection cause retained exit code: %#v", cause)
		}
	}
}

func TestLastAgentErrorNullExitCodeStaysMissing(t *testing.T) {
	for _, tt := range []struct {
		name     string
		exitCode any
		present  bool
		wantZero bool
	}{
		{name: "missing"},
		{name: "explicit null", exitCode: nil, present: true},
		{name: "zero", exitCode: 0, present: true, wantZero: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cause := map[string]interface{}{
				"operation": AgentErrorCauseOperationResume,
				"code":      AgentErrorCauseCodeProcessExited,
				"reason":    AgentErrorCauseReasonMiseConfigUntrusted,
			}
			if tt.present {
				cause["exit_code"] = tt.exitCode
			}
			loaded, ok := LoadLastAgentError(map[string]interface{}{
				SessionMetaKeyLastAgentError: map[string]interface{}{
					"message":     AgentErrorCauseProcessExitedDetail,
					"occurred_at": "2026-11-07T12:00:00Z",
					"phase":       LaunchErrorPhaseBootstrap,
					"causes":      []interface{}{cause},
				},
			})
			if !ok || len(loaded.Causes) != 1 {
				t.Fatalf("loaded error = %#v, ok=%v", loaded, ok)
			}
			got := loaded.Causes[0].ExitCode
			if tt.wantZero {
				if got == nil || *got != 0 {
					t.Fatalf("exit code = %#v, want status zero", got)
				}
				return
			}
			if got != nil {
				t.Fatalf("exit code = %#v, want missing status", got)
			}
		})
	}
}

func TestLastAgentErrorRoundTripPreservesProcessExitStatus(t *testing.T) {
	exitCode := 0
	value := LastAgentError{
		Message:    AgentErrorCauseProcessExitedDetail,
		OccurredAt: time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC),
		Phase:      LaunchErrorPhaseBootstrap,
		AttemptID:  "resume-1",
		Causes: []AgentErrorCause{{
			Operation: AgentErrorCauseOperationResume,
			Code:      AgentErrorCauseCodeProcessExited,
			Reason:    AgentErrorCauseReasonMiseConfigUntrusted,
			ExitCode:  &exitCode,
		}},
	}
	metadata := map[string]interface{}{SessionMetaKeyLastAgentError: value}
	loaded, ok := LoadLastAgentError(metadata)
	if !ok {
		t.Fatal("LastAgentError was not loaded")
	}
	if len(loaded.Causes) != 1 || loaded.Causes[0].ExitCode == nil || *loaded.Causes[0].ExitCode != 0 {
		t.Fatalf("loaded causes = %#v, want status zero", loaded.Causes)
	}
	encoded, err := json.Marshal(loaded)
	if err != nil || !json.Valid(encoded) {
		t.Fatalf("marshal loaded error: %v", err)
	}
}
