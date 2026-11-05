package plugins

import (
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestUnsupportedAuditSchemaKeepsAClosedErrorClassification(t *testing.T) {
	service, _, binding, request := sandboxWireService(t)
	_, err := service.ReadSandboxReport(t.Context(), binding, &pluginsdk.ReadSandboxReportRequest{Context: request.Context, Policy: request.Policy, Audit: []byte("fixture")})
	require.ErrorIs(t, err, ErrSandboxReportUnsupported)
	require.Equal(t, ErrSandboxReportUnsupported.Error(), err.Error(), "private provider error text never crosses the boundary")
	_, err = service.DescribeSandboxProvider(t.Context(), binding, &pluginsdk.DescribeSandboxProviderRequest{Context: request.Context, Runtime: request.Runtime})
	require.ErrorIs(t, err, ErrSandboxProviderUnavailable, "a missing non-report RPC keeps its existing classification")
	require.NotErrorIs(t, err, ErrSandboxReportUnsupported)
}
