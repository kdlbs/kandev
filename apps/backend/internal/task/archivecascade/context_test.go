package archivecascade

import (
	"context"
	"testing"
	"time"
)

func TestContinuationContextSurvivesCancellationAndRetainsDeadline(t *testing.T) {
	parent, cancelParent := context.WithDeadline(context.Background(), time.Now().Add(time.Minute))
	ctx, cancel := ContinuationContext(parent)
	defer cancel()
	defer cancelParent()

	parentDeadline, _ := parent.Deadline()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("continuation context has no deadline")
	}
	if deadline.After(parentDeadline) {
		t.Fatalf("continuation deadline %s exceeds caller deadline %s", deadline, parentDeadline)
	}

	cancelParent()
	if err := ctx.Err(); err != nil {
		t.Fatalf("continuation canceled with caller: %v", err)
	}
}

func TestContinuationContextUsesCapturedDeadline(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()
	capturedDeadline := time.Now().Add(time.Minute)

	ctx, cancel := ContinuationContextUntil(parent, capturedDeadline)
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("continuation context has no deadline")
	}
	if deadline.After(capturedDeadline) {
		t.Fatalf("continuation deadline %s exceeds captured deadline %s", deadline, capturedDeadline)
	}

	cancelParent()
	if err := ctx.Err(); err != nil {
		t.Fatalf("continuation canceled with caller: %v", err)
	}
}
