package plugins

import (
	"context"
	"testing"

	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestSandboxProviderHostRefusalIsTypedAndStillUnavailable(t *testing.T) {
	svc, provider, binding, req := sandboxWireService(t)
	provider.compile = func(context.Context, *pluginsdk.CompileSandboxPolicyRequest) (*pluginsdk.CompileSandboxPolicyResponse, error) {
		return nil, pluginsdk.ErrSandboxHostUnsupported()
	}
	_, err := svc.CompileSandboxPolicy(t.Context(), binding, req)
	require.ErrorIs(t, err, ErrSandboxHostUnsupported)
	require.ErrorIs(t, err, ErrSandboxProviderUnavailable, "existing callers keep matching the unavailable class")
}

func TestSandboxProviderOtherPreconditionFailuresStayOpaque(t *testing.T) {
	svc, provider, binding, req := sandboxWireService(t)
	for _, failure := range []error{
		status.Error(codes.FailedPrecondition, "host_unsupported: private detail"),
		status.Error(codes.Unavailable, pluginsdk.SandboxHostUnsupportedMessage),
		status.Error(codes.Internal, "private diagnostic"),
	} {
		provider.compile = func(context.Context, *pluginsdk.CompileSandboxPolicyRequest) (*pluginsdk.CompileSandboxPolicyResponse, error) {
			return nil, failure
		}
		_, err := svc.CompileSandboxPolicy(t.Context(), binding, req)
		require.ErrorIs(t, err, ErrSandboxProviderUnavailable)
		require.NotErrorIs(t, err, ErrSandboxHostUnsupported)
		require.NotContains(t, err.Error(), "private")
	}
}
