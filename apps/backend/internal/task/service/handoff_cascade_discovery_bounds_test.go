package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/archivecascade"
	"github.com/kandev/kandev/internal/task/models"
)

func TestArchiveCascadeDiscoveryRejectsMemberSentinel(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	for i := range archiveCascadeMaxMembers {
		tasks.addTask(fmt.Sprintf("child-%05d", i), "root", "ws-1")
	}
	svc := NewHandoffService(newCascadeRepo(tasks), nil, nil, nil, newCascadeWSGroupRepo(), nil)
	_, err := svc.ArchiveTaskTree(context.Background(), "root", true)
	var sizeErr *archivecascade.SizeExceededError
	if !errors.As(err, &sizeErr) || !errors.Is(err, archivecascade.ErrSizeExceeded) {
		t.Fatalf("discovery error = %v, want typed archive_size_exceeded", err)
	}
	if sizeErr.Dimension != "members" || sizeErr.Limit != archiveCascadeMaxMembers || sizeErr.Submitted != archiveCascadeMaxMembers+1 {
		t.Fatalf("size error = %+v, want members %d/%d", sizeErr, archiveCascadeMaxMembers, archiveCascadeMaxMembers+1)
	}
	root, err := tasks.GetTask(context.Background(), "root")
	if err != nil {
		t.Fatal(err)
	}
	if root.ArchivedAt != nil {
		t.Fatal("root was mutated despite discovery overflow")
	}
	child, err := tasks.GetTask(context.Background(), "child-00000")
	if err != nil {
		t.Fatal(err)
	}
	if child.ArchivedAt != nil {
		t.Fatal("child was mutated despite discovery overflow")
	}
}

func TestArchiveCascadeDiscoveryRejectsForeignWorkspace(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("foreign-child", "root", "ws-2")

	svc := NewHandoffService(newCascadeRepo(tasks), nil, nil, nil, newCascadeWSGroupRepo(), nil)
	_, err := svc.ArchiveTaskTree(context.Background(), "root", true)
	var scopeErr *archivecascade.CrossWorkspaceDescendantError
	if !errors.As(err, &scopeErr) || !errors.Is(err, archivecascade.ErrCrossWorkspaceDescendant) {
		t.Fatalf("discovery error = %v, want typed cross_workspace_descendant", err)
	}
	if scopeErr.TaskID != "foreign-child" || scopeErr.ExpectedWorkspaceID != "ws-1" || scopeErr.WorkspaceID != "ws-2" {
		t.Fatalf("scope error = %+v, want child foreign-child ws-2/ws-1", scopeErr)
	}
	root, err := tasks.GetTask(context.Background(), "root")
	if err != nil {
		t.Fatal(err)
	}
	child, err := tasks.GetTask(context.Background(), "foreign-child")
	if err != nil {
		t.Fatal(err)
	}
	if root.ArchivedAt != nil || child.ArchivedAt != nil {
		t.Fatal("foreign-workspace discovery mutated task rows")
	}
}

type failingCascadeTaskReadRepo struct {
	*fakeCascadeRepo
	failID string
}

func (r *failingCascadeTaskReadRepo) GetTask(ctx context.Context, id string) (*models.Task, error) {
	if id == r.failID {
		return nil, errors.New("task read failed")
	}
	return r.fakeCascadeRepo.GetTask(ctx, id)
}

func TestArchivedCascadeDiscoveryPropagatesTaskReadError(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addArchivedTask("root", "", "ws-1", "cascade-1")
	tasks.addArchivedTask("child", "root", "ws-1", "cascade-1")
	repo := &failingCascadeTaskReadRepo{fakeCascadeRepo: newCascadeRepo(tasks), failID: "child"}
	svc := NewHandoffService(repo, nil, nil, nil, newCascadeWSGroupRepo(), nil)

	_, err := svc.collectArchivedTreeByCascade(context.Background(), "root", "cascade-1")
	if err == nil || !strings.Contains(err.Error(), "task read failed") {
		t.Fatalf("discovery error = %v, want propagated task read failure", err)
	}
}

