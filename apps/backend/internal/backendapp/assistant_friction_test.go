package backendapp

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAssistantFrictionGateOrigins(t *testing.T) {
	for _, tc := range []struct{ code, origin, reason string }{
		{"permission_denied_by_user", "native", "denied_authority"},
		{"auth_required", "authentication", "authentication_required"},
		{"authentication_expired", "authentication", "expired_authentication"},
		{"missing_credentials", "configuration", "missing_capability"},
		{"network_unavailable", "provider", "transport_failure"},
		{"agent_transport_lost", "native", "transport_failure"},
		{"task_error", "unknown", "task_defect"},
		{"policy_boundary", "unknown", "policy_boundary"},
		{"suspected_classifier_error", "unknown", "suspected_classifier_error"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			source := errorAttentionSource("session", "CANARY_PRIVATE_PROMPT", tc.code)
			require.NotNil(t, source.Friction)
			require.Equal(t, tc.origin, source.Friction.Origin)
			require.Equal(t, tc.reason, source.Friction.Reason)
			require.Equal(t, tc.code, source.Friction.Cause)
			require.NotContains(t, source.Summary, "CANARY_PRIVATE_PROMPT")
		})
	}
	require.Nil(t, errorAttentionSource("session", "stamp", "CANARY_PRIVATE_PROMPT").Friction, "untrusted error text cannot invent classifier provenance")
}
