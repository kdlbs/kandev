//go:build linux

package processidentity

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBirthBootMatches(t *testing.T) {
	identity, err := Capture(os.Getpid())
	require.NoError(t, err)

	matches, err := BirthBootMatches(identity)
	require.NoError(t, err)
	require.True(t, matches, "a live self-capture must match the current boot")

	other := identity
	other.BirthToken = "linux:not-the-current-boot:1"
	matches, err = BirthBootMatches(other)
	require.NoError(t, err)
	require.False(t, matches)

	unsupported := identity
	unsupported.BirthToken = "bad"
	_, err = BirthBootMatches(unsupported)
	require.ErrorIs(t, err, ErrUnverifiableIdentity)
}
