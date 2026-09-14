package service

import (
	"context"
	"errors"
	"testing"
)

func TestArchiveTaskTreeMarksCleanupStartFailureAsPostCommit(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	cleanupErr := errors.New("cleanup worker unavailable")
	coordinator := &recordingCleanupCoordinator{startErr: cleanupErr}
	svc := NewHandoffService(newCascadeRepo(tasks), nil, nil, nil, nil, nil)
	svc.SetTaskResourceCleaner(coordinator)

	out, err := svc.ArchiveTaskTree(context.Background(), "root", false)
	if out == nil || len(out.ArchivedTaskIDs) != 1 || out.ArchivedTaskIDs[0] != "root" {
		t.Fatalf("archive outcome = %#v, want committed root archive", out)
	}
	var postCommitErr *CascadePostCommitError
	if !errors.As(err, &postCommitErr) || !errors.Is(err, cleanupErr) {
		t.Fatalf("archive error = %v, want post-commit cleanup error", err)
	}
	archived, getErr := tasks.GetTask(context.Background(), "root")
	if getErr != nil || archived == nil || archived.ArchivedAt == nil {
		t.Fatalf("archived task = %#v, error = %v, want durable archive", archived, getErr)
	}
}

func TestDeleteTaskTreeMarksCleanupStartFailureAsPostCommit(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	cleanupErr := errors.New("cleanup worker unavailable")
	coordinator := &recordingCleanupCoordinator{startErr: cleanupErr}
	svc := NewHandoffService(&fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)}, nil, nil, nil, nil, nil)
	svc.SetTaskResourceCleaner(coordinator)

	out, err := svc.DeleteTaskTree(context.Background(), "root", false)
	if out == nil || len(out.ArchivedTaskIDs) != 1 || out.ArchivedTaskIDs[0] != "root" {
		t.Fatalf("delete outcome = %#v, want committed root deletion", out)
	}
	var postCommitErr *CascadePostCommitError
	if !errors.As(err, &postCommitErr) || !errors.Is(err, cleanupErr) {
		t.Fatalf("delete error = %v, want post-commit cleanup error", err)
	}
	deleted, getErr := tasks.GetTask(context.Background(), "root")
	if getErr != nil {
		t.Fatalf("GetTask after delete: %v", getErr)
	}
	if deleted != nil {
		t.Fatalf("deleted task = %#v, want nil", deleted)
	}
}

func TestUnarchiveTaskTreeMarksRestorationFailureAsPostCommit(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addArchivedTask("root", "", "ws-1", "cascade-1")
	groups := newCascadeWSGroupRepo()
	restoreErr := errors.New("membership restoration unavailable")
	groups.restoreErr = restoreErr
	svc := NewHandoffService(newCascadeRepo(tasks), nil, nil, nil, groups, nil)

	out, err := svc.UnarchiveTaskTree(context.Background(), "root")
	if out == nil || len(out.ArchivedTaskIDs) != 1 || out.ArchivedTaskIDs[0] != "root" {
		t.Fatalf("unarchive outcome = %#v, want committed root restoration", out)
	}
	var postCommitErr *CascadePostCommitError
	if !errors.As(err, &postCommitErr) || !errors.Is(err, restoreErr) {
		t.Fatalf("unarchive error = %v, want post-commit restoration error", err)
	}
	restored, getErr := tasks.GetTask(context.Background(), "root")
	if getErr != nil || restored == nil || restored.ArchivedAt == nil || restored.ArchivedByCascadeID != "cascade-1" {
		t.Fatalf("restored task = %#v, error = %v, want archived task retaining cascade provenance", restored, getErr)
	}
}
