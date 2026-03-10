package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
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

// shouldFire determines if a scheduled trigger is due based on its cron expression.
// Uses a simplified interval-based approach: parses the cron expression to derive
// an interval and checks if enough time has passed since last evaluation.
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

	interval, err := parseCronInterval(cfg.CronExpression)
	if err != nil {
		cs.logger.Debug("unparseable cron expression",
			zap.String("trigger_id", t.ID),
			zap.String("expression", cfg.CronExpression),
			zap.Error(err))
		return false
	}

	if t.LastEvaluatedAt == nil {
		return true
	}
	return now.Sub(*t.LastEvaluatedAt) >= interval
}

func (cs *CronScheduler) fire(ctx context.Context, t *AutomationTrigger, now time.Time) {
	data, _ := json.Marshal(map[string]string{
		"source":    "scheduled",
		"timestamp": now.Format(time.RFC3339),
	})
	dedupKey := fmt.Sprintf("scheduled:%s:%d", t.ID, now.Unix()/60) // Dedup by minute

	if err := cs.svc.FireTrigger(ctx, t.AutomationID, t.ID, TriggerTypeScheduled, data, dedupKey); err != nil {
		cs.logger.Error("failed to fire scheduled trigger",
			zap.String("trigger_id", t.ID),
			zap.String("automation_id", t.AutomationID),
			zap.Error(err))
	}
}

// parseCronInterval converts common cron expressions to a duration.
// Supports standard 5-field cron and shorthand like "@every 5m".
func parseCronInterval(expr string) (time.Duration, error) {
	if d, ok := parseCronShorthand(expr); ok {
		return d, nil
	}
	return parseCronFields(expr)
}

func parseCronShorthand(expr string) (time.Duration, bool) {
	// Handle @every shorthand.
	if len(expr) > 7 && expr[:6] == "@every" {
		d, err := time.ParseDuration(expr[7:])
		if err == nil {
			return d, true
		}
	}
	// Common presets.
	switch expr {
	case "@hourly", "0 * * * *":
		return time.Hour, true
	case "@daily", "0 0 * * *":
		return 24 * time.Hour, true
	case "@weekly", "0 0 * * 0":
		return 7 * 24 * time.Hour, true
	}
	return 0, false
}

func parseCronFields(expr string) (time.Duration, error) {
	fields := splitFields(expr)
	if len(fields) < 2 {
		return 0, fmt.Errorf("invalid cron expression: %s", expr)
	}

	minuteInterval := parseFieldInterval(fields[0], 60)
	hourInterval := parseFieldInterval(fields[1], 24)

	if minuteInterval > 0 && minuteInterval < 60 {
		return time.Duration(minuteInterval) * time.Minute, nil
	}
	if hourInterval > 0 && hourInterval < 24 {
		return time.Duration(hourInterval) * time.Hour, nil
	}
	return time.Hour, nil
}

func splitFields(expr string) []string {
	var fields []string
	field := ""
	for _, c := range expr {
		if c == ' ' || c == '\t' {
			if field != "" {
				fields = append(fields, field)
				field = ""
			}
		} else {
			field += string(c)
		}
	}
	if field != "" {
		fields = append(fields, field)
	}
	return fields
}

// parseFieldInterval extracts the step value from a cron field (e.g., "*/5" → 5).
func parseFieldInterval(field string, max int) int {
	for i, c := range field {
		if c == '/' {
			val := 0
			for _, d := range field[i+1:] {
				if d >= '0' && d <= '9' {
					val = val*10 + int(d-'0')
				}
			}
			if val > 0 && val <= max {
				return val
			}
		}
	}
	return 0
}
