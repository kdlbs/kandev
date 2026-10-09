package coordinator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestWithCoordinatorLock_MissingCoordinatorSkipsFn(t *testing.T) {
	store := newTestStore(t)
	ran := false
	err := store.withCoordinatorLock(context.Background(), "nope", func(coordinatorExec) error {
		ran = true
		return nil
	})
	if !errors.Is(err, ErrNotFound) || ran {
		t.Fatalf("err = %v, ran = %v", err, ran)
	}
}

func TestWithCoordinatorLock_CommitAndRollback(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	set := func(tx coordinatorExec, name string) error {
		_, err := tx.ExecContext(ctx, `UPDATE coordinators SET name = ? WHERE id = ?`, name, c.ID)
		return err
	}
	if err := store.withCoordinatorLock(ctx, c.ID, func(tx coordinatorExec) error { return set(tx, "kept") }); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom")
	err := store.withCoordinatorLock(ctx, c.ID, func(tx coordinatorExec) error {
		if err := set(tx, "discarded"); err != nil {
			return err
		}
		return boom
	})
	if err != boom {
		t.Fatalf("err = %v, want the fn error unwrapped", err)
	}
	got, err := store.GetCoordinatorByID(ctx, c.ID)
	if err != nil || got.Name != "kept" {
		t.Fatalf("name = %q, err = %v", got.Name, err)
	}
}

func TestWithCoordinatorLock_QueryContextOnHandle(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	err := store.withCoordinatorLock(ctx, c.ID, func(tx coordinatorExec) error {
		rows, err := tx.QueryContext(ctx, `SELECT id FROM coordinators WHERE id = ?`, c.ID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		if !rows.Next() {
			t.Error("no row through the locked handle")
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Two locked sections on one coordinator never overlap, and a rolled-back
// section does not leave the lock held for the next one.
func TestWithCoordinatorLock_Serializes(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")

	entered := make(chan struct{})
	release := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	var order []string

	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = store.withCoordinatorLock(ctx, c.ID, func(coordinatorExec) error {
			close(entered)
			<-release
			mu.Lock()
			order = append(order, "first")
			mu.Unlock()
			return errors.New("rolled back")
		})
	}()
	<-entered
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = store.withCoordinatorLock(ctx, c.ID, func(coordinatorExec) error {
			mu.Lock()
			order = append(order, "second")
			mu.Unlock()
			return nil
		})
	}()
	<-time.After(150 * time.Millisecond)
	mu.Lock()
	if len(order) != 0 {
		mu.Unlock()
		t.Fatalf("second section ran while the first held the lock: %v", order)
	}
	mu.Unlock()
	close(release)
	wg.Wait()
	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Fatalf("order = %v", order)
	}
}
