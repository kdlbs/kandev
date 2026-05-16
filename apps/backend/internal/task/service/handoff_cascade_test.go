package service

import (
	"context"
	"sync"
	"testing"
	"time"

	orchmodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
)

// fakeCascadeRepo extends phase4TaskRepo with archive/unarchive support
// keyed by id. This is a minimal fake — a single map-of-tasks is enough
// to exercise the cascade walk.
type fakeCascadeRepo struct {
	*phase4TaskRepo
}

func newCascadeRepo(base *fakeTaskRepo) *fakeCascadeRepo {
	return &fakeCascadeRepo{phase4TaskRepo: &phase4TaskRepo{base: base}}
}

func (r *fakeCascadeRepo) ArchiveTaskIfActive(_ context.Context, id, cascadeID string) (bool, error) {
	r.base.mu.Lock()
	defer r.base.mu.Unlock()
	t := r.base.tasks[id]
	if t == nil || t.ArchivedAt != nil {
		return false, nil
	}
	now := time.Now().UTC()
	t.ArchivedAt = &now
	t.ArchivedByCascadeID = cascadeID
	return true, nil
}

func (r *fakeCascadeRepo) UnarchiveTaskByCascade(_ context.Context, id, cascadeID string) (bool, error) {
	r.base.mu.Lock()
	defer r.base.mu.Unlock()
	t := r.base.tasks[id]
	if t == nil || t.ArchivedByCascadeID != cascadeID {
		return false, nil
	}
	t.ArchivedAt = nil
	t.ArchivedByCascadeID = ""
	return true, nil
}

// fakeWSGroupRepoCascade extends fakeWSGroupRepo with the phase 6
// release/restore/cleanup-status methods.
type fakeWSGroupRepoCascade struct {
	*fakeWSGroupRepo
	releaseCalls []struct {
		groupID, taskID, reason, cascadeID string
	}
	cleanupStatuses map[string]string
	// allMembers records every member that ever joined the group
	// (including released ones). Tests opt-in by populating it
	// directly — when empty, ListWorkspaceGroupMembers falls back to
	// the active members map.
	allMembers map[string]map[string]string
}

func newCascadeWSGroupRepo() *fakeWSGroupRepoCascade {
	return &fakeWSGroupRepoCascade{
		fakeWSGroupRepo: newFakeWSGroupRepo(),
		cleanupStatuses: map[string]string{},
		allMembers:      map[string]map[string]string{},
	}
}

func (f *fakeWSGroupRepoCascade) ListWorkspaceGroupMembers(ctx context.Context, groupID string) ([]orchmodels.WorkspaceGroupMember, error) {
	f.mu.Lock()
	if hist, ok := f.allMembers[groupID]; ok && len(hist) > 0 {
		out := make([]orchmodels.WorkspaceGroupMember, 0, len(hist))
		for tid, role := range hist {
			out = append(out, orchmodels.WorkspaceGroupMember{
				WorkspaceGroupID: groupID,
				TaskID:           tid,
				Role:             role,
			})
		}
		f.mu.Unlock()
		return out, nil
	}
	f.mu.Unlock()
	return f.fakeWSGroupRepo.ListWorkspaceGroupMembers(ctx, groupID)
}

func (f *fakeWSGroupRepoCascade) ReleaseWorkspaceGroupMember(_ context.Context, groupID, taskID, reason, cascadeID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.releaseCalls = append(f.releaseCalls, struct {
		groupID, taskID, reason, cascadeID string
	}{groupID, taskID, reason, cascadeID})
	// Mark the member released by removing it from the live members map.
	delete(f.members[groupID], taskID)
	return nil
}

func (f *fakeWSGroupRepoCascade) RestoreWorkspaceGroupMemberByCascade(_ context.Context, taskID, cascadeID string) error {
	// no-op for these tests
	_ = taskID
	_ = cascadeID
	return nil
}

