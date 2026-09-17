package statussummary

import (
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

func deriveSummary(state *projectionState) TaskStatusSummary {
	primary, foregroundActivity, activeSubagentCount := deriveSessionFields(state)
	return TaskStatusSummary{
		PrimarySession:      primary,
		ForegroundActivity:  foregroundActivity,
		ActiveSubagentCount: activeSubagentCount,
		PendingAction:       derivePendingAction(state),
		ActiveError:         deriveActiveError(state),
		TaskError:           cloneActiveError(state.taskError),
		Git:                 deriveGitSummary(state),
		PullRequest:         derivePullRequestSummary(state),
		QueuedPromptCount:   state.queuedCount,
		LastActivityAt:      cloneTimePtr(state.lastActivityAt),
		LaunchQueue:         cloneLaunchQueue(state.launchQueue),
	}
}

func cloneLaunchQueue(queue *LaunchQueueSummary) *LaunchQueueSummary {
	if queue == nil {
		return nil
	}
	copy := *queue
	if queue.Capacity != nil {
		capacity := *queue.Capacity
		copy.Capacity = &capacity
	}
	return &copy
}

func equalLaunchQueue(left, right *LaunchQueueSummary) bool {
	if left == nil || right == nil {
		return left == right
	}
	if *left != *right {
		return false
	}
	if left.Capacity == nil || right.Capacity == nil {
		return left.Capacity == right.Capacity
	}
	return *left.Capacity == *right.Capacity
}

func cloneActiveError(value *ActiveErrorSummary) *ActiveErrorSummary {
	if value == nil {
		return nil
	}
	copy := *value
	copy.RecoveryActions = append([]string(nil), value.RecoveryActions...)
	copy.Causes = append([]models.AgentErrorCause(nil), value.Causes...)
	return &copy
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
	var active *ActiveErrorSummary
	if !state.errorsObserved && state.current != nil && state.current.ActiveError != nil &&
		(!state.taskErrorObserved || activeErrorScope(state.current.ActiveError) == models.ErrorScopeSession) {
		copy := *state.current.ActiveError
		active = &copy
	}
	for _, candidate := range state.errors {
		if candidate == nil || !newerActiveError(candidate, active) {
			continue
		}
		copy := *candidate
		active = &copy
	}
	if state.taskError != nil && newerActiveError(state.taskError, active) {
		copy := *state.taskError
		active = &copy
	}
	return active
}

// activeErrorScope routes explicit ownership first. Older summaries omitted
// scope, so their session-id inference remains the compatibility fallback.
func activeErrorScope(value *ActiveErrorSummary) string {
	if value != nil && (value.Scope == models.ErrorScopeSession || value.Scope == models.ErrorScopeTask) {
		return value.Scope
	}
	if value != nil && value.SessionID != "" {
		return models.ErrorScopeSession
	}
	return models.ErrorScopeTask
}

func newerActiveError(candidate, current *ActiveErrorSummary) bool {
	if candidate == nil {
		return false
	}
	if current == nil {
		return true
	}
	if candidate.OccurredAt.After(current.OccurredAt) {
		return true
	}
	if !candidate.OccurredAt.Equal(current.OccurredAt) {
		return false
	}
	if candidate.Stamp != current.Stamp {
		return candidate.Stamp > current.Stamp
	}
	if candidate.SessionID != current.SessionID {
		return candidate.SessionID > current.SessionID
	}
	return candidate.TaskRepositoryID > current.TaskRepositoryID
}

func deriveGitSummary(state *projectionState) *GitSummary {
	if !state.gitObserved && state.gitBaseline != nil {
		copy := *state.gitBaseline
		return &copy
	}
	var git GitSummary
	for _, observation := range state.git {
		if observation.ComparisonUnavailable {
			return &GitSummary{ComparisonUnavailable: true}
		}
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
