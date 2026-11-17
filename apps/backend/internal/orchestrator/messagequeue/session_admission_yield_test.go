package messagequeue

import (
	"context"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestYieldedSessionAdmissionAllowsInputAndCannotReuseOwnership(t *testing.T) {
	service := NewServiceMemory(logger.Default())
	yielder, ok := any(service).(interface{ YieldSessionAdmission(context.Context) bool })
	require.True(t, ok, "provider control must release queue admission after its durable dispatch barrier")
	require.NoError(t, service.WithSessionAdmission(context.Background(), "session", func(admitted context.Context) error {
		require.True(t, yielder.YieldSessionAdmission(admitted))
		require.False(t, yielder.YieldSessionAdmission(admitted))
		require.NoError(t, service.WithSessionAdmission(context.Background(), "session", func(context.Context) error {
			cancelled, cancel := context.WithCancel(admitted)
			cancel()
			err := service.WithSessionAdmission(cancelled, "session", func(context.Context) error { t.Error("released context bypassed another owner's gate"); return nil })
			require.ErrorIs(t, err, context.Canceled)
			return nil
		}))
		return nil
	}))
}
