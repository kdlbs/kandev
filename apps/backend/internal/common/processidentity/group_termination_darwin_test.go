//go:build darwin

package processidentity

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOriginalOwnerPreservesDarwinRuntimeOnlyProof(t *testing.T) {
	runtime, err := Capture(os.Getpid())
	require.NoError(t, err)
	owner := runtime.WithAgentGroup(Identity{PID: 42, GroupID: 42, BirthToken: "darwin:1:0:0"})
	terminated, err := OriginalOwnerTerminated(owner)
	require.NoError(t, err, "unsupported group proof preserves the known runtime result")
	require.False(t, terminated)

	owner.BirthToken = ""
	terminated, err = OriginalOwnerTerminated(owner)
	require.ErrorIs(t, err, ErrUnverifiableIdentity)
	require.False(t, terminated, "unsupported group proof never hides an invalid runtime")
}
