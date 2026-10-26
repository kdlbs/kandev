package sandboxruntime

import (
	"errors"
	"testing"
	"time"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/stretchr/testify/require"
)

func stubGroupMembers(t *testing.T, read func(int) ([]int, error)) {
	t.Helper()
	previous := groupMembers
	groupMembers = read
	t.Cleanup(func() { groupMembers = previous })
}

func TestAwaitGroupEndedReturnsOnceMembersExit(t *testing.T) {
	calls := 0
	stubGroupMembers(t, func(pgid int) ([]int, error) {
		require.Equal(t, 4242, pgid)
		calls++
		if calls < 3 {
			return []int{4243}, nil
		}
		return nil, nil
	})
	require.NoError(t, awaitGroupEnded(t.Context(), 4242))
	require.Equal(t, 3, calls)
}

func TestAwaitGroupEndedFailsClosedOnSurvivorAndReadError(t *testing.T) {
	stubGroupMembers(t, func(int) ([]int, error) { return nil, errors.New("sysctl unavailable") })
	require.ErrorIs(t, awaitGroupEnded(t.Context(), 4242), policy.ErrConflict)
	shortenGroupBound(t)
	stubGroupMembers(t, func(int) ([]int, error) { return []int{4243}, nil })
	require.ErrorIs(t, awaitGroupEnded(t.Context(), 4242), policy.ErrConflict)
}

func shortenGroupBound(t *testing.T) {
	t.Helper()
	previous := groupScanBound
	groupScanBound = 250 * time.Millisecond
	t.Cleanup(func() { groupScanBound = previous })
}
