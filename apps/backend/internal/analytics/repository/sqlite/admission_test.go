package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics"
)

func TestAnalyticsAdmissionBound(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	first, releaseFirst, err := repo.beginAnalyticsOperation(context.Background())
	if err != nil {
		t.Fatalf("first admission: %v", err)
	}
	defer releaseFirst()
	second, releaseSecond, err := repo.beginAnalyticsOperation(context.Background())
	if err != nil {
		t.Fatalf("second admission: %v", err)
	}
	defer releaseSecond()
	if first == nil || second == nil {
		t.Fatal("expected both operations to acquire admission")
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := repo.GetGlobalStats(ctx, "workspace", nil)
		result <- err
	}()
	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("queued operation error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued operation did not observe cancellation")
	}
}

func TestAnalyticsAdmissionCancellation(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	_, releaseFirst, err := repo.beginAnalyticsOperation(context.Background())
	if err != nil {
		t.Fatalf("first admission: %v", err)
	}
	defer releaseFirst()
	_, releaseSecond, err := repo.beginAnalyticsOperation(context.Background())
	if err != nil {
		t.Fatalf("second admission: %v", err)
	}
	defer releaseSecond()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = repo.GetGlobalStats(ctx, "workspace", nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline error = %v, want context deadline", err)
	}
}

func TestAnalyticsAdmissionReleasesOnError(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}
	if err := dbConn.Close(); err != nil {
		t.Fatalf("close test db: %v", err)
	}

	if _, err := repo.GetGlobalStats(context.Background(), "workspace", nil); err == nil {
		t.Fatal("expected closed database error")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, release, err := repo.beginAnalyticsOperation(ctx)
	if err != nil {
		t.Fatalf("admission remained occupied after query error: %v", err)
	}
	release()

}

func TestNormalizeAnalyticsOperationErrorPreservesNonTimeoutDatabaseErrors(t *testing.T) {
	operation, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	storageErr := errors.New("database closed")

	got := normalizeAnalyticsOperationError(context.Background(), operation, storageErr)

	if !errors.Is(got, storageErr) {
		t.Fatalf("normalized error = %v, want the storage error", got)
	}
	if analytics.IsAnalyticsBusy(got) {
		t.Fatalf("normalized storage error = %v, want no analytics-busy classification", got)
	}
}
