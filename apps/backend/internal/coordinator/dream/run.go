package dream

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/coordinator/replay"
)

// start launches the dream's goroutine. The row is already stored; when the
// scheduler is closed the row is left to lease expiry.
func (s *Scheduler) start(parent context.Context, c *coordinator.Coordinator, p plan) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.WithoutCancel(parent))
	s.cancels[p.dream.ID] = cancel
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		defer func() {
			cancel()
			s.mu.Lock()
			delete(s.cancels, p.dream.ID)
			s.mu.Unlock()
		}()
		s.run(ctx, c, p)
	}()
}

func (s *Scheduler) run(ctx context.Context, c *coordinator.Coordinator, p plan) {
	stopRefresh := s.keepLease(ctx, c.ID, p.dream.ID)
	defer stopRefresh()
	reason, d, items := s.episode(ctx, c, p)
	if reason != "" {
		s.fail(p.dream.ID, reason, ctx)
		return
	}
	now := s.d.Clock()
	if ok, err := s.d.Store.FinishDream(context.WithoutCancel(ctx), d, items, now); err != nil {
		s.d.Log.Warn("dream finish", zap.String("dream_id", d.ID), zap.Error(err))
		s.fail(d.ID, ReasonStoreError, ctx)
	} else if !ok {
		s.d.Log.Info("dream finish lost the lease", zap.String("dream_id", d.ID))
	}
}

// fail stores a failure on a context that outlives the dream's own. A fallback
// write that fails leaves the row to lease expiry.
func (s *Scheduler) fail(id, reason string, ctx context.Context) {
	if cause := context.Cause(ctx); reason == ReasonRunError && errors.Is(cause, context.DeadlineExceeded) {
		reason = ReasonTimeout
	}
	if _, err := s.d.Store.FailDream(context.WithoutCancel(ctx), id, reason, s.d.Clock()); err != nil {
		s.d.Log.Warn("dream fail write", zap.String("dream_id", id), zap.Error(err))
	}
}

// keepLease refreshes the row each interval and re-checks the conditions a
// running dream must keep; a breached condition fails the row and cancels.
func (s *Scheduler) keepLease(ctx context.Context, coordinatorID, dreamID string) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(s.d.Refresh)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if !s.refresh(ctx, coordinatorID, dreamID) {
					return
				}
			}
		}
	}()
	return func() { cancel(); <-done }
}

func (s *Scheduler) refresh(ctx context.Context, coordinatorID, dreamID string) bool {
	ok, err := s.d.Store.RefreshDream(ctx, dreamID, s.d.Clock())
	if err != nil {
		return true
	}
	if !ok {
		s.Cancel(dreamID)
		return false
	}
	if reason := s.breach(ctx, coordinatorID); reason != "" {
		s.fail(dreamID, reason, ctx)
		s.Cancel(dreamID)
		return false
	}
	return true
}

func (s *Scheduler) breach(ctx context.Context, coordinatorID string) string {
	c, err := s.d.Store.GetCoordinatorByID(ctx, coordinatorID)
	if err != nil {
		return ""
	}
	switch {
	case c.PausedAt != nil:
		return ReasonPaused
	case !c.AutonomyEnabled:
		return ReasonAutonomyOff
	case !s.d.Conditions.ContainmentOK(ctx, c):
		return ReasonContainment
	}
	if measurable, atCeiling := s.d.Conditions.Spend(ctx, c, s.d.Clock()); !measurable || atCeiling {
		return ReasonCeiling
	}
	return ""
}

// episode runs the conversation and builds the finished row. A non-empty
// reason means the dream failed.
func (s *Scheduler) episode(ctx context.Context, c *coordinator.Coordinator, p plan) (string, coordinator.Dream, []coordinator.DreamItem) {
	d := p.dream
	st := s.d.Store
	taskID, err := s.d.Episode.CreateTask(ctx, c)
	if err != nil {
		s.archiveUnbound(ctx, taskID)
		return ReasonRunError, d, nil
	}
	bound, err := st.SetDreamEpisodeTask(context.WithoutCancel(ctx), d.ID, taskID)
	if err != nil {
		s.archiveUnbound(ctx, taskID)
		return ReasonStoreError, d, nil
	}
	if !bound {
		s.archiveUnbound(ctx, taskID)
		return ReasonLeaseLost, d, nil
	}
	sessionID, err := s.d.Episode.CreateSession(ctx, taskID)
	if err != nil {
		return ReasonRunError, d, nil
	}
	if linked, err := st.SetDreamEpisodeSession(context.WithoutCancel(ctx), d.ID, sessionID); err != nil {
		return ReasonStoreError, d, nil
	} else if !linked {
		return ReasonLeaseLost, d, nil
	}
	promptCtx, stop := context.WithTimeout(ctx, s.d.Bound)
	raw, err := s.d.Episode.Prompt(promptCtx, taskID, sessionID, Frame+p.evidence.Message)
	timedOut := errors.Is(context.Cause(promptCtx), context.DeadlineExceeded)
	stop()
	if err != nil {
		if timedOut {
			return ReasonTimeout, d, nil
		}
		return ReasonRunError, d, nil
	}
	ans, err := ParseAnswer(raw)
	if err != nil {
		return ReasonBadOutput, d, nil
	}
	items, replayErr := s.report(ctx, c, p, ans)
	if replayErr != "" {
		return replayErr, d, nil
	}
	d.Considered = ans.Considered
	d.Status = statusOf(items)
	return "", d, items
}

// archiveUnbound archives a task the dream row does not name, so the tick
// cleanup could never find it.
func (s *Scheduler) archiveUnbound(ctx context.Context, taskID string) {
	if taskID == "" {
		return
	}
	if err := s.d.Episode.Archive(context.WithoutCancel(ctx), taskID); err != nil {
		s.d.Log.Warn("dream episode: archive unbound task", zap.String("task_id", taskID), zap.Error(err))
	}
}

func statusOf(items []coordinator.DreamItem) string {
	if len(items) == 0 {
		return coordinator.DreamOK
	}
	for _, it := range items {
		if it.Gate != GatePass || it.Verdict == "" || it.Verdict == replay.VerdictUnmeasured {
			return coordinator.DreamPartial
		}
	}
	return coordinator.DreamClean
}
