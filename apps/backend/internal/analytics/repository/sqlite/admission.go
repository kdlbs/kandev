package sqlite

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/analytics"
)

const (
	analyticsAdmissionLimit = 2
	analyticsOperationLimit = 10 * time.Second
)

type analyticsAdmission struct {
	once sync.Once
	gate chan struct{}
}

func (r *Repository) ensureAnalyticsAdmission() {
	r.admission.once.Do(func() {
		r.admission.gate = make(chan struct{}, analyticsAdmissionLimit)
	})
}

// beginAnalyticsOperation reserves one analytics slot and bounds the total
// queue-plus-query lifetime. The returned context is the only context passed
// to analytics SQL so a canceled or expired operation cannot start late.
func (r *Repository) beginAnalyticsOperation(parent context.Context) (context.Context, func(), error) {
	r.ensureAnalyticsAdmission()
	operationCtx, cancel := context.WithTimeout(parent, analyticsOperationLimit)
	select {
	case r.admission.gate <- struct{}{}:
		var releaseOnce sync.Once
		release := func() {
			releaseOnce.Do(func() {
				cancel()
				<-r.admission.gate
			})
		}
		return operationCtx, release, nil
	case <-operationCtx.Done():
		cancel()
		if parent.Err() != nil {
			return nil, nil, parent.Err()
		}
		return nil, nil, analytics.NewAnalyticsBusyError(operationCtx.Err())
	}
}

func normalizeAnalyticsOperationError(
	parent context.Context,
	operation context.Context,
	err error,
) error {
	if err == nil || parent.Err() != nil {
		return err
	}
	if operation.Err() == context.DeadlineExceeded && errors.Is(err, context.DeadlineExceeded) {
		return analytics.NewAnalyticsBusyError(err)
	}
	return err
}
