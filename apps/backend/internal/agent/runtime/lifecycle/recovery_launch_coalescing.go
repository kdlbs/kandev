package lifecycle

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/kandev/kandev/internal/agent/runtime/activity"
)

func (m *Manager) coalesceSessionLaunch(ctx context.Context, req *LaunchRequest) (interface{}, error) {
	var ownsAttempt atomic.Bool
	operation := func(sharedCtx context.Context) (interface{}, error) {
		ownsAttempt.Store(true)
		activityLease, err := m.acquireActivity(sharedCtx, activity.KindExecutionStarting)
		if err != nil {
			return nil, err
		}
		defer activityLease.Release()
		activityLease.SetKind(activity.KindExecutionPreparing)
		return m.launchInternal(sharedCtx, req)
	}
	recoveryLaunch := req.RecoveryAction != "" || req.CandidateExecutionID != ""
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		value, err := m.doCoalescedExecution(ctx, req.SessionID, operation)
		// Workspace readers share the infrastructure slot but cannot admit an
		// unresolved session. Recovery must run its own admission after such a
		// reader fails, while retaining serialization with concurrent launches.
		if ownsAttempt.Load() || !recoveryLaunch || !errors.Is(err, ErrSessionRecoveryRequired) {
			return value, err
		}
	}
	return nil, ErrSessionRecoveryRequired
}