func (f *fakeWSGroupRepoCascade) ListActiveWorkspaceGroupMembers(_ context.Context, groupID string) ([]orchmodels.WorkspaceGroupMember, error) {
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

func (f *fakeWSGroupRepoCascade) UpdateWorkspaceGroupCleanupStatus(_ context.Context, id, status, _ string, _ *time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleanupStatuses[id] = status
	return nil
}

// addArchivedTask seeds an already-archived task on the fake — used by
// the regression test for the resurrection bug.
func (f *fakeTaskRepo) addArchivedTask(id, parentID, ws, cascadeID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now().UTC().Add(-time.Hour) // archived earlier
	f.tasks[id] = &models.Task{
		ID:                  id,
		ParentID:            parentID,
		WorkspaceID:         ws,
		ArchivedAt:          &now,
		ArchivedByCascadeID: cascadeID,
	}
	if parentID != "" {
		f.children[parentID] = append(f.children[parentID], id)
	}
}

func newCascadeService(t *testing.T, tasks *fakeTaskRepo, ws *fakeWSGroupRepoCascade) *HandoffService {
	t.Helper()
	tr := newCascadeRepo(tasks)
	return NewHandoffService(tr, nil, nil, nil, ws, nil)
}

func TestArchiveTaskTree_StampsCascadeAcrossDescendants(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("c1", "root", "ws-1")
	tasks.addTask("c2", "root", "ws-1")
	tasks.addTask("g1", "c1", "ws-1")
	groups := newCascadeWSGroupRepo()
	svc := newCascadeService(t, tasks, groups)

	out, err := svc.ArchiveTaskTree(context.Background(), "root")
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if out.CascadeID == "" {
		t.Fatal("cascade ID should be set")
	}
	if len(out.ArchivedTaskIDs) != 4 {
		t.Errorf("archived = %d, want 4", len(out.ArchivedTaskIDs))
	}
	for _, id := range []string{"root", "c1", "c2", "g1"} {
		got, _ := tasks.GetTask(context.Background(), id)
		if got.ArchivedAt == nil {
			t.Errorf("%s should be archived", id)
		}
		if got.ArchivedByCascadeID != out.CascadeID {
			t.Errorf("%s cascade id = %q, want %q", id, got.ArchivedByCascadeID, out.CascadeID)
		}
	}
}

// REGRESSION: an unarchive cascade must NOT resurrect descendants that
// the user manually archived before the cascade ran. Phase 6 scopes
// restoration to tasks tagged with the same cascade ID.
func TestUnarchiveTaskTree_LeavesPriorlyArchivedDescendantsAlone(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("c1", "root", "ws-1")
	// c2 was manually archived BEFORE the cascade — different cascade id.
	tasks.addArchivedTask("c2", "root", "ws-1", "")
	groups := newCascadeWSGroupRepo()
	svc := newCascadeService(t, tasks, groups)

	archive, err := svc.ArchiveTaskTree(context.Background(), "root")
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	// c2 should not be in the cascade's archive set.
	for _, id := range archive.ArchivedTaskIDs {
		if id == "c2" {
			t.Error("c2 should not be re-archived by the cascade")
		}
	}

	if _, err := svc.UnarchiveTaskTree(context.Background(), "root"); err != nil {
		t.Fatalf("unarchive: %v", err)
	}
	// root + c1 should be active; c2 should still be archived because
	// its cascade id ("") does not match.
	for _, id := range []string{"root", "c1"} {
		got, _ := tasks.GetTask(context.Background(), id)
		if got.ArchivedAt != nil {
			t.Errorf("%s should be unarchived", id)
		}
	}
	c2, _ := tasks.GetTask(context.Background(), "c2")
	if c2.ArchivedAt == nil {
		t.Error("c2 should remain archived (different cascade id)")
	}
}

func TestArchiveTaskTree_ReleasesGroupMemberships(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("c1", "root", "ws-1")
	groups := newCascadeWSGroupRepo()
	groups.groups["g1"] = &orchmodels.WorkspaceGroup{
		ID: "g1", WorkspaceID: "ws-1", OwnerTaskID: "root",
		MaterializedKind: orchmodels.WorkspaceGroupKindSingleRepo,
		CleanupStatus:    orchmodels.WorkspaceCleanupStatusActive,
	}
	groups.members["g1"] = map[string]string{
		"root": orchmodels.WorkspaceMemberRoleOwner,
		"c1":   orchmodels.WorkspaceMemberRoleMember,
	}
	svc := newCascadeService(t, tasks, groups)

	out, err := svc.ArchiveTaskTree(context.Background(), "root")
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if len(out.ReleasedGroupIDs) != 1 || out.ReleasedGroupIDs[0] != "g1" {
		t.Errorf("ReleasedGroupIDs = %v", out.ReleasedGroupIDs)
	}
	if len(groups.releaseCalls) != 2 {
		t.Errorf("expected 2 release calls (root + c1), got %d", len(groups.releaseCalls))
	}
	for _, call := range groups.releaseCalls {
		if call.cascadeID != out.CascadeID {
			t.Errorf("release cascade id = %q, want %q", call.cascadeID, out.CascadeID)
		}
		if call.reason != orchmodels.WorkspaceReleaseReasonArchived {
			t.Errorf("release reason = %q", call.reason)
		}
	}
}

func TestEvaluateWorkspaceGroupCleanup_NoOpForUserOwned(t *testing.T) {
	tasks := newFakeTaskRepo()
	groups := newCascadeWSGroupRepo()
	groups.groups["g1"] = &orchmodels.WorkspaceGroup{
		ID: "g1", WorkspaceID: "ws-1", OwnerTaskID: "x",
		MaterializedKind: orchmodels.WorkspaceGroupKindPlainFolder,
		OwnedByKandev:    false, // user-owned
		CleanupPolicy:    orchmodels.WorkspaceCleanupPolicyNeverDelete,
		CleanupStatus:    orchmodels.WorkspaceCleanupStatusActive,
	}
	groups.members["g1"] = map[string]string{}
	svc := newCascadeService(t, tasks, groups)

	if err := svc.evaluateWorkspaceGroupCleanup(context.Background(), "g1"); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if status := groups.cleanupStatuses["g1"]; status != "" {
		t.Errorf("user-owned group cleanup_status was mutated to %q", status)
	}
}

func TestEvaluateWorkspaceGroupCleanup_PendingWhenLastMemberLeavesKandevGroup(t *testing.T) {
	tasks := newFakeTaskRepo()
	groups := newCascadeWSGroupRepo()
	groups.groups["g1"] = &orchmodels.WorkspaceGroup{
		ID: "g1", WorkspaceID: "ws-1", OwnerTaskID: "x",
		MaterializedKind: orchmodels.WorkspaceGroupKindSingleRepo,
		OwnedByKandev:    true,
		CleanupPolicy:    orchmodels.WorkspaceCleanupPolicyDeleteWhenLastMemberArchivedOrDel,
		CleanupStatus:    orchmodels.WorkspaceCleanupStatusActive,
	}
	groups.members["g1"] = map[string]string{}
	svc := newCascadeService(t, tasks, groups)

	if err := svc.evaluateWorkspaceGroupCleanup(context.Background(), "g1"); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if status := groups.cleanupStatuses["g1"]; status != orchmodels.WorkspaceCleanupStatusPending {
		t.Errorf("kandev-owned group cleanup_status = %q, want cleanup_pending", status)
	}
}

// fakeRunCanceller records every CancelTaskExecution call so cascade
// tests can verify run cancellation runs before archive / delete.
type fakeRunCanceller struct {
	mu     sync.Mutex
	calls  []string
	failOn map[string]error
}

func (f *fakeRunCanceller) CancelTaskExecution(_ context.Context, taskID, _ string, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, taskID)
	return f.failOn[taskID]
}

