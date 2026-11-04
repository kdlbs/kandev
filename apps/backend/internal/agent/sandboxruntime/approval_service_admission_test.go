package sandboxruntime

import (
	"testing"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestServiceApprovalsRequireImplementedChannelOwnership(t *testing.T) {
	for _, capability := range []bool{false, true} {
		compiled := &pluginsdk.SandboxCompiledPolicy{ApprovalServicesV2: []*pluginsdk.SandboxApprovalServiceV2{{ServiceKey: "network-proxy", RouteId: "connect", OperationKind: "network", EndpointReference: "sandbox.approval.service.network-proxy.connect.endpoint"}}}
		if capability {
			compiled.RequiredCapabilities = []string{pluginsdk.SandboxServiceApprovalCapabilityV2}
		}
		require.ErrorIs(t, validateCompiledOverrides(PrepareRequest{}, compiled), policy.ErrIneligible,
			"service declarations cannot activate a callback before owned channel dispatch is implemented")
	}
	require.NoError(t, validateCompiledOverrides(PrepareRequest{}, &pluginsdk.SandboxCompiledPolicy{}), "legacy launch admission remains available")
}
