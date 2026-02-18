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

	wt.workspaceSubMu.Lock()
	wt.workspaceStreamSubscribers[sub] = struct{}{}
	count := len(wt.workspaceStreamSubscribers)
	wt.workspaceSubMu.Unlock()
	wt.logger.Info("workspace stream subscriber added", zap.Int("subscribers", count))

	// Send current git status immediately
	wt.mu.RLock()
	currentStatus := wt.currentStatus
	wt.mu.RUnlock()

	if currentStatus.Timestamp.IsZero() {
		currentStatus.Timestamp = time.Now()
	}

	// Send git status
	select {
	case sub <- types.NewWorkspaceGitStatus(&currentStatus):
	default:
	}

	return sub
}

// UnsubscribeWorkspaceStream removes and closes a workspace stream subscriber
func (wt *WorkspaceTracker) UnsubscribeWorkspaceStream(sub types.WorkspaceStreamSubscriber) {
	wt.workspaceSubMu.Lock()
	delete(wt.workspaceStreamSubscribers, sub)
	count := len(wt.workspaceStreamSubscribers)
	wt.workspaceSubMu.Unlock()
	close(sub)
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
func (wt *WorkspaceTracker) NotifyGitCommit(commit *types.GitCommitNotification) {
	// Update cached HEAD to the new commit SHA so polling doesn't re-detect it
	if commit.CommitSHA != "" {
		wt.gitStateMu.Lock()
		wt.cachedHeadSHA = commit.CommitSHA
		wt.gitStateMu.Unlock()
	}

	wt.notifyWorkspaceStreamGitCommit(commit)
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
