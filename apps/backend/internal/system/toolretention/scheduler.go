package toolretention

import (
	"context"
	"errors"
	"github.com/jmoiron/sqlx"
	"time"
)

func (s *Service) Start(ctx context.Context) {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if !s.supported() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return
	}
	worker, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.wg.Add(1)
	go s.loop(worker)
}

func (s *Service) Stop() {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
		s.wg.Wait()
	}
}

func (s *Service) loop(ctx context.Context) {
	defer s.wg.Done()
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()
	first := true
	var burst time.Time
	failures := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
		case <-timer.C:
		}
		if ctx.Err() != nil {
			return
		}
		if first {
			if err := s.reconcile(ctx); err != nil {
				timer.Reset(time.Minute)
				continue
			}
			first = false
		}
		err := s.tick(ctx)
		failures = nextFailureCount(failures, err)
		if failures >= 5 {
			s.failActive(ctx)
			failures = 0
		}
		timer.Reset(s.nextDelay(ctx, err, &burst))
	}
}

func nextFailureCount(previous int, err error) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, errMaintenanceBusy) {
		return previous
	}
	return previous + 1
}

func (s *Service) nextDelay(ctx context.Context, workErr error, burst *time.Time) time.Duration {
	status, err := s.Get(ctx)
	if err != nil || workErr != nil || status.Operation == nil || status.Operation.State != stateRunning {
		*burst = time.Time{}
		return time.Minute
	}
	now := s.opts.Now()
	if burst.IsZero() {
		*burst = now
	}
	if now.Sub(*burst) >= 30*time.Second {
		*burst = time.Time{}
		return time.Minute
	}
	return 100 * time.Millisecond
}

func (s *Service) tick(ctx context.Context) error {
	r, err := readRecord(ctx, s.pool.Reader())
	if err != nil {
		return err
	}
	work, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.activeCancel = cancel
	s.activeRevision = r.Policy.Revision
	s.activeID = ""
	if r.Operation != nil {
		s.activeID = r.Operation.ID
	}
	s.mu.Unlock()
	defer func() { cancel(); s.mu.Lock(); s.activeCancel = nil; s.activeID = ""; s.mu.Unlock() }()
	switch {
	case r.Preparation.State == statePending:
		err = s.stepPreparation(work)
	case r.Preparation.State == stateRunning:
		// Ticks are serialized; a running preparation here has no backup worker.
		err = s.recoverPreparation(work, r)
	case r.Operation != nil && r.Operation.State == stateRunning:
		err = s.stepScan(work)
	default:
		err = s.stepScheduled(work)
	}
	if err == nil && s.opts.Report != nil {
		status, getErr := s.Get(ctx)
		if getErr == nil && status.Operation != nil {
			s.opts.Report(ctx, status.Operation)
		}
	}
	return err
}

func (s *Service) stepScheduled(ctx context.Context) error {
	r, err := readRecord(ctx, s.pool.Reader())
	if err != nil {
		return err
	}
	if !approved(&r) || operationBusy(&r) || r.NextDueAt == nil || s.opts.Now().Before(*r.NextDueAt) {
		return nil
	}
	_, err = s.change(ctx, func(r *record, tx *sqlx.Tx) error {
		if !approved(r) || operationBusy(r) || r.NextDueAt == nil || s.opts.Now().Before(*r.NextDueAt) {
			return nil
		}
		return startOperation(ctx, tx, r, kindCleanup, r.Policy.Age, s.opts.Now())
	})
	return err
}

func (s *Service) reconcile(ctx context.Context) error {
	r, err := readRecord(ctx, s.pool.Reader())
	if err != nil {
		return err
	}
	if r.Preparation.State != stateRunning {
		return nil
	}
	return s.recoverPreparation(ctx, r)
}

func (s *Service) failActive(ctx context.Context) {
	_, _ = s.change(ctx, func(r *record, tx *sqlx.Tx) error {
		if r.Preparation.State == stateRunning {
			s.failPreparation(r)
			return nil
		}
		if r.Operation == nil || r.Operation.State != stateRunning {
			return nil
		}
		state := stateFailed
		if r.Operation.Scanned > 0 {
			state = statePartial
		}
		finishOperation(r, state, "scan_failed", s.opts.Now())
		if r.Policy.Enabled {
			next := s.opts.Now().Add(24 * time.Hour)
			r.NextDueAt = &next
		}
		return nil
	})
}
