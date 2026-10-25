package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// raceInjectingDeleteRepo wraps fakeDeleteRepo and, on the first call for a
// chosen task ID, mutates that task's persisted workspace.mode before
// delegating to the real guarded write — standing in for a concurrent writer
// (another delete, a reparent, the generic metadata PATCH surface) that flips
// mode between resolveDeleteSet's guard-capture read and its write. The guard
// passed in was built from the pre-mutation state, so the delegated call must
// lose it against the post-mutation persisted row.
type raceInjectingDeleteRepo struct {
	*fakeDeleteRepo
	raceTaskID string
	injected   bool
}

func (r *raceInjectingDeleteRepo) SetTaskWorkspaceMetadataIfUnchanged(
	ctx context.Context, taskID string, guard models.OrphanWriteGuard, value map[string]interface{},
) (bool, error) {
	if taskID == r.raceTaskID && !r.injected {
		r.injected = true
		r.base.mu.Lock()
		if t := r.base.tasks[taskID]; t != nil {
			if ws, ok := t.Metadata["workspace"].(map[string]interface{}); ok {
				ws["mode"] = workspaceModeSharedGroup
			}
		}
		r.base.mu.Unlock()
	}
	return r.fakeDeleteRepo.SetTaskWorkspaceMetadataIfUnchanged(ctx, taskID, guard, value)
}

// RVW2-F3: resolveDeleteSet's site-4 guarded write (normalizing an
// inherit_parent child's workspace mode before a non-cascade delete
// reparents it) has a lost-guard error path that was untested. A concurrent
// mode change between the guard-capture read and the guarded write must
// surface as an error wrapping errWorkspaceMetadataChangedConcurrently, and
// the reparent that would otherwise follow must not run — the child must
// still point at the (about to be deleted) root.
func TestDeleteTaskTree_NoCascadeConcurrentModeChangeAbortsBeforeReparent(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("child", "root", "ws-1")
	tasks.tasks["child"].Metadata = map[string]interface{}{
		"workspace": map[string]interface{}{
			"mode": workspaceModeInheritParent,
		},
	}

	repo := &raceInjectingDeleteRepo{
		fakeDeleteRepo: &fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)},
		raceTaskID:     "child",
	}
	svc := NewHandoffService(repo, nil, nil, nil, nil, nil)

	_, err := svc.DeleteTaskTree(context.Background(), "root", false)
	if err == nil {
		t.Fatal("DeleteTaskTree: want error after concurrent mode change lost the guard, got nil")
	}
	if !errors.Is(err, errWorkspaceMetadataChangedConcurrently) {
		t.Fatalf("DeleteTaskTree error = %v, want it to wrap errWorkspaceMetadataChangedConcurrently", err)
	}

	child, getErr := tasks.GetTask(context.Background(), "child")
	if getErr != nil {
		t.Fatalf("GetTask(child): %v", getErr)
	}
	if child == nil {
		t.Fatal("child was deleted despite the aborted normalize")
	}
	if child.ParentID != "root" {
		t.Fatalf("child.ParentID = %q after aborted normalize, want unchanged %q (reparent must not run)", child.ParentID, "root")
	}
	root, getErr := tasks.GetTask(context.Background(), "root")
	if getErr != nil {
		t.Fatalf("GetTask(root): %v", getErr)
	}
	if root == nil {
		t.Fatal("root was deleted despite the aborted normalize")
	}
}
