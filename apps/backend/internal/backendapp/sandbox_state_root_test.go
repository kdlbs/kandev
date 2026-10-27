package backendapp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRefuseDeniedStateRootOnDarwin(t *testing.T) {
	for _, base := range []string{"/tmp/x/sandbox-runtime", "/private/tmp/x", "/var/folders/ab/cd/T/sandbox-runtime", "/private/var/folders/ab", "/tmp"} {
		require.ErrorIs(t, refuseDeniedStateRoot("darwin", base), errSandboxRuntimeUnavailable, base)
	}
	for _, base := range []string{"/Users/op/.kandev/sandbox-runtime", "/private/tmpfiles/x", "/var/foldersx/y"} {
		require.NoError(t, refuseDeniedStateRoot("darwin", base), base)
	}
}

func TestRefuseDeniedStateRootIgnoresOtherHosts(t *testing.T) {
	require.NoError(t, refuseDeniedStateRoot("linux", "/tmp/x"))
}
