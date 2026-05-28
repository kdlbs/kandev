package process

import (
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

// SubscribeWorkspaceStream creates a new unified workspace stream subscriber
// and sends current git status and file list immediately
func (wt *WorkspaceTracker) SubscribeWorkspaceStream() types.WorkspaceStreamSubscriber {
	sub := make(types.WorkspaceStreamSubscriber, 100)
	wt.AttachWorkspaceStreamSubscriber(sub)
	return sub
}

// AttachWorkspaceStreamSubscriber registers an existing subscriber channel with
// this tracker and immediately replays the current git status. Used by the
// process Manager to fan out a single client subscription across multiple
// per-repo trackers (multi-repo task roots) without giving the client a
// channel per tracker.
//
// Before replaying, run a fresh git status update if the cached snapshot may be
// stale. The cache can lag the working tree when an agent committed via its
// own shell tool (which bypasses GitOperator) while polling was paused or in
// slow mode, or in the brief window between a HEAD-changing operation and the
// next poll tick. Without this refresh, the new subscriber sees the old
// "file=modified" state and only the next poll tick (up to ~30s away in slow
// mode, never in paused mode) corrects it.
func (wt *WorkspaceTracker) AttachWorkspaceStreamSubscriber(sub types.WorkspaceStreamSubscriber) {
	wt.workspaceSubMu.Lock()
	wt.workspaceStreamSubscribers[sub] = struct{}{}
	count := len(wt.workspaceStreamSubscribers)
	wt.workspaceSubMu.Unlock()
	wt.logger.Info("workspace stream subscriber added", zap.Int("subscribers", count))

	// Bare trackers (multi-repo task roots) don't track their own git state —
	// `gitIndexPath == ""` means the workDir isn't itself a git repo, so any
	// `currentStatus` here is either empty (initial) or stale from a previous
	// shape of the tracker. Skip the replay so the subscriber doesn't see a
	// ghost entry under the empty `repository_name`.
	if wt.gitIndexPath == "" {
		return
	}

	// Refresh the cache opportunistically before replaying. Use TryLock so a
	// concurrent poll/refresh doesn't block the attach; in that case we fall
	// through to replaying whatever's cached, which is still no worse than
	// the previous behaviour. We deliberately don't go through
	// tryUpdateGitStatus here because its broadcast would (a) duplicate the
	// manual replay below for this subscriber and (b) push a redundant frame
	// to every already-attached subscriber.
	//
	// Use the tracker's cancellable context (not context.Background) so
	// Stop() can kill an in-flight `git status` here without waiting it out.
	if wt.updateMu.TryLock() {
		if status, err := wt.getGitStatus(wt.cancelCtx); err == nil {
			wt.mu.Lock()
			wt.currentStatus = status
			wt.mu.Unlock()
		}
		wt.updateMu.Unlock()
	}

	wt.mu.RLock()
	currentStatus := wt.currentStatus
	wt.mu.RUnlock()

	if currentStatus.Timestamp.IsZero() {
		currentStatus.Timestamp = time.Now()
	}
	if currentStatus.RepositoryName == "" {
		currentStatus.RepositoryName = wt.repositoryName
	}

	select {
	case sub <- types.NewWorkspaceGitStatus(&currentStatus):
	default:
	}
}

// UnsubscribeWorkspaceStream removes a workspace stream subscriber and closes
// the channel. Use DetachWorkspaceStreamSubscriber when the same channel is
// shared across multiple trackers — only the final detach should close it.
func (wt *WorkspaceTracker) UnsubscribeWorkspaceStream(sub types.WorkspaceStreamSubscriber) {
	wt.DetachWorkspaceStreamSubscriber(sub)
	close(sub)
}

// DetachWorkspaceStreamSubscriber removes the subscriber from this tracker
// without closing its channel. Used by Manager.UnsubscribeWorkspaceStream
// which detaches from each tracker first and then closes once.
func (wt *WorkspaceTracker) DetachWorkspaceStreamSubscriber(sub types.WorkspaceStreamSubscriber) {
	wt.workspaceSubMu.Lock()
	delete(wt.workspaceStreamSubscribers, sub)
	count := len(wt.workspaceStreamSubscribers)
	wt.workspaceSubMu.Unlock()
	wt.logger.Info("workspace stream subscriber removed", zap.Int("subscribers", count))
}

// notifyWorkspaceStreamGitStatus sends git status to all workspace stream subscribers
func (wt *WorkspaceTracker) notifyWorkspaceStreamGitStatus(update types.GitStatusUpdate) {
	wt.workspaceSubMu.RLock()
	defer wt.workspaceSubMu.RUnlock()

	msg := types.NewWorkspaceGitStatus(&update)
	for sub := range wt.workspaceStreamSubscribers {
		select {
		case sub <- msg:
		default:
			// Subscriber is slow, skip
		}
	}
}

// notifyWorkspaceStreamGitCommit sends git commit notification to all workspace stream subscribers
func (wt *WorkspaceTracker) notifyWorkspaceStreamGitCommit(commit *types.GitCommitNotification) {
	wt.workspaceSubMu.RLock()
	defer wt.workspaceSubMu.RUnlock()

	msg := types.NewWorkspaceGitCommit(commit)
	for sub := range wt.workspaceStreamSubscribers {
		select {
		case sub <- msg:
		default:
			// Subscriber is slow, skip
		}
	}
}

// NotifyGitCommit notifies all subscribers about a new git commit.
// It also updates the cached HEAD SHA to prevent polling from re-detecting the same commit.
// Callers that mutate the working tree are responsible for refreshing
// currentStatus separately (see GitOperator's triggerRefresh) — keeping that
// responsibility on the caller avoids implicit blocking inside this Notify*
// path, which is otherwise a fast event emit.
func (wt *WorkspaceTracker) NotifyGitCommit(commit *types.GitCommitNotification) {
	// Update cached HEAD to the new commit SHA so polling doesn't re-detect it
	if commit.CommitSHA != "" {
		wt.gitStateMu.Lock()
		wt.cachedHeadSHA = commit.CommitSHA
		wt.gitStateMu.Unlock()
	}

	wt.notifyWorkspaceStreamGitCommit(commit)
}

// NotifyGitReset notifies all subscribers about a git reset (HEAD moved backward).
// It also updates the cached HEAD SHA to the new position.
func (wt *WorkspaceTracker) NotifyGitReset(reset *types.GitResetNotification) {
	// Update cached HEAD to the new position
	if reset.CurrentHead != "" {
		wt.gitStateMu.Lock()
		wt.cachedHeadSHA = reset.CurrentHead
		wt.gitStateMu.Unlock()
	}

	wt.notifyWorkspaceStreamGitReset(reset)
}

// notifyWorkspaceStreamGitReset sends git reset notification to all workspace stream subscribers
func (wt *WorkspaceTracker) notifyWorkspaceStreamGitReset(reset *types.GitResetNotification) {
	wt.workspaceSubMu.RLock()
	defer wt.workspaceSubMu.RUnlock()

	msg := types.NewWorkspaceGitReset(reset)
	for sub := range wt.workspaceStreamSubscribers {
		select {
		case sub <- msg:
		default:
			// Subscriber is slow, skip
		}
	}
}

// notifyWorkspaceStreamBranchSwitch sends branch switch notification to all workspace stream subscribers
func (wt *WorkspaceTracker) notifyWorkspaceStreamBranchSwitch(branchSwitch *types.GitBranchSwitchNotification) {
	wt.workspaceSubMu.RLock()
	defer wt.workspaceSubMu.RUnlock()

	msg := types.NewWorkspaceBranchSwitch(branchSwitch)
	for sub := range wt.workspaceStreamSubscribers {
		select {
		case sub <- msg:
		default:
			// Subscriber is slow, skip
		}
	}
}

// notifyWorkspaceStreamFileChange sends file change notification to all workspace stream subscribers
func (wt *WorkspaceTracker) notifyWorkspaceStreamFileChange(notification types.FileChangeNotification) {
	wt.workspaceSubMu.RLock()
	defer wt.workspaceSubMu.RUnlock()

	msg := types.NewWorkspaceFileChange(&notification)
	for sub := range wt.workspaceStreamSubscribers {
		select {
		case sub <- msg:
		default:
			// Subscriber is slow, skip
		}
	}
}

// notifyWorkspaceStreamProcessOutput sends process output to all workspace stream subscribers
func (wt *WorkspaceTracker) notifyWorkspaceStreamProcessOutput(output *types.ProcessOutput) {
	wt.workspaceSubMu.RLock()
	defer wt.workspaceSubMu.RUnlock()

	msg := types.NewWorkspaceProcessOutput(output)
	wt.logger.Debug("broadcast process output",
		zap.String("session_id", output.SessionID),
		zap.String("process_id", output.ProcessID),
		zap.String("kind", string(output.Kind)),
		zap.Int("subscribers", len(wt.workspaceStreamSubscribers)),
	)
	for sub := range wt.workspaceStreamSubscribers {
		select {
		case sub <- msg:
		default:
			// Subscriber is slow, skip
		}
	}
}

// notifyWorkspaceStreamProcessStatus sends process status updates to all workspace stream subscribers
func (wt *WorkspaceTracker) notifyWorkspaceStreamProcessStatus(status *types.ProcessStatusUpdate) {
	wt.workspaceSubMu.RLock()
	defer wt.workspaceSubMu.RUnlock()

	msg := types.NewWorkspaceProcessStatus(status)
	wt.logger.Debug("broadcast process status",
		zap.String("session_id", status.SessionID),
		zap.String("process_id", status.ProcessID),
		zap.String("status", string(status.Status)),
		zap.Int("subscribers", len(wt.workspaceStreamSubscribers)),
	)
	for sub := range wt.workspaceStreamSubscribers {
		select {
		case sub <- msg:
		default:
			// Subscriber is slow, skip
		}
	}
}
