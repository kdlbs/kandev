package sandbox

import (
	"context"
	"database/sql"
	"sync"
)

type approvalChangeNotifier struct {
	mu     sync.RWMutex
	notify func(context.Context, string, string)
}

// SetApprovalChangeNotifier connects committed approval changes to browser invalidation.
func (s *Service) SetApprovalChangeNotifier(notify func(context.Context, string, string)) {
	s.approvalChanges.mu.Lock()
	s.approvalChanges.notify = notify
	s.approvalChanges.mu.Unlock()
}

func (s *Service) notifyApprovalChange(ctx context.Context, orgID, attemptID string) {
	s.approvalChanges.mu.RLock()
	notify := s.approvalChanges.notify
	s.approvalChanges.mu.RUnlock()
	if notify != nil {
		notify(ctx, orgID, attemptID)
	}
}

func (s *Service) notifyApprovalResult(ctx context.Context, orgID, attemptID string, result sql.Result) error {
	count, err := result.RowsAffected()
	if err == nil && count > 0 {
		s.notifyApprovalChange(ctx, orgID, attemptID)
	}
	return err
}
