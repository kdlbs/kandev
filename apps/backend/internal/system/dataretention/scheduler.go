package dataretention

import (
	"context"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
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
	// failedID is the operation the consecutive failures belong to. A failure
	// on a different operation starts a new count, so retry exhaustion is only
	// ever recorded against the operation that actually failed.
	failedID := ""
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
		id, err := s.tick(ctx)
		if id != failedID {
			failures, failedID = 0, id
		}
		failures = nextFailureCount(failures, err)
		if failures >= 5 {
			s.failActive(ctx, id, err)
			failures = 0
		}
		timer.Reset(s.nextDelay(ctx, err, &burst))
	}
}

func nextFailureCount(previous int, err error) int {
	if err == nil || errors.Is(err, errMaintenanceBusy) {
		return 0
	}
	return previous + 1
}

// nextDelay keeps a running pass moving in short continuations for at most half
// a minute of wall clock, then returns to a one-minute idle tick.
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

// tick runs one unit of work and reports the identity of the operation it
// worked on, so the caller can bind failures to that operation.
func (s *Service) tick(ctx context.Context) (id string, err error) {
	r, err := readRecord(ctx, s.pool.Reader())
	if err != nil {
		return "", err
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
	defer func() {
		cancel()
		s.mu.Lock()
		// claimActive may have replaced the identity with the operation this
		// tick started, which is the one any failure belongs to.
		id = s.activeID
		s.activeCancel = nil
		s.activeID = ""
		s.mu.Unlock()
	}()
	switch {
	case r.Preparation.State == statePending:
		err = s.stepPreparation(work)
	case r.Preparation.State == stateRunning:
		// Ticks are serialized, so a running preparation here has no backup worker.
		err = s.recoverPreparation(work, r)
	case r.Operation != nil && r.Operation.State == stateRunning:
		err = s.stepScan(work, r)
	default:
		err = s.stepScheduled(work)
	}
	if err == nil && s.opts.Report != nil {
		status, getErr := s.Get(ctx)
		if getErr == nil && status.Operation != nil {
			s.opts.Report(ctx, status.Operation)
		}
	}
	return "", err
}

// stepScheduled starts the daily pass once the approved revision is due. Both
// targets share one pass so the operator sees a single dated result.
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
		now := s.opts.Now()
		if err := s.startOperation(ctx, tx, r, kindCleanup, r.Policy.ArchivedAge.Cutoff(now), r.Policy.CleanupAge.Cutoff(now), now); err != nil {
			return err
		}
		r.NextDueAt = nil
		return nil
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

// failActive records a terminal failure for work that exhausted its retries. A
// pass that already reduced something is partial, not failed, so the effect
// stays visible. Only the operation that failed is finished: a cancel or a new
// operation committed since then is left alone.
func (s *Service) failActive(ctx context.Context, id string, cause error) {
	_, _ = s.change(ctx, func(r *record, tx *sqlx.Tx) error {
		if id == "" || currentID(r) != id {
			return nil
		}
		if r.Preparation.State == stateRunning {
			s.failPreparation(r)
			return nil
		}
		if r.Operation == nil || r.Operation.State != stateRunning {
			return nil
		}
		state := stateFailed
		if r.Operation.Messages.Reduced > 0 || r.Operation.CleanupJobs.Reduced > 0 {
			state = statePartial
		}
		code := "scan_failed"
		if errors.Is(cause, errBackupVerificationFailed) {
			code = errBackupVerificationFailed.Error()
		}
		finishOperation(r, state, code, s.opts.Now())
		scheduleNextPass(r, s.opts.Now())
		return nil
	})
}
