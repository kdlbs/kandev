package coordinator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// fakeProjects is a ProjectReader over maps that counts its reads and can fail
// each listing.
type fakeProjects struct {
	mu        sync.Mutex
	sets      []*taskmodels.RepositorySet
	repos     []*taskmodels.Repository
	taskRepos map[string][]string
	failSets  error
	failRepos error
	failTasks error
	reads     int
	onList    func()
}

func newFakeProjects() *fakeProjects { return &fakeProjects{taskRepos: map[string][]string{}} }

func (f *fakeProjects) addSet(id, name string, repoIDs ...string) {
	set := &taskmodels.RepositorySet{ID: id, WorkspaceID: "ws-1", Name: name}
	for _, r := range repoIDs {
		set.Items = append(set.Items, taskmodels.RepositorySetItem{RepositoryID: r})
	}
	f.sets = append(f.sets, set)
}

func (f *fakeProjects) addRepo(id, name string) {
	f.repos = append(f.repos, &taskmodels.Repository{ID: id, WorkspaceID: "ws-1", Name: name})
}

func (f *fakeProjects) readCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reads
}

func (f *fakeProjects) ListRepositorySets(context.Context, string) ([]*taskmodels.RepositorySet, error) {
	if f.onList != nil {
		f.onList()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	return f.sets, f.failSets
}

func (f *fakeProjects) ListRepositories(context.Context, string) ([]*taskmodels.Repository, error) {
	if f.onList != nil {
		f.onList()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	return f.repos, f.failRepos
}

func (f *fakeProjects) ListTaskRepositoryIDsByTaskIDs(_ context.Context, ids []string) (map[string][]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	if f.failTasks != nil {
		return nil, f.failTasks
	}
	out := map[string][]string{}
	for _, id := range ids {
		out[id] = f.taskRepos[id]
	}
	return out, nil
}

// projectScope stores a selected project scope for the coordinator.
func projectScope(t *testing.T, store *Store, coordinatorID string, includeNoRepo bool, entries ...ProjectEntry) {
	t.Helper()
	mustExec(t, store, `UPDATE coordinators SET project_scope = 'selected', include_no_repository = ? WHERE id = ?`, includeNoRepo, coordinatorID)
	for _, e := range entries {
		mustExec(t, store, `INSERT INTO coordinator_watch_projects (coordinator_id, entry_kind, entry_id, workspace_id, created_at) VALUES (?, ?, ?, 'ws-1', ?)`,
			coordinatorID, e.Kind, e.ID, time.Now().UTC())
	}
}

func setEntry(id string) ProjectEntry  { return ProjectEntry{Kind: projectKindSet, ID: id} }
func repoEntry(id string) ProjectEntry { return ProjectEntry{Kind: projectKindRepository, ID: id} }

func wireProjects(e *wakeEnv, f *fakeProjects) {
	e.svc.SetProjectReader(f)
}

func TestRecorder_ProjectScopeFiltersOwnTasks(t *testing.T) {
	e := newWakeEnv(t)
	p := newFakeProjects()
	p.addRepo("repo-a", "a")
	p.addRepo("repo-b", "b")
	wireProjects(e, p)
	for _, id := range []string{"t-a", "t-b", "t-none"} {
		e.ownTask(t, id)
		e.sources.state[id] = "COMPLETED"
	}
	p.taskRepos["t-a"] = []string{"repo-a"}
	p.taskRepos["t-b"] = []string{"repo-b"}
	projectScope(t, e.store, e.fix.c.ID, false, repoEntry("repo-a"))
	e.start(t)
	for _, id := range []string{"t-a", "t-b", "t-none"} {
		e.publish(t, events.TaskStateChanged, map[string]any{"task_id": id, "state": "COMPLETED"})
	}
	wantWakes(t, e, storedWake{"t-a", "completed", "completed", "pending"})

	mustExec(t, e.store, `UPDATE coordinators SET include_no_repository = 1 WHERE id = ?`, e.fix.c.ID)
	e.publish(t, events.TaskStateChanged, map[string]any{"task_id": "t-none", "state": "COMPLETED"})
	wantWakes(t, e,
		storedWake{"t-a", "completed", "completed", "pending"},
		storedWake{"t-none", "completed", "completed", "pending"})
}

func TestRecorder_SetMembershipWidensOnTheNextEvent(t *testing.T) {
	e := newWakeEnv(t)
	p := newFakeProjects()
	p.addRepo("repo-a", "a")
	p.addRepo("repo-c", "c")
	p.addSet("set-1", "Payments", "repo-a")
	wireProjects(e, p)
	e.ownTask(t, "t-c")
	e.sources.state["t-c"] = "COMPLETED"
	p.taskRepos["t-c"] = []string{"repo-c"}
	projectScope(t, e.store, e.fix.c.ID, false, setEntry("set-1"))
	e.start(t)
	e.publish(t, events.TaskStateChanged, map[string]any{"task_id": "t-c", "state": "COMPLETED"})
	wantWakes(t, e)
	p.sets[0].Items = append(p.sets[0].Items, taskmodels.RepositorySetItem{RepositoryID: "repo-c"})
	e.publish(t, events.TaskStateChanged, map[string]any{"task_id": "t-c", "state": "COMPLETED"})
	wantWakes(t, e, storedWake{"t-c", "completed", "completed", "pending"})
}

func TestRecorder_ResolverFailureDropsTheWakeAndCounts(t *testing.T) {
	e := newWakeEnv(t)
	p := newFakeProjects()
	p.addRepo("repo-a", "a")
	p.failRepos = errBoom
	wireProjects(e, p)
	e.ownTask(t, "t-a")
	e.sources.state["t-a"] = "COMPLETED"
	p.taskRepos["t-a"] = []string{"repo-a"}
	projectScope(t, e.store, e.fix.c.ID, true, repoEntry("repo-a"))
	e.start(t)
	before := wakeDroppedCounter(dropReasonReadError)
	e.publish(t, events.TaskStateChanged, map[string]any{"task_id": "t-a", "state": "COMPLETED"})
	wantWakes(t, e)
	if wakeDroppedCounter(dropReasonReadError) != before+1 {
		t.Fatal("read_error not counted")
	}
	p.failRepos = nil
	e.publish(t, events.TaskStateChanged, map[string]any{"task_id": "t-a", "state": "COMPLETED"})
	wantWakes(t, e, storedWake{"t-a", "completed", "completed", "pending"})
}

func TestRecorder_TaskRepositoryReadFailureTreatsTheTaskAsOutside(t *testing.T) {
	e := newWakeEnv(t)
	p := newFakeProjects()
	p.addRepo("repo-a", "a")
	p.failTasks = errBoom
	wireProjects(e, p)
	e.ownTask(t, "t-a")
	e.sources.state["t-a"] = "COMPLETED"
	projectScope(t, e.store, e.fix.c.ID, true, repoEntry("repo-a"))
	e.start(t)
	e.publish(t, events.TaskStateChanged, map[string]any{"task_id": "t-a", "state": "COMPLETED"})
	wantWakes(t, e)
}

func TestRecorder_EveryProjectScopeReadsNothing(t *testing.T) {
	e := newWakeEnv(t)
	p := newFakeProjects()
	p.failSets, p.failRepos, p.failTasks = errBoom, errBoom, errBoom
	wireProjects(e, p)
	e.ownTask(t, "t-a")
	e.sources.state["t-a"] = "COMPLETED"
	e.start(t)
	e.publish(t, events.TaskStateChanged, map[string]any{"task_id": "t-a", "state": "COMPLETED"})
	wantWakes(t, e, storedWake{"t-a", "completed", "completed", "pending"})
	if n := p.readCount(); n != 0 {
		t.Fatalf("reads under every project = %d, want 0", n)
	}
}

func TestBackstop_ProjectScopeFiltersAndResolverFailureSkipsTheCoordinator(t *testing.T) {
	e := newWakeEnv(t)
	p := newFakeProjects()
	p.addRepo("repo-a", "a")
	p.addRepo("repo-b", "b")
	wireProjects(e, p)
	for _, id := range []string{"t-a", "t-b"} {
		e.ownTask(t, id)
		e.sources.state[id] = "COMPLETED"
	}
	p.taskRepos["t-a"] = []string{"repo-a"}
	p.taskRepos["t-b"] = []string{"repo-b"}
	projectScope(t, e.store, e.fix.c.ID, false, repoEntry("repo-a"))

	p.failRepos = errBoom
	skipped := backstopSkippedTotal.Value()
	e.pass(t)
	wantWakes(t, e)
	if backstopSkippedTotal.Value() == skipped {
		t.Fatal("a resolver failure must be counted as a skipped pass")
	}

	p.failRepos = nil
	e.pass(t)
	wantWakes(t, e, storedWake{"t-a", "completed", "completed", "pending"})
}

func TestDeliver_ResolverFailureAbortsWithoutSupersedingAnything(t *testing.T) {
	f := newDeliverFixture(t)
	p := newFakeProjects()
	p.addRepo("repo-a", "a")
	f.svc.SetProjectReader(p)
	f.wake(t, "t1", "one", time.Now().UTC())
	f.wake(t, "t2", "two", time.Now().UTC().Add(time.Second))
	p.taskRepos["t1"] = []string{"repo-a"}
	p.taskRepos["t2"] = []string{"repo-b"}
	projectScope(t, f.store, f.c.ID, false, repoEntry("repo-a"))

	p.failRepos = errBoom
	before := deliveryAbortedCounter(abortReasonProjectsRead)
	if err := f.deliver(); err == nil {
		t.Fatal("deliver must report the resolver failure")
	}
	for _, id := range []string{"w-t1", "w-t2"} {
		if status, _ := f.wakeStatus(t, id); status != "pending" {
			t.Fatalf("%s = %s, want pending after an aborted recheck", id, status)
		}
	}
	if got := deliveryAbortedCounter(abortReasonProjectsRead); got != before+1 {
		t.Fatalf("aborted counter = %d, want %d", got, before+1)
	}

	p.failRepos = nil
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	if status, _ := f.wakeStatus(t, "w-t2"); status != "superseded" {
		t.Fatalf("w-t2 = %s, want superseded once the scope reads", status)
	}
	if status, _ := f.wakeStatus(t, "w-t1"); status != "delivered" {
		t.Fatalf("w-t1 = %s, want delivered", status)
	}
}

func TestProposeTask_ProjectScopeRefusesAnOutOfScopeRepository(t *testing.T) {
	f := newProposalTestFixture(t)
	createWorkflowsTable(t, f.svc.store)
	addWorkflow(t, f.svc.store, f.workflowID, f.workspaceID)
	svc := watchedProposalService(t, f)
	p := newFakeProjects()
	p.addRepo(f.repository.ID, "in")
	p.addRepo("repo-other", "out")
	svc.SetProjectReader(p)
	projectScope(t, svc.store, f.coordinator.ID, false, repoEntry(f.repository.ID))
	ctx := context.Background()

	req := f.baseRequest()
	if _, _, err := svc.ProposeTask(ctx, f.coordinator.ID, req); err == nil {
		t.Fatal("no repository with the toggle off must be refused")
	} else {
		wantProposalFieldError(t, err, "repository_id")
	}

	req.RepositoryID = f.repository.ID
	if _, _, err := svc.ProposeTask(ctx, f.coordinator.ID, req); err != nil {
		t.Fatalf("in-scope repository: %v", err)
	}

	mustExec(t, svc.store, `UPDATE coordinators SET include_no_repository = 1 WHERE id = ?`, f.coordinator.ID)
	req.RepositoryID = ""
	if _, _, err := svc.ProposeTask(ctx, f.coordinator.ID, req); err != nil {
		t.Fatalf("no repository with the toggle on: %v", err)
	}

	p.failRepos = errBoom
	req.RepositoryID = f.repository.ID
	if _, _, err := svc.ProposeTask(ctx, f.coordinator.ID, req); err == nil {
		t.Fatal("a resolver failure must refuse the proposal")
	}
}

func TestCountOpenWatchedTasks_ProjectScope(t *testing.T) {
	f := newGoalFixture(t)
	f.createTasksTable()
	f.addTask("t1", "wf1", "TODO", "manual", 0, false)
	f.addTask("t2", "wf1", "TODO", "manual", 0, false)
	f.addTask("t3", "wf1", "TODO", "manual", 0, false)
	p := newFakeProjects()
	p.addRepo("repo-a", "a")
	p.taskRepos["t1"] = []string{"repo-a"}
	p.taskRepos["t2"] = []string{"repo-b"}
	f.store.projects.Store(func() *ProjectReader { var r ProjectReader = p; return &r }())
	ctx := context.Background()

	count := func(set WatchSet) int64 {
		t.Helper()
		n, err := f.store.CountOpenWatchedTasks(ctx, f.store.ro, f.c.WorkspaceID, set)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	scope := func(noRepo bool) WatchSet {
		set := WatchSet{All: true}
		set.Projects.Selected, set.Projects.RepoIDs, set.Projects.IncludeNoRepo = true, []string{"repo-a"}, noRepo
		return set
	}
	if n := count(scope(false)); n != 1 {
		t.Fatalf("listed repository only = %d, want 1", n)
	}
	if n := count(scope(true)); n != 2 {
		t.Fatalf("with no-repository tasks = %d, want 2", n)
	}
	if n := count(WatchSet{All: true}); n != 3 {
		t.Fatalf("every project = %d, want 3", n)
	}
	p.failRepos = errBoom
	if _, err := f.store.CountOpenWatchedTasks(ctx, f.store.ro, f.c.WorkspaceID, scope(false)); err == nil {
		t.Fatal("a resolver failure must be an error, not a zero")
	}
}