// fakeDeleteRepo extends fakeCascadeRepo with DeleteTask support so the
// delete cascade test can verify rows are actually removed.
type fakeDeleteRepo struct {
	*fakeCascadeRepo
}

func (r *fakeDeleteRepo) DeleteTask(_ context.Context, id string) error {
	r.base.mu.Lock()
	defer r.base.mu.Unlock()
	delete(r.base.tasks, id)
	return nil
}

// REGRESSION (post-review #4): a parent with already-archived children
// must still have those children deleted by the cascade. The original
// collectTaskTree used ListChildren which filtered archived rows; the
// fix is to use ListChildrenIncludingArchived for delete.
func TestDeleteTaskTree_IncludesArchivedDescendants(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("active-child", "root", "ws-1")
	// archived-child was manually archived BEFORE the delete cascade.
	tasks.addArchivedTask("archived-child", "root", "ws-1", "")
	groups := newCascadeWSGroupRepo()
	tr := &fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)}
	svc := NewHandoffService(tr, nil, nil, nil, groups, nil)

	out, err := svc.DeleteTaskTree(context.Background(), "root")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(out.ArchivedTaskIDs) != 3 {
		t.Errorf("expected 3 tasks deleted (root + active + archived); got %d (ids=%v)",
			len(out.ArchivedTaskIDs), out.ArchivedTaskIDs)
	}
	if _, exists := tasks.tasks["archived-child"]; exists {
		t.Error("archived-child must be removed by the delete cascade — leaving it behind is a data-loss bug")
	}
}

