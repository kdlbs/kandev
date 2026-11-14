package handlers

import (
	"context"
	"sync"
)

type sendOperationRetentionWorker struct {
	mu      sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	prune   func(context.Context)
	done    chan struct{}
	stopped bool
}

func newSendOperationRetentionWorker(ctx context.Context, prune func(context.Context)) *sendOperationRetentionWorker {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	return &sendOperationRetentionWorker{ctx: ctx, cancel: cancel, prune: prune}
}

func (w *sendOperationRetentionWorker) tryRun() {
	if w == nil {
		return
	}
	w.mu.Lock()
	if w.stopped || w.done != nil || w.prune == nil || w.ctx.Err() != nil {
		w.mu.Unlock()
		return
	}
	done := make(chan struct{})
	w.done = done
	ctx, cancel := context.WithTimeout(w.ctx, sendOperationRetentionTimeout)
	w.mu.Unlock()
	go func() {
		defer func() {
			cancel()
			w.mu.Lock()
			w.done = nil
			close(done)
			w.mu.Unlock()
		}()
		w.prune(ctx)
	}()
}

func (w *sendOperationRetentionWorker) stop() {
	w.mu.Lock()
	w.stopped = true
	w.cancel()
	done := w.done
	w.mu.Unlock()
	if done != nil {
		<-done
	}
}
