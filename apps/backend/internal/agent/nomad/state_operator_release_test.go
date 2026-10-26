package nomad

import (
	"strings"
	"testing"
)

func releasedTestState() EnvironmentState {
	state := validTestState()
	state.Phase = PhaseOperatorReleased
	state.OperatorAttestationDigest = strings.Repeat("c", 64)
	return state
}

func TestNomadWorkerQuarantineOperatorReleasedRequiresAttestationDigest(t *testing.T) {
	if err := releasedTestState().Validate(); err != nil {
		t.Fatalf("valid operator_released state rejected: %v", err)
	}
	for name, mutate := range map[string]func(*EnvironmentState){
		"missing digest":   func(s *EnvironmentState) { s.OperatorAttestationDigest = "" },
		"malformed digest": func(s *EnvironmentState) { s.OperatorAttestationDigest = "XYZ" },
		"digest on other phase": func(s *EnvironmentState) {
			s.Phase = "cleanup_pending"
		},
	} {
		t.Run(name, func(t *testing.T) {
			state := releasedTestState()
			mutate(&state)
			if err := state.Validate(); err == nil {
				t.Fatal("Validate() accepted an inconsistent operator attestation")
			}
		})
	}
}

func TestNomadWorkerQuarantineOperatorReleasedKeepsOtherValidation(t *testing.T) {
	state := releasedTestState()
	state.AllocationID = "alloc-1"
	if err := state.Validate(); err == nil {
		t.Fatal("operator_released relaxed allocation identity validation")
	}
}

func TestNomadWorkerQuarantineAttestationDigestBindsEveryField(t *testing.T) {
	state := validTestState()
	base := OperatorAttestation{ProcessTreeAbsent: true, JobStoppedPurgeFalse: true, FenceRemoved: true, Operator: "op"}
	want := OperatorAttestationDigest(state, base)
	if !digestPattern.MatchString(want) {
		t.Fatalf("digest %q is not a sha256 hex string", want)
	}
	other := base
	other.Operator = "op2"
	changedJob := state
	changedJob.JobID = "other-job"
	changedNode := state
	changedNode.NodeID = "other-node"
	for name, got := range map[string]string{
		"operator": OperatorAttestationDigest(state, other),
		"job":      OperatorAttestationDigest(changedJob, base),
		"node":     OperatorAttestationDigest(changedNode, base),
	} {
		if got == want {
			t.Fatalf("digest ignores %s", name)
		}
	}
}
