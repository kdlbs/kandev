package service

import (
	"context"
	"strconv"

	"github.com/kandev/kandev/internal/task/models"
)

// SetPendingActionProjectionEpoch installs the durable generation allocated at
// backend startup. Callers must set it before serving requests.
func (s *Service) SetPendingActionProjectionEpoch(epoch uint64) {
	if epoch == 0 {
		return
	}
	s.pendingActionProjectionMu.Lock()
	defer s.pendingActionProjectionMu.Unlock()
	s.pendingActionProjectionEpoch = strconv.FormatUint(epoch, 10)
	s.pendingActionProjectionSequence = 0
	s.pendingActionProjectionObserved = make(map[string]pendingActionProjectionState)
	s.pendingActionSnapshotValues = make(map[string]pendingActionProjectionState)
	s.lastPendingActionProjections = make(map[string]pendingActionProjectionState)
}

// GetPendingActionProjectionsForSessions returns the authoritative action and
// a cross-channel revision for every requested session. The logical revision
// is reserved before the database read: if this read is delayed behind a newer
// message event, clients can reject it without relying on HTTP completion order.
func (s *Service) GetPendingActionProjectionsForSessions(
	ctx context.Context,
	sessionIDs []string,
) (
	map[string]models.TaskPendingAction,
	map[string]models.PendingActionRevision,
	error,
) {
	revisions := s.reservePendingActionProjectionRevisions(sessionIDs)
	actions, err := s.GetPendingActionsForSessions(ctx, sessionIDs)
	if err != nil {
		return nil, nil, err
	}
	actions, revisions = s.stabilizePendingActionEventProjections(sessionIDs, actions, revisions)
	return actions, revisions, nil
}

// GetPendingActionSnapshotProjectionsForSessions retains the latest revision
// while a snapshot's pending-action value is unchanged. The reserved read
// revision still orders this snapshot against delayed events and snapshots.
func (s *Service) GetPendingActionSnapshotProjectionsForSessions(
	ctx context.Context,
	sessionIDs []string,
) (
	map[string]models.TaskPendingAction,
	map[string]models.PendingActionRevision,
	error,
) {
	candidates := s.reservePendingActionProjectionRevisions(sessionIDs)
	actions, err := s.GetPendingActionsForSessions(ctx, sessionIDs)
	if err != nil {
		return nil, nil, err
	}
	actions, revisions := s.stabilizePendingActionSnapshotProjections(sessionIDs, actions, candidates)
	return actions, revisions, nil
}

func (s *Service) stabilizePendingActionSnapshotProjections(
	sessionIDs []string,
	actions map[string]models.TaskPendingAction,
	candidates map[string]models.PendingActionRevision,
) (map[string]models.TaskPendingAction, map[string]models.PendingActionRevision) {
	return s.completePendingActionProjectionReads(sessionIDs, actions, candidates, true)
}

func (s *Service) stabilizePendingActionEventProjections(
	sessionIDs []string,
	actions map[string]models.TaskPendingAction,
	candidates map[string]models.PendingActionRevision,
) (map[string]models.TaskPendingAction, map[string]models.PendingActionRevision) {
	return s.completePendingActionProjectionReads(sessionIDs, actions, candidates, false)
}

func (s *Service) completePendingActionProjectionReads(
	sessionIDs []string,
	actions map[string]models.TaskPendingAction,
	candidates map[string]models.PendingActionRevision,
	stableRevision bool,
) (map[string]models.TaskPendingAction, map[string]models.PendingActionRevision) {
	s.pendingActionProjectionMu.Lock()
	defer s.pendingActionProjectionMu.Unlock()
	if s.pendingActionProjectionObserved == nil {
		s.pendingActionProjectionObserved = make(map[string]pendingActionProjectionState)
	}
	if s.pendingActionSnapshotValues == nil {
		s.pendingActionSnapshotValues = make(map[string]pendingActionProjectionState)
	}

	stableActions := make(map[string]models.TaskPendingAction, len(actions))
	stableRevisions := make(map[string]models.PendingActionRevision, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		candidate, ok := candidates[sessionID]
		if !ok || sessionID == "" {
			continue
		}
		action := actions[sessionID]
		observed, observedExists := s.pendingActionProjectionObserved[sessionID]
		stable, stableExists := s.pendingActionSnapshotValues[sessionID]
		if !observedExists || pendingActionRevisionAfter(candidate, observed.revision) {
			observed = pendingActionProjectionState{action: action, revision: candidate}
			s.pendingActionProjectionObserved[sessionID] = observed
			if !stableExists || stable.action != action {
				stable = pendingActionProjectionState{action: action, revision: candidate}
				s.pendingActionSnapshotValues[sessionID] = stable
			}
		} else if !stableExists {
			stable = observed
			s.pendingActionSnapshotValues[sessionID] = stable
		}
		if stable.action != "" {
			stableActions[sessionID] = stable.action
		}
		if stableRevision {
			stableRevisions[sessionID] = stable.revision
		} else {
			stableRevisions[sessionID] = observed.revision
		}
	}
	return stableActions, stableRevisions
}

func (s *Service) reservePendingActionProjectionRevisions(
	sessionIDs []string,
) map[string]models.PendingActionRevision {
	s.pendingActionProjectionMu.Lock()
	defer s.pendingActionProjectionMu.Unlock()
	revisions := make(map[string]models.PendingActionRevision, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		if sessionID == "" {
			continue
		}
		if _, exists := revisions[sessionID]; exists {
			continue
		}
		s.pendingActionProjectionSequence++
		revisions[sessionID] = models.PendingActionRevision{
			Epoch:    s.pendingActionProjectionEpoch,
			Sequence: s.pendingActionProjectionSequence,
		}
	}
	return revisions
}
