package maintenance

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/db"
	"testing"
)

func TestAdmissionIsSharedPerPool(t *testing.T) {
	pool := db.NewPool(nil, nil)
	g := ForPool(pool)
	release, ok := g.TryAcquire()
	if !ok {
		t.Fatal("first lease unavailable")
	}
	if _, ok := ForPool(pool).TryAcquire(); ok {
		t.Fatal("concurrent admission")
	}
	other, ok := ForPool(db.NewPool(nil, nil)).TryAcquire()
	if !ok {
		t.Fatal("unrelated pool blocked")
	}
	other()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := g.Acquire(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("want canceled, got %v", err)
	}
	release()
	release()
	next, ok := g.TryAcquire()
	if !ok {
		t.Fatal("release did not reopen admission")
	}
	next()
}

func TestWaitingAdmissionCancels(t *testing.T) {
	g := ForPool(db.NewPool(nil, nil))
	release, _ := g.TryAcquire()
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := g.Acquire(ctx); done <- err }()
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
