package orchestrator

import "sync"

// routeActionOperationLock tracks active waiters so its map entry can be
// removed after the last route action releases the per-session mutex.
type routeActionOperationLock struct {
	mu              sync.Mutex
	refs            int
	routeActionRefs int
}

// acquireRouteActionOperationLock serializes route actions for one session.
// The returned function must be called exactly once after the handler returns.
func (s *Service) acquireRouteActionOperationLock(sessionID string) func() {
	return s.acquireSerializedSessionRouteOperationLock(sessionID, true)
}

// acquireSessionRouteOperationLock serializes session recovery against route
// actions without marking recovery itself as a route action.
func (s *Service) acquireSessionRouteOperationLock(sessionID string) func() {
	return s.acquireSerializedSessionRouteOperationLock(sessionID, false)
}

func (s *Service) acquireSerializedSessionRouteOperationLock(sessionID string, routeAction bool) func() {
	if sessionID == "" {
		return func() {}
	}

	s.routeActionLocksMu.Lock()
	if s.routeActionLocks == nil {
		s.routeActionLocks = make(map[string]*routeActionOperationLock)
	}
	lock, ok := s.routeActionLocks[sessionID]
	if !ok {
		lock = &routeActionOperationLock{}
		s.routeActionLocks[sessionID] = lock
	}
	lock.refs++
	if routeAction {
		lock.routeActionRefs++
	}
	s.routeActionLocksMu.Unlock()

	lock.mu.Lock()
	var released bool
	return func() {
		if released {
			return
		}
		released = true
		lock.mu.Unlock()
		s.routeActionLocksMu.Lock()
		lock.refs--
		if routeAction {
			lock.routeActionRefs--
		}
		if lock.refs == 0 {
			delete(s.routeActionLocks, sessionID)
		}
		s.routeActionLocksMu.Unlock()
	}
}

func (s *Service) isRouteActionInFlight(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	s.routeActionLocksMu.Lock()
	lock := s.routeActionLocks[sessionID]
	inFlight := lock != nil && lock.routeActionRefs > 0
	s.routeActionLocksMu.Unlock()
	return inFlight
}
