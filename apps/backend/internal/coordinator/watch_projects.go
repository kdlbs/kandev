package coordinator

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	taskmodels "github.com/kandev/kandev/internal/task/models"

	"github.com/kandev/kandev/internal/coordinator/watch"
)

// ProjectReader is the read surface the project scope resolves through: the
// workspace's repository sets and live repositories, and the repositories of
// tasks. Satisfied by an adapter over the task service.
type ProjectReader interface {
	ListRepositorySets(ctx context.Context, workspaceID string) ([]*taskmodels.RepositorySet, error)
	ListRepositories(ctx context.Context, workspaceID string) ([]*taskmodels.Repository, error)
	ListTaskRepositoryIDsByTaskIDs(ctx context.Context, taskIDs []string) (map[string][]string, error)
}

// SetProjectReader wires the project scope's reads; nil until then, and a
// selected scope without it fails closed.
func (s *Service) SetProjectReader(r ProjectReader) {
	s.wakeMu.Lock()
	s.projects = r
	s.wakeMu.Unlock()
	if r == nil {
		s.store.projects.Store(nil)
		return
	}
	s.store.projects.Store(&r)
}

func (s *Service) projectReader() ProjectReader {
	s.wakeMu.Lock()
	defer s.wakeMu.Unlock()
	return s.projects
}

// projectReads adapts a ProjectReader to watch.Reader.
type projectReads struct{ r ProjectReader }

func (p projectReads) Sets(ctx context.Context, workspaceID string) ([]watch.SetMembers, error) {
	sets, err := p.r.ListRepositorySets(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]watch.SetMembers, 0, len(sets))
	for _, set := range sets {
		if set != nil {
			out = append(out, watch.SetMembers{ID: set.ID, RepositoryIDs: set.RepositoryIDs()})
		}
	}
	return out, nil
}

func (p projectReads) RepositoryIDs(ctx context.Context, workspaceID string) ([]string, error) {
	repos, err := p.r.ListRepositories(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(repos))
	for _, repo := range repos {
		if repo != nil {
			out = append(out, repo.ID)
		}
	}
	return out, nil
}

// watchGate is an effective watch set with its project membership read once.
// Every enforcement path builds one and asks it, so the repository comparison
// is made only by watch.Task.
type watchGate struct {
	set      WatchSet
	projects watch.Resolved
}

// newWatchGate resolves the set's project scope. A scope of every project
// reads nothing; a failed read is the error and the caller fails closed.
func (s *Service) newWatchGate(ctx context.Context, set WatchSet, workspaceID string) (*watchGate, error) {
	return newWatchGateFrom(ctx, s.projectReader(), set, workspaceID)
}

func newWatchGateFrom(ctx context.Context, r ProjectReader, set WatchSet, workspaceID string) (*watchGate, error) {
	resolved, err := newWatchGateFromScope(ctx, r, workspaceID, set.Projects)
	if err != nil {
		return nil, err
	}
	return &watchGate{set: set, projects: resolved}, nil
}

// needsRepositories reports whether task repositories must be read at all.
func (g *watchGate) needsRepositories() bool { return g.projects.Selected() }

// task reports whether a task in workflowID with repoIDs is watched.
func (g *watchGate) task(workflowID string, repoIDs []string) bool {
	return watch.Task(g.set.Contains(workflowID), g.projects, repoIDs)
}

// taskRepositoryIDs reads the repositories of taskIDs in one read, or nothing
// when the scope does not need them.
func (s *Service) taskRepositoryIDs(ctx context.Context, g *watchGate, taskIDs []string) (map[string][]string, error) {
	if !g.needsRepositories() {
		return nil, nil
	}
	r := s.projectReader()
	if r == nil {
		return nil, fmt.Errorf("coordinator: project reads are not wired")
	}
	return r.ListTaskRepositoryIDsByTaskIDs(ctx, taskIDs)
}

// taskWatched reports whether one task is in the coordinator's Watches. A
// failed resolver read is the error; a failed read of the task's own
// repositories counts as outside, logged.
func (s *Service) taskWatched(ctx context.Context, set WatchSet, workspaceID, taskID, workflowID string) (bool, error) {
	g, err := s.newWatchGate(ctx, set, workspaceID)
	if err != nil {
		return false, err
	}
	repos, err := s.taskRepositoryIDs(ctx, g, []string{taskID})
	if err != nil {
		s.logger.Warn("coordinator: task repository read failed; treating task as outside the projects",
			zap.String("task_id", taskID), zap.Error(err))
		return false, nil
	}
	return g.task(workflowID, repos[taskID]), nil
}

// TaskWatched reports whether the task is in the coordinator's Watches.
func (s *Service) TaskWatched(ctx context.Context, coordinatorID, taskID, workflowID string) (bool, error) {
	c, err := s.store.GetCoordinatorByID(ctx, coordinatorID)
	if err != nil {
		return false, err
	}
	set, err := s.store.EffectiveWatchSet(ctx, s.store.ro, c.ID, c.WorkspaceID)
	if err != nil {
		return false, err
	}
	return s.taskWatched(ctx, set, c.WorkspaceID, taskID, workflowID)
}

// TaskRef names a task and its workflow for a batch watch check.
type TaskRef struct{ ID, WorkflowID string }

// WatchedTaskIDs returns the ids of refs that are in the coordinator's
// Watches. A failed resolver read is the error; a failed read of the tasks'
// repositories leaves every task outside, logged.
func (s *Service) WatchedTaskIDs(ctx context.Context, coordinatorID string, refs []TaskRef) (map[string]bool, error) {
	c, err := s.store.GetCoordinatorByID(ctx, coordinatorID)
	if err != nil {
		return nil, err
	}
	set, err := s.store.EffectiveWatchSet(ctx, s.store.ro, c.ID, c.WorkspaceID)
	if err != nil {
		return nil, err
	}
	g, err := s.newWatchGate(ctx, set, c.WorkspaceID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(refs))
	for _, r := range refs {
		ids = append(ids, r.ID)
	}
	repos, err := s.taskRepositoryIDs(ctx, g, ids)
	if err != nil {
		s.logger.Warn("coordinator: task repository read failed; treating tasks as outside the projects", zap.Error(err))
		return map[string]bool{}, nil
	}
	out := make(map[string]bool, len(refs))
	for _, r := range refs {
		if g.task(r.WorkflowID, repos[r.ID]) {
			out[r.ID] = true
		}
	}
	return out, nil
}
