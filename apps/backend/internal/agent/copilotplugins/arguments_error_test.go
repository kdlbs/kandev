package copilotplugins

import (
	"strings"
	"testing"
)

func TestPolicyUnverifiedRecovery(t *testing.T) {
	policyMessage := (&Error{Reason: ReasonPolicyUnverified}).Error()
	policyLower := strings.ToLower(policyMessage)
	if !strings.Contains(policyLower, "uncheck") || !strings.Contains(policyLower, "save") {
		t.Fatalf("policy recovery = %q, want uncheck and save guidance", policyMessage)
	}
	if strings.Contains(policyMessage, "supported Copilot runtime") {
		t.Fatalf("policy recovery suggests a runtime switch: %q", policyMessage)
	}

	runtimeMessage := (&Error{Reason: ReasonRuntimeUnsupported}).Error()
	if !strings.Contains(runtimeMessage, "supported Copilot runtime") {
		t.Fatalf("runtime recovery = %q, want runtime guidance", runtimeMessage)
	}
}