func TestDeleteTaskTree_RemovesAllAndCancelsRuns(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("c1", "root", "ws-1")
	tasks.addTask("g1", "c1", "ws-1")
	groups := newCascadeWSGroupRepo()
	groups.groups["grp"] = &orchmodels.WorkspaceGroup{
		ID: "grp", WorkspaceID: "ws-1", OwnerTaskID: "root",
		MaterializedKind: orchmodels.WorkspaceGroupKindSingleRepo,
		CleanupStatus:    orchmodels.WorkspaceCleanupStatusActive,
	}
	groups.members["grp"] = map[string]string{"root": orchmodels.WorkspaceMemberRoleOwner}
	tr := &fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)}
	svc := NewHandoffService(tr, nil, nil, nil, groups, nil)
	canceller := &fakeRunCanceller{}
	svc.SetRunCanceller(canceller)

	out, err := svc.DeleteTaskTree(context.Background(), "root")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(out.ArchivedTaskIDs) != 3 {
		t.Errorf("deleted count = %d, want 3", len(out.ArchivedTaskIDs))
	}
	for _, id := range []string{"root", "c1", "g1"} {
		if _, ok := tasks.tasks[id]; ok {
			t.Errorf("%s should be removed", id)
		}
	}
	// Run cancellation must have been called for every task in the cascade.
	if len(canceller.calls) != 3 {
		t.Errorf("expected 3 cancel calls, got %d", len(canceller.calls))
	}
	// Group membership released with reason=deleted.
	for _, c := range groups.releaseCalls {
		if c.taskID == "root" && c.reason != orchmodels.WorkspaceReleaseReasonDeleted {
			t.Errorf("root release reason = %q, want deleted", c.reason)
		}
	}
}

func TestArchiveTaskTree_CancelsRunsBeforeArchive(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("c1", "root", "ws-1")
	groups := newCascadeWSGroupRepo()
	svc := newCascadeService(t, tasks, groups)
	canceller := &fakeRunCanceller{}
	svc.SetRunCanceller(canceller)

	if _, err := svc.ArchiveTaskTree(context.Background(), "root"); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if len(canceller.calls) != 2 {
		t.Errorf("expected 2 cancel calls (root + c1), got %d", len(canceller.calls))
	}
}

