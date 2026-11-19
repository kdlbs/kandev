package hostutility

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"

	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

type doneSignaledContext struct {
	context.Context
	doneCalled      chan struct{}
	continueAcquire chan struct{}
	once            sync.Once
}

func (ctx *doneSignaledContext) Done() <-chan struct{} {
	ctx.once.Do(func() {
		close(ctx.doneCalled)
		<-ctx.continueAcquire
	})
	return ctx.Context.Done()
}

func TestHostControlReturnsErrorWhenRuntimeLeaseRetiresDuringAcquire(t *testing.T) {
	previousProcs := runtime.GOMAXPROCS(1)
	t.Cleanup(func() { runtime.GOMAXPROCS(previousProcs) })
	log := newTestLogger(t)
	owner := agentctlclient.NewRuntimeOwner(nil, log, "boot-one")
	t.Cleanup(owner.Stop)
	candidate, err := owner.PrepareBinding()
	if err != nil {
		t.Fatalf("prepare runtime binding: %v", err)
	}
	if err := candidate.Configure("127.0.0.1", 39429, "secret", 1234, nil); err != nil {
		t.Fatalf("configure runtime binding: %v", err)
	}
	if err := candidate.Commit(); err != nil {
		t.Fatalf("publish runtime binding: %v", err)
	}

	ctx := &doneSignaledContext{
		Context:         context.Background(),
		doneCalled:      make(chan struct{}),
		continueAcquire: make(chan struct{}),
	}
	manager := &Manager{runtimeOwner: owner, log: log}
	result := make(chan struct {
		client  *agentctlclient.ControlClient
		release func()
		err     error
	}, 1)
	go func() {
		client, _, _, release, err := manager.hostControl(ctx)
		result <- struct {
			client  *agentctlclient.ControlClient
			release func()
			err     error
		}{client: client, release: release, err: err}
	}()

	<-ctx.doneCalled
	close(ctx.continueAcquire)
	if !owner.MarkUnavailableEpoch(candidate.Epoch(), agentctlclient.AvailabilityReasonAgentctlExited) {
		t.Fatal("runtime binding was not retired")
	}
	got := <-result
	if got.release != nil {
		got.release()
	}
	if !errors.Is(got.err, agentctlclient.ErrRuntimeLeaseRetired) {
		t.Fatalf("hostControl error = %v, want runtime lease retired", got.err)
	}
	if got.client != nil {
		t.Fatal("hostControl returned a control client for a retired runtime lease")
	}
}
