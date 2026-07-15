package activity

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTaskAdmissionCancelsAndDrainsMaintenance(t *testing.T) {
	coordinator := NewCoordinator(Options{})
	maintenance, _, err := coordinator.TryAcquireMaintenance(context.Background(), 0)
	if err != nil {
		t.Fatalf("TryAcquireMaintenance: %v", err)
	}

	admitted := make(chan *TaskLease, 1)
	go func() {
		lease, acquireErr := coordinator.AcquireTask(context.Background(), KindExecutionStarting)
		if acquireErr == nil {
			admitted <- lease
		}
	}()

	select {
	case <-maintenance.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("task admission did not cancel maintenance")
	}
	select {
	case <-admitted:
		t.Fatal("task admitted before cancelled maintenance drained")
	default:
	}
	maintenance.Release()
	select {
	case lease := <-admitted:
		lease.Release()
	case <-time.After(time.Second):
		t.Fatal("task was not admitted after maintenance drained")
	}
}

func TestQuietPeriodUsesLastReleasedTaskActivity(t *testing.T) {
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	coordinator := NewCoordinator(Options{Now: func() time.Time { return now }})
	lease, err := coordinator.AcquireTask(context.Background(), KindShellCommand)
	if err != nil {
		t.Fatalf("AcquireTask: %v", err)
	}
	lease.Release()

	now = now.Add(9 * time.Minute)
	if _, _, err := coordinator.TryAcquireMaintenance(context.Background(), 10*time.Minute); !errors.Is(err, ErrBusy) {
		t.Fatalf("maintenance at 9m error = %v, want ErrBusy", err)
	}
	now = now.Add(time.Minute)
	maintenance, _, err := coordinator.TryAcquireMaintenance(context.Background(), 10*time.Minute)
	if err != nil {
		t.Fatalf("maintenance at 10m: %v", err)
	}
	maintenance.Release()
}
