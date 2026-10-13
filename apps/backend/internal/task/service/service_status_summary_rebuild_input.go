package service

import (
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/statussummary"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"strings"
	"time"
)

func (s *Service) rebuildInput(
	taskError *statussummary.ActiveErrorSummary,
	sessions []*models.TaskSession,
	taskEnvironmentIDs []string,
	pendingBySession map[string]models.TaskPendingAction,
	gitByEnvironment map[string][]*models.GitSnapshot,
	gitObserved bool,
	prs []statussummary.PullRequestInput,
	prObserved bool,
	queuedPromptCount int,
	launchQueue *statussummary.LaunchQueueSummary,
	completionGate *statussummary.CompletionGateSummary,
	completionGateObserved bool,
	lastActivityAt time.Time,
	activityObserved bool,
	sessionsObserved bool,
	now time.Time,
) statussummary.RebuildInput {
	input := statussummary.RebuildInput{
		Sessions:               make([]statussummary.RebuildSession, 0, len(sessions)),
		SessionsObserved:       sessionsObserved,
		TaskError:              taskError,
		PendingActions:         make(map[string]string),
		ActivityObserved:       s.foregroundActivity != nil,
		LastActivityAt:         nil,
		PullRequests:           prs,
		PRObserved:             prObserved,
		GitObserved:            gitObserved,
		QueuedPromptCount:      maxInt(queuedPromptCount, 0),
		LaunchQueue:            cloneLaunchQueueForService(launchQueue),
		CompletionGate:         cloneCompletionGateForService(completionGate),
		CompletionGateObserved: completionGateObserved,
		Now:                    now,
	}
	if activityObserved && !lastActivityAt.IsZero() {
		activityCopy := lastActivityAt.UTC()
		input.LastActivityAt = &activityCopy
	}
	for _, session := range sessions {
		if session == nil || session.ID == "" {
			continue
		}
		input.Sessions = append(input.Sessions, s.rebuildSessionInput(session))
		if action := string(pendingBySession[session.ID]); strings.TrimSpace(action) != "" {
			input.PendingActions[session.ID] = action
		}
	}
	for _, environmentID := range taskEnvironmentIDs {
		for _, snapshot := range gitByEnvironment[environmentID] {
			if snapshot == nil {
				continue
			}
			input.Git = append(input.Git, statussummary.RebuildGit{
				Repository: snapshotRepositoryKey(snapshot, ""),
				Summary:    gitSummaryFromSnapshot(snapshot),
			})
		}
	}
	return input
}

func (s *Service) rebuildSessionInput(session *models.TaskSession) statussummary.RebuildSession {
	countProvider, hasCountProvider := s.foregroundActivity.(activeSubagentCountProvider)
	activity := ""
	activeSubagentCount := 0
	if s.foregroundActivity != nil {
		value := s.foregroundActivity.ForegroundActivity(session.ID)
		if session.State == models.TaskSessionStateRunning || value == v1.ForegroundActivityBackground {
			activity = string(value)
		}
		if hasCountProvider {
			activeSubagentCount = countProvider.ActiveSubagentCount(session.ID)
		}
	}
	var activeError *statussummary.ActiveErrorSummary
	if lastError, ok := models.LoadLastAgentError(session.Metadata); ok && !lastError.IsDismissed() {
		activeError = &statussummary.ActiveErrorSummary{
			Scope:            models.ErrorScopeSession,
			SessionID:        session.ID,
			TaskRepositoryID: lastError.TaskRepositoryID,
			ExecutionID:      lastError.ExecutionID,
			AttemptID:        lastError.AttemptID,
			Phase:            lastError.Phase,
			Stamp:            lastError.Stamp(),
			OccurredAt:       lastError.OccurredAt,
			Preview:          lastError.Message,
			Details:          lastError.Details,
			Category:         lastError.Code,
			RecoveryActions:  lastError.RecoveryActions,
			Causes:           lastError.Causes,
		}
	}
	return statussummary.RebuildSession{
		ID:                  session.ID,
		State:               string(session.State),
		IsPrimary:           session.IsPrimary,
		ForegroundActivity:  activity,
		ActiveSubagentCount: activeSubagentCount,
		ActiveError:         activeError,
	}
}
