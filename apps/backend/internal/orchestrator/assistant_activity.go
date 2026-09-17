package orchestrator

// HasOutstandingSessionWork is a conservative veto for completion, not an
// authorization or an admission signal. Session state and pending native gates
// must still be checked. Unlike the UI activity label it can be empty at rest.
func (s *Service) HasOutstandingSessionWork(sessionID string) bool {
	ta := s.lockTurnActivity(sessionID, false)
	if ta == nil {
		return false
	}
	defer ta.mu.Unlock()
	return ta.promptInFlight || len(ta.background) > 0
}
