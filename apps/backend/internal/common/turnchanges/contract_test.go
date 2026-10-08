package turnchanges

import (
	"encoding/json"
	"testing"
)

func TestAvailabilityContract(t *testing.T) {
	states := []Availability{Pending, Ready, Unavailable, Failed, Expired}
	for _, state := range states {
		if !state.Valid() {
			t.Errorf("%q is not a valid availability state", state)
		}
		encoded, err := json.Marshal(state)
		if err != nil {
			t.Fatalf("marshal %q: %v", state, err)
		}
		var decoded Availability
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("unmarshal %q: %v", state, err)
		}
		if decoded != state {
			t.Errorf("availability round trip = %q, want %q", decoded, state)
		}
	}
	if Availability("unknown").Valid() {
		t.Fatal("unknown availability state must be rejected")
	}
}

func TestReasonCodeContract(t *testing.T) {
	codes := []ReasonCode{
		"", ReasonCaptureDisabled, ReasonPolicyReadFailed, ReasonUnsupportedExecutor, ReasonNoGitRepository,
		ReasonCheckoutUnavailable, ReasonUnsafeGitState, ReasonCaptureFailed,
		ReasonComparisonFailed, ReasonContentUnavailable, ReasonContentTruncated,
		ReasonSizeLimit, ReasonEntryLimit, ReasonInvalidPathEncoding, ReasonSparseCaptureUnsupported,
		ReasonExpiredAge, ReasonExpiredTaskLimit, ReasonExpiredInstallLimit,
	}
	for _, code := range codes {
		if !code.Valid() {
			t.Errorf("%q is not a valid reason code", code)
		}
	}
	if ReasonCode("unrecognized").Valid() {
		t.Fatal("unrecognized reason code must be rejected")
	}
}

func TestPolicyResolutionKindContract(t *testing.T) {
	kinds := []PolicyResolutionKind{
		PolicyAuthenticatedUser, PolicyDefaultUser, PolicySyntheticDefault, PolicyReadFailed,
	}
	for _, kind := range kinds {
		if !kind.Valid() {
			t.Errorf("%q is not a valid policy resolution kind", kind)
		}
	}
	if PolicyResolutionKind("unrecognized").Valid() {
		t.Fatal("unrecognized policy resolution kind must be rejected")
	}
}
