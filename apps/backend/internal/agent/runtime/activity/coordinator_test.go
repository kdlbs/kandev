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

func TestAcquireTaskRejectsAlreadyCancelledContext(t *testing.T) {
	coordinator := NewCoordinator(Options{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	lease, err := coordinator.AcquireTask(ctx, KindExecutionStarting)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("AcquireTask error = %v, want context.Canceled", err)
	}
	if lease != nil {
		t.Fatal("AcquireTask returned a lease for a cancelled context")
	}
	if busy := coordinator.BusyKinds(); len(busy) != 0 {
		t.Fatalf("BusyKinds = %v, want empty", busy)
	}
}

func TestMaintenanceAlreadyRunningReportsMaintenanceKind(t *testing.T) {
	coordinator := NewCoordinator(Options{})
	lease, _, err := coordinator.TryAcquireMaintenance(context.Background(), 0)
	if err != nil {
		t.Fatalf("TryAcquireMaintenance: %v", err)
	}
	defer lease.Release()

	_, busy, err := coordinator.TryAcquireMaintenance(context.Background(), 0)
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("second TryAcquireMaintenance error = %v, want ErrBusy", err)
	}
	want := KindMaintenanceRunning
	if len(busy) != 1 || busy[0] != want {
		t.Fatalf("busy kinds = %v, want [%s]", busy, want)
	}
}

func TestForcedMaintenanceIgnoresCurrentTaskButRemainsPreemptible(t *testing.T) {
	coordinator := NewCoordinator(Options{})
	activeTask, err := coordinator.AcquireTask(context.Background(), KindTestCommand)
	if err != nil {
		t.Fatalf("AcquireTask: %v", err)
	}
	defer activeTask.Release()

	maintenance, busy, err := coordinator.TryAcquireMaintenanceForce(context.Background())
	if err != nil {
		t.Fatalf("TryAcquireMaintenanceForce: %v", err)
	}
	if len(busy) != 0 {
		t.Fatalf("forced maintenance busy kinds = %v, want empty", busy)
	}

	admitted := make(chan *TaskLease, 1)
	go func() {
		lease, acquireErr := coordinator.AcquireTask(context.Background(), KindShellCommand)
		if acquireErr == nil {
			admitted <- lease
		}
	}()

	select {
	case <-maintenance.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("new task admission did not preempt forced maintenance")
	}
	select {
	case <-admitted:
		t.Fatal("new task admitted before forced maintenance drained")
	default:
	}
	maintenance.Release()
	select {
	case lease := <-admitted:
		lease.Release()
	case <-time.After(time.Second):
		t.Fatal("new task was not admitted after forced maintenance drained")
	}
}

func TestBusyResourcesExposeStableLabels(t *testing.T) {
	coordinator := NewCoordinator(Options{})
	lease, err := coordinator.AcquireTask(context.Background(), KindDockerImageBuild)
	if err != nil {
		t.Fatalf("AcquireTask: %v", err)
	}
	defer lease.Release()

	resources := coordinator.BusyResources()
	if len(resources) != 1 {
		t.Fatalf("BusyResources = %v, want one resource", resources)
	}
	if resources[0].Kind != KindDockerImageBuild || resources[0].Label == "" {
		t.Fatalf("BusyResources = %#v, want docker build resource with label", resources)
	}
}
