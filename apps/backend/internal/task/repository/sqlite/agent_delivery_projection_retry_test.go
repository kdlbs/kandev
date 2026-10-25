package sqlite

import (
	"context"
	"database/sql/driver"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestCanonicalDeliveryAttemptCancellation(t *testing.T) {
	for _, stage := range []string{"load canonical delivery cursor", "commit"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			attemptError := fmt.Errorf("%s: %w", stage, driver.ErrBadConn)
			calls := 0
			result, err := projectCanonicalDeliveryWithRetry(ctx, func() ([]bool, error) {
				calls++
				cancel()
				return nil, attemptError
			})
			require.Nil(t, result)
			require.ErrorIs(t, err, context.Canceled)
			require.ErrorIs(t, err, attemptError)
			require.ErrorIs(t, err, driver.ErrBadConn)
			require.Equal(t, 1, calls)
		})
	}
}

func TestCanonicalDeliveryAttemptSuccessfulCommitAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result, err := projectCanonicalDeliveryWithRetry(ctx, func() ([]bool, error) {
		cancel()
		return []bool{true, false}, nil
	})
	require.NoError(t, err)
	require.Equal(t, []bool{true, false}, result)
}

func TestCanonicalDeliveryAttemptActiveContextError(t *testing.T) {
	attemptError := fmt.Errorf("commit: %w", driver.ErrBadConn)
	calls := 0
	_, err := projectCanonicalDeliveryWithRetry(t.Context(), func() ([]bool, error) {
		calls++
		return nil, attemptError
	})
	require.Same(t, attemptError, err)
	require.Equal(t, 1, calls)
}

func TestCanonicalDeliveryAttemptContentionRetry(t *testing.T) {
	calls := 0
	result, err := projectCanonicalDeliveryWithRetry(t.Context(), func() ([]bool, error) {
		calls++
		if calls == 1 {
			return nil, &pgconn.PgError{Code: "55P03"}
		}
		return []bool{true}, nil
	})
	require.NoError(t, err)
	require.Equal(t, []bool{true}, result)
	require.Equal(t, 2, calls)
}
