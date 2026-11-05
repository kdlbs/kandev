package sandbox

// SetAuditChangeNotifier connects committed audit and trust changes to projection.
func (s *Service) SetAuditChangeNotifier(notify func()) {
	s.auditChanges.mu.Lock()
	s.auditChanges.notify = notify
	s.auditChanges.mu.Unlock()
}

func (s *Service) notifyAuditChange() {
	s.auditChanges.mu.RLock()
	notify := s.auditChanges.notify
	s.auditChanges.mu.RUnlock()
	if notify != nil {
		notify()
	}
}
