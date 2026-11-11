//go:build !linux

package sshreconcile

import (
	"context"

	"github.com/kandev/kandev/internal/common/sshlaunch"
)

// reconcileProcess reports the platform as unsupported. The existing guarded
// stop and confirmed-absence paths remain the only behavior there.
func reconcileProcess(_ context.Context, _ sshlaunch.Receipt, _ sshlaunch.ReconcileMode) sshlaunch.ReconcileResult {
	return sshlaunch.Unresolved(sshlaunch.ReasonUnsupportedIdentity, "pidfd reconciliation requires Linux")
}
