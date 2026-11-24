package messagequeue

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// @covers AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.6
func TestSessionAdmissionTimeoutLogsHolder(t *testing.T) {
	previousTimeout := admissionWaitTimeout
	admissionWaitTimeout = 20 * time.Millisecond
	t.Cleanup(func() { admissionWaitTimeout = previousTimeout })

	core, logs := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	service := NewServiceMemory(log)

	const sessionID = "session-held-admission"
	holderAcquired := make(chan struct{})
	releaseHolder := make(chan struct{})
	holderDone := make(chan error, 1)
	go func() {
		holderDone <- service.WithSessionAdmission(context.Background(), sessionID, func(context.Context) error {
			close(holderAcquired)
			<-releaseHolder
			return nil
		})
	}()
	select {
	case <-holderAcquired:
	case <-time.After(time.Second):
		close(releaseHolder)
		require.NoError(t, <-holderDone)
		t.Fatal("admission holder did not start")
	}

	err = service.WithSessionAdmission(context.Background(), sessionID, func(context.Context) error { return nil })
	require.ErrorIs(t, err, ErrSessionAdmissionTimeout)

	entries := logs.FilterLevelExact(zapcore.WarnLevel).FilterMessage("queue session admission timed out").All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, sessionID, fields["session_id"])
	heldFor, ok := fields["held_for"].(time.Duration)
	require.True(t, ok, "held_for field is not a duration: %#v", fields["held_for"])
	require.Greater(t, heldFor, time.Duration(0))

	close(releaseHolder)
	require.NoError(t, <-holderDone)
}
