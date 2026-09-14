package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
)

// isAC1SessionState reports whether state counts toward the session ceiling's
// population: task_sessions in STARTING or RUNNING (AC-1).
func isAC1SessionState(state models.TaskSessionState) bool {
	return state == models.TaskSessionStateStarting || state == models.TaskSessionStateRunning
}

// confirmCeilingReservation is AC-52's acceptance edge: the launch reached its
// first AC-1 row, so the in-flight reservation taken at admission is no
// longer needed to count it — the row does that from here on. It is silent
// and does not signal the retry driver, because confirming a reservation
// does not free capacity.
func (s *Service) confirmCeilingReservation(sessionID string) {
	if s.sessionCeiling == nil || sessionID == "" {
		return
	}
	s.sessionCeiling.release(sessionID)
}

// releaseCeilingReservation is AC-15/AC-51's release edge: a session left the
// AC-1 population, or a launch failed before ever reaching it, so its
// reservation is dropped and the retry driver is signalled to fill the freed
// slot. Releasing an id that holds no reservation is a defined no-op (AC-31),
// so callers are never required to know whether one was actually held.
func (s *Service) releaseCeilingReservation(sessionID string) {
	if s.sessionCeiling == nil || sessionID == "" {
		return
	}
	s.sessionCeiling.release(sessionID)
	s.signalCeilingSweep()
}

// ReleaseCeilingReservation is the exported form of releaseCeilingReservation,
// satisfying the narrow interfaces other packages (mcp/handlers, task/service)
// use to release a reservation for a write they make directly against the
// repository, bypassing this package's own persistence funnels (AC-51a).
func (s *Service) ReleaseCeilingReservation(sessionID string) {
	s.releaseCeilingReservation(sessionID)
}

// releaseCeilingIfLeftPopulation is the shared guard the two persistence
// funnels (persistTaskSessionState, persistStrictTaskSessionState) and the
// enumerated bypass writers (AC-51a) apply before releasing: a release fires
// only for a transition that actually left the AC-1 population, never for a
// transition that stayed inside it or moved into it (AC-51c).
func (s *Service) releaseCeilingIfLeftPopulation(sessionID string, priorState, nextState models.TaskSessionState) {
	if !isAC1SessionState(priorState) || isAC1SessionState(nextState) {
		return
	}
	s.releaseCeilingReservation(sessionID)
}

// IsSessionCeilingBacked implements executor.CeilingBackingChecker (AC-41):
// the single permitted route by which Executor's observation-only bypass
// detector (AC-41a) reads whether a session is backed by a reservation or a
// counted AC-1 row. It is consulted for logging only and never gates a
// launch.
func (s *Service) IsSessionCeilingBacked(ctx context.Context, sessionID string) (bool, error) {
	return s.sessionCeiling.isSessionCeilingBacked(ctx, sessionID)
}
