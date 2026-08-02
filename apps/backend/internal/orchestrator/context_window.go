package orchestrator

import (
	"context"
	"sync"

	"github.com/kandev/kandev/internal/task/models"
)

const contextWindowMetadataKey = models.SessionMetaKeyContextWindow

// contextWindowWriteGuard serializes context-window metadata writes for one
// session and advances its generation at each agent reset boundary. A context
// update that was observed before a reset cannot write after the reset's clear.
type contextWindowWriteGuard struct {
	mu         sync.Mutex
	generation uint64
}

func (s *Service) contextWindowGuard(sessionID string) *contextWindowWriteGuard {
	guard := &contextWindowWriteGuard{}
	actual, _ := s.contextWindowGuards.LoadOrStore(sessionID, guard)
	return actual.(*contextWindowWriteGuard)
}

func (s *Service) captureContextWindowGeneration(sessionID string) uint64 {
	guard := s.contextWindowGuard(sessionID)
	guard.mu.Lock()
	defer guard.mu.Unlock()
	return guard.generation
}

// clearContextWindowForReset advances the session's context generation while
// holding the same lock used by asynchronous context-window persistence.
func (s *Service) clearContextWindowForReset(ctx context.Context, sessionID string) error {
	guard := s.contextWindowGuard(sessionID)
	guard.mu.Lock()
	defer guard.mu.Unlock()
	guard.generation++
	return s.repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyContextWindow, nil)
}

// ResetContextWindow clears the persisted context-window reading while
// invalidating any update captured before the reset. It is exposed as a narrow
// callback for services that can reset a session without going through the
// orchestrator's reset state machine.
func (s *Service) ResetContextWindow(ctx context.Context, sessionID string) error {
	return s.clearContextWindowForReset(ctx, sessionID)
}

// persistContextWindowUpdate stores an update only when it belongs to the
// current context generation. The bool is false when a reset superseded it.
func (s *Service) persistContextWindowUpdate(
	ctx context.Context,
	sessionID string,
	generation uint64,
	contextWindowData map[string]interface{},
) (bool, int64, error) {
	guard := s.contextWindowGuard(sessionID)
	guard.mu.Lock()
	defer guard.mu.Unlock()
	if guard.generation != generation {
		return false, 0, nil
	}
	count, err := s.repo.UpdateSessionContextWindow(ctx, sessionID, contextWindowData)
	if err != nil {
		return false, 0, err
	}
	return true, count, nil
}

func clearInMemoryContextWindow(session *models.TaskSession) {
	if session == nil {
		return
	}
	if session.Metadata == nil {
		session.Metadata = make(map[string]interface{})
	}
	session.Metadata[contextWindowMetadataKey] = nil
}
