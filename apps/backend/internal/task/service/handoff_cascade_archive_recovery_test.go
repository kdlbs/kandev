package service

import (
	"context"
	"testing"
)

type synchronousArchiveRunCanceller struct {
	asyncCalls       int
	synchronousCalls int
}

func (c *synchronousArchiveRunCanceller) CancelTaskExecution(context.Context, string, string, bool) error {
	c.asyncCalls++
	return nil
}

func (c *synchronousArchiveRunCanceller) CancelTaskExecutionSynchronously(context.Context, string, string, bool) error {
	c.synchronousCalls++
	return nil
}

func TestCancelActiveRuns_UsesSynchronousStopWhenAvailable(t *testing.T) {
	svc := NewHandoffService(nil, nil, nil, nil, nil, nil)
	canceller := &synchronousArchiveRunCanceller{}
	svc.SetRunCanceller(canceller)

	svc.cancelActiveRuns(context.Background(), []string{"task-archive"}, "task tree archived")

	if canceller.synchronousCalls != 1 {
		t.Fatalf("synchronous stop calls = %d, want 1", canceller.synchronousCalls)
	}
	if canceller.asyncCalls != 0 {
		t.Fatalf("asynchronous stop calls = %d, want 0", canceller.asyncCalls)
	}
}
