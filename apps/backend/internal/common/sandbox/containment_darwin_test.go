//go:build darwin

package sandbox

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigureContainmentIsNoOpOnDarwin(t *testing.T) {
	release, err := ConfigureContainment(exec.Command("/usr/bin/true"), &Launch{Version: 1})
	require.NoError(t, err)
	require.NotNil(t, release)
	release()
}
