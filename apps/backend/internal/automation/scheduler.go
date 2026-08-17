package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	schedulercron "github.com/kandev/kandev/internal/scheduler/cron"
)

const defaultSchedulerCheckInterval = 30 * time.Second

// CronScheduler evaluates scheduled triggers on a timer.
// It checks all enabled scheduled triggers and fires those whose cron expression
// indicates they should run based on time elapsed since last evaluation.
type CronScheduler struct {
	svc    *Service
	logger *logger.Logger

	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started bool
}

// NewCronScheduler creates a new cron scheduler.
func NewCronScheduler(svc *Service, log *logger.Logger) *CronScheduler {
	return &CronScheduler{
		svc:    svc,
		logger: log,
	}
}

// Start begins the scheduler loop.
func (cs *CronScheduler) Start(ctx context.Context) {
	if cs.started {
		return
	}
	cs.started = true
	ctx, cs.cancel = context.WithCancel(ctx)

	cs.wg.Add(1)
	go cs.loop(ctx)

	cs.logger.Info("automation cron scheduler started")
}

// Stop cancels the scheduler and waits for it to finish.
func (cs *CronScheduler) Stop() {
	if !cs.started {
		return
	}
	if cs.cancel != nil {
		cs.cancel()
	}
	cs.wg.Wait()
	cs.started = false
	cs.logger.Info("automation cron scheduler stopped")
}

func (cs *CronScheduler) loop(ctx context.Context) {
	defer cs.wg.Done()

	// Initial check on startup.
	cs.evaluate(ctx)

	ticker := time.NewTicker(defaultSchedulerCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cs.evaluate(ctx)
		}
	}
}

// evaluate checks all enabled scheduled triggers and fires those that are due.
func (cs *CronScheduler) evaluate(ctx context.Context) {
	triggers, err := cs.svc.Store().ListEnabledTriggersByType(ctx, TriggerTypeScheduled)
	if err != nil {
		cs.logger.Error("failed to list scheduled triggers", zap.Error(err))
		return
	}
	if len(triggers) == 0 {
		return
	}

	now := time.Now()
	for i := range triggers {
		t := &triggers[i]
		if cs.shouldFire(t, now) {
			cs.fire(ctx, t, now)
		}
	}
}

// shouldFire determines if a scheduled trigger is due based on its cron
// expression. It computes the next fire time after the trigger's anchor (the
// last time it was evaluated, or its creation time on first evaluation) in the
// trigger's configured timezone, and reports whether that time has arrived.
//
// This is a true wall-clock schedule, not an interval approximation: pinned
// expressions ("0 9 * * *"), weekday ranges, and day-of-month schedules all
// resolve to concrete instants, and the configured timezone (including its DST
// transitions) is honored. A schedule missed while the scheduler was down
// fires once on the next tick rather than once per missed occurrence, matching
// the per-minute dedup key in fire().
func (cs *CronScheduler) shouldFire(t *AutomationTrigger, now time.Time) bool {
	var cfg ScheduledTriggerConfig
	if err := json.Unmarshal(t.Config, &cfg); err != nil {
		cs.logger.Debug("invalid scheduled trigger config",
			zap.String("trigger_id", t.ID), zap.Error(err))
		return false
	}
	if cfg.CronExpression == "" {
		return false
	}

	// Anchor the next-fire computation to the last evaluation, falling back to
	// the trigger's creation time so a pinned schedule created mid-day doesn't
	// fire immediately — its first fire is the next scheduled occurrence after
	// creation.
	anchor := t.CreatedAt
	if t.LastEvaluatedAt != nil {
		anchor = *t.LastEvaluatedAt
	}

	next, err := schedulercron.NextScheduleTime(cfg.CronExpression, cfg.Timezone, anchor)
	if err != nil {
		cs.logger.Debug("unparseable cron expression",
			zap.String("trigger_id", t.ID),
			zap.String("expression", cfg.CronExpression),
			zap.Error(err))
		return false
	}
	return !next.After(now)
}

func (cs *CronScheduler) fire(ctx context.Context, t *AutomationTrigger, now time.Time) {
	data, _ := json.Marshal(map[string]string{
		triggerDataSourceKey: string(TriggerTypeScheduled),
		"timestamp":          now.Format(time.RFC3339),
	})
	dedupKey := fmt.Sprintf("scheduled:%s:%d", t.ID, now.Unix()/60) // Dedup by minute

	if _, err := cs.svc.FireTrigger(ctx, t.AutomationID, t.ID, TriggerTypeScheduled, data, dedupKey); err != nil {
		cs.logger.Error("failed to fire scheduled trigger",
			zap.String("trigger_id", t.ID),
			zap.String("automation_id", t.AutomationID),
			zap.Error(err))
	}
}

// nextCronFire computes the next time the given cron expression fires strictly
// after `after`, interpreted in the given timezone. An empty timezone means
// UTC, so scheduling is deterministic regardless of the host's local clock.
//
// Wall-clock schedules honor the timezone's DST transitions (a "0 9 * * *"
// daily schedule always fires at 09:00 local, whether that is 13:00 or 14:00
// UTC). The @every interval shorthand is timezone-independent by nature.
func nextCronFire(expr, timezone string, after time.Time) (time.Time, error) {
	return schedulercron.NextScheduleTime(expr, timezone, after)
}
