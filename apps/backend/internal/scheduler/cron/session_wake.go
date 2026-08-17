package cron

import (
	"context"
	"time"
)

type SessionWakeTicker interface {
	TickSessionWakes(context.Context, interface {
		DeliverSessionWake(context.Context, string, string, string, string) (string, error)
	}, time.Time) error
}

type SessionWakeDeliverer interface {
	DeliverSessionWake(context.Context, string, string, string, string) (string, error)
}

type SessionWakeHandler struct {
	ticker    SessionWakeTicker
	deliverer SessionWakeDeliverer
	now       func() time.Time
}

func NewSessionWakeHandler(ticker SessionWakeTicker, deliverer SessionWakeDeliverer, now func() time.Time) *SessionWakeHandler {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &SessionWakeHandler{ticker: ticker, deliverer: deliverer, now: now}
}
func (h *SessionWakeHandler) Name() string { return "session_wakes" }
func (h *SessionWakeHandler) Tick(ctx context.Context) error {
	if h == nil || h.ticker == nil || h.deliverer == nil {
		return nil
	}
	return h.ticker.TickSessionWakes(ctx, h.deliverer, h.now())
}
