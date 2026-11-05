package plugins

import (
	"context"
	"testing"
	"time"

	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// @covers AC-AGENTS-SANDBOX-010.3
func TestSandboxReportRPCSeparatesExplicitVerificationBudget(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		verify                    bool
		parent, request, expected time.Duration
	}{
		{name: "ordinary report", expected: 5 * time.Second},
		{name: "explicit verification", verify: true, expected: 30 * time.Second},
		{name: "parent deadline", verify: true, parent: 3 * time.Second, expected: 3 * time.Second},
		{name: "wire deadline", verify: true, request: 2 * time.Second, expected: 2 * time.Second},
		{name: "earliest deadline", verify: true, parent: time.Second, request: 3 * time.Second, expected: time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, provider, binding, compile := sandboxWireService(t)
			record, err := service.Get(binding.PluginID)
			require.NoError(t, err)
			record.SandboxProviders[0].ProtocolMajor = 2
			require.NoError(t, service.store.Save(record))
			service.registry.Add(record)
			binding.ProtocolMajor, compile.Context.ProtocolMajor = 2, 2
			observed := make(chan time.Time, 1)
			provider.report = func(ctx context.Context, _ *pluginsdk.ReadSandboxReportRequest) (*pluginsdk.ReadSandboxReportResponse, error) {
				deadline, _ := ctx.Deadline()
				observed <- deadline
				return nil, status.Error(codes.Unimplemented, "fixture observes deadline only")
			}
			request := &pluginsdk.ReadSandboxReportRequest{Context: compile.Context, Policy: compile.Policy, VerifyAuditV2: tc.verify, ChunkV2: auditVerificationChunk()}
			started := time.Now()
			ctx := t.Context()
			if tc.parent > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tc.parent)
				defer cancel()
			}
			if tc.request > 0 {
				request.Context.DeadlineUnixMs = started.Add(tc.request).UnixMilli()
			}
			_, err = service.ReadSandboxReport(ctx, binding, request)
			require.ErrorIs(t, err, ErrSandboxReportUnsupported, "the real gRPC dispatch must reach the fixture")
			require.WithinDuration(t, started.Add(tc.expected), <-observed, time.Second)
		})
	}
}
