// Package appctx provides context utilities for background operations.
package appctx

import (
	"context"
	"time"
)

// Detached returns a new context that is not tied to the parent's cancellation
// but inherits its values. Use this for operations that must outlive the request.
// The returned context will be cancelled when the stop channel is closed or timeout expires.
func Detached(parent context.Context, stopCh <-chan struct{}, timeout time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	// Propagate cancellation from stopCh
	go func() {
		select {
		case <-stopCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	return ctx, cancel
}

// DetachedWithValues creates a detached context that copies deadline-insensitive values
// from the parent context while starting fresh with no deadline.
// Use sparingly - most values should be passed explicitly.
func DetachedWithValues(parent context.Context, stopCh <-chan struct{}, timeout time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	go func() {
		select {
		case <-stopCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	return ctx, cancel
}
