package service

import (
	"context"
	"sort"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/statussummary"
)

// summaryUnreadObservation is one authoritative unread read for the sessions
// of a task-list load. observed is false when the session repository has no
// unread capability or the read failed; the stored aggregate then stands.
type summaryUnreadObservation struct {
	bySession map[string]statussummary.SessionUnreadState
	observed  bool
}

// summaryUnreadValue is one task's sorted unread session IDs and whether they
// are authoritative.
type summaryUnreadValue struct {
	sessionIDs []string
	observed   bool
}

// loadSummaryUnread reads unread state for every listed session; a cursorless
// session reads from a zero cursor.
func (s *Service) loadSummaryUnread(ctx context.Context, sessions []*models.TaskSession) summaryUnreadObservation {
	reader, ok := s.sessions.(repository.SessionUnreadStateRepository)
	if !ok {
		return summaryUnreadObservation{}
	}
	sessionIDs := make([]string, 0, len(sessions))
	for _, session := range sessions {
		if session != nil && session.ID != "" {
			sessionIDs = append(sessionIDs, session.ID)
		}
	}
	if len(sessionIDs) == 0 {
		return summaryUnreadObservation{observed: true}
	}
	bySession, err := reader.ListSessionUnreadState(ctx, sessionIDs)
	if err != nil {
		statussummary.RecordUnreadLoadFailure()
		if s.logger != nil {
			s.logger.Warn("failed to load unread state for status summary repair", zap.Error(err))
		}
		return summaryUnreadObservation{}
	}
	return summaryUnreadObservation{bySession: bySession, observed: true}
}

// forTask derives one task's sorted unread session IDs from the batch read.
func (o summaryUnreadObservation) forTask(sessions []*models.TaskSession) summaryUnreadValue {
	if !o.observed {
		return summaryUnreadValue{}
	}
	var ids []string
	for _, session := range sessions {
		if session == nil {
			continue
		}
		if state, ok := o.bySession[session.ID]; ok && state.Unread() {
			ids = append(ids, session.ID)
		}
	}
	sort.Strings(ids)
	return summaryUnreadValue{sessionIDs: ids, observed: true}
}

// applyToRebuild attaches each session's ranks to a rebuild input.
func (o summaryUnreadObservation) applyToRebuild(input *statussummary.RebuildInput) {
	input.UnreadObserved = o.observed
	if !o.observed {
		return
	}
	for i := range input.Sessions {
		if state, ok := o.bySession[input.Sessions[i].ID]; ok {
			stateCopy := state
			input.Sessions[i].Unread = &stateCopy
		}
	}
}

// reloadTaskUnread re-reads one task's aggregate after a rejected
// compare-and-set, so a concurrent mark-read is not overwritten by a value
// computed before it.
func (s *Service) reloadTaskUnread(ctx context.Context, taskID string, previous summaryUnreadValue) (summaryUnreadValue, error) {
	if !previous.observed {
		return previous, nil
	}
	sessions, err := s.sessions.ListTaskSessions(ctx, taskID)
	if err != nil {
		return summaryUnreadValue{}, err
	}
	return s.loadSummaryUnread(ctx, sessions).forTask(sessions), nil
}

func flattenTaskSessions(tasks []*models.Task, sessionsByTask map[string][]*models.TaskSession) []*models.TaskSession {
	var sessions []*models.TaskSession
	for _, task := range tasks {
		if task != nil {
			sessions = append(sessions, sessionsByTask[task.ID]...)
		}
	}
	return sessions
}
