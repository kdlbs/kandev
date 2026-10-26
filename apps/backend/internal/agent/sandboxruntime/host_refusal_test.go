package sandboxruntime

import (
	"errors"
	"testing"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/stretchr/testify/require"
)

func TestHostRefusalMapsOnlyOnADarwinHost(t *testing.T) {
	previous := hostGOOS
	t.Cleanup(func() { hostGOOS = previous })
	hostGOOS = "darwin"
	require.ErrorIs(t, HostRefusal(plugins.ErrSandboxHostUnsupported), policy.ErrHostUnsupported)
	plain := errors.New("other")
	require.Equal(t, plain, HostRefusal(plain))
	hostGOOS = "linux"
	require.ErrorIs(t, HostRefusal(plugins.ErrSandboxHostUnsupported), plugins.ErrSandboxProviderUnavailable)
	require.NotErrorIs(t, HostRefusal(plugins.ErrSandboxHostUnsupported), policy.ErrHostUnsupported)
}
