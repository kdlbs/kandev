// Package sshreconcile implements the private `agentctl ssh-reconcile`
// operation: owner-bound verification and, after session-directory loss,
// termination of a recorded SSH controller.
//
// It is deliberately separate from the ordinary stop path. Ordinary stops are
// guarded by the session directory's pidfile; this helper re-proves the
// recorded process from its launch receipt, opens a pidfd, and signals through
// that handle only. It never removes a path and never falls back to a numeric
// PID signal.
package sshreconcile

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/common/sshlaunch"
)

// Reconcile validates the request and performs the platform-specific
// reconciliation. Every failure is a closed unresolved outcome; the caller
// keeps the recorded owner and its cleanup inventory.
func Reconcile(ctx context.Context, req sshlaunch.ReconcileRequest) sshlaunch.ReconcileResult {
	if err := req.Validate(); err != nil {
		return sshlaunch.Unresolved(sshlaunch.ReasonProbeUnknown, "launch receipt is incomplete")
	}
	if req.Mode.Effective() == sshlaunch.ReconcileModeTerminate {
		if failure, ok := directoryAbsenceFailure(req.Receipt.Owner.SessionDir); !ok {
			return failure
		}
	}
	return reconcileProcess(ctx, req.Receipt, req.Mode.Effective())
}

// directoryAbsenceFailure admits only a positively absent canonical session
// path. A present directory, file, or symlink refuses the terminate route
// before any signal, and an unreadable path is unknown rather than absent.
func directoryAbsenceFailure(sessionDir string) (sshlaunch.ReconcileResult, bool) {
	if strings.TrimSpace(sessionDir) == "" {
		return sshlaunch.Unresolved(sshlaunch.ReasonProbeUnknown, "session directory is not recorded"), false
	}
	if _, err := os.Lstat(sessionDir); err == nil {
		return sshlaunch.Unresolved(sshlaunch.ReasonIdentityConflict, "session directory exists"), false
	} else if !errors.Is(err, fs.ErrNotExist) {
		return sshlaunch.Unresolved(sshlaunch.ReasonProbeUnknown, "session directory could not be inspected"), false
	}
	parent := filepath.Dir(sessionDir)
	if info, err := os.Lstat(parent); err == nil {
		if !info.IsDir() {
			return sshlaunch.Unresolved(sshlaunch.ReasonIdentityConflict, "session parent is not a directory"), false
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return sshlaunch.Unresolved(sshlaunch.ReasonProbeUnknown, "session parent could not be inspected"), false
	}
	return sshlaunch.ReconcileResult{}, true
}
