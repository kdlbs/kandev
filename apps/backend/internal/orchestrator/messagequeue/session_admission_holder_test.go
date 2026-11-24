package messagequeue

import (
	"context"
	"sync"
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
	holderAcquired, holderDone, releaseHolder := startSessionAdmissionHolder(t, service, sessionID)
	select {
	case <-holderAcquired:
	case <-time.After(time.Second):
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

	releaseHolder()
	require.NoError(t, <-holderDone)
}

// @covers AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.6
func TestSessionAdmissionCallerDeadlineLogsHolder(t *testing.T) {
	previousTimeout := admissionWaitTimeout
	admissionWaitTimeout = time.Second
	t.Cleanup(func() { admissionWaitTimeout = previousTimeout })

	core, logs := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	service := NewServiceMemory(log)

	const sessionID = "session-caller-deadline"
	holderAcquired, holderDone, releaseHolder := startSessionAdmissionHolder(t, service, sessionID)
	select {
	case <-holderAcquired:
	case <-time.After(time.Second):
		t.Fatal("admission holder did not start")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err = service.WithSessionAdmission(ctx, sessionID, func(context.Context) error { return nil })
	require.ErrorIs(t, err, context.DeadlineExceeded)

	entries := logs.FilterLevelExact(zapcore.WarnLevel).FilterMessage("queue session admission timed out").All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, sessionID, fields["session_id"])
	heldFor, ok := fields["held_for"].(time.Duration)
	require.True(t, ok, "held_for field is not a duration: %#v", fields["held_for"])
	require.Greater(t, heldFor, time.Duration(0))

	releaseHolder()
	require.NoError(t, <-holderDone)
}

func startSessionAdmissionHolder(
	t *testing.T,
	service *Service,
	sessionID string,
) (chan struct{}, <-chan error, func()) {
	t.Helper()
	holderAcquired := make(chan struct{})
	releaseHolderCh := make(chan struct{})
	var releaseOnce sync.Once
	releaseHolder := func() { releaseOnce.Do(func() { close(releaseHolderCh) }) }
	holderDone := make(chan error, 1)
	holderExited := make(chan struct{})
	t.Cleanup(func() {
		releaseHolder()
		select {
		case <-holderExited:
		case <-time.After(time.Second):
			t.Error("admission holder did not exit")
		}
	})
	go func() {
		defer close(holderExited)
		holderDone <- service.WithSessionAdmission(context.Background(), sessionID, func(context.Context) error {
			close(holderAcquired)
			<-releaseHolderCh
			return nil
		})
	}()
	return holderAcquired, holderDone, releaseHolder
}
