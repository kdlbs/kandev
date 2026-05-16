package service

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

// ancestorWalkHopCap bounds the parent-chain walk so corrupt parent_id
// pointers cannot loop forever. Real task trees never approach this depth.
const ancestorWalkHopCap = 64

// ErrAccessDenied is returned by access guards when the calling task is
// not allowed to read or write documents on the target task.
var ErrAccessDenied = errors.New("document access denied")

// taskLookup is the minimal repository surface the access guards depend on.
// Defined here (and not as a public interface) to keep the dependency arrow
// in this file alone.
type taskLookup interface {
	GetTask(ctx context.Context, id string) (*models.Task, error)
}

// blockerLookup is the minimal repository surface canReadDocuments
// needs to grant read access across blocker edges (office task-handoffs
// spec: a consumer task reads its blocker's documents after the blocker
// resolves). Implemented by office/repository/sqlite via
// BlockerRepository.ListTaskBlockers.
type blockerLookup interface {
	BlockerTaskIDs(ctx context.Context, taskID string) ([]string, error)
}

// canReadDocuments returns true when currentTaskID may READ documents on
// targetTaskID. Allowed cases (all require same workspace_id):
//   - target == current
//   - target is an ancestor of current (walk parent chain; must reach
//     target before reaching empty parent_id)
//   - target is a descendant of current (walk parent chain from target;
//     must reach current before reaching empty parent_id)
//   - target is a sibling of current: target.parent_id == current.parent_id
//     AND current.parent_id != "" (root tasks have no siblings)
//   - target is a BLOCKER of current. Office task-handoffs uses blocker
//     edges as the document-handoff readiness gate (the simplified Phase
//     3 model from the spec): a consumer task is blocked-by the producer
//     and reads the producer's documents once the blocker resolves.
//     Without this branch, the simplified handoff model is unreachable
//     end-to-end. Blocker access is read-only — canWriteDocuments does
//     NOT grant write across blocker edges.
//
// Workspace mismatch is always denied.
func canReadDocuments(ctx context.Context, repo taskLookup, blockers blockerLookup, currentTaskID, targetTaskID string) (bool, error) {
	current, target, ok, err := loadAccessPair(ctx, repo, currentTaskID, targetTaskID)
	if err != nil || !ok {
		return ok, err
	}
	if current.ID == target.ID {
		return true, nil
	}
	if current.ParentID != "" && current.ParentID == target.ParentID {
		return true, nil
	}
	if in, err := inAncestorChain(ctx, repo, current.ID, target.ID, true); err != nil || in {
		return in, err
	}
	if blockers != nil {
		blockerIDs, err := blockers.BlockerTaskIDs(ctx, current.ID)
		if err == nil {
			for _, bid := range blockerIDs {
				if bid == target.ID {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

// canWriteDocuments returns true when currentTaskID may WRITE documents on
// targetTaskID. Allowed cases (all require same workspace_id):
//   - target == current
//   - target is an ancestor of current (child→parent coordination writes)
//
// Sibling and descendant writes are NOT allowed by default — agents should
// publish coordination docs to the shared parent. Workspace mismatch is denied.
func canWriteDocuments(ctx context.Context, repo taskLookup, currentTaskID, targetTaskID string) (bool, error) {
	current, target, ok, err := loadAccessPair(ctx, repo, currentTaskID, targetTaskID)
	if err != nil || !ok {
		return ok, err
	}
	if current.ID == target.ID {
		return true, nil
	}
	return inAncestorChain(ctx, repo, current.ID, target.ID, false)
}

// loadAccessPair resolves both tasks and runs the always-required guards:
// non-empty IDs and matching workspaces. Returns (current, target, true)
// only when access evaluation should proceed.
func loadAccessPair(ctx context.Context, repo taskLookup, currentTaskID, targetTaskID string) (*models.Task, *models.Task, bool, error) {
	if currentTaskID == "" || targetTaskID == "" {
		return nil, nil, false, nil
	}
	current, err := repo.GetTask(ctx, currentTaskID)
	if err != nil {
		return nil, nil, false, err
	}
	target, err := repo.GetTask(ctx, targetTaskID)
	if err != nil {
		return nil, nil, false, err
	}
	if current == nil || target == nil {
		return nil, nil, false, nil
	}
	if current.WorkspaceID != target.WorkspaceID {
		return nil, nil, false, nil
	}
	return current, target, true, nil
}

// inAncestorChain returns true when targetID appears in current's ancestor
// chain. When checkDescendant is true, also returns true when currentID
// appears in target's ancestor chain (i.e. target is a descendant of
// current).
func inAncestorChain(ctx context.Context, repo taskLookup, currentID, targetID string, checkDescendant bool) (bool, error) {
	ancestors, err := ancestorIDs(ctx, repo, currentID)
	if err != nil {
		return false, err
	}
	for _, id := range ancestors {
		if id == targetID {
			return true, nil
		}
	}
	if !checkDescendant {
		return false, nil
	}
	descAncestors, err := ancestorIDs(ctx, repo, targetID)
	if err != nil {
		return false, err
	}
	for _, id := range descAncestors {
		if id == currentID {
			return true, nil
		}
	}
	return false, nil
}

// ancestorIDs walks parent_id up from taskID, returning the chain of
// ancestor IDs (excluding taskID itself). The walk stops at empty
// parent_id or after ancestorWalkHopCap hops, whichever comes first.
func ancestorIDs(ctx context.Context, repo taskLookup, taskID string) ([]string, error) {
	var out []string
	current := taskID
	visited := map[string]bool{taskID: true}
	for i := 0; i < ancestorWalkHopCap; i++ {
		t, err := repo.GetTask(ctx, current)
		if err != nil {
			return nil, err
		}
		if t == nil || t.ParentID == "" {
			return out, nil
		}
		if visited[t.ParentID] {
			// Cycle — corrupt data. Bail without erroring; the caller
			// gets the chain accumulated so far.
			return out, nil
		}
		visited[t.ParentID] = true
		out = append(out, t.ParentID)
		current = t.ParentID
	}
	return out, nil
}

// repoTaskLookupAdapter adapts a repository.TaskRepository (the full
// interface used by Service) to the taskLookup minimal surface. Used to
// keep the access tests independent of the full task repo.
type repoTaskLookupAdapter struct {
	r repository.TaskRepository
}

func (a repoTaskLookupAdapter) GetTask(ctx context.Context, id string) (*models.Task, error) {
	return a.r.GetTask(ctx, id)
}

// blockerLookupAdapter adapts the office BlockerRepository to the
// blockerLookup minimal surface used by canReadDocuments. The full
// repository returns *TaskBlocker values; the adapter projects to
// blocker task ids only.
type blockerLookupAdapter struct {
	repo BlockerRepository
}

func (a blockerLookupAdapter) BlockerTaskIDs(ctx context.Context, taskID string) ([]string, error) {
	if a.repo == nil {
		return nil, nil
	}
	blockers, err := a.repo.ListTaskBlockers(ctx, taskID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(blockers))
	for _, b := range blockers {
		if b != nil {
			out = append(out, b.BlockerTaskID)
		}
	}
	return out, nil
}
