package service

import (
	"context"
	"sync"
	"testing"
	"time"

	orchmodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// fakeWSGroupRepo is an in-memory implementation of WorkspaceGroupRepo
// covering the slim surface AttachWorkspacePolicy needs.
type fakeWSGroupRepo struct {
	groups   map[string]*orchmodels.WorkspaceGroup
	members  map[string]map[string]string // groupID -> taskID -> role
	mu       sync.Mutex
	createID int
}

func newFakeWSGroupRepo() *fakeWSGroupRepo {
	return &fakeWSGroupRepo{
		groups:  map[string]*orchmodels.WorkspaceGroup{},
		members: map[string]map[string]string{},
	}
}

func (f *fakeWSGroupRepo) CreateWorkspaceGroup(_ context.Context, g *orchmodels.WorkspaceGroup) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if g.ID == "" {
		f.createID++
		g.ID = "g-" + itoa(f.createID)
	}
	if g.CleanupStatus == "" {
		g.CleanupStatus = orchmodels.WorkspaceCleanupStatusActive
	}
	f.groups[g.ID] = g
	f.members[g.ID] = map[string]string{}
	return nil
}

func (f *fakeWSGroupRepo) GetWorkspaceGroup(_ context.Context, id string) (*orchmodels.WorkspaceGroup, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.groups[id], nil
}

func (f *fakeWSGroupRepo) GetWorkspaceGroupForTask(_ context.Context, taskID string) (*orchmodels.WorkspaceGroup, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for gid, mem := range f.members {
		if _, ok := mem[taskID]; ok {
			return f.groups[gid], nil
		}
	}
	return nil, nil
}

func (f *fakeWSGroupRepo) AddWorkspaceGroupMember(_ context.Context, groupID, taskID, role string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.members[groupID] == nil {
		f.members[groupID] = map[string]string{}
	}
	if _, ok := f.members[groupID][taskID]; !ok {
		f.members[groupID][taskID] = role
	}
	return nil
}

// Phase 6 surface — base implementation. The cascade-specific
// fakeWSGroupRepoCascade overrides Release/List/UpdateStatus.
func (f *fakeWSGroupRepo) ReleaseWorkspaceGroupMember(_ context.Context, groupID, taskID, _, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.members[groupID] != nil {
		delete(f.members[groupID], taskID)
	}
	return nil
}

func (f *fakeWSGroupRepo) RestoreWorkspaceGroupMemberByCascade(_ context.Context, _, _ string) error {
	return nil
}

func (f *fakeWSGroupRepo) ListActiveWorkspaceGroupMembers(_ context.Context, groupID string) ([]orchmodels.WorkspaceGroupMember, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []orchmodels.WorkspaceGroupMember{}
	for tid, role := range f.members[groupID] {
		out = append(out, orchmodels.WorkspaceGroupMember{
			WorkspaceGroupID: groupID,
			TaskID:           tid,
			Role:             role,
		})
	}
	return out, nil
}

// ListWorkspaceGroupMembers returns the same set as the Active variant
// in the fake — tests that need to distinguish released vs active
// override this on fakeWSGroupRepoCascade.
func (f *fakeWSGroupRepo) ListWorkspaceGroupMembers(_ context.Context, groupID string) ([]orchmodels.WorkspaceGroupMember, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []orchmodels.WorkspaceGroupMember{}
	for tid, role := range f.members[groupID] {
		out = append(out, orchmodels.WorkspaceGroupMember{
			WorkspaceGroupID: groupID,
			TaskID:           tid,
			Role:             role,
		})
	}
	return out, nil
}

func (f *fakeWSGroupRepo) UpdateWorkspaceGroupCleanupStatus(_ context.Context, _, _, _ string, _ *time.Time) error {
	return nil
}

func (f *fakeWSGroupRepo) MarkWorkspaceMaterialized(_ context.Context, id string, m orchmodels.MaterializedWorkspace) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	g := f.groups[id]
	if g == nil {
		return nil
	}
	g.MaterializedPath = m.Path
	g.MaterializedEnvironmentID = m.EnvironmentID
	g.MaterializedKind = m.Kind
	g.OwnedByKandev = m.OwnedByKandev
	g.RestoreConfigJSON = m.RestoreConfig
	if m.OwnedByKandev {
		g.CleanupPolicy = orchmodels.WorkspaceCleanupPolicyDeleteWhenLastMemberArchivedOrDel
	} else {
		g.CleanupPolicy = orchmodels.WorkspaceCleanupPolicyNeverDelete
	}
	return nil
}

