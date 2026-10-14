package orchestrator

import (
	"context"
	"errors"
	taskservice "github.com/kandev/kandev/internal/task/service"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

func (s *Service) handleBackgroundWorkUpdatedEvent(ctx context.Context, payload *runtime.AgentStreamEventPayload) {
	sessionID := payload.SessionID
	if sessionID == "" || payload.Data.BackgroundWork == nil {
		return
	}
	obs := *payload.Data.BackgroundWork
	obs.SessionID = sessionID
	if s.backgroundWorkObserver != nil {
		_ = s.backgroundWorkObserver.RecordBackgroundWorkloadObservation(ctx, obs, payload.TaskID, sessionID)
	}
	if s.eventBus != nil {
		subject := events.BuildBackgroundWorkUpdatedSubject(sessionID)
		_ = s.eventBus.Publish(ctx, subject, bus.NewEvent(events.BackgroundWorkUpdated, "orchestrator", &obs))
	}
}

func (s *Service) handleBackgroundWorkOutputEvent(ctx context.Context, payload *runtime.AgentStreamEventPayload) {
	sessionID := payload.SessionID
	if sessionID == "" || payload.Data.BackgroundWorkOutput == nil {
		return
	}
	chunk := *payload.Data.BackgroundWorkOutput
	chunk.SessionID = sessionID
	if s.backgroundWorkObserver != nil {
		_ = s.backgroundWorkObserver.AppendBackgroundWorkloadOutput(ctx, chunk, sessionID)
	}
	if s.eventBus != nil {
		subject := events.BuildBackgroundWorkOutputSubject(sessionID)
		_ = s.eventBus.Publish(ctx, subject, bus.NewEvent(events.BackgroundWorkOutput, "orchestrator", &chunk))
	}
}

func buildObservationFromNormalized(payload *runtime.AgentStreamEventPayload, originTurnID string) *streams.WorkloadRunObservation {
	if payload.Data.Normalized == nil {
		return nil
	}
	bw := payload.Data.Normalized.BackgroundWork()
	if bw == nil {
		return nil
	}
	state := streams.RunStateRunning
	if bw.Ended || (!bw.Detached && isTerminalToolStatus(payload.Data.ToolStatus)) {
		state = streams.RunStateCompleted
		if payload.Data.ToolStatus == agentEventFailed || payload.Data.ToolStatus == agentEventError {
			state = streams.RunStateFailed
		}
	}
	title := payload.Data.ToolTitle
	if title == "" {
		title = payload.Data.ToolName
	}
	workID := bw.WorkID
	if workID == "" {
		workID = payload.Data.ToolCallID
	}
	return &streams.WorkloadRunObservation{
		SessionID:    payload.SessionID,
		WorkID:       workID,
		Kind:         streams.NormalizeWorkloadKind(string(bw.Kind)),
		Title:        title,
		State:        state,
		ParentWorkID: payload.Data.ParentToolCallID,
		SourceCallID: payload.Data.ToolCallID,
		OriginTurnID: originTurnID,
	}
}

func (s *Service) recordBackgroundWorkObservationFromToolPayload(ctx context.Context, payload *runtime.AgentStreamEventPayload) {
	if payload.SessionID == "" {
		return
	}
	var obs streams.WorkloadRunObservation
	switch {
	case payload.Data.BackgroundWork != nil:
		obs = *payload.Data.BackgroundWork
		obs.SessionID = payload.SessionID
		if obs.OriginTurnID == "" {
			obs.OriginTurnID = s.nonCreatingActiveTurnID(ctx, payload.SessionID)
		}
	case payload.Data.Normalized != nil:
		fromNorm := buildObservationFromNormalized(payload, s.nonCreatingActiveTurnID(ctx, payload.SessionID))
		if fromNorm == nil {
			return
		}
		obs = *fromNorm
	default:
		return
	}

	if s.backgroundWorkObserver != nil {
		_ = s.backgroundWorkObserver.RecordBackgroundWorkloadObservation(ctx, obs, payload.TaskID, payload.SessionID)
	}
	if s.eventBus != nil {
		subject := events.BuildBackgroundWorkUpdatedSubject(payload.SessionID)
		_ = s.eventBus.Publish(ctx, subject, bus.NewEvent(events.BackgroundWorkUpdated, "orchestrator", &obs))
	}
}

// BackgroundWorkSettler ends background workloads whose owning agent process
// is gone. It is optional on the observer so recorders without durable storage
// keep working.
type BackgroundWorkSettler interface {
	SettleOpenBackgroundWorkloads(ctx context.Context, sessionID string) ([]streams.WorkloadRunObservation, error)
	ListSessionIDsWithOpenBackgroundWork(ctx context.Context) ([]string, error)
}

var _ BackgroundWorkSettler = (*taskservice.Service)(nil)

func (s *Service) backgroundWorkSettler() BackgroundWorkSettler {
	settler, _ := s.backgroundWorkObserver.(BackgroundWorkSettler)
	return settler
}

func (s *Service) settleOpenBackgroundWork(ctx context.Context, sessionID string) {
	s.settleBackgroundWork(ctx, sessionID, "")
}

func (s *Service) backgroundWorkProcessGone(ctx context.Context, sessionID string, stoppedExecutionID string) bool {
	if s.agentManager == nil {
		return false
	}
	probedExecutionID, _ := s.agentManager.GetExecutionIDForSession(ctx, sessionID)
	running, err := s.probeAgentRunning(ctx, sessionID)
	if err != nil || running {
		return false
	}
	executionID, identityErr := s.agentManager.GetExecutionIDForSession(ctx, sessionID)
	if executionID != probedExecutionID {
		return false
	}
	row, err := s.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil {
		return errors.Is(err, models.ErrExecutorRunningNotFound)
	}
	if row == nil {
		return true
	}
	// Stop evidence belongs to an execution; a retained successor still owns
	// its work when a delayed completion arrives for the previous execution.
	if stoppedExecutionID != "" && stoppedExecutionID == row.AgentExecutionID {
		return true
	}
	if identityErr == nil && executionID != "" && executionID == row.AgentExecutionID {
		return true
	}
	return s.rowLiveness(row) == models.ProcessLivenessDead
}

func (s *Service) settleBackgroundWork(ctx context.Context, sessionID string, stoppedExecutionID string) {
	if sessionID == "" || s.backgroundWorkSettler() == nil {
		return
	}
	release, acquired := s.tryAcquireSessionLifecycleLock(sessionID)
	if !acquired {
		return
	}
	defer release()
	s.settleBackgroundWorkLocked(ctx, sessionID, stoppedExecutionID)
}

// Teardown callbacks run outside launch admission and can wait for its fence.
// State-change hooks may already hold it and use the opportunistic path above.
func (s *Service) settleBackgroundWorkAfterTeardown(ctx context.Context, sessionID, stoppedExecutionID string) {
	if sessionID == "" || s.backgroundWorkSettler() == nil {
		return
	}
	release := s.acquireSessionLifecycleLock(sessionID)
	defer release()
	s.settleBackgroundWorkLocked(ctx, sessionID, stoppedExecutionID)
}

func (s *Service) settleBackgroundWorkLocked(ctx context.Context, sessionID string, stoppedExecutionID string) {
	settler := s.backgroundWorkSettler()
	if settler == nil || sessionID == "" {
		return
	}
	if !s.backgroundWorkProcessGone(ctx, sessionID, stoppedExecutionID) {
		return
	}
	settled, err := settler.SettleOpenBackgroundWorkloads(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to settle open background work",
			zap.String("session_id", sessionID), zap.Error(err))
	}
	if s.eventBus == nil {
		return
	}
	subject := events.BuildBackgroundWorkUpdatedSubject(sessionID)
	for i := range settled {
		obs := settled[i]
		_ = s.eventBus.Publish(ctx, subject, bus.NewEvent(events.BackgroundWorkUpdated, "orchestrator", &obs))
	}
}

// settleOrphanedBackgroundWorkOnStartup ends open workloads of every session
// that has no live agent after a backend restart. A live agent keeps its work
// open because it can still report completion, and a failed probe is treated
// as live.
func (s *Service) settleOrphanedBackgroundWorkOnStartup(ctx context.Context) {
	settler := s.backgroundWorkSettler()
	if settler == nil {
		return
	}
	sessionIDs, err := settler.ListSessionIDsWithOpenBackgroundWork(ctx)
	if err != nil {
		s.logger.Warn("failed to list sessions with open background work", zap.Error(err))
		return
	}
	for _, sessionID := range sessionIDs {
		if s.agentManager != nil {
			running, err := s.probeAgentRunning(ctx, sessionID)
			if err != nil || running {
				continue
			}
		}
		s.settleBackgroundWorkAfterTeardown(ctx, sessionID, "")
	}
}
