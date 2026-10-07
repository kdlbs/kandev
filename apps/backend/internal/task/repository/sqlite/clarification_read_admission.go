package sqlite

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	clarificationReadAdmissionLimit = 1
	clarificationReadOperationLimit = 10 * time.Second
	clarificationReadSlowThreshold  = time.Second
	clarificationReadErrorClassNone = sidebarGroupNone
)

type clarificationReadAdmission struct {
	once sync.Once
	gate chan struct{}
}

func (r *Repository) beginClarificationRead(parent context.Context) (context.Context, func(), time.Duration, error) {
	if parent == nil {
		parent = context.Background()
	}
	r.clarificationAdmission.once.Do(func() {
		r.clarificationAdmission.gate = make(chan struct{}, clarificationReadAdmissionLimit)
	})

	started := time.Now()
	operationCtx, cancel := context.WithTimeout(parent, clarificationReadOperationLimit)
	select {
	case r.clarificationAdmission.gate <- struct{}{}:
		var releaseOnce sync.Once
		release := func() {
			releaseOnce.Do(func() {
				cancel()
				<-r.clarificationAdmission.gate
			})
		}
		if err := operationCtx.Err(); err != nil {
			release()
			if parent.Err() != nil {
				return nil, nil, time.Since(started), parent.Err()
			}
			return nil, nil, time.Since(started), err
		}
		return operationCtx, release, time.Since(started), nil
	case <-operationCtx.Done():
		wait := time.Since(started)
		cancel()
		if parent.Err() != nil {
			return nil, nil, wait, parent.Err()
		}
		return nil, nil, wait, operationCtx.Err()
	}
}

func normalizeClarificationReadError(parent, operation context.Context, err error) error {
	if err == nil || parent.Err() != nil {
		return err
	}
	if operation.Err() == context.DeadlineExceeded && errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return err
}

func (r *Repository) logClarificationRead(
	name string,
	wait, execution time.Duration,
	err error,
) {
	if r.log == nil || (err == nil && wait+execution < clarificationReadSlowThreshold) {
		return
	}

	class := clarificationReadErrorClassNone
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		class = "deadline_exceeded"
	case errors.Is(err, context.Canceled):
		class = "canceled"
	case err != nil:
		class = "database_error"
	}

	fields := []zap.Field{
		zap.String("operation", name),
		zap.Duration("admission_wait", wait),
		zap.Duration("execution", execution),
		zap.String("error_class", class),
	}
	if r.ro != nil {
		stats := r.ro.Stats()
		fields = append(fields,
			zap.Int("reader_open_connections", stats.OpenConnections),
			zap.Int("reader_in_use", stats.InUse),
			zap.Int64("reader_wait_count", stats.WaitCount),
			zap.Duration("reader_wait_duration", stats.WaitDuration),
		)
	}
	r.log.Warn("clarification read completed", fields...)
}
