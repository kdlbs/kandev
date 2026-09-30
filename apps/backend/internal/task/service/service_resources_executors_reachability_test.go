package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// fakeExecutorSaveObserver records every OnExecutorSaved call it receives, so
// a test can assert the exact before/after pair the service handed it without
// depending on the reachability package's own behavior.
type fakeExecutorSaveObserver struct {
	calls []executorSaveCall
}

type executorSaveCall struct {
	before *models.Executor
	after  *models.Executor
}

func (f *fakeExecutorSaveObserver) OnExecutorSaved(_ context.Context, before, after *models.Executor) {
	f.calls = append(f.calls, executorSaveCall{before: before, after: after})
}

func sshExecutorCreateRequest(name string, config map[string]string) *CreateExecutorRequest {
	return &CreateExecutorRequest{
		Name: name, Type: models.ExecutorTypeSSH, Status: models.ExecutorStatusActive,
		Resumable: true, Config: config,
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-003.1
//
// Creating an SSH executor always counts as a connection-configuration
// change — there is no prior state to compare against — so the observer must
// see before=nil paired with the newly created executor.
func TestCreateExecutorNotifiesSaveObserverWithNilBefore(t *testing.T) {
	svc, _, _ := createTestService(t)
	observer := &fakeExecutorSaveObserver{}
	svc.SetExecutorSaveObserver(observer)
	ctx := context.Background()

	executor, err := svc.CreateExecutor(ctx, sshExecutorCreateRequest("SSH box", map[string]string{
		"ssh_host": "10.0.0.5", "ssh_user": "deploy",
	}))
	if err != nil {
		t.Fatalf("CreateExecutor: %v", err)
	}

	if len(observer.calls) != 1 {
		t.Fatalf("observer calls = %d, want 1", len(observer.calls))
	}
	call := observer.calls[0]
	if call.before != nil {
		t.Fatalf("before = %+v, want nil on create", call.before)
	}
	if call.after == nil || call.after.ID != executor.ID {
		t.Fatalf("after = %+v, want the created executor", call.after)
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-003.1
//
// The service notifies the observer on every executor create, regardless of
// type — deciding whether a save is SSH-reachability-relevant is the
// observer implementation's job (internal/executors/reachability.SaveObserver),
// not the service's. This keeps the service layer decoupled from SSH-specific
// eligibility rules, the same way publishExecutorEvent fires unconditionally
// and lets subscribers filter.
func TestCreateExecutorNotifiesSaveObserverForNonSSHType(t *testing.T) {
	svc, _, _ := createTestService(t)
	observer := &fakeExecutorSaveObserver{}
	svc.SetExecutorSaveObserver(observer)
	ctx := context.Background()

	executor, err := svc.CreateExecutor(ctx, &CreateExecutorRequest{
		Name: "Local runner", Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive, Resumable: true,
	})
	if err != nil {
		t.Fatalf("CreateExecutor: %v", err)
	}

	if len(observer.calls) != 1 {
		t.Fatalf("observer calls = %d, want 1 (gating is the observer's job, not the service's)", len(observer.calls))
	}
	if observer.calls[0].after == nil || observer.calls[0].after.ID != executor.ID {
		t.Fatalf("after = %+v, want the created executor", observer.calls[0].after)
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-003.1, AC-EXECUTORS-SSH-REACHABILITY-003.2
//
// Updating an SSH executor's host must notify the observer with the literal
// old-host/new-host pair — captured strictly before applyExecutorUpdates
// mutates the executor in place, per the work order's own risk callout. A
// self-comparison (before and after pointing at the same, already-mutated
// struct) would silently make this test pass with either host string, so it
// asserts both values explicitly.
func TestUpdateExecutorNotifiesSaveObserverOnHostChange(t *testing.T) {
	svc, _, _ := createTestService(t)
	executor := createTestExecutor(t, svc, "SSH box", models.ExecutorTypeSSH)
	ctx := context.Background()
	if _, err := svc.UpdateExecutor(ctx, executor.ID, &UpdateExecutorRequest{
		Config: map[string]string{"ssh_host": "10.0.0.1", "ssh_user": "deploy"},
	}); err != nil {
		t.Fatalf("seed UpdateExecutor: %v", err)
	}

	observer := &fakeExecutorSaveObserver{}
	svc.SetExecutorSaveObserver(observer)

	if _, err := svc.UpdateExecutor(ctx, executor.ID, &UpdateExecutorRequest{
		Config: map[string]string{"ssh_host": "10.0.0.2", "ssh_user": "deploy"},
	}); err != nil {
		t.Fatalf("UpdateExecutor: %v", err)
	}

	if len(observer.calls) != 1 {
		t.Fatalf("observer calls = %d, want 1", len(observer.calls))
	}
	call := observer.calls[0]
	if call.before == nil || call.before.Config["ssh_host"] != "10.0.0.1" {
		t.Fatalf("before host = %+v, want 10.0.0.1", call.before)
	}
	if call.after == nil || call.after.Config["ssh_host"] != "10.0.0.2" {
		t.Fatalf("after host = %+v, want 10.0.0.2", call.after)
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-003.1
//
// A name-only update still notifies the observer (the service does not
// pre-filter by which fields changed) — the observer's own
// connectionConfigChanged comparison is what decides a name-only save isn't
// reachability-relevant, and that decision is covered in the reachability
// package's own save_observer_test.go, not here. This test only pins that the
// service keeps calling with the correct before/after regardless of which
// fields the caller supplied.
func TestUpdateExecutorNotifiesSaveObserverForNameOnlyChange(t *testing.T) {
	svc, _, _ := createTestService(t)
	executor := createTestExecutor(t, svc, "SSH box", models.ExecutorTypeSSH)
	ctx := context.Background()
	if _, err := svc.UpdateExecutor(ctx, executor.ID, &UpdateExecutorRequest{
		Config: map[string]string{"ssh_host": "10.0.0.1", "ssh_user": "deploy"},
	}); err != nil {
		t.Fatalf("seed UpdateExecutor: %v", err)
	}

	observer := &fakeExecutorSaveObserver{}
	svc.SetExecutorSaveObserver(observer)

	newName := "Renamed SSH box"
	if _, err := svc.UpdateExecutor(ctx, executor.ID, &UpdateExecutorRequest{Name: &newName}); err != nil {
		t.Fatalf("UpdateExecutor: %v", err)
	}

	if len(observer.calls) != 1 {
		t.Fatalf("observer calls = %d, want 1", len(observer.calls))
	}
	if observer.calls[0].before.Config["ssh_host"] != "10.0.0.1" || observer.calls[0].after.Config["ssh_host"] != "10.0.0.1" {
		t.Fatalf("host should be unchanged across a name-only update: %+v", observer.calls[0])
	}
	if observer.calls[0].after.Name != newName {
		t.Fatalf("after.Name = %q, want %q", observer.calls[0].after.Name, newName)
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-003.1
//
// A nil observer (the default in every test not exercising reachability) must
// never be dereferenced by CreateExecutor/UpdateExecutor.
func TestExecutorSaveObserverNilSafe(t *testing.T) {
	svc, _, _ := createTestService(t)
	ctx := context.Background()

	executor, err := svc.CreateExecutor(ctx, sshExecutorCreateRequest("SSH box", map[string]string{
		"ssh_host": "10.0.0.5",
	}))
	if err != nil {
		t.Fatalf("CreateExecutor: %v", err)
	}

	newHost := "10.0.0.9"
	if _, err := svc.UpdateExecutor(ctx, executor.ID, &UpdateExecutorRequest{
		Config: map[string]string{"ssh_host": newHost},
	}); err != nil {
		t.Fatalf("UpdateExecutor: %v", err)
	}
}
