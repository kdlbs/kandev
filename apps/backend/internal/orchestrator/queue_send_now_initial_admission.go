package orchestrator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
)

var errSendNowInitialPromptUncertain = errors.New("initial Send Now prompt acceptance is uncertain")

func (s *Service) configureSendNowInitialAdmission(
	ctx context.Context, claim *messagequeue.SendNowClaim, options *startCreatedSessionOptions,
) func() error {
	outcome := make(chan error, 1)
	var once sync.Once
	var closed atomic.Bool
	options.beforeProviderAdmission = func(string) error {
		if closed.Load() {
			return context.Canceled
		}
		return ctx.Err()
	}
	finish := func(err error) { once.Do(func() { outcome <- err }) }
	options.onInitialPromptAccepted = func(string) {
		if err := s.markSendNowClaimAcceptedWithRetry(ctx, claim); err != nil {
			finish(errors.Join(errSendNowInitialPromptUncertain, err))
			return
		}
		finish(nil)
	}
	options.onInitialPromptAdmissionFailed = finish
	options.onInitialPromptFailed = func() { finish(errSendNowInitialPromptUncertain) }
	return func() error {
		defer closed.Store(true)
		// Match the executor's startup deadline. Timeout or shutdown retains the
		// durable claim because the detached runtime may still accept its prompt.
		timer := time.NewTimer(5 * time.Minute)
		defer timer.Stop()
		select {
		case err := <-outcome:
			return err
		case <-ctx.Done():
			return errors.Join(errSendNowInitialPromptUncertain, ctx.Err())
		case <-timer.C:
			return errSendNowInitialPromptUncertain
		}
	}
}
