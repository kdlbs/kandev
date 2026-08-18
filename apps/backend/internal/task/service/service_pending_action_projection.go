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
	return actions, revisions, nil
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