func (f *fakeWSGroupRepo) UpdateWorkspaceGroupRestoreStatus(_ context.Context, _, _, _ string) error {
	return nil
}

// fakeBlockerRepo is a tiny in-memory implementation of BlockerRepository
// so AttachWorkspacePolicy's sequential-chain logic can be exercised.
type fakeBlockerRepo struct {
	mu       sync.Mutex
	blockers []*orchmodels.TaskBlocker
}

func (f *fakeBlockerRepo) CreateTaskBlocker(_ context.Context, b *orchmodels.TaskBlocker) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.blockers = append(f.blockers, b)
	return nil
}

func (f *fakeBlockerRepo) ListTaskBlockers(_ context.Context, taskID string) ([]*orchmodels.TaskBlocker, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []*orchmodels.TaskBlocker{}
	for _, b := range f.blockers {
		if b.TaskID == taskID {
			out = append(out, b)
		}
	}
	return out, nil
}

func (f *fakeBlockerRepo) DeleteTaskBlocker(_ context.Context, taskID, blockerTaskID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, b := range f.blockers {
		if b.TaskID == taskID && b.BlockerTaskID == blockerTaskID {
			f.blockers = append(f.blockers[:i], f.blockers[i+1:]...)
			return nil
		}
	}
	return nil
}

func (f *fakeBlockerRepo) ListTasksBlockedBy(_ context.Context, blockerTaskID string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var ids []string
	for _, b := range f.blockers {
		if b.BlockerTaskID == blockerTaskID {
			ids = append(ids, b.TaskID)
		}
	}
	return ids, nil
}

// fakeTaskRepo provides the minimal TaskRepository surface AttachWorkspacePolicy
// uses. It supports GetTask + ListChildren so sibling lookup works.
type fakeTaskRepo struct {
	mu       sync.Mutex
	tasks    map[string]*models.Task
	children map[string][]string // parentID -> ordered child IDs
}

func newFakeTaskRepo() *fakeTaskRepo {
	return &fakeTaskRepo{
		tasks:    map[string]*models.Task{},
		children: map[string][]string{},
	}
}

func (f *fakeTaskRepo) addTask(id, parentID, ws string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tasks[id] = &models.Task{ID: id, ParentID: parentID, WorkspaceID: ws}
	if parentID != "" {
		f.children[parentID] = append(f.children[parentID], id)
	}
}

func (f *fakeTaskRepo) GetTask(_ context.Context, id string) (*models.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.tasks[id], nil
}

func (f *fakeTaskRepo) ListChildren(_ context.Context, parentID string) ([]*models.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []*models.Task{}
	for _, id := range f.children[parentID] {
		if t, ok := f.tasks[id]; ok && t.ArchivedAt == nil {
			out = append(out, t)
		}
	}
	return out, nil
}

func (f *fakeTaskRepo) ListChildrenIncludingArchived(_ context.Context, parentID string) ([]*models.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []*models.Task{}
	for _, id := range f.children[parentID] {
		if t, ok := f.tasks[id]; ok {
			out = append(out, t)
		}
	}
	return out, nil
}

// Stubs to satisfy repository.TaskRepository — only the methods actually
// used by AttachWorkspacePolicy do real work.
func (f *fakeTaskRepo) ListSiblings(context.Context, string) ([]*models.Task, error) {
	return nil, nil
}

func newPhase4Service(t *testing.T, fakeTasks *fakeTaskRepo, blockers BlockerRepository, ws WorkspaceGroupRepo) *HandoffService {
	t.Helper()
	tr := &phase4TaskRepo{base: fakeTasks}
	return NewHandoffService(tr, nil, nil, blockers, ws, nil)
}

// phase4TaskRepo satisfies repository.TaskRepository by delegating the
// methods AttachWorkspacePolicy actually calls and panicking on the rest
// (which would catch accidental new dependencies in tests).
type phase4TaskRepo struct {
	base *fakeTaskRepo
}

