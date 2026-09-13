package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/waveidentity"
)

// resolveWaveIdentity performs the terminality-confirming last read
// shared by the two engine-routed task_children_completed producers:
// the edge-triggered path (event_subscribers.go, queueChildrenCompletedRun)
// and the backstop reconciler (scheduler_wake_reconciler.go,
// ParentWakeReconciler.reconcileOne). A wave identity is derived from, and
// only from, a wave-member read that itself observed every member
// terminal — not from an earlier AreAllChildrenTerminal / GetChildSetKey
// read, which counts every child rather than just wave members. A read
// error, any non-terminal wave member, or an empty wave-member set (a
// parent with no wave members has no wave) all report ok=false: the
// caller must queue no run, and the log param (nilable) records why at
// debug.
func resolveWaveIdentity(
	ctx context.Context, repo *sqlite.Repository, parentID string, log *logger.Logger,
) (waveKey, waveString string, ok bool) {
	members, err := repo.ListWaveMembers(ctx, parentID)
	if err != nil {
		if log != nil {
			log.Debug("list wave members failed",
				zap.String("parent_id", parentID), zap.Error(err))
		}
		return "", "", false
	}
	if len(members) == 0 {
		return "", "", false
	}
	ids := make([]string, 0, len(members))
	for _, m := range members {
		if m.State != "COMPLETED" && m.State != "CANCELLED" {
			return "", "", false
		}
		ids = append(ids, m.TaskID)
	}
	return waveidentity.WaveKey(parentID, ids), waveidentity.WaveString(parentID, ids), true
}
