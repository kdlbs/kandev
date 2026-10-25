package sandbox

import "sync"

type attemptChangeNotifier struct {
	mu     sync.RWMutex
	notify func()
}

// SetAttemptChangeNotifier connects committed lifecycle changes to the core observer.
func (s *Service) SetAttemptChangeNotifier(notify func()) {
	s.attemptChanges.mu.Lock()
	s.attemptChanges.notify = notify
	s.attemptChanges.mu.Unlock()
}

func (s *Service) notifyAttemptChange() {
	s.attemptChanges.mu.RLock()
	notify := s.attemptChanges.notify
	s.attemptChanges.mu.RUnlock()
	if notify != nil {
		notify()
	}
}