func (r *phase4TaskRepo) GetTask(ctx context.Context, id string) (*models.Task, error) {
	return r.base.GetTask(ctx, id)
}
func (r *phase4TaskRepo) ListChildren(ctx context.Context, parentID string) ([]*models.Task, error) {
	return r.base.ListChildren(ctx, parentID)
}
func (r *phase4TaskRepo) ListChildrenIncludingArchived(ctx context.Context, parentID string) ([]*models.Task, error) {
	return r.base.ListChildrenIncludingArchived(ctx, parentID)
}

// All other TaskRepository methods panic — the AttachWorkspacePolicy
// path should only call GetTask + ListChildren. If a future change
// reaches into another method the panic forces a deliberate update.
func (r *phase4TaskRepo) panicNotUsed(name string) {
	panic("phase4TaskRepo: unexpected call to " + name)
}

// The full TaskRepository interface has many methods; we panic-stub them so
// the compiler is happy. In tests we only ever drive AttachWorkspacePolicy.
func (r *phase4TaskRepo) CreateTask(context.Context, *models.Task) error {
	r.panicNotUsed("CreateTask")
	return nil
}
func (r *phase4TaskRepo) UpdateTask(context.Context, *models.Task) error {
	r.panicNotUsed("UpdateTask")
	return nil
}
func (r *phase4TaskRepo) DeleteTask(context.Context, string) error {
	r.panicNotUsed("DeleteTask")
	return nil
}
func (r *phase4TaskRepo) ListTasks(context.Context, string) ([]*models.Task, error) {
	r.panicNotUsed("ListTasks")
	return nil, nil
}
func (r *phase4TaskRepo) ListTasksByWorkspace(context.Context, string, string, string, string, int, int, bool, bool, bool, bool) ([]*models.Task, int, error) {
	r.panicNotUsed("ListTasksByWorkspace")
	return nil, 0, nil
}
func (r *phase4TaskRepo) ListTasksByWorkflowStep(context.Context, string) ([]*models.Task, error) {
	r.panicNotUsed("ListTasksByWorkflowStep")
	return nil, nil
}
func (r *phase4TaskRepo) ArchiveTask(context.Context, string) error {
	r.panicNotUsed("ArchiveTask")
	return nil
}
func (r *phase4TaskRepo) ArchiveTaskIfActive(context.Context, string, string) (bool, error) {
	return false, nil
}
func (r *phase4TaskRepo) UnarchiveTaskByCascade(context.Context, string, string) (bool, error) {
	return false, nil
}
func (r *phase4TaskRepo) ListTasksForAutoArchive(context.Context) ([]*models.Task, error) {
	r.panicNotUsed("ListTasksForAutoArchive")
	return nil, nil
}
func (r *phase4TaskRepo) UpdateTaskState(context.Context, string, v1.TaskState) error {
	r.panicNotUsed("UpdateTaskState")
	return nil
}
func (r *phase4TaskRepo) CountTasksByWorkflow(context.Context, string) (int, error) {
	r.panicNotUsed("CountTasksByWorkflow")
	return 0, nil
}
func (r *phase4TaskRepo) CountTasksByWorkflowStep(context.Context, string) (int, error) {
	r.panicNotUsed("CountTasksByWorkflowStep")
	return 0, nil
}
func (r *phase4TaskRepo) AddTaskToWorkflow(context.Context, string, string, string, int) error {
	r.panicNotUsed("AddTaskToWorkflow")
	return nil
}
func (r *phase4TaskRepo) RemoveTaskFromWorkflow(context.Context, string, string) error {
	r.panicNotUsed("RemoveTaskFromWorkflow")
	return nil
}
func (r *phase4TaskRepo) ListTasksByProject(context.Context, string) ([]*models.Task, error) {
	r.panicNotUsed("ListTasksByProject")
	return nil, nil
}
func (r *phase4TaskRepo) ListTasksByAssignee(context.Context, string) ([]*models.Task, error) {
	r.panicNotUsed("ListTasksByAssignee")
	return nil, nil
}
func (r *phase4TaskRepo) ListTaskTree(context.Context, string, models.TaskTreeFilters) ([]*models.Task, error) {
	r.panicNotUsed("ListTaskTree")
	return nil, nil
}
func (r *phase4TaskRepo) ListSiblings(ctx context.Context, taskID string) ([]*models.Task, error) {
	// Implemented via the fake's data so phase 7 GetTaskContext tests
	// can exercise the sibling rule without relying on the real repo.
	r.base.mu.Lock()
	self := r.base.tasks[taskID]
	r.base.mu.Unlock()
	if self == nil || self.ParentID == "" {
		return []*models.Task{}, nil
	}
	siblings, err := r.base.ListChildren(ctx, self.ParentID)
	if err != nil {
		return nil, err
	}
	out := make([]*models.Task, 0, len(siblings))
	for _, s := range siblings {
		if s.ID != taskID {
			out = append(out, s)
		}
	}
	return out, nil
}
func (r *phase4TaskRepo) IncrementTaskSequence(context.Context, string) (int, error) {
	r.panicNotUsed("IncrementTaskSequence")
	return 0, nil
}
func (r *phase4TaskRepo) GetWorkspaceTaskPrefix(context.Context, string) (string, string, error) {
	r.panicNotUsed("GetWorkspaceTaskPrefix")
	return "", "", nil
}

