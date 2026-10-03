package automation

import (
	"context"
)

func (s *Service) AcknowledgeRetryEvent(ctx context.Context, eventID, leaseToken, runID string, version int64) error {
	if eventID == "" || leaseToken == "" {
		return nil
	}
	if version == 0 {
		version = 1
	}
	return s.store.AcknowledgeRetryEvent(ctx, eventID, leaseToken, runID, version)
}
