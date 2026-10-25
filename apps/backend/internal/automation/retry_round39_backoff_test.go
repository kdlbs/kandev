package automation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRetryDelayExponentialBoundary(t *testing.T) {
	boundaryPolicy := RetryPolicy{Mode: RetryModeFinite, MaxRetries: "10",
		DelaySeconds: "1", Backoff: RetryBackoffExponential}
	for _, tc := range []struct {
		retryNumber int64
		want        time.Duration
	}{{retryNumber: 5, want: 16 * time.Second}, {retryNumber: 6, want: 32 * time.Second}} {
		boundaryDelay, boundaryErr := RetryDelay(boundaryPolicy, tc.retryNumber)
		require.NoError(t, boundaryErr)
		require.Equal(t, tc.want, boundaryDelay)
	}
}
