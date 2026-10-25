package retention

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestMutationLeaseBlocksConcurrentCleanupAndAllowsNestedCalls(t *testing.T) {
	ctx, release, err := AcquireMutation(context.Background())
	require.NoError(t, err)
	defer release()
	_, nested, err := AcquireMutation(ctx)
	require.NoError(t, err)
	nested()
	blocked, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, _, err = AcquireMutation(blocked)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	release()
	_, next, err := AcquireMutation(context.Background())
	require.NoError(t, err)
	next()
}

func TestNestedMutationKeepsLeaseAfterParentReturns(t *testing.T) {
	ctx, parent, err := AcquireMutation(context.Background())
	require.NoError(t, err)
	_, child, err := AcquireMutation(ctx)
	require.NoError(t, err)
	defer child()
	parent()
	blocked, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, release, err := AcquireMutation(blocked)
	if release != nil {
		release()
	}
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