func TestArchiveCascadeDiscoveryRejectsCyclesAndDuplicates(t *testing.T) {
	tests := []struct {
		name string
		seed func(*fakeTaskRepo)
	}{
		{
			name: "cycle",
			seed: func(tasks *fakeTaskRepo) {
				tasks.addTask("root", "", "ws-1")
				tasks.children["root"] = append(tasks.children["root"], "root")
			},
		},
		{
			name: "duplicate",
			seed: func(tasks *fakeTaskRepo) {
				tasks.addTask("root", "", "ws-1")
				tasks.addTask("child", "root", "ws-1")
				tasks.children["root"] = append(tasks.children["root"], "child")
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tasks := newFakeTaskRepo()
			tc.seed(tasks)
			svc := NewHandoffService(newCascadeRepo(tasks), nil, nil, nil, newCascadeWSGroupRepo(), nil)
			_, err := svc.collectTaskTree(context.Background(), "root")
			var structureErr *archivecascade.StructureError
			if !errors.As(err, &structureErr) || !errors.Is(err, archivecascade.ErrInvalidStructure) {
				t.Fatalf("discovery error = %v, want typed invalid structure", err)
			}
			if structureErr.TaskID == "" {
				t.Fatalf("structure error = %+v, want task ID", structureErr)
			}
		})
	}
}

func TestArchiveCascadeDiscoveryRejectsDepthSentinel(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	parentID := "root"
	for i := range archiveCascadeMaxDepth + 1 {
		childID := fmt.Sprintf("depth-%03d", i)
		tasks.addTask(childID, parentID, "ws-1")
		parentID = childID
	}

	svc := NewHandoffService(newCascadeRepo(tasks), nil, nil, nil, newCascadeWSGroupRepo(), nil)
	_, err := svc.collectTaskTree(context.Background(), "root")
	var sizeErr *archivecascade.SizeExceededError
	if !errors.As(err, &sizeErr) || !errors.Is(err, archivecascade.ErrSizeExceeded) {
		t.Fatalf("discovery error = %v, want typed archive_size_exceeded", err)
	}
	if sizeErr.Dimension != "depth" || sizeErr.Limit != archiveCascadeMaxDepth || sizeErr.Submitted != archiveCascadeMaxDepth+1 {
		t.Fatalf("size error = %+v, want depth %d/%d", sizeErr, archiveCascadeMaxDepth, archiveCascadeMaxDepth+1)
	}
}

func TestArchiveCascadeDiscoveryAcceptsConfiguredBoundaries(t *testing.T) {
	t.Run("members", func(t *testing.T) {
		tasks := newFakeTaskRepo()
		tasks.addTask("root", "", "ws-1")
		for i := range archiveCascadeMaxMembers - 1 {
			tasks.addTask(fmt.Sprintf("child-%05d", i), "root", "ws-1")
		}

		svc := NewHandoffService(newCascadeRepo(tasks), nil, nil, nil, newCascadeWSGroupRepo(), nil)
		got, err := svc.collectTaskTree(context.Background(), "root")
		if err != nil {
			t.Fatalf("discovery error = %v", err)
		}
		if len(got) != archiveCascadeMaxMembers {
			t.Fatalf("discovered %d members, want %d", len(got), archiveCascadeMaxMembers)
		}
	})

	t.Run("depth", func(t *testing.T) {
		tasks := newFakeTaskRepo()
		tasks.addTask("root", "", "ws-1")
		parentID := "root"
		for i := range archiveCascadeMaxDepth {
			childID := fmt.Sprintf("depth-%03d", i)
			tasks.addTask(childID, parentID, "ws-1")
			parentID = childID
		}

		svc := NewHandoffService(newCascadeRepo(tasks), nil, nil, nil, newCascadeWSGroupRepo(), nil)
		got, err := svc.collectTaskTree(context.Background(), "root")
		if err != nil {
			t.Fatalf("discovery error = %v", err)
		}
		if len(got) != archiveCascadeMaxDepth+1 {
			t.Fatalf("discovered %d members, want %d", len(got), archiveCascadeMaxDepth+1)
		}
	})
}
