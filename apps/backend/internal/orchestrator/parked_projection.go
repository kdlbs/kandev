package orchestrator

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// probe result strings — match agentctl/server/process probe constants.
const (
	probeResultLive    = "live"
	probeResultSettled = "settled"
)

// computeParked returns true when all three projection terms are satisfied.
func (ps *sessionParkedState) computeParked(sessionState models.TaskSessionState) bool {
	return ps.observedDetached &&
		ps.lastSample == probeResultLive &&
		sessionState == models.TaskSessionStateWaitingForInput
}

// onSessionParkedHook fires synchronously after a session transitions to
// WAITING_FOR_INPUT. It runs the first probe, updates the parked projection,
// and starts the background sampler goroutine if the session is parked.
func (s *Service) onSessionParkedHook(ctx context.Context, taskID, sessionID string) {
	if s.backgroundProbe == nil {
		return
	}
	sample := s.runProbe(ctx, sessionID)

	s.parkedStatesMu.Lock()
	ps := s.parkedStates[sessionID]
	if ps == nil {
		s.parkedStatesMu.Unlock()
		return
	}
	ps.lastSample = sample
	oldParked := ps.parked
	newParked := ps.computeParked(models.TaskSessionStateWaitingForInput)
	var startSampler bool
	var samplerCtx context.Context
	if newParked != oldParked {
		ps.parked = newParked
		ps.revision++
	}
	if newParked {
		ps.stopSampler()
		samplerCtx, ps.samplerCancel = context.WithCancel(context.Background())
		startSampler = true
	} else {
		ps.stopSampler()
	}
	revision := ps.revision
	s.parkedStatesMu.Unlock()

	if newParked != oldParked {
		s.publishParkedChanged(taskID, sessionID, newParked, s.parkedEpoch, revision)
		s.updateTaskParkedState(ctx, taskID, sessionID, newParked)
	}

	if startSampler {
		go s.runParkingSampler(samplerCtx, taskID, sessionID)
	}
}

// runProbe calls the background probe with the configured budget.
func (s *Service) runProbe(parent context.Context, sessionID string) string {
	probeCtx, cancel := context.WithTimeout(parent, s.config.BackgroundProbeBudget)
	defer cancel()
	result, _ := s.backgroundProbe.ProbeBackgroundWorkloads(probeCtx, sessionID)
	if result == "" {
		result = "unknown"
	}
	return result
}

// sampleAndPublishParked runs one probe, updates the session parked state,
// and publishes if the parked value changed. Returns true if the sampler
// should continue running.
func (s *Service) sampleAndPublishParked(ctx context.Context, taskID, sessionID string) bool {
	if s.backgroundProbe == nil {
		return false
	}
	sample := s.runProbe(ctx, sessionID)

	s.parkedStatesMu.Lock()
	ps := s.parkedStates[sessionID]
	if ps == nil {
		s.parkedStatesMu.Unlock()
		return false
	}
	ps.lastSample = sample
	oldParked := ps.parked
	newParked := ps.computeParked(models.TaskSessionStateWaitingForInput)
	if newParked != oldParked {
		ps.parked = newParked
		ps.revision++
	}
	if !newParked {
		ps.stopSampler()
	}
	revision := ps.revision
	s.parkedStatesMu.Unlock()

	if newParked != oldParked {
		s.publishParkedChanged(taskID, sessionID, newParked, s.parkedEpoch, revision)
		s.updateTaskParkedState(ctx, taskID, sessionID, newParked)
	}

	return newParked
}

// runParkingSampler ticks at BackgroundSampleInterval until context is done
// or the session is no longer parked.
func (s *Service) runParkingSampler(ctx context.Context, taskID, sessionID string) {
	interval := s.config.BackgroundSampleInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			keepRunning := s.sampleAndPublishParked(ctx, taskID, sessionID)
			if !keepRunning {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// resetParkedSampler cancels any running sampler and clears lastSample
// for the given session. Called when turn_started arrives.
func (s *Service) resetParkedSampler(sessionID string) {
	s.parkedStatesMu.Lock()
	defer s.parkedStatesMu.Unlock()
	ps := s.parkedStates[sessionID]
	if ps == nil {
		return
	}
	ps.stopSampler()
	ps.lastSample = ""
}

// publishParkedChanged sends a TaskSessionParkedChanged event on the bus.
func (s *Service) publishParkedChanged(taskID, sessionID string, parked bool, epoch int64, revision uint64) {
	if s.eventBus == nil || sessionID == "" {
		return
	}
	payload := map[string]interface{}{
		"task_id":                   taskID,
		"session_id":                sessionID,
		"parked_on_background_work": parked,
		"parked_epoch":              epoch,
		"parked_revision":           revision,
	}
	ev := bus.NewEvent(events.TaskSessionParkedChanged, "orchestrator", payload)
	if err := s.eventBus.Publish(context.Background(), events.TaskSessionParkedChanged, ev); err != nil {
		if s.logger != nil {
			s.logger.Warn("failed to publish parked state",
				zap.String("session_id", sessionID),
				zap.Bool("parked", parked),
				zap.Uint64("revision", revision),
				zap.Error(err))
		}
	}
}

// updateTaskParkedState updates the task-level OR and publishes a task update
// if the OR changed.
func (s *Service) updateTaskParkedState(ctx context.Context, taskID, sessionID string, parked bool) {
	if taskID == "" {
		return
	}
	ts := s.getOrCreateTaskParkedState(taskID)
	ts.mu.Lock()
	if ts.members == nil {
		ts.members = make(map[string]bool)
	}
	ts.members[sessionID] = parked
	newTaskParked := anyParkedMember(ts.members)
	changed := newTaskParked != ts.parked
	if changed {
		ts.parked = newTaskParked
		ts.revision++
	}
	ts.mu.Unlock()

	if changed {
		s.publishTaskParkedChanged(ctx, taskID)
	}
}

// anyParkedMember returns true if any member value is true.
func anyParkedMember(members map[string]bool) bool {
	for _, v := range members {
		if v {
			return true
		}
	}
	return false
}

// publishTaskParkedChanged triggers a task.updated event so the task DTO
// carries the refreshed task-level parked projection to clients.
func (s *Service) publishTaskParkedChanged(ctx context.Context, taskID string) {
	s.publishTaskActivityIfChanged(ctx, taskID)
}

// ParkedProjectionSnapshot implements dto.ParkedProjectionProvider.
func (s *Service) ParkedProjectionSnapshot(sessionID string) (bool, int64, uint64) {
	s.parkedStatesMu.RLock()
	ps := s.parkedStates[sessionID]
	if ps == nil {
		s.parkedStatesMu.RUnlock()
		return false, s.parkedEpoch, 0
	}
	parked, revision := ps.parked, ps.revision
	s.parkedStatesMu.RUnlock()
	return parked, s.parkedEpoch, revision
}

// TaskParkedProjectionSnapshot implements dto.TaskParkedProjectionProvider.
func (s *Service) TaskParkedProjectionSnapshot(taskID string) (bool, uint64) {
	s.taskParkedStatesMu.Lock()
	ts := s.taskParkedStates[taskID]
	s.taskParkedStatesMu.Unlock()
	if ts == nil {
		return false, 0
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.parked, ts.revision
}
