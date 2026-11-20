package workspacescope

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func TestResolveMigrationTargetContextObservesCancellationWhileWaitingForConnection(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	connection, err := db.Connx(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, resolveErr := ResolveMigrationTargetContext(ctx, db)
		result <- resolveErr
	}()

	deadline := time.NewTimer(2 * time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()
	for db.Stats().WaitCount == 0 {
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("workspace lookup did not wait for the occupied database connection")
		}
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("workspace lookup error = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("workspace lookup did not stop after its request context was canceled")
	}
}
