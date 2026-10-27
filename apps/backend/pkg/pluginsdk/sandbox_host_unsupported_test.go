package pluginsdk

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestSandboxHostUnsupportedConstructorAndMatcher(t *testing.T) {
	err := ErrSandboxHostUnsupported()
	require.True(t, IsSandboxHostUnsupported(err))
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
	require.Equal(t, "host_unsupported", status.Convert(err).Message())
	require.False(t, IsSandboxHostUnsupported(nil))
	require.False(t, IsSandboxHostUnsupported(errors.New("host_unsupported")), "a plain error is not a status")
	require.False(t, IsSandboxHostUnsupported(status.Error(codes.FailedPrecondition, "host_unsupported ")))
	require.False(t, IsSandboxHostUnsupported(status.Error(codes.Unavailable, "host_unsupported")))
}
