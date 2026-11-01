package messagequeue

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

// TestSessionAdmissionActiveReflectsHeldAndWaitingGates pins the peek used by
// cancellation to distinguish an in-flight dispatch from a settled session.
func TestSessionAdmissionActiveReflectsHeldAndWaitingGates(t *testing.T) {
	svc := NewServiceMemory(logger.Default())
	ctx := context.Background()
	const sessionID = "session-admission-active"

	require.False(t, svc.SessionAdmissionActive(sessionID), "idle session reported an active admission")

	entered := make(chan struct{})
	release := make(chan struct{})
	holderDone := make(chan struct{})
	go func() {
		defer close(holderDone)
		_ = svc.WithSessionAdmission(ctx, sessionID, func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	require.True(t, svc.SessionAdmissionActive(sessionID), "held admission was not reported active")

	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		_ = svc.WithSessionAdmission(ctx, sessionID, func(context.Context) error { return nil })
	}()
	require.True(t, svc.SessionAdmissionActive(sessionID), "waiting admission was not reported active")

	close(release)
	<-holderDone
	<-watcherDone
	require.Eventually(t, func() bool {
		return !svc.SessionAdmissionActive(sessionID)
	}, 2*time.Second, 10*time.Millisecond, "admission stayed active after every holder returned")
}
