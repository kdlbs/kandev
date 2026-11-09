package types

import (
	"encoding/json"
	"testing"
)

func TestManagedStartupEvidenceExitCodeNullMissingAndZero(t *testing.T) {
	for _, tt := range []struct {
		name string
		raw  string
		want *int
	}{
		{name: "missing", raw: `{"process_generation":1,"exit_disposition":"ordinary_exit"}`},
		{name: "explicit null", raw: `{"process_generation":1,"exit_disposition":"ordinary_exit","exit_code":null}`},
		{name: "zero", raw: `{"process_generation":1,"exit_disposition":"ordinary_exit","exit_code":0}`, want: intPointer(0)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var evidence ManagedStartupEvidence
			if err := json.Unmarshal([]byte(tt.raw), &evidence); err != nil {
				t.Fatalf("decode evidence: %v", err)
			}
			if !intPointersEqual(evidence.ExitCode, tt.want) {
				t.Fatalf("exit code = %#v, want %#v", evidence.ExitCode, tt.want)
			}
		})
	}
}

func TestManagedStartupEvidenceMalformedNPMFieldsInvalidateCompleteness(t *testing.T) {
	for _, tt := range []struct {
		name string
		raw  string
	}{
		{
			name: "present flag is a string",
			raw:  `{"process_generation":1,"exit_disposition":"ordinary_exit","exit_code":1,"collection_complete":true,"npm_diagnostic_complete":true,"npm_diagnostic_present":"yes"}`,
		},
		{
			name: "complete flag is a string",
			raw:  `{"process_generation":1,"exit_disposition":"ordinary_exit","exit_code":1,"collection_complete":true,"npm_diagnostic_complete":"true"}`,
		},
		{
			name: "unclassified flag is null",
			raw:  `{"process_generation":1,"exit_disposition":"ordinary_exit","exit_code":1,"collection_complete":true,"npm_diagnostic_complete":true,"unclassified_npm_code":null}`,
		},
		{
			name: "unclassified flag is a string",
			raw:  `{"process_generation":1,"exit_disposition":"ordinary_exit","exit_code":1,"collection_complete":true,"npm_diagnostic_complete":true,"unclassified_npm_code":"no"}`,
		},
		{
			name: "npm code is a number",
			raw:  `{"process_generation":1,"exit_disposition":"ordinary_exit","exit_code":1,"collection_complete":true,"npm_diagnostic_complete":true,"npm_code":17}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var evidence ManagedStartupEvidence
			if err := json.Unmarshal([]byte(tt.raw), &evidence); err != nil {
				t.Fatalf("decode evidence: %v", err)
			}
			if evidence.NPMDiagnosticComplete {
				t.Fatalf("evidence = %#v, malformed npm fields must invalidate completeness", evidence)
			}
			if evidence.ExitCode == nil || *evidence.ExitCode != 1 || evidence.ExitDisposition != ManagedStartupExitOrdinary {
				t.Fatalf("evidence = %#v, bounded exit evidence must survive", evidence)
			}
		})
	}
}

func TestManagedStartupEvidenceValidEmptyNPMDiagnosticStaysComplete(t *testing.T) {
	var evidence ManagedStartupEvidence
	if err := json.Unmarshal([]byte(`{"process_generation":1,"exit_disposition":"ordinary_exit","exit_code":1,"collection_complete":true,"npm_diagnostic_complete":true}`), &evidence); err != nil {
		t.Fatalf("decode evidence: %v", err)
	}
	if !evidence.NPMDiagnosticComplete || evidence.NPMDiagnosticPresent || evidence.UnclassifiedNPMCode {
		t.Fatalf("evidence = %#v, want valid empty npm diagnostics", evidence)
	}
}

func TestManagedStartupEvidencePreservesCacheEvidence(t *testing.T) {
	var evidence ManagedStartupEvidence
	if err := json.Unmarshal([]byte(`{"process_generation":1,"exit_disposition":"ordinary_exit","exit_code":1,"cache_evidence":{"condition":"missing_file","package_spec":"pkg@1.2.3","process_generation":1,"probe_attempt_id":"attempt-1"}}`), &evidence); err != nil {
		t.Fatalf("decode evidence: %v", err)
	}
	if evidence.CacheEvidence == nil ||
		evidence.CacheEvidence.Condition != "missing_file" ||
		evidence.CacheEvidence.PackageSpec != "pkg@1.2.3" ||
		evidence.CacheEvidence.ProcessGeneration != 1 ||
		evidence.CacheEvidence.ProbeAttemptID != "attempt-1" {
		t.Fatalf("cache evidence = %#v", evidence.CacheEvidence)
	}
}

func intPointer(value int) *int { return &value }

func intPointersEqual(left, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
