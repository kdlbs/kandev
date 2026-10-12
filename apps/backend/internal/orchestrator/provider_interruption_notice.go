package orchestrator

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
)

func (s *Service) updateContinuationPhase(ctx context.Context, taskID, sessionID string, entry *transientRetryEntry, phase string) {
	state, release := s.acquireTransientRetryNoticeState(sessionID)
	state.mu.Lock()
	defer state.mu.Unlock()
	defer release()
	if current, ok := s.transientRetries.Load(sessionID); !ok || current != entry || ctx.Err() != nil {
		return
	}
	data := watcher.AgentEventData{TaskID: taskID, SessionID: sessionID, RecoveryMode: recoveryModeContinue, RecoveryPhase: phase}
	data.ProviderError = &streams.ProviderError{ProviderID: entry.providerID, ModelID: entry.modelID}
	s.createTransientRetryStatusMessageLocked(state, ctx, data, &routingerr.Error{Code: routingerr.CodeAgentTransportLost}, entry.attempt, 0, time.Now().UTC())
}