func TestWorkspacePolicy_MetadataBlock(t *testing.T) {
	pol := WorkspacePolicy{
		Mode:                  "inherit_parent",
		DefaultChildWorkspace: "new_workspace",
		DefaultChildOrdering:  "sequential",
	}
	meta := pol.MetadataBlock()
	ws, ok := meta["workspace"].(map[string]interface{})
	if !ok {
		t.Fatalf("workspace map missing: %#v", meta)
	}
	if ws["mode"] != "inherit_parent" {
		t.Errorf("mode = %v", ws["mode"])
	}
	if ws["default_child_workspace"] != "new_workspace" {
		t.Errorf("default_child_workspace = %v", ws["default_child_workspace"])
	}
	if ws["default_child_ordering"] != "sequential" {
		t.Errorf("default_child_ordering = %v", ws["default_child_ordering"])
	}

	empty := WorkspacePolicy{Mode: "new_workspace"}
	got := empty.MetadataBlock()
	if got == nil {
		t.Fatal("non-empty policy must return a metadata block")
	}
	none := WorkspacePolicy{}
	if got := none.MetadataBlock(); got != nil {
		t.Fatalf("empty policy should return nil, got %v", got)
	}
}

func TestWorkspacePolicy_NeedsAttachment(t *testing.T) {
	cases := []struct {
		name string
		pol  WorkspacePolicy
		want bool
	}{
		{"new_workspace + parallel parent", WorkspacePolicy{Mode: "new_workspace"}, false},
		{"inherit_parent", WorkspacePolicy{Mode: "inherit_parent"}, true},
		{"shared_group", WorkspacePolicy{Mode: "shared_group", GroupID: "g1"}, true},
		{"new_workspace + sequential parent", WorkspacePolicy{Mode: "new_workspace", ParentOrdering: "sequential"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.pol.NeedsAttachment(); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAttachWorkspacePolicy_InheritParentCreatesGroupAndJoinsChild(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("parent", "", "ws-1")
	tasks.addTask("child", "parent", "ws-1")
	groups := newFakeWSGroupRepo()
	svc := newPhase4Service(t, tasks, &fakeBlockerRepo{}, groups)

	pol := WorkspacePolicy{Mode: "inherit_parent"}
	if err := svc.AttachWorkspacePolicy(context.Background(), "child", "parent", pol); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if len(groups.groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups.groups))
	}
	var gid string
	for id := range groups.groups {
		gid = id
	}
	if groups.members[gid]["parent"] != orchmodels.WorkspaceMemberRoleOwner {
		t.Errorf("parent should be owner, got %q", groups.members[gid]["parent"])
	}
	if groups.members[gid]["child"] != orchmodels.WorkspaceMemberRoleMember {
		t.Errorf("child should be member, got %q", groups.members[gid]["child"])
	}

	// A second inherit_parent child reuses the existing group.
	tasks.addTask("child2", "parent", "ws-1")
	if err := svc.AttachWorkspacePolicy(context.Background(), "child2", "parent", pol); err != nil {
		t.Fatalf("attach 2: %v", err)
	}
	if len(groups.groups) != 1 {
		t.Errorf("group must be reused; got %d groups", len(groups.groups))
	}
	if _, ok := groups.members[gid]["child2"]; !ok {
		t.Error("child2 should be added to the same group")
	}
}

func TestAttachWorkspacePolicy_SharedGroupRefusesInactive(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("child", "parent", "ws-1")
	groups := newFakeWSGroupRepo()
	g := &orchmodels.WorkspaceGroup{
		ID: "g-cleaned", WorkspaceID: "ws-1", OwnerTaskID: "x",
		MaterializedKind: orchmodels.WorkspaceGroupKindSingleRepo,
		CleanupStatus:    orchmodels.WorkspaceCleanupStatusCleaned,
	}
	groups.groups[g.ID] = g
	groups.members[g.ID] = map[string]string{}
	svc := newPhase4Service(t, tasks, &fakeBlockerRepo{}, groups)
	err := svc.AttachWorkspacePolicy(context.Background(), "child", "parent",
		WorkspacePolicy{Mode: "shared_group", GroupID: "g-cleaned"})
	if err == nil {
		t.Fatal("expected error for cleaned group")
	}
}

func TestAttachWorkspacePolicy_SequentialChainsSiblings(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("parent", "", "ws-1")
	blockers := &fakeBlockerRepo{}
	svc := newPhase4Service(t, tasks, blockers, newFakeWSGroupRepo())

	pol := WorkspacePolicy{Mode: "new_workspace", ParentOrdering: "sequential"}
	// Attach in creation order — the production caller (handleCreateTask)
	// always invokes attach immediately after creating the task, so the
	// child is the most recently-added sibling at attach time.
	tasks.addTask("a", "parent", "ws-1")
	if err := svc.AttachWorkspacePolicy(context.Background(), "a", "parent", pol); err != nil {
		t.Fatalf("attach a: %v", err)
	}
	tasks.addTask("b", "parent", "ws-1")
	// Attach for b: b should be blocked-by a.
	if err := svc.AttachWorkspacePolicy(context.Background(), "b", "parent", pol); err != nil {
		t.Fatalf("attach b: %v", err)
	}
	tasks.addTask("c", "parent", "ws-1")
	// Attach for c: c should be blocked-by b.
	if err := svc.AttachWorkspacePolicy(context.Background(), "c", "parent", pol); err != nil {
		t.Fatalf("attach c: %v", err)
	}

	if len(blockers.blockers) != 2 {
		t.Fatalf("expected 2 blocker edges, got %d", len(blockers.blockers))
	}
	bb, _ := blockers.ListTaskBlockers(context.Background(), "b")
	if len(bb) != 1 || bb[0].BlockerTaskID != "a" {
		t.Errorf("b blockers = %+v", bb)
	}
	cb, _ := blockers.ListTaskBlockers(context.Background(), "c")
	if len(cb) != 1 || cb[0].BlockerTaskID != "b" {
		t.Errorf("c blockers = %+v", cb)
	}
}

// TestAttachWorkspacePolicy_SequentialIsRaceFree exercises the per-parent
// mutex by attaching multiple siblings concurrently. The blocker chain
// should be deterministic (no duplicate or self-blocking edges).
//
// Real callers (handleCreateTask) call addTask + attach atomically per
// sibling; we model that here by adding+attaching each goroutine in one
// critical section so the contention is over the per-parent attach lock.
func TestAttachWorkspacePolicy_SequentialIsRaceFree(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("parent", "", "ws-1")
	blockers := &fakeBlockerRepo{}
	svc := newPhase4Service(t, tasks, blockers, newFakeWSGroupRepo())

	pol := WorkspacePolicy{Mode: "new_workspace", ParentOrdering: "sequential"}
	const n = 8
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			tasks.addTask(id, "parent", "ws-1")
			_ = svc.AttachWorkspacePolicy(context.Background(), id, "parent", pol)
		}("k" + itoa(i))
	}
	wg.Wait()

	// Under raw concurrency the first task to claim the per-parent lock
	// may already see one or more siblings (their addTask raced ahead),
	// so the count of blockers depends on interleaving — anywhere from
	// n-1 (deterministic chain) up to n (every attach saw a previous
	// sibling). The contract we DO enforce is: no self-edges, and each
	// sibling has at most one blocker (the lock prevents double-attach).
	if len(blockers.blockers) > n {
		t.Errorf("blocker edges = %d, want <= %d", len(blockers.blockers), n)
	}
	for _, b := range blockers.blockers {
		if b.TaskID == b.BlockerTaskID {
			t.Errorf("self-blocker detected: %s", b.TaskID)
		}
	}
	seen := map[string]bool{}
	for _, b := range blockers.blockers {
		if seen[b.TaskID] {
			t.Errorf("task %s has more than one blocker (lock leaked)", b.TaskID)
		}
		seen[b.TaskID] = true
	}
}
