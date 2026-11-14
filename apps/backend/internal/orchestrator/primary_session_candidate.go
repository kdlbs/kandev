package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
)

// primaryCandidateRank orders sessions for an automatic primary promotion. The
// ranking prefers a workflow-compatible session, then an unmatched session,
// then a parked session. A removal can fall back to a session awaiting recovery;
// a stop excludes that last rank.
type primaryCandidateRank int

const (
	// primaryCandidateAwaitingRecovery is available only as a removal fallback.
	primaryCandidateAwaitingRecovery primaryCandidateRank = iota
	// primaryCandidateParked stays available after workflow parking, ahead of
	// sessions awaiting recovery but below every other healthy candidate.
	primaryCandidateParked
	// primaryCandidateOther is a session whose profile is not the current
	// step's effective profile.
	primaryCandidateOther
	// primaryCandidateCompatible is a session that already uses the current
	// step's effective profile.
	primaryCandidateCompatible
)

// sessionStatePreference keeps the RUNNING > active > idle order as the
// tiebreak inside one rank, so the selector changes which rank wins and never
// how equal sessions are ordered.
func sessionStatePreference(state models.TaskSessionState) int {
	if state == models.TaskSessionStateRunning {
		return 2
	}
	if isActiveSessionState(state) {
		return 1
	}
	return 0
}

// workflowCompatibleSession reports whether a session uses the profile the
// task's current step expects. A task without a step, or with no resolvable
// profile, imposes no constraint.
func workflowCompatibleSession(task *models.Task, session *models.TaskSession, effectiveProfileID string) bool {
	if task == nil || task.WorkflowStepID == "" || effectiveProfileID == "" {
		return true
	}
	return session != nil && session.AgentProfileID == effectiveProfileID
}

// effectiveWorkflowProfileForTask resolves the profile the task's current step
// expects, or "" when the task has no step or the step cannot be read.
func (s *Service) effectiveWorkflowProfileForTask(ctx context.Context, task *models.Task) string {
	if task == nil || task.WorkflowStepID == "" || s.workflowStepGetter == nil {
		return ""
	}
	step, err := s.workflowStepGetter.GetStep(ctx, task.WorkflowStepID)
	if err != nil || step == nil {
		return ""
	}
	return s.resolveStepAgentProfileForTask(ctx, task, step)
}

// bestPrimarySessionCandidate returns the session an automatic promotion
// should take, or "" when no session is eligible.
//
// Terminal sessions are never candidates. activeOnly keeps each caller's
// eligibility: maybePromotePrimary passes true, so only an active session can
// inherit the role after a stop; promoteNextPrimaryAfterRemoval passes false,
// so any nonterminal session can inherit it after a deletion. Stop promotions
// exclude sessions awaiting recovery; removal promotions rank them last.
func (s *Service) bestPrimarySessionCandidate(
	ctx context.Context, taskID string, sessions []*models.TaskSession, excludeSessionID string,
	activeOnly, excludeAwaitingRecovery bool,
) string {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		task = nil
	}
	effectiveProfileID := s.effectiveWorkflowProfileForTask(ctx, task)

	var (
		best      *models.TaskSession
		bestRank  primaryCandidateRank
		bestState int
	)
	for _, session := range sessions {
		if session == nil || session.ID == excludeSessionID || isTerminalSessionState(session.State) {
			continue
		}
		if activeOnly && !isActiveSessionState(session.State) {
			continue
		}
		rank := s.rankPrimarySessionCandidate(ctx, task, session, effectiveProfileID)
		if excludeAwaitingRecovery && rank == primaryCandidateAwaitingRecovery {
			continue
		}
		state := sessionStatePreference(session.State)
		if best == nil || rank > bestRank || (rank == bestRank && state > bestState) {
			best, bestRank, bestState = session, rank, state
		}
	}
	if best == nil {
		return ""
	}
	return best.ID
}

// rankPrimarySessionCandidate keeps recovery availability ahead of workflow
// preference while preserving the existing ordering among healthy sessions.
func (s *Service) rankPrimarySessionCandidate(
	ctx context.Context, task *models.Task, session *models.TaskSession, effectiveProfileID string,
) primaryCandidateRank {
	if s.sessionAwaitsRecoveryAction(ctx, session) {
		return primaryCandidateAwaitingRecovery
	}
	if _, parked := models.LoadWorkflowParking(session.Metadata); parked {
		return primaryCandidateParked
	}
	if workflowCompatibleSession(task, session, effectiveProfileID) {
		return primaryCandidateCompatible
	}
	return primaryCandidateOther
}
