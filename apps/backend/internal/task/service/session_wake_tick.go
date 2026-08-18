package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/scheduler/cron"
	"github.com/kandev/kandev/internal/task/models"
)

// TickSessionWakes claims each due schedule before delivery, preventing two
// backend processes from emitting the same wake. A delayed tick advances to
// the first future occurrence and delivers only once.
func (s *Service) TickSessionWakes(ctx context.Context, deliverer interface {
	DeliverSessionWake(context.Context, string, string, string, string) (string, error)
}, now time.Time) error {
	if s.sessionWakes == nil || deliverer == nil {
		return nil
	}
	now = now.UTC()
	wakes, err := s.sessionWakes.ListDueTaskSessionWakes(ctx, now, 100)
	if err != nil {
		return fmt.Errorf("list due session wakes: %w", err)
	}
	for _, wake := range wakes {
		next, err := nextFutureWakeTime(wake, now)
		if err != nil {
			_ = s.sessionWakes.RecordTaskSessionWakeDelivery(ctx, wake.ID, models.SessionWakeDeliveryTerminal, err.Error(), nil)
			continue
		}
		claimed, err := s.sessionWakes.ClaimTaskSessionWake(ctx, wake.ID, wake.NextRunAt, next, now)
		if err != nil || !claimed {
			continue
		}
		status, deliveryErr := deliverer.DeliverSessionWake(ctx, wake.TaskID, wake.SessionID, wake.ID, wake.Prompt)
		if deliveryErr != nil {
			if errors.Is(deliveryErr, models.ErrSessionWakeTerminalDelivery) {
				_ = s.sessionWakes.RecordTaskSessionWakeDelivery(ctx, wake.ID, models.SessionWakeDeliveryTerminal, deliveryErr.Error(), nil)
				continue
			}
			_ = s.sessionWakes.RecordTaskSessionWakeDelivery(ctx, wake.ID, models.SessionWakeDeliveryFailed, deliveryErr.Error(), nil)
			continue
		}
		firedAt := now
		if status == "" {
			status = models.SessionWakeDeliverySent
		}
		_ = s.sessionWakes.RecordTaskSessionWakeDelivery(ctx, wake.ID, status, "", &firedAt)
	}
	return nil
}

func nextFutureWakeTime(wake *models.TaskSessionWake, now time.Time) (time.Time, error) {
	next, err := cron.NextScheduleTime(wake.CronExpression, wake.Timezone, wake.NextRunAt)
	if err != nil {
		return time.Time{}, err
	}
	for !next.After(now) {
		next, err = cron.NextScheduleTime(wake.CronExpression, wake.Timezone, next)
		if err != nil {
			return time.Time{}, err
		}
	}
	return next.UTC(), nil
}
