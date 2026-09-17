// Package dispatcher owns the single durable run claim loop. Features provide handlers.
package dispatcher

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/kandev/kandev/internal/runs/models"
)

type Queue interface {
	ClaimNextEligibleRun(context.Context) (*models.Run, error)
	FinishRun(context.Context, string, string, *string) error
	UpdateRunOutputSummary(context.Context, string, string, string) error
}
type Handler func(context.Context, *models.Run) (bool, error)
type Dispatcher struct {
	Queue    Queue
	Handlers []Handler
	Before   func(context.Context)
	After    func(context.Context)
	OnError  func(error)
}

func (d *Dispatcher) report(err error) {
	if err != nil && d.OnError != nil {
		d.OnError(err)
	}
}
func (d *Dispatcher) Tick(ctx context.Context) {
	if d.Before != nil {
		d.Before(ctx)
	}
	for i := 0; i < 10 && ctx.Err() == nil; i++ {
		run, err := d.Queue.ClaimNextEligibleRun(ctx)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && run == nil) {
			break
		}
		if err != nil {
			d.report(err)
			break
		}
		d.dispatch(ctx, run)
	}
	if d.After != nil {
		d.After(ctx)
	}
}
func (d *Dispatcher) dispatch(ctx context.Context, run *models.Run) {
	for _, handler := range d.Handlers {
		handled, err := handler(ctx, run)
		if err != nil {
			d.fail(ctx, run, err)
			return
		}
		if handled {
			return
		}
	}
	d.fail(ctx, run, fmt.Errorf("no enabled runtime handles this run"))
}
func (d *Dispatcher) fail(ctx context.Context, run *models.Run, err error) {
	d.report(err)
	d.report(d.Queue.UpdateRunOutputSummary(ctx, run.ID, "", err.Error()))
	d.report(d.Queue.FinishRun(ctx, run.ID, "failed", nil))
}
