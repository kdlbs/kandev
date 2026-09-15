package service

import (
	"context"
	"testing"
	"time"
)

func TestDetachedCleanupTransitionContextUsesFreshBoundedDeadline(t *testing.T) {
	parentDeadline := time.Now().Add(100 * time.Millisecond)
	parent, cancelParent := context.WithDeadline(context.Background(), parentDeadline)
	defer cancelParent()

	ctx, cancel := detachedCleanupTransitionContext(parent)
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("cleanup transition context has no deadline")
	}
	if !deadline.After(parentDeadline) {
		t.Fatalf("cleanup deadline %s should outlive parent deadline %s", deadline, parentDeadline)
	}
	if remaining := time.Until(deadline); remaining > 5*time.Second || remaining <= 0 {
		t.Fatalf("cleanup deadline has %s remaining, want at most 5 seconds", remaining)
	}

	cancelParent()
	if err := ctx.Err(); err != nil {
		t.Fatalf("cleanup transition context canceled with parent: %v", err)
	}
}
