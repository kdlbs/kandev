package lifecycle

import (
	"encoding/json"
	"testing"
)

func TestCapturedResourceCircuitIdentityFencesExecutionAndProfile(t *testing.T) {
	metadata := map[string]interface{}{}
	captureResourceCircuitIdentity(metadata, "execution", &AgentProfileInfo{ProfileID: "profile", ResourceCircuitKey: "credential:captured"})
	serialized, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	var restored map[string]interface{}
	if err := json.Unmarshal(serialized, &restored); err != nil {
		t.Fatal(err)
	}
	for _, source := range []map[string]interface{}{metadata, restored} {
		for _, tc := range []struct{ execution, profile, want string }{
			{"execution", "profile", "credential:captured"}, {"replacement", "profile", ""}, {"execution", "replacement", ""}, {"", "profile", ""}, {"execution", "", ""},
		} {
			if got := CapturedResourceCircuitKey(source, tc.execution, tc.profile); got != tc.want {
				t.Fatalf("execution=%q profile=%q: %q", tc.execution, tc.profile, got)
			}
		}
	}
	if got := CapturedResourceCircuitKey(nil, "execution", "profile"); got != "" {
		t.Fatalf("legacy execution acquired key %q", got)
	}
}
