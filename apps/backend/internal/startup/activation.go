package startup

import (
	"context"
	"errors"
)

// AfterReady registers a worker start behind the bootstrap activation gate.
// Compositions without a bootstrap reporter start immediately.
func AfterReady(ctx context.Context, start func()) {
	if start == nil {
		return
	}
	_ = AfterReadyError(ctx, func() error { start(); return nil })
}

// AfterReadyError defers a fallible start and propagates its failure at activation.
func AfterReadyError(ctx context.Context, start func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r := FromContext(ctx)
	if r == nil {
		return start()
	}
	r.mu.Lock()
	if !r.activated || !r.activationComplete {
		r.activations = append(r.activations, func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			return start()
		})
		r.mu.Unlock()
		return nil
	}
	activationErr := r.activationErr
	r.mu.Unlock()
	if activationErr != nil {
		return activationErr
	}
	return start()
}

// Activate releases registered starts after readiness and router publication.
// Callbacks run outside the reporter lock and at most once.
func Activate(ctx context.Context) (result error) {
	r := FromContext(ctx)
	if r == nil {
		return ctx.Err()
	}
	r.mu.Lock()
	if r.phase != Ready {
		r.mu.Unlock()
		return errors.New("startup activation requires readiness")
	}
	if r.activated {
		r.mu.Unlock()
		return WaitForActivation(ctx)
	}
	r.activated = true
	starts := r.activations
	r.activations = nil
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.completeActivationLocked(result)
		r.mu.Unlock()
	}()
	for {
		for _, start := range starts {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := start(); err != nil {
				return err
			}
		}
		r.mu.Lock()
		starts = r.activations
		r.activations = nil
		if len(starts) == 0 {
			result = ctx.Err()
			r.completeActivationLocked(result)
			r.mu.Unlock()
			return result
		}
		r.mu.Unlock()
	}
}

func (r *Reporter) completeActivationLocked(err error) {
	if r.activationComplete {
		return
	}
	r.activationErr = err
	r.activationComplete = true
	r.activations = nil
	close(r.activationDone)
}

// WaitForActivation joins worker admission before a restore drains the owners.
// The restore caller cancels worker contexts first, then waits with its own
// lifetime so cancellation cannot let a late Start race closed persistence.
func WaitForActivation(ctx context.Context) error {
	r := FromContext(ctx)
	if r == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.activationDone:
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.activationErr
	}
}
