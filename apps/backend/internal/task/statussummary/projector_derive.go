package statussummary

import (
	"strings"
)

func deriveSummary(state *projectionState) TaskStatusSummary {
	primary, foregroundActivity, activeSubagentCount := deriveSessionFields(state)
	return TaskStatusSummary{
		PrimarySession:      primary,
		ForegroundActivity:  foregroundActivity,
		ActiveSubagentCount: activeSubagentCount,
		PendingAction:       derivePendingAction(state),
		ActiveError:         deriveActiveError(state),
		Git:                 deriveGitSummary(state),
		PullRequest:         derivePullRequestSummary(state),
		QueuedPromptCount:   state.queuedCount,
		LastActivityAt:      cloneTimePtr(state.lastActivityAt),
	}
}

func deriveSessionFields(state *projectionState) (*PrimarySessionSummary, string, int) {
	var primary *sessionObservation
	activities := make([]string, 0, len(state.sessions))
	activeSubagentCount := 0
	for _, session := range state.sessions {
		if session.isPrimary && (primary == nil || session.id < primary.id) {
			copy := session
			primary = &copy
		}
		if state.activityObserved {
			if session.state == sessionStateRunning || session.foregroundActivity == activityBackground {
				activities = append(activities, session.foregroundActivity)
			}
			if isActiveSessionState(session.state) {
				activeSubagentCount += maxInt(session.activeSubagentCount, 0)
			}
		}
	}
	var primarySummary *PrimarySessionSummary
	if primary != nil {
		primarySummary = &PrimarySessionSummary{ID: primary.id, State: primary.state}
	}
	foregroundActivity := ""
	if state.activityObserved {
		foregroundActivity = deriveForegroundActivity(activities)
	} else if state.current != nil {
		foregroundActivity = state.current.ForegroundActivity
		activeSubagentCount = state.current.ActiveSubagentCount
	}
	return primarySummary, foregroundActivity, activeSubagentCount
}

func isActiveSessionState(state string) bool {
	return state == sessionStateStarting || state == sessionStateRunning || state == sessionStateWaitingForInput
}

func deriveForegroundActivity(activities []string) string {
	hasBackground := false
	for _, activity := range activities {
		if activity == activityGenerating {
			return activityGenerating
		}
		if activity == activityBackground {
			hasBackground = true
		}
	}
	if hasBackground {
		return activityBackground
	}
	return ""
}

func derivePendingAction(state *projectionState) string {
	if !state.pendingObserved && state.current != nil {
		return state.current.PendingAction
	}
	action := state.taskPending
	for _, candidate := range state.pending {
		if candidate == pendingPermission {
			return candidate
		}
		if candidate == pendingClarification && action == "" {
			action = candidate
		}
	}
	return action
}

func deriveActiveError(state *projectionState) *ActiveErrorSummary {
	if !state.errorsObserved && state.current != nil && state.current.ActiveError != nil {
		copy := *state.current.ActiveError
		return &copy
	}
	var active *ActiveErrorSummary
	for _, candidate := range state.errors {
		if candidate == nil || (active != nil && !candidate.OccurredAt.After(active.OccurredAt)) {
			continue
		}
		copy := *candidate
		active = &copy
	}
	return active
}

func deriveGitSummary(state *projectionState) *GitSummary {
	if !state.gitObserved && state.gitBaseline != nil {
		copy := *state.gitBaseline
		return &copy
	}
	var git GitSummary
	for _, observation := range state.git {
		git.Additions += observation.Additions
		git.Deletions += observation.Deletions
		git.ChangedFiles += observation.ChangedFiles
		git.Ahead += observation.Ahead
		git.Behind += observation.Behind
	}
	if git == (GitSummary{}) {
		return nil
	}
	return &git
}

// isGoneTaskPersistErr reports whether a summary write failed because the task
// row is already gone (FK cascade after delete). Transient failures must not
// match so zero-count updates can retry on a still-live task.
func isGoneTaskPersistErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "foreign key") ||
		strings.Contains(msg, "constraint failed") ||
		strings.Contains(msg, "violates foreign key")
}

// isMissingTaskResolveErr reports whether workspace resolution failed because
// the task no longer exists. Transient repository errors must not match.
func isMissingTaskResolveErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") ||
		strings.Contains(msg, "no rows") ||
		strings.Contains(msg, "sql: no rows in result set")
}
