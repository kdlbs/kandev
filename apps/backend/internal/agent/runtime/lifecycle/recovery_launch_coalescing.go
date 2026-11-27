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
	recoveryAdmissionAttempts := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		value, err := m.doCoalescedExecution(ctx, req.SessionID, operation)
		if ownsAttempt.Load() {
			return value, err
		}
		if errors.Is(err, errWorkspaceEnvironmentCreating) {
			// Workspace readers cannot admit a creating environment. Let launch
			// re-enter the slot without spending a recovery admission attempt.
			continue
		}
		// Workspace readers share the infrastructure slot but cannot admit an
		// unresolved session. Recovery must run its own admission after such a
		// reader fails, while retaining serialization with concurrent launches.
		if !recoveryLaunch || !errors.Is(err, ErrSessionRecoveryRequired) {
			return value, err
		}
		recoveryAdmissionAttempts++
		if recoveryAdmissionAttempts >= 3 {
			return nil, ErrSessionRecoveryRequired
		}
	}
}