// REGRESSION (post-review #5): cleanup must NOT proceed while a
// member session still has an executors_running row. Even when no
// active group members remain (all released by cascade), a lingering
// executor would be writing to the workspace we're about to delete.
// The safety gate parks the group in cleanup_pending with a clear
// reason instead.
func TestEvaluateWorkspaceGroupCleanup_BlockedByActiveExecutor(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	groups := newCascadeWSGroupRepo()
	groups.groups["g1"] = &orchmodels.WorkspaceGroup{
		ID: "g1", WorkspaceID: "ws-1", OwnerTaskID: "root",
		MaterializedKind: orchmodels.WorkspaceGroupKindSingleRepo,
		OwnedByKandev:    true,
		CleanupPolicy:    orchmodels.WorkspaceCleanupPolicyDeleteWhenLastMemberArchivedOrDel,
		CleanupStatus:    orchmodels.WorkspaceCleanupStatusActive,
	}
	// Active members list is empty (all released), but the
	// historical-members list still carries "root" so the gate can
	// inspect its sessions for lingering executors.
	groups.members["g1"] = map[string]string{}
	groups.allMembers = map[string]map[string]string{
		"g1": {"root": orchmodels.WorkspaceMemberRoleOwner},
	}

	sr := newFakeSessionReader()
	sr.sessions["root"] = []*models.TaskSession{{ID: "s1", IsPrimary: true, TaskID: "root"}}
	sr.sessionsWithRunningExecutor["s1"] = true

	tr := newCascadeRepo(tasks)
	svc := NewHandoffService(tr, nil, nil, nil, groups, nil)
	svc.SetSessionReader(sr)

	if err := svc.evaluateWorkspaceGroupCleanup(context.Background(), "g1"); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if status := groups.cleanupStatuses["g1"]; status != orchmodels.WorkspaceCleanupStatusPending {
		t.Errorf("cleanup_status = %q, want cleanup_pending (active executor blocks)", status)
	}
}

// Verify the cascade is race-free under concurrent invocations.
func TestArchiveTaskTree_RaceFree(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("c1", "root", "ws-1")
	tasks.addTask("c2", "root", "ws-1")
	groups := newCascadeWSGroupRepo()
	svc := newCascadeService(t, tasks, groups)

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = svc.ArchiveTaskTree(context.Background(), "root")
		}()
	}
	wg.Wait()
	// Whatever order the goroutines ran in, every task should be
	// archived exactly once (CAS guard) and the cascade IDs must be
	// consistent within each task — though different tasks may carry
	// different cascade IDs depending on which goroutine won the race.
	for _, id := range []string{"root", "c1", "c2"} {
		got, _ := tasks.GetTask(context.Background(), id)
		if got.ArchivedAt == nil {
			t.Errorf("%s should be archived", id)
		}
	}
}

// fakeEventPublisher captures the task IDs delivered to each cascade
// event-publish callback. It exists so the tests below can verify that
// ArchiveTaskTree / DeleteTaskTree actually invoke the publisher hook —
// without the hook firing, the gateway never broadcasts task.updated /
// task.deleted and the kanban board's All-Workflows view shows stale
// rows after a cascade.
type fakeEventPublisher struct {
	mu       sync.Mutex
	updated  []string
	deleted  []string
	archived []bool // archivedAt nil/non-nil per updated entry
}

func (f *fakeEventPublisher) PublishTaskUpdated(_ context.Context, task *models.Task) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updated = append(f.updated, task.ID)
	f.archived = append(f.archived, task.ArchivedAt != nil)
}

func (f *fakeEventPublisher) PublishTaskDeleted(_ context.Context, task *models.Task) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, task.ID)
}

