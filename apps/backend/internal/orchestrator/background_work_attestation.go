package orchestrator

// setObservedDetachedLaunch records that a registered launch recogniser
// attested a Detached=true background-shell launch during the turn that is
// currently settling for sessionID (D3). It is read by the parked-projection
// computation at turn-settle (spec: docs/specs/disambiguate-waiting/spec.md,
// D2/D3) and cleared on the next turn's boundary — see
// clearObservedDetachedLaunch.
func (s *Service) setObservedDetachedLaunch(sessionID string) {
	if sessionID == "" {
		return
	}
	s.observedDetachedMu.Lock()
	defer s.observedDetachedMu.Unlock()
	if s.observedDetached == nil {
		s.observedDetached = make(map[string]bool)
	}
	s.observedDetached[sessionID] = true
}

// clearObservedDetachedLaunch clears sessionID's attestation. D3 names one
// turn boundary for the whole feature — agentctl's session/prompt dispatch,
// including the synthetic one a ScheduleWakeup self-resume issues — and
// requires clearing on that same boundary rather than on the backend's own
// prompt-admission path, which a synthetic dispatch never reaches.
func (s *Service) clearObservedDetachedLaunch(sessionID string) {
	if sessionID == "" {
		return
	}
	s.observedDetachedMu.Lock()
	defer s.observedDetachedMu.Unlock()
	delete(s.observedDetached, sessionID)
}

// ObservedDetachedLaunch reports whether sessionID has an outstanding
// Detached=true background-shell attestation for the turn that settled.
func (s *Service) ObservedDetachedLaunch(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	s.observedDetachedMu.Lock()
	defer s.observedDetachedMu.Unlock()
	return s.observedDetached[sessionID]
}
