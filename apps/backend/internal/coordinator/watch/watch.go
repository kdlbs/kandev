// Package watch is the project half of a coordinator's Watches: the one place
// that decides whether a task's repositories are in the coordinator's scope.
package watch

import (
	"context"
	"fmt"
	"sort"
)

// Scope is the stored project scope. The zero value is every project.
type Scope struct {
	Selected      bool
	SetIDs        []string
	RepoIDs       []string
	IncludeNoRepo bool
}

// SetMembers is one repository set and its current members.
type SetMembers struct {
	ID            string
	RepositoryIDs []string
}

// Reader lists the workspace's live sets and repositories.
type Reader interface {
	Sets(ctx context.Context, workspaceID string) ([]SetMembers, error)
	RepositoryIDs(ctx context.Context, workspaceID string) ([]string, error)
}

// Resolver turns a stored Scope into live membership. Nothing it reads is
// stored, so a set that gains a repository widens the scope on the next call.
type Resolver struct{ reader Reader }

// NewResolver returns a Resolver over reader.
func NewResolver(reader Reader) Resolver { return Resolver{reader: reader} }

// Resolved is a Scope with its membership read at one moment.
type Resolved struct {
	selected      bool
	includeNoRepo bool
	repos         map[string]struct{}
}

// Resolve reads only what the scope needs: nothing under every project, the
// set listing when a set is listed, and the repository listing when a
// repository is listed. A failed read is an error, never an empty membership.
func (r Resolver) Resolve(ctx context.Context, workspaceID string, scope Scope) (Resolved, error) {
	out := Resolved{selected: scope.Selected, includeNoRepo: scope.IncludeNoRepo, repos: map[string]struct{}{}}
	if !scope.Selected {
		return out, nil
	}
	if len(scope.SetIDs) > 0 {
		if r.reader == nil {
			return Resolved{}, fmt.Errorf("watch: project reads are not wired")
		}
		sets, err := r.reader.Sets(ctx, workspaceID)
		if err != nil {
			return Resolved{}, fmt.Errorf("read repository sets: %w", err)
		}
		listed := toSet(scope.SetIDs)
		for _, s := range sets {
			if _, ok := listed[s.ID]; !ok {
				continue
			}
			for _, id := range s.RepositoryIDs {
				out.repos[id] = struct{}{}
			}
		}
	}
	if len(scope.RepoIDs) > 0 {
		if r.reader == nil {
			return Resolved{}, fmt.Errorf("watch: project reads are not wired")
		}
		live, err := r.reader.RepositoryIDs(ctx, workspaceID)
		if err != nil {
			return Resolved{}, fmt.Errorf("read repositories: %w", err)
		}
		liveSet := toSet(live)
		for _, id := range scope.RepoIDs {
			if _, ok := liveSet[id]; ok {
				out.repos[id] = struct{}{}
			}
		}
	}
	return out, nil
}

func toSet(ids []string) map[string]struct{} {
	m := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		m[id] = struct{}{}
	}
	return m
}

// Selected reports whether the scope narrows by project.
func (r Resolved) Selected() bool { return r.selected }

// InProjects reports whether a task with these repositories is in scope. A
// task in several repositories is in scope when any one matches; a task with
// none is in scope under every project or when the toggle is on.
func (r Resolved) InProjects(taskRepoIDs []string) bool {
	if !r.selected {
		return true
	}
	if len(taskRepoIDs) == 0 {
		return r.includeNoRepo
	}
	for _, id := range taskRepoIDs {
		if _, ok := r.repos[id]; ok {
			return true
		}
	}
	return false
}

// RepositoryInScope reports whether one repository id is in scope.
func (r Resolved) RepositoryInScope(id string) bool {
	if !r.selected {
		return true
	}
	_, ok := r.repos[id]
	return ok
}

// RepositoryIDs is the sorted union of the listed live repositories and the
// members of the listed sets.
func (r Resolved) RepositoryIDs() []string {
	ids := make([]string, 0, len(r.repos))
	for id := range r.repos {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Task is the single function that combines the workflow test and the project
// test; every enforcement path calls it and none compares repositories itself.
func Task(workflowWatched bool, projects Resolved, taskRepoIDs []string) bool {
	return workflowWatched && projects.InProjects(taskRepoIDs)
}