// TestArchiveTaskTree_PublishesTaskUpdatedPerTask pins the regression
// the office HandoffService introduced: cascade-archive walks the repo
// directly and bypasses Service.ArchiveTask, which is the only caller
// of publishTaskEvent. Before the event-publisher wiring, the WS
// gateway never saw the cascade and the kanban board stayed populated
// with archived cards until a full reload.
func TestArchiveTaskTree_PublishesTaskUpdatedPerTask(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("c1", "root", "ws-1")
	tasks.addTask("c2", "root", "ws-1")
	groups := newCascadeWSGroupRepo()
	svc := newCascadeService(t, tasks, groups)
	pub := &fakeEventPublisher{}
	svc.SetTaskEventPublisher(pub)

	if _, err := svc.ArchiveTaskTree(context.Background(), "root"); err != nil {
		t.Fatalf("archive: %v", err)
	}

	pub.mu.Lock()
	defer pub.mu.Unlock()
	if got, want := len(pub.updated), 3; got != want {
		t.Fatalf("PublishTaskUpdated calls = %d, want %d (one per archived task)", got, want)
	}
	for i, taskID := range pub.updated {
		if !pub.archived[i] {
			t.Errorf("task %s published BEFORE archived_at was stamped; the WS handler keys off archived_at to remove the card", taskID)
		}
	}
	// Every task in the cascade must be in the published set; map keys
	// guarantee order-independent membership.
	want := map[string]bool{"root": true, "c1": true, "c2": true}
	for _, id := range pub.updated {
		delete(want, id)
	}
	if len(want) > 0 {
		t.Errorf("missing PublishTaskUpdated for: %v", want)
	}
}

// TestDeleteTaskTree_PublishesTaskDeletedPerTask is the delete-side
// equivalent of the archive test above. The cascade snapshots each
// task before the repository row is gone so the published event carries
// the workflow_id that the WS handler keys off to pick the right
// swimlane in All-Workflows view.
func TestDeleteTaskTree_PublishesTaskDeletedPerTask(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("c1", "root", "ws-1")
	groups := newCascadeWSGroupRepo()
	tr := &fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)}
	svc := NewHandoffService(tr, nil, nil, nil, groups, nil)
	pub := &fakeEventPublisher{}
	svc.SetTaskEventPublisher(pub)

	if _, err := svc.DeleteTaskTree(context.Background(), "root"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	pub.mu.Lock()
	defer pub.mu.Unlock()
	if got, want := len(pub.deleted), 2; got != want {
		t.Fatalf("PublishTaskDeleted calls = %d, want %d (one per deleted task)", got, want)
	}
	want := map[string]bool{"root": true, "c1": true}
	for _, id := range pub.deleted {
		delete(want, id)
	}
	if len(want) > 0 {
		t.Errorf("missing PublishTaskDeleted for: %v", want)
	}
}

// TestCascade_NilEventPublisher_NoCrash defends the optional-wiring
// contract: legacy / test setups that don't call SetTaskEventPublisher
// must still complete the cascade. Without this branch a future
// refactor could turn the publisher into a required dependency and
// break every test that doesn't wire it.
func TestCascade_NilEventPublisher_NoCrash(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("c1", "root", "ws-1")
	groups := newCascadeWSGroupRepo()
	tr := &fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)}
	svc := NewHandoffService(tr, nil, nil, nil, groups, nil)
	// Intentionally NOT calling SetTaskEventPublisher.

	if _, err := svc.ArchiveTaskTree(context.Background(), "root"); err != nil {
		t.Fatalf("archive with nil publisher: %v", err)
	}
	if _, err := svc.DeleteTaskTree(context.Background(), "root"); err != nil {
		t.Fatalf("delete with nil publisher: %v", err)
	}
}

// fakeResourceCleaner captures the (taskID, deleteEnvRow) pair delivered
// to CleanupTaskResources by each cascade invocation. Mirrors
// fakeEventPublisher above — the same regression class (cascade silently
// dropping a per-task callback) caused both the WS-event leak and the
// Docker-container leak.
type fakeResourceCleaner struct {
	mu    sync.Mutex
	calls []resourceCleanerCall
}

type resourceCleanerCall struct {
	taskID       string
	deleteEnvRow bool
}

