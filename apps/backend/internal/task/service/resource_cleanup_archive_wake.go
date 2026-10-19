package service

import (
	"context"
	"fmt"
	"time"
)

type waitingArchiveReclaimWaker interface {
	WakeWaitingArchiveReclaimJobs(context.Context, time.Time, string, int) (string, bool, error)
}

func (s *Service) wakeWaitingArchiveReclaims(ctx context.Context, cutoff time.Time) error {
	waker, ok := s.resourceCleanups.(waitingArchiveReclaimWaker)
	if !ok {
		return nil
	}
	s.archiveReclaimBackfillMu.Lock()
	defer s.archiveReclaimBackfillMu.Unlock()
	if s.archiveReclaimWakeDone {
		return nil
	}
	if s.archiveReclaimWakeCutoff.IsZero() {
		s.archiveReclaimWakeCutoff = cutoff
	}
	cursor, done, err := waker.WakeWaitingArchiveReclaimJobs(ctx, s.archiveReclaimWakeCutoff, s.archiveReclaimWakeAfterID, archiveReclaimBackfillBatchSize)
	if err != nil {
		return fmt.Errorf("wake waiting archive reclaim jobs: %w", err)
	}
	s.archiveReclaimWakeAfterID = cursor
	s.archiveReclaimWakeDone = done
	return nil
}
