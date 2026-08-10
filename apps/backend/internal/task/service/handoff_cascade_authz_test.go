package service

import (
	"context"
	"errors"
	"testing"
)

// Per-user scoping for the cascade entry points.
//
// backendapp always wires a HandoffService into TaskHandlers, so in a shipped
// instance DELETE /tasks/:id, POST /tasks/:id/archive and the task.archive WS
// action never reach the guarded Service.DeleteTask / Service.ArchiveTask — they
// land here instead, and this type had no identity awareness at all. Restoring
// the caller identity on the HTTP delete handler is inert without these guards.

var errCascadeDenied = errors.New("task not found")

// cascadeAuthzCase invokes one cascade entry point against a task ID.
type cascadeAuthzCase struct {
	name   string
	invoke func(*HandoffService, string) error
}

func cascadeAuthzCases() []cascadeAuthzCase {
	return []cascadeAuthzCase{
		{"ArchiveTaskTree", func(s *HandoffService, id string) error {
			_, err := s.ArchiveTaskTree(context.Background(), id, true)
			return err
		}},
		{"DeleteTaskTree", func(s *HandoffService, id string) error {
			_, err := s.DeleteTaskTree(context.Background(), id, true)
			return err
		}},
		{"UnarchiveTaskTree", func(s *HandoffService, id string) error {
			_, err := s.UnarchiveTaskTree(context.Background(), id)
			return err
		}},
	}
}

// cascadeAuthzChecker admits task-mine and refuses everything else, which is
// what Service.AuthorizeTaskAccess does for a caller who owns one workspace.
func cascadeAuthzChecker(_ context.Context, taskID string) error {
	if taskID == "task-mine" {
		return nil
	}
	return errCascadeDenied
}

func TestCascadeEntryPointsDenyForeignTask(t *testing.T) {
	for _, tc := range cascadeAuthzCases() {
		t.Run(tc.name, func(t *testing.T) {
			tasks := newFakeTaskRepo()
			tasks.addTask("task-mine", "", "ws-a")
			tasks.addArchivedTask("task-b", "", "ws-b", "")
			repo := &fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)}
			svc := NewHandoffService(repo, nil, nil, nil, newCascadeWSGroupRepo(), nil)
			svc.SetTaskAccessChecker(cascadeAuthzChecker)

			if err := tc.invoke(svc, "task-b"); !errors.Is(err, errCascadeDenied) {
				t.Fatalf("foreign cascade: err = %v, want denial", err)
			}

			tasks.mu.Lock()
			victim, present := tasks.tasks["task-b"]
			stillArchived := present && victim.ArchivedAt != nil
			tasks.mu.Unlock()
			if !present {
				t.Fatal("a denied cascade deleted the foreign task")
			}
			if !stillArchived {
				t.Fatal("a denied cascade unarchived the foreign task")
			}

			// The owner must get past the guard, otherwise the denial above
			// would also pass with the checker refusing everything.
			if err := tc.invoke(svc, "task-mine"); errors.Is(err, errCascadeDenied) {
				t.Fatalf("owner cascade was denied: %v", err)
			}
		})
	}
}

// TestCascadeEntryPointsGuardBeforeDependencies pins guard placement: with the
// task repo nil, a guard placed after the first repo read panics instead of
// returning the denial.
func TestCascadeEntryPointsGuardBeforeDependencies(t *testing.T) {
	for _, tc := range cascadeAuthzCases() {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("guard runs too late — panicked on a nil dependency: %v", r)
				}
			}()
			svc := NewHandoffService(nil, nil, nil, nil, nil, nil)
			svc.SetTaskAccessChecker(cascadeAuthzChecker)

			if err := tc.invoke(svc, "task-b"); !errors.Is(err, errCascadeDenied) {
				t.Fatalf("err = %v, want denial", err)
			}
		})
	}
}

// TestCascadeEntryPointsUnscopedWhenUnwired pins the compatibility contract: no
// checker installed means nothing is denied by scoping, so identity-less
// internal callers and every existing bare-NewHandoffService fixture are
// unaffected. The calls may still fail on their own dependencies — this asserts
// only that the denial sentinel does not appear.
func TestCascadeEntryPointsUnscopedWhenUnwired(t *testing.T) {
	for _, tc := range cascadeAuthzCases() {
		t.Run(tc.name, func(t *testing.T) {
			tasks := newFakeTaskRepo()
			tasks.addArchivedTask("task-b", "", "ws-b", "")
			repo := &fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)}
			svc := NewHandoffService(repo, nil, nil, nil, newCascadeWSGroupRepo(), nil)

			if err := tc.invoke(svc, "task-b"); errors.Is(err, errCascadeDenied) {
				t.Fatal("unwired checker denied the call; pre-auth behavior broken")
			}
		})
	}
}