func (f *fakeResourceCleaner) CleanupTaskResources(_ context.Context, taskID string, deleteEnvRow bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, resourceCleanerCall{taskID: taskID, deleteEnvRow: deleteEnvRow})
}

// TestArchiveTaskTree_InvokesResourceCleanerPerTask pins the regression
// where cascade-archive stopped active runs (which stops the agent /
// container) but never invoked the env-teardown branch — so containers
// kept existing on disk forever. Archive preserves the env row, so
// deleteEnvRow must be false on every call.
func TestArchiveTaskTree_InvokesResourceCleanerPerTask(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("c1", "root", "ws-1")
	tasks.addTask("c2", "root", "ws-1")
	groups := newCascadeWSGroupRepo()
	svc := newCascadeService(t, tasks, groups)
	cleaner := &fakeResourceCleaner{}
	svc.SetTaskResourceCleaner(cleaner)

	if _, err := svc.ArchiveTaskTree(context.Background(), "root"); err != nil {
		t.Fatalf("archive: %v", err)
	}

	cleaner.mu.Lock()
	defer cleaner.mu.Unlock()
	if got, want := len(cleaner.calls), 3; got != want {
		t.Fatalf("CleanupTaskResources calls = %d, want %d (one per archived task)", got, want)
	}
	want := map[string]bool{"root": true, "c1": true, "c2": true}
	for _, call := range cleaner.calls {
		if call.deleteEnvRow {
			t.Errorf("task %s: archive cascade passed deleteEnvRow=true, want false (archive preserves the env row)", call.taskID)
		}
		delete(want, call.taskID)
	}
	if len(want) > 0 {
		t.Errorf("missing CleanupTaskResources for: %v", want)
	}
}

// TestDeleteTaskTree_InvokesResourceCleanerPerTask is the delete-side
// equivalent: every deleted task gets its runtime resources torn down,
// and the env row is removed (deleteEnvRow=true) since the task itself
// is gone.
func TestDeleteTaskTree_InvokesResourceCleanerPerTask(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("c1", "root", "ws-1")
	groups := newCascadeWSGroupRepo()
	tr := &fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)}
	svc := NewHandoffService(tr, nil, nil, nil, groups, nil)
	cleaner := &fakeResourceCleaner{}
	svc.SetTaskResourceCleaner(cleaner)

	if _, err := svc.DeleteTaskTree(context.Background(), "root"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	cleaner.mu.Lock()
	defer cleaner.mu.Unlock()
	if got, want := len(cleaner.calls), 2; got != want {
		t.Fatalf("CleanupTaskResources calls = %d, want %d (one per deleted task)", got, want)
	}
	want := map[string]bool{"root": true, "c1": true}
	for _, call := range cleaner.calls {
		if !call.deleteEnvRow {
			t.Errorf("task %s: delete cascade passed deleteEnvRow=false, want true (the task is gone, the env row must follow)", call.taskID)
		}
		delete(want, call.taskID)
	}
	if len(want) > 0 {
		t.Errorf("missing CleanupTaskResources for: %v", want)
	}
}

// TestCascade_NilResourceCleaner_NoCrash defends the optional-wiring
// contract for the resource cleaner. Legacy / test setups that don't
// wire one must still complete the cascade — the cleaner is a runtime
// hook, not a correctness requirement of the cascade itself.
func TestCascade_NilResourceCleaner_NoCrash(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("c1", "root", "ws-1")
	groups := newCascadeWSGroupRepo()
	tr := &fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)}
	svc := NewHandoffService(tr, nil, nil, nil, groups, nil)
	// Intentionally NOT calling SetTaskResourceCleaner.

	if _, err := svc.ArchiveTaskTree(context.Background(), "root"); err != nil {
		t.Fatalf("archive with nil cleaner: %v", err)
	}
	if _, err := svc.DeleteTaskTree(context.Background(), "root"); err != nil {
		t.Fatalf("delete with nil cleaner: %v", err)
	}
}
