package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	orchmodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/archivecascade"
	"github.com/kandev/kandev/internal/task/models"
)

// fakeCascadeRepo extends phase4TaskRepo with archive/unarchive support
// keyed by id. This is a minimal fake — a single map-of-tasks is enough
// to exercise the cascade walk.
type fakeCascadeRepo struct {
	*phase4TaskRepo
	autoArchiveCASLoss bool
	unarchiveErr       error
}

func newCascadeRepo(base *fakeTaskRepo) *fakeCascadeRepo {
	return &fakeCascadeRepo{phase4TaskRepo: &phase4TaskRepo{base: base}}
}

func (r *fakeCascadeRepo) ArchiveTaskIfActive(ctx context.Context, id, cascadeID string) (bool, error) {
	_, changed, err := r.ArchiveTaskIfActiveWithVacatedStep(ctx, id, cascadeID)
	return changed, err
}

func (r *fakeCascadeRepo) ArchiveTaskIfAutoArchiveEligible(
	ctx context.Context,
	id string,
	_ time.Time,
	cascadeID string,
) (bool, error) {
	if r.autoArchiveCASLoss {
		return false, nil
	}
	return r.ArchiveTaskIfActive(ctx, id, cascadeID)
}

func (r *fakeCascadeRepo) ArchiveTaskIfActiveWithVacatedStep(
	_ context.Context,
	id string,
	cascadeID string,
) (string, bool, error) {
	r.base.mu.Lock()
	defer r.base.mu.Unlock()
	t := r.base.tasks[id]
	if t == nil || t.ArchivedAt != nil {
		return "", false, nil
	}
	now := time.Now().UTC()
	t.ArchivedAt = &now
	t.ArchivedByCascadeID = cascadeID
	return t.WorkflowStepID, true, nil
}
func (r *fakeCascadeRepo) UnarchiveTaskByCascade(_ context.Context, id, cascadeID string) (bool, error) {
	r.base.mu.Lock()
	defer r.base.mu.Unlock()
	if r.unarchiveErr != nil {
		return false, r.unarchiveErr
	}
	t := r.base.tasks[id]
	if t == nil || t.ArchivedByCascadeID != cascadeID {
		return false, nil
	}
	t.ArchivedAt = nil
	t.ArchivedByCascadeID = ""
	return true, nil
}

func (r *fakeCascadeRepo) UnarchiveTask(_ context.Context, id string) (bool, error) {
	r.base.mu.Lock()
	defer r.base.mu.Unlock()
	t := r.base.tasks[id]
	// Mirror the production CAS: only rows without a cascade stamp are
	// restorable through the manual path.
	if t == nil || t.ArchivedAt == nil || t.ArchivedByCascadeID != "" {
		return false, nil
	}
	t.ArchivedAt = nil
	return true, nil
}

// fakeWSGroupRepoCascade extends fakeWSGroupRepo with the phase 6

type archiveSerializationProbe struct {
	*fakeCascadeRepo
	firstMutationStarted  chan struct{}
	secondMutationStarted chan struct{}
	releaseFirstMutation  chan struct{}
	mu                    sync.Mutex
	changedMutations      int
}

func (r *archiveSerializationProbe) ArchiveTaskIfActiveWithVacatedStep(
	ctx context.Context,
	id, cascadeID string,
) (string, bool, error) {
	stepID, changed, err := r.fakeCascadeRepo.ArchiveTaskIfActiveWithVacatedStep(ctx, id, cascadeID)
	if err != nil || !changed {
		return stepID, changed, err
	}
	r.mu.Lock()
	r.changedMutations++
	mutation := r.changedMutations
	r.mu.Unlock()
	switch mutation {
	case 1:
		close(r.firstMutationStarted)
		<-r.releaseFirstMutation
	case 2:
		close(r.secondMutationStarted)
	}
	return stepID, changed, nil
}

// release/restore/cleanup-status methods.
type fakeWSGroupRepoCascade struct {
	*fakeWSGroupRepo
	releaseErr      error
	releaseErrAfter int
	restoreErr      error
	releaseCalls    []struct {
		groupID, taskID, reason, cascadeID string
	}
	restoreCalls []struct {
		taskID, cascadeID string
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
	if f.releaseErr != nil && (f.releaseErrAfter == 0 || len(f.releaseCalls) >= f.releaseErrAfter) {
		return f.releaseErr
	}
	f.releaseCalls = append(f.releaseCalls, struct {
		groupID, taskID, reason, cascadeID string
	}{groupID, taskID, reason, cascadeID})
	// Mark the member released by removing it from the live members map.
	delete(f.members[groupID], taskID)
	return nil
}

type recordingCleanupCoordinator struct {
	mu                    sync.Mutex
	prepareErr            error
	startErr              error
	prepared              []string
	deleteEnvironmentRows []bool
	started               []string
	cancelled             []string
	restored              []string
	cleaned               []string
}

type projectRestoreCleanupRace struct {
	started chan struct{}
	release chan struct{}
	mu      sync.Mutex
	calls   int
}

func (c *projectRestoreCleanupRace) CleanupTaskResources(context.Context, string, bool) {}

func (c *projectRestoreCleanupRace) PrepareTaskResourceCleanup(
	context.Context,
	string,
	models.TaskResourceCleanupTrigger,
	string,
	bool,
) error {
	return nil
}

func (c *projectRestoreCleanupRace) StartPreparedTaskResourceCleanup(context.Context, string) error {
	return nil
}

func (c *projectRestoreCleanupRace) CancelPreparedTaskResourceCleanup(
	ctx context.Context,
	operationID string,
) error {
	c.mu.Lock()
	c.calls++
	call := c.calls
	c.mu.Unlock()
	if call != 1 {
		return nil
	}
	close(c.started)
	select {
	case <-c.release:
		return ErrCleanupCancellationRace
	case <-ctx.Done():
		return ctx.Err()
	}
}

type deleteAdmissionCall struct {
	taskIDs []string
	discard bool
}

type recordingDeleteAdmissionCleaner struct {
	mu       sync.Mutex
	calls    []deleteAdmissionCall
	dirtyErr error
}

func (c *recordingDeleteAdmissionCleaner) CleanupTaskResources(context.Context, string, bool) {}

func (c *recordingDeleteAdmissionCleaner) ValidateTaskDeleteWorktrees(
	_ context.Context, taskIDs []string, discardWorktreeChanges bool,
) error {
	c.mu.Lock()
	c.calls = append(c.calls, deleteAdmissionCall{
		taskIDs: append([]string(nil), taskIDs...), discard: discardWorktreeChanges,
	})
	c.mu.Unlock()
	if !discardWorktreeChanges {
		return c.dirtyErr
	}
	return nil
}

func (c *recordingCleanupCoordinator) CleanupTaskResources(_ context.Context, taskID string, _ bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cleaned = append(c.cleaned, taskID)
}

type recordingRunCanceller struct {
	calls []string
}

func (r *recordingRunCanceller) CancelTaskExecution(_ context.Context, taskID, _ string, _ bool) error {
	r.calls = append(r.calls, taskID)
	return nil
}
func (c *recordingCleanupCoordinator) PrepareTaskResourceCleanup(
	_ context.Context,
	_ string,
	_ models.TaskResourceCleanupTrigger,
	operationID string,
	deleteEnvironmentRow bool,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.prepared = append(c.prepared, operationID)
	c.deleteEnvironmentRows = append(c.deleteEnvironmentRows, deleteEnvironmentRow)
	return c.prepareErr
}

func TestArchiveAutoTaskCASLossCancelsPreparedCleanupWithoutStoppingRun(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("candidate", "", "ws-1")
	repo := newCascadeRepo(tasks)
	repo.autoArchiveCASLoss = true
	coordinator := &recordingCleanupCoordinator{}
	canceller := &recordingRunCanceller{}
	svc := NewHandoffService(repo, nil, nil, nil, nil, nil)
	svc.SetTaskResourceCleaner(coordinator)
	svc.SetRunCanceller(canceller)

	out, err := svc.ArchiveAutoTask(context.Background(), tasks.tasks["candidate"])
	if err != nil {
		t.Fatalf("ArchiveAutoTask: %v", err)
	}
	if out == nil || len(out.ArchivedTaskIDs) != 0 || len(out.SkippedTaskIDs) != 1 {
		t.Fatalf("outcome = %#v, want skipped candidate", out)
	}
	if len(canceller.calls) != 0 {
		t.Fatalf("run cancellation calls = %v, want none after CAS loss", canceller.calls)
	}
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	if len(coordinator.prepared) != 1 || len(coordinator.cancelled) != 1 {
		t.Fatalf("cleanup prepared=%v cancelled=%v, want one of each", coordinator.prepared, coordinator.cancelled)
	}
}

func TestDeleteTaskTreePreparedCleanupDeletesEnvironmentRow(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	coordinator := &recordingCleanupCoordinator{}
	svc := NewHandoffService(&fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)}, nil, nil, nil, nil, nil)
	svc.SetTaskResourceCleaner(coordinator)

	if _, err := svc.DeleteTaskTree(context.Background(), "root", false); err != nil {
		t.Fatalf("DeleteTaskTree: %v", err)
	}
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	if len(coordinator.deleteEnvironmentRows) != 1 || !coordinator.deleteEnvironmentRows[0] {
		t.Fatalf("deleteEnvironmentRows = %v, want [true]", coordinator.deleteEnvironmentRows)
	}
}

func TestDeleteTaskTreeWithOptionsPreflightsWholeTreeAndPropagatesDiscardConsent(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("child", "root", "ws-1")
	tasks.addTask("grandchild", "child", "ws-1")
	cleaner := &recordingDeleteAdmissionCleaner{dirtyErr: errors.New("dirty worktree")}
	svc := NewHandoffService(
		&fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)}, nil, nil, nil, nil, nil,
	)
	svc.SetTaskResourceCleaner(cleaner)

	if _, err := svc.DeleteTaskTreeWithOptions(context.Background(), "root", true, DeleteTaskOptions{}); !errors.Is(err, cleaner.dirtyErr) {
		t.Fatalf("unconsented cascade error = %v, want dirty admission error", err)
	}
	for _, taskID := range []string{"root", "child", "grandchild"} {
		if task, err := tasks.GetTask(context.Background(), taskID); err != nil || task == nil {
			t.Fatalf("unconsented cascade mutated %s: task=%#v err=%v", taskID, task, err)
		}
	}

	cleaner.mu.Lock()
	if len(cleaner.calls) != 1 {
		t.Fatalf("admission calls after refusal = %d, want 1", len(cleaner.calls))
	}
	if cleaner.calls[0].discard {
		t.Fatal("unconsented cascade passed discard consent")
	}
	if len(cleaner.calls[0].taskIDs) != 3 {
		t.Fatalf("admission task IDs = %v, want complete tree", cleaner.calls[0].taskIDs)
	}
	cleaner.mu.Unlock()

	if _, err := svc.DeleteTaskTreeWithOptions(context.Background(), "root", true, DeleteTaskOptions{
		DiscardWorktreeChanges: true,
	}); err != nil {
		t.Fatalf("consented cascade: %v", err)
	}
	for _, taskID := range []string{"root", "child", "grandchild"} {
		if task, err := tasks.GetTask(context.Background(), taskID); err != nil {
			t.Fatalf("get deleted task %s: %v", taskID, err)
		} else if task != nil {
			t.Fatalf("consented cascade retained %s: %#v", taskID, task)
		}
	}
	cleaner.mu.Lock()
	defer cleaner.mu.Unlock()
	if len(cleaner.calls) != 2 || !cleaner.calls[1].discard {
		t.Fatalf("admission calls after consent = %+v, want second call with discard=true", cleaner.calls)
	}
}

func TestDeleteTaskTree_NoCascadeNormalizesInheritedChildren(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("child", "root", "ws-1")
	tasks.tasks["child"].Metadata = map[string]interface{}{
		"workspace": map[string]interface{}{
			"mode":     workspaceModeInheritParent,
			"group_id": "group-1",
		},
		"keep": "this field",
	}
	svc := NewHandoffService(&fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)}, nil, nil, nil, nil, nil)
	if _, err := svc.DeleteTaskTree(context.Background(), "root", false); err != nil {
		t.Fatalf("DeleteTaskTree: %v", err)
	}

	child, err := tasks.GetTask(context.Background(), "child")
	if err != nil {
		t.Fatalf("GetTask(child): %v", err)
	}
	if child == nil {
		t.Fatal("no-cascade delete removed the child")
	}
	if child.ParentID != "" {
		t.Fatalf("child parent_id = %q, want root", child.ParentID)
	}
	workspace, _ := child.Metadata["workspace"].(map[string]interface{})
	if workspace["mode"] != workspaceModeSharedGroup {
		t.Fatalf("child workspace mode = %#v, want %q", workspace["mode"], workspaceModeSharedGroup)
	}
	if workspace["group_id"] != "group-1" || child.Metadata["keep"] != "this field" {
		t.Fatalf("child metadata was not preserved: %#v", child.Metadata)
	}
}

func TestDeleteTaskTree_NoCascadeRejectsCrossWorkspaceChild(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("foreign-child", "root", "ws-2")
	svc := NewHandoffService(&fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)}, nil, nil, nil, nil, nil)

	_, err := svc.DeleteTaskTree(context.Background(), "root", false)
	if !errors.Is(err, archivecascade.ErrCrossWorkspaceDescendant) {
		t.Fatalf("DeleteTaskTree error = %v, want cross-workspace error", err)
	}
	root, rootErr := tasks.GetTask(context.Background(), "root")
	child, childErr := tasks.GetTask(context.Background(), "foreign-child")
	if rootErr != nil || childErr != nil || root == nil || child == nil {
		t.Fatalf("tasks after rejected delete = root=%#v child=%#v errors=%v/%v", root, child, rootErr, childErr)
	}
	if child.ParentID != "root" {
		t.Fatalf("foreign child parent_id = %q, want unchanged root", child.ParentID)
	}
}

// AC-005.6: a marked inherit_parent child whose parent is non-cascade
// deleted ends with mode = shared_group AND no orphan keys, in one write.
func TestDeleteTaskTree_NoCascadeClearsOrphanMarkerOnInheritedChildren(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("child", "root", "ws-1")
	tasks.tasks["child"].Metadata = map[string]interface{}{
		"workspace": map[string]interface{}{
			"mode":                     workspaceModeInheritParent,
			orphanedWorkspaceKey:       true,
			orphanedReasonWorkspaceKey: orphanedReasonParentArchived,
			orphanedParentIDKey:        "root",
			orphanedAtKey:              "2026-09-05T00:00:00Z",
		},
	}

	svc := NewHandoffService(&fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)}, nil, nil, nil, nil, nil)
	if _, err := svc.DeleteTaskTree(context.Background(), "root", false); err != nil {
		t.Fatalf("DeleteTaskTree: %v", err)
	}

	child, err := tasks.GetTask(context.Background(), "child")
	if err != nil {
		t.Fatalf("GetTask(child): %v", err)
	}
	if child == nil {
		t.Fatal("no-cascade delete removed the child")
	}
	workspace, _ := child.Metadata["workspace"].(map[string]interface{})
	if workspace["mode"] != workspaceModeSharedGroup {
		t.Fatalf("child workspace mode = %#v, want %q", workspace["mode"], workspaceModeSharedGroup)
	}
	for _, key := range []string{orphanedWorkspaceKey, orphanedReasonWorkspaceKey, orphanedParentIDKey, orphanedAtKey} {
		if _, ok := workspace[key]; ok {
			t.Fatalf("child workspace[%q] still present after non-cascade delete: %#v", key, workspace)
		}
	}
}

func (c *recordingCleanupCoordinator) StartPreparedTaskResourceCleanup(_ context.Context, operationID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.started = append(c.started, operationID)
	return c.startErr
}

func (c *recordingCleanupCoordinator) CancelPreparedTaskResourceCleanup(_ context.Context, operationID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cancelled = append(c.cancelled, operationID)
	return nil
}

func (c *recordingCleanupCoordinator) RestoreCancelledTaskResourceCleanup(_ context.Context, operationID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.restored = append(c.restored, operationID)
	return nil
}

func TestDeleteTaskTree_MembershipReleaseFailureCancelsEveryPreparedCleanup(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("child", "root", "ws-1")
	groups := newCascadeWSGroupRepo()
	releaseErr := errors.New("membership release unavailable")
	groups.releaseErr = releaseErr
	if err := groups.CreateWorkspaceGroup(context.Background(), &orchmodels.WorkspaceGroup{
		ID: "group-1", WorkspaceID: "ws-1",
	}); err != nil {
		t.Fatalf("CreateWorkspaceGroup: %v", err)
	}
	for _, taskID := range []string{"root", "child"} {
		if err := groups.AddWorkspaceGroupMember(context.Background(), "group-1", taskID, "member"); err != nil {
			t.Fatalf("AddWorkspaceGroupMember(%s): %v", taskID, err)
		}
	}
	coordinator := &recordingCleanupCoordinator{}
	svc := NewHandoffService(&fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)}, nil, nil, nil, groups, nil)
	svc.SetTaskResourceCleaner(coordinator)

	_, err := svc.DeleteTaskTree(context.Background(), "root", true)
	if !errors.Is(err, releaseErr) {
		t.Fatalf("DeleteTaskTree error = %v, want membership release error", err)
	}
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	if len(coordinator.prepared) != 2 {
		t.Fatalf("prepared cleanup count = %d, want 2", len(coordinator.prepared))
	}
	if len(coordinator.cancelled) != len(coordinator.prepared) {
		t.Fatalf("cancelled cleanup count = %d, want all %d prepared operations", len(coordinator.cancelled), len(coordinator.prepared))
	}
	if len(coordinator.started) != 0 || len(coordinator.cleaned) != 0 {
		t.Fatalf("cleanup ran after release failure: started=%v cleaned=%v", coordinator.started, coordinator.cleaned)
	}
	if task, getErr := tasks.GetTask(context.Background(), "root"); getErr != nil || task == nil {
		t.Fatalf("root task mutated after release failure: task=%#v err=%v", task, getErr)
	}
}

func (f *fakeWSGroupRepoCascade) RestoreWorkspaceGroupMemberByCascade(_ context.Context, taskID, cascadeID string) error {
	f.restoreCalls = append(f.restoreCalls, struct {
		taskID, cascadeID string
	}{taskID, cascadeID})
	return f.restoreErr
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

	out, err := svc.ArchiveTaskTree(context.Background(), "root", true)
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

func TestArchiveTaskTree_IgnoresConflictingIndependentChildCascades(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("c1", "root", "ws-1")
	tasks.addTask("c2", "root", "ws-1")
	repo := newCascadeRepo(tasks)
	if _, changed, err := repo.ArchiveTaskIfActiveWithVacatedStep(
		context.Background(), "c1", "independent-cascade-1",
	); err != nil || !changed {
		t.Fatalf("archive c1: changed=%v err=%v", changed, err)
	}
	if _, changed, err := repo.ArchiveTaskIfActiveWithVacatedStep(
		context.Background(), "c2", "independent-cascade-2",
	); err != nil || !changed {
		t.Fatalf("archive c2: changed=%v err=%v", changed, err)
	}

	svc := NewHandoffService(repo, nil, nil, nil, newCascadeWSGroupRepo(), nil)
	out, err := svc.ArchiveTaskTree(context.Background(), "root", true)
	if err != nil {
		t.Fatalf("archive parent with independently archived children: %v", err)
	}
	if len(out.ArchivedTaskIDs) != 1 || out.ArchivedTaskIDs[0] != "root" {
		t.Fatalf("archived task IDs = %v, want [root]", out.ArchivedTaskIDs)
	}
	root, _ := tasks.GetTask(context.Background(), "root")
	if root.ArchivedByCascadeID == "" {
		t.Fatal("parent archive did not receive a cascade identity")
	}
	for _, id := range []string{"c1", "c2"} {
		child, _ := tasks.GetTask(context.Background(), id)
		if child.ArchivedByCascadeID == root.ArchivedByCascadeID {
			t.Fatalf("%s was incorrectly claimed by parent cascade %q", id, root.ArchivedByCascadeID)
		}
	}
}

func TestArchiveTaskTreeDoesNotAdoptIndependentChildCascade(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("child", "root", "ws-1")
	repo := newCascadeRepo(tasks)
	svc := NewHandoffService(repo, nil, nil, nil, newCascadeWSGroupRepo(), nil)

	if _, err := svc.ArchiveTaskTree(context.Background(), "child", false); err != nil {
		t.Fatalf("archive child: %v", err)
	}
	child, _ := tasks.GetTask(context.Background(), "child")
	childCascadeID := child.ArchivedByCascadeID

	out, err := svc.ArchiveTaskTree(context.Background(), "root", true)
	if err != nil {
		t.Fatalf("archive parent: %v", err)
	}
	root, _ := tasks.GetTask(context.Background(), "root")
	if root.ArchivedByCascadeID == "" || root.ArchivedByCascadeID == childCascadeID {
		t.Fatalf("parent cascade ID = %q, child cascade ID = %q", root.ArchivedByCascadeID, childCascadeID)
	}
	if _, err := svc.UnarchiveTaskTree(context.Background(), "root"); err != nil {
		t.Fatalf("unarchive parent: %v", err)
	}
	child, _ = tasks.GetTask(context.Background(), "child")
	if child.ArchivedAt == nil {
		t.Fatal("independently archived child was restored with parent")
	}
	if len(out.ArchivedTaskIDs) != 1 || out.ArchivedTaskIDs[0] != "root" {
		t.Fatalf("parent archive IDs = %v, want [root]", out.ArchivedTaskIDs)
	}
}

func TestUnarchiveAgentProjectTreeRetriesWhileArchiveCleanupRuns(t *testing.T) {
	ctx := context.Background()
	tasks := newFakeTaskRepo()
	tasks.addTask("project-root", "", "ws-1")
	tasks.mu.Lock()
	tasks.tasks["project-root"].AgentProjectID = "project-1"
	tasks.mu.Unlock()
	repo := newCascadeRepo(tasks)
	const cascadeID = "cascade-project-restore"
	if changed, err := repo.ArchiveTaskIfActive(ctx, "project-root", cascadeID); err != nil || !changed {
		t.Fatalf("archive project coordinator: changed=%v err=%v", changed, err)
	}
	cleanup := &projectRestoreCleanupRace{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	handoff := NewHandoffService(repo, nil, nil, nil, nil, nil)
	handoff.SetTaskResourceCleaner(cleanup)
	handoff.SetAgentProjectActionAuthorizer(func(_ context.Context, projectID, taskID string) error {
		if projectID != "project-1" || taskID != "project-root" {
			t.Fatalf("project restore authorization = %q/%q", projectID, taskID)
		}
		return nil
	})

	result := make(chan error, 1)
	go func() {
		_, err := handoff.UnarchiveAgentProjectTree(ctx, "project-1", "project-root")
		result <- err
	}()
	select {
	case <-cleanup.started:
	case <-time.After(time.Second):
		t.Fatal("project restore did not attempt to cancel archive cleanup")
	}
	stillArchived, err := tasks.GetTask(ctx, "project-root")
	if err != nil {
		t.Fatalf("load project task while cleanup is running: %v", err)
	}
	if stillArchived.ArchivedAt == nil {
		t.Fatal("project task became active before archive cleanup finished")
	}
	close(cleanup.release)

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("UnarchiveAgentProjectTree after cleanup finished: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("project restore did not resume after archive cleanup finished")
	}
	active, err := tasks.GetTask(ctx, "project-root")
	if err != nil {
		t.Fatalf("load restored project task: %v", err)
	}
	if active.ArchivedAt != nil {
		t.Fatal("project task remains archived after restore")
	}
}

func TestArchiveTaskTree_TransfersSharedEnvironmentFromDepartingOwner(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("child", "root", "ws-1")
	tasks.taskEnvironments = map[string]*models.TaskEnvironment{
		"env-shared": {ID: "env-shared", TaskID: "root"},
	}
	groups := newCascadeWSGroupRepo()
	groups.groups["g1"] = &orchmodels.WorkspaceGroup{
		ID: "g1", WorkspaceID: "ws-1", OwnerTaskID: "root",
		MaterializedEnvironmentID: "env-shared",
		OwnedByKandev:             true,
		CleanupPolicy:             orchmodels.WorkspaceCleanupPolicyDeleteWhenLastMemberArchivedOrDel,
		CleanupStatus:             orchmodels.WorkspaceCleanupStatusActive,
	}
	groups.members["g1"] = map[string]string{
		"root":  orchmodels.WorkspaceMemberRoleOwner,
		"child": orchmodels.WorkspaceMemberRoleMember,
	}
	svc := newCascadeService(t, tasks, groups)

	if _, err := svc.ArchiveTaskTree(context.Background(), "root", false); err != nil {
		t.Fatalf("ArchiveTaskTree: %v", err)
	}
	env, err := tasks.GetTaskEnvironment(context.Background(), "env-shared")
	if err != nil {
		t.Fatalf("GetTaskEnvironment: %v", err)
	}
	if env.TaskID != "child" {
		t.Fatalf("shared environment owner = %q, want surviving child", env.TaskID)
	}
	if status := groups.cleanupStatuses["g1"]; status != "" {
		t.Fatalf("group cleanup status = %q, want unchanged while child remains active", status)
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

	archive, err := svc.ArchiveTaskTree(context.Background(), "root", true)
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

type restoringCascadeWSGroupRepo struct {
	*fakeWSGroupRepoCascade
}

func (f *restoringCascadeWSGroupRepo) RestoreWorkspaceGroupMemberByCascade(
	ctx context.Context, taskID, cascadeID string,
) error {
	if err := f.fakeWSGroupRepoCascade.RestoreWorkspaceGroupMemberByCascade(ctx, taskID, cascadeID); err != nil {
		return err
	}
	f.members["g1"][taskID] = orchmodels.WorkspaceMemberRoleMember
	return nil
}

func TestUnarchiveTaskTreeRestoresCleanedWorkspaceGroupAfterMembershipRestore(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	groups := &restoringCascadeWSGroupRepo{fakeWSGroupRepoCascade: newCascadeWSGroupRepo()}
	groups.groups["g1"] = &orchmodels.WorkspaceGroup{
		ID: "g1", WorkspaceID: "ws-1",
		CleanupStatus:     orchmodels.WorkspaceCleanupStatusCleaned,
		MaterializedKind:  orchmodels.WorkspaceGroupKindSingleRepo,
		RestoreConfigJSON: `{"kind":"single_repo","worktree_ids":{"r":"wt"}}`,
	}
	groups.members["g1"] = map[string]string{"root": orchmodels.WorkspaceMemberRoleMember}
	svc := NewHandoffService(newCascadeRepo(tasks), nil, nil, nil, groups, nil)

	archive, err := svc.ArchiveTaskTree(context.Background(), "root", false)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if _, err := svc.UnarchiveTaskTree(context.Background(), "root"); err != nil {
		t.Fatalf("unarchive: %v", err)
	}
	if got := groups.cleanupStatuses["g1"]; got != orchmodels.WorkspaceCleanupStatusActive {
		t.Fatalf("group cleanup status = %q, want active", got)
	}
	if len(archive.ArchivedTaskIDs) != 1 {
		t.Fatalf("archived tasks = %v, want root only", archive.ArchivedTaskIDs)
	}
}

func TestUnarchiveTaskTree_RestoresCleanupAfterMutationFailure(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addArchivedTask("root", "", "ws-1", "cascade-1")
	repo := newCascadeRepo(tasks)
	repo.unarchiveErr = errors.New("unarchive unavailable")
	coordinator := &recordingCleanupCoordinator{}
	svc := NewHandoffService(repo, nil, nil, nil, nil, nil)
	svc.SetTaskResourceCleaner(coordinator)

	_, err := svc.UnarchiveTaskTree(context.Background(), "root")
	if err == nil {
		t.Fatal("UnarchiveTaskTree unexpectedly succeeded")
	}
	if len(coordinator.cancelled) != 1 || coordinator.cancelled[0] != "cascade_archive:cascade-1:root" {
		t.Fatalf("cancelled cleanup operations = %v", coordinator.cancelled)
	}
	if len(coordinator.restored) != 1 || coordinator.restored[0] != "cascade_archive:cascade-1:root" {
		t.Fatalf("restored cleanup operations = %v", coordinator.restored)
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

	out, err := svc.ArchiveTaskTree(context.Background(), "root", true)
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

func TestArchiveTaskTreeEvaluatesGroupsReleasedBeforeMembershipFailure(t *testing.T) {
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
	groups.releaseErr = errors.New("membership release unavailable")
	groups.releaseErrAfter = 1
	svc := newCascadeService(t, tasks, groups)

	out, err := svc.ArchiveTaskTree(context.Background(), "root", true)
	if err == nil {
		t.Fatal("archive succeeded, want post-commit membership error")
	}
	if len(out.ReleasedGroupIDs) != 1 || out.ReleasedGroupIDs[0] != "g1" {
		t.Fatalf("ReleasedGroupIDs = %v, want [g1]", out.ReleasedGroupIDs)
	}
	if len(groups.releaseCalls) != 1 || groups.releaseCalls[0].taskID != "c1" {
		t.Fatalf("release calls = %v, want successful child release", groups.releaseCalls)
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
	svc.SetSessionReader(newFakeSessionReader())

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

type recordingArchiveCapture struct {
	calls []string
	err   error
}

func (r *recordingArchiveCapture) CaptureArchiveSnapshot(_ context.Context, sessionID string) error {
	r.calls = append(r.calls, sessionID)
	return r.err
}

type cancellableCascadeSessionReader struct {
	*fakeSessionReader
	cancelCalls  []string
	cancelReason []string
}

func (f *cancellableCascadeSessionReader) CancelActiveTaskSessionsByTaskID(
	_ context.Context,
	taskID, reason string,
) ([]*models.TaskSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancelCalls = append(f.cancelCalls, taskID)
	f.cancelReason = append(f.cancelReason, reason)

	now := time.Now().UTC()
	var cancelled []*models.TaskSession
	for _, session := range f.sessions[taskID] {
		if session == nil {
			continue
		}
		switch session.State {
		case models.TaskSessionStateCreated,
			models.TaskSessionStateStarting,
			models.TaskSessionStateRunning,
			models.TaskSessionStateWaitingForInput:
			session.State = models.TaskSessionStateCancelled
			session.ErrorMessage = reason
			session.CompletedAt = &now
			session.UpdatedAt = now
			cancelled = append(cancelled, session)
		}
	}
	return cancelled, nil
}

func TestArchiveTaskTree_FinalizesActiveSessionWhenRuntimeCancelFails(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	sessions := &cancellableCascadeSessionReader{fakeSessionReader: newFakeSessionReader()}
	sessions.sessions["root"] = []*models.TaskSession{
		{ID: "session-root", TaskID: "root", State: models.TaskSessionStateRunning},
	}

	tr := newCascadeRepo(tasks)
	svc := NewHandoffService(tr, nil, nil, nil, newCascadeWSGroupRepo(), nil)
	svc.SetSessionReader(sessions)
	svc.SetRunCanceller(&fakeRunCanceller{
		failOn: map[string]error{"root": errors.New("runtime missing")},
	})

	if _, err := svc.ArchiveTaskTree(context.Background(), "root", false); err != nil {
		t.Fatalf("archive: %v", err)
	}

	if got := sessions.sessions["root"][0].State; got != models.TaskSessionStateCancelled {
		t.Fatalf("session state = %q, want CANCELLED", got)
	}
	if len(sessions.cancelCalls) != 1 || sessions.cancelCalls[0] != "root" {
		t.Fatalf("cancel calls = %v, want [root]", sessions.cancelCalls)
	}
	if len(sessions.cancelReason) != 1 || sessions.cancelReason[0] != models.SessionArchiveTreeCancelReason {
		t.Fatalf("cancel reasons = %v, want [%q]", sessions.cancelReason, models.SessionArchiveTreeCancelReason)
	}
}

type archiveErrorCascadeRepo struct {
	*fakeCascadeRepo
	err error
}

func (r *archiveErrorCascadeRepo) ArchiveTaskIfActive(context.Context, string, string) (bool, error) {
	return false, r.err
}

func (r *archiveErrorCascadeRepo) ArchiveTaskIfActiveWithVacatedStep(
	context.Context,
	string,
	string,
) (string, bool, error) {
	return "", false, r.err
}

func TestArchiveTaskTree_DoesNotFinalizeSessionWhenArchiveFails(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	sessions := &cancellableCascadeSessionReader{fakeSessionReader: newFakeSessionReader()}
	sessions.sessions["root"] = []*models.TaskSession{
		{ID: "session-root", TaskID: "root", State: models.TaskSessionStateRunning},
	}

	archiveErr := errors.New("archive unavailable")
	svc := NewHandoffService(&archiveErrorCascadeRepo{
		fakeCascadeRepo: newCascadeRepo(tasks),
		err:             archiveErr,
	}, nil, nil, nil, newCascadeWSGroupRepo(), nil)
	svc.SetSessionReader(sessions)

	if _, err := svc.ArchiveTaskTree(context.Background(), "root", false); !errors.Is(err, archiveErr) {
		t.Fatalf("archive error = %v, want %v", err, archiveErr)
	}
	if got := sessions.sessions["root"][0].State; got != models.TaskSessionStateRunning {
		t.Fatalf("session state = %q, want RUNNING after failed archive", got)
	}
	if len(sessions.cancelCalls) != 0 {
		t.Fatalf("cancel calls = %v, want none after failed archive", sessions.cancelCalls)
	}
}

// fakeDeleteRepo extends fakeCascadeRepo with DeleteTask support so the
// delete cascade test can verify rows are actually removed.
type fakeDeleteRepo struct {
	*fakeCascadeRepo
}

func (r *fakeDeleteRepo) ListUnarchivedTasksWithActiveSessions(ctx context.Context) ([]*models.Task, error) {
	return nil, nil
}

func (r *fakeDeleteRepo) DeleteTask(ctx context.Context, id string) error {
	_, err := r.DeleteTaskWithVacatedStep(ctx, id)
	return err
}

func (r *fakeDeleteRepo) UpdateTask(_ context.Context, task *models.Task) error {
	r.base.mu.Lock()
	defer r.base.mu.Unlock()
	r.base.tasks[task.ID] = task
	return nil
}
func (r *fakeDeleteRepo) RestoreTaskParentIfUnchanged(
	_ context.Context,
	taskID, expectedParentID, restoredParentID, restoredWorkspaceMode string,
) error {
	r.base.mu.Lock()
	defer r.base.mu.Unlock()
	task := r.base.tasks[taskID]
	if task == nil {
		return errors.New("task not found")
	}
	if task.ParentID != expectedParentID {
		return errors.New("task parent changed during compensation")
	}
	task.ParentID = restoredParentID
	if restoredWorkspaceMode == workspaceModeInheritParent {
		if workspace, ok := task.Metadata["workspace"].(map[string]interface{}); ok &&
			workspace["mode"] == workspaceModeSharedGroup {
			workspace["mode"] = restoredWorkspaceMode
		}
	}
	return nil
}

func (r *fakeDeleteRepo) DeleteTaskWithVacatedStep(_ context.Context, id string) (string, error) {
	r.base.mu.Lock()
	defer r.base.mu.Unlock()
	task := r.base.tasks[id]
	if task == nil {
		return "", errors.New("task not found")
	}
	delete(r.base.tasks, id)
	return task.WorkflowStepID, nil
}

func (r *fakeDeleteRepo) DeleteExpiredQuickChatTask(context.Context, string, time.Time) (bool, error) {
	panic("DeleteExpiredQuickChatTask should not be used by delete cascade tests")
}

type deleteErrorCascadeRepo struct {
	*fakeDeleteRepo
	err error
}

func (r *deleteErrorCascadeRepo) DeleteTask(context.Context, string) error {
	return r.err
}

func (r *deleteErrorCascadeRepo) DeleteTaskWithVacatedStep(context.Context, string) (string, error) {
	return "", r.err
}

func TestDeleteTaskTree_DoesNotFinalizeSessionWhenDeleteFails(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	sessions := &cancellableCascadeSessionReader{fakeSessionReader: newFakeSessionReader()}
	sessions.sessions["root"] = []*models.TaskSession{
		{ID: "session-root", TaskID: "root", State: models.TaskSessionStateRunning},
	}

	deleteErr := errors.New("delete unavailable")
	svc := NewHandoffService(&deleteErrorCascadeRepo{
		fakeDeleteRepo: &fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)},
		err:            deleteErr,
	}, nil, nil, nil, nil, nil)
	svc.SetSessionReader(sessions)

	if _, err := svc.DeleteTaskTree(context.Background(), "root", false); !errors.Is(err, deleteErr) {
		t.Fatalf("delete error = %v, want %v", err, deleteErr)
	}
	if got := sessions.sessions["root"][0].State; got != models.TaskSessionStateRunning {
		t.Fatalf("session state = %q, want RUNNING after failed delete", got)
	}
	if len(sessions.cancelCalls) != 0 {
		t.Fatalf("cancel calls = %v, want none after failed delete", sessions.cancelCalls)
	}
}

func TestDeleteTaskTreeRestoresReparentedChildrenAfterRootDeleteFailure(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("child", "root", "ws-1")
	deleteErr := errors.New("delete unavailable")
	svc := NewHandoffService(&deleteErrorCascadeRepo{
		fakeDeleteRepo: &fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)},
		err:            deleteErr,
	}, nil, nil, nil, nil, nil)

	if _, err := svc.DeleteTaskTree(context.Background(), "root", false); !errors.Is(err, deleteErr) {
		t.Fatalf("delete error = %v, want %v", err, deleteErr)
	}
	tasks.mu.Lock()
	defer tasks.mu.Unlock()
	if got := tasks.tasks["child"].ParentID; got != "root" {
		t.Fatalf("child parent after failed delete = %q, want root", got)
	}
}

func TestDeleteTaskTreeRestoresMembershipAfterDeleteFailure(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	groups := newCascadeWSGroupRepo()
	groups.groups["group-1"] = &orchmodels.WorkspaceGroup{ID: "group-1", WorkspaceID: "ws-1"}
	groups.members["group-1"] = map[string]string{"root": orchmodels.WorkspaceMemberRoleOwner}
	deleteErr := errors.New("delete unavailable")
	svc := NewHandoffService(&deleteErrorCascadeRepo{
		fakeDeleteRepo: &fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)},
		err:            deleteErr,
	}, nil, nil, nil, groups, nil)

	if _, err := svc.DeleteTaskTree(context.Background(), "root", false); !errors.Is(err, deleteErr) {
		t.Fatalf("delete error = %v, want %v", err, deleteErr)
	}
	if len(groups.restoreCalls) != 1 || groups.restoreCalls[0].taskID != "root" {
		t.Fatalf("membership restore calls = %v, want root restore", groups.restoreCalls)
	}
	if groups.restoreCalls[0].cascadeID == "" {
		t.Fatal("membership restore did not retain cascade identity")
	}
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

	out, err := svc.DeleteTaskTree(context.Background(), "root", true)
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

	out, err := svc.DeleteTaskTree(context.Background(), "root", true)
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

func TestDeleteTaskTreeRemovesDependencyEdges(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("dependent", "", "ws-1")
	blockers := &fakeBlockerRepo{}
	blockers.blockers = append(blockers.blockers, &orchmodels.TaskBlocker{
		TaskID: "dependent", BlockerTaskID: "root",
	})
	repo := &fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)}
	svc := NewHandoffService(repo, nil, nil, blockers, nil, nil)

	if _, err := svc.DeleteTaskTree(context.Background(), "root", false); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got := len(blockers.blockers); got != 0 {
		t.Fatalf("dependency edges after delete = %d, want 0", got)
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

	if _, err := svc.ArchiveTaskTree(context.Background(), "root", true); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if len(canceller.calls) != 2 {
		t.Errorf("expected 2 cancel calls (root + c1), got %d", len(canceller.calls))
	}
}
func TestArchiveTaskTreeCapturesActiveSessionBeforeArchive(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	sessions := newFakeSessionReader()
	sessions.sessions["root"] = []*models.TaskSession{
		{ID: "session-root", TaskID: "root", State: models.TaskSessionStateRunning},
	}
	svc := newCascadeService(t, tasks, newCascadeWSGroupRepo())
	svc.SetSessionReader(sessions)
	capture := &recordingArchiveCapture{err: errors.New("capture unavailable")}
	svc.SetGitArchiveCapture(capture)

	if _, err := svc.ArchiveTaskTree(context.Background(), "root", false); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if len(capture.calls) != 1 || capture.calls[0] != "session-root" {
		t.Fatalf("capture calls = %v, want [session-root]", capture.calls)
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
			_, _ = svc.ArchiveTaskTree(context.Background(), "root", true)
		}()
	}
	wg.Wait()
	// Whatever order the goroutines ran in, every task should be
	// archived exactly once (CAS guard) under one cascade identity.
	// A later unarchive must be able to restore the complete tree.
	for _, id := range []string{"root", "c1", "c2"} {
		got, _ := tasks.GetTask(context.Background(), id)
		if got.ArchivedAt == nil {
			t.Errorf("%s should be archived", id)
		}
	}
}

func TestArchiveTaskTree_SerializesCascadeIdentity(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("c1", "root", "ws-1")
	base := newCascadeRepo(tasks)
	probe := &archiveSerializationProbe{
		fakeCascadeRepo:       base,
		firstMutationStarted:  make(chan struct{}),
		secondMutationStarted: make(chan struct{}),
		releaseFirstMutation:  make(chan struct{}),
	}
	groups := &restoringCascadeWSGroupRepo{fakeWSGroupRepoCascade: newCascadeWSGroupRepo()}
	groups.groups["g1"] = &orchmodels.WorkspaceGroup{
		ID: "g1", WorkspaceID: "ws-1",
		MaterializedKind: orchmodels.WorkspaceGroupKindSingleRepo,
	}
	groups.members["g1"] = map[string]string{
		"root": orchmodels.WorkspaceMemberRoleMember,
		"c1":   orchmodels.WorkspaceMemberRoleMember,
	}
	svc := NewHandoffService(probe, nil, nil, nil, groups, nil)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = svc.ArchiveTaskTree(context.Background(), "root", true)
	}()
	go func() {
		defer wg.Done()
		_, _ = svc.ArchiveTaskTree(context.Background(), "root", true)
	}()

	select {
	case <-probe.firstMutationStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first archive mutation did not start")
	}
	concurrentMutation := false
	select {
	case <-probe.secondMutationStarted:
		concurrentMutation = true
	case <-time.After(100 * time.Millisecond):
	}
	close(probe.releaseFirstMutation)
	wg.Wait()
	if concurrentMutation {
		t.Fatal("archive cascades mutated the same tree concurrently")
	}

	root, _ := tasks.GetTask(context.Background(), "root")
	child, _ := tasks.GetTask(context.Background(), "c1")
	if root.ArchivedByCascadeID == "" || root.ArchivedByCascadeID != child.ArchivedByCascadeID {
		t.Fatalf("archive cascade IDs = root %q, child %q, want one identity",
			root.ArchivedByCascadeID, child.ArchivedByCascadeID)
	}
	if _, err := svc.UnarchiveTaskTree(context.Background(), "root"); err != nil {
		t.Fatalf("unarchive serialized archive: %v", err)
	}
	root, _ = tasks.GetTask(context.Background(), "root")
	child, _ = tasks.GetTask(context.Background(), "c1")
	if root.ArchivedAt != nil || child.ArchivedAt != nil {
		t.Fatalf("unarchive left tasks archived: root=%v child=%v", root.ArchivedAt, child.ArchivedAt)
	}
}

// fakeEventPublisher captures the task IDs delivered to each cascade
// event-publish callback. It exists so the tests below can verify that
// ArchiveTaskTree / DeleteTaskTree actually invoke the publisher hook —
// without the hook firing, the gateway never broadcasts task.updated /
// task.deleted and the kanban board's All-Workflows view shows stale
// rows after a cascade.
type fakeEventPublisher struct {
	mu                sync.Mutex
	updated           []string
	deleted           []string
	dependencyChanges []string
	archived          []bool // archivedAt nil/non-nil per updated entry
}

func (f *fakeEventPublisher) PublishTaskUpdated(_ context.Context, task *models.Task, _ ...string) {
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

func (f *fakeEventPublisher) PublishDependencyChange(_ context.Context, taskIDs ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dependencyChanges = append(f.dependencyChanges, taskIDs...)
}

func TestDeleteTaskTreePublishesSurvivingDependencyChanges(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("dependent", "", "ws-1")
	blockers := &mockBlockerRepo{blockers: []*orchmodels.TaskBlocker{
		{TaskID: "dependent", BlockerTaskID: "root"},
	}}
	repo := &fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)}
	publisher := &fakeEventPublisher{}
	svc := NewHandoffService(repo, nil, nil, blockers, nil, nil)
	svc.SetTaskEventPublisher(publisher)

	if _, err := svc.DeleteTaskTree(context.Background(), "root", false); err != nil {
		t.Fatalf("DeleteTaskTree: %v", err)
	}
	if len(publisher.dependencyChanges) != 1 || publisher.dependencyChanges[0] != "dependent" {
		t.Fatalf("dependency projection updates = %v, want [dependent]", publisher.dependencyChanges)
	}
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

	if _, err := svc.ArchiveTaskTree(context.Background(), "root", true); err != nil {
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

	if _, err := svc.DeleteTaskTree(context.Background(), "root", true); err != nil {
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

	if _, err := svc.ArchiveTaskTree(context.Background(), "root", true); err != nil {
		t.Fatalf("archive with nil publisher: %v", err)
	}
	if _, err := svc.DeleteTaskTree(context.Background(), "root", true); err != nil {
		t.Fatalf("delete with nil publisher: %v", err)
	}
}

func TestCleanupWorkspaceGroupsUsesStoredMaterializedHandles(t *testing.T) {
	groups := newCascadeWSGroupRepo()
	ctx := context.Background()
	if err := groups.CreateWorkspaceGroup(ctx, &orchmodels.WorkspaceGroup{
		ID:               "group-owned",
		WorkspaceID:      "ws-delete",
		OwnerTaskID:      "task-1",
		MaterializedPath: "/tmp/kandev-owned-group",
		MaterializedKind: orchmodels.WorkspaceGroupKindPlainFolder,
		OwnedByKandev:    true,
		CleanupPolicy:    orchmodels.WorkspaceCleanupPolicyDeleteWhenLastMemberArchivedOrDel,
		CleanupStatus:    orchmodels.WorkspaceCleanupStatusActive,
	}); err != nil {
		t.Fatalf("create owned group: %v", err)
	}
	if err := groups.CreateWorkspaceGroup(ctx, &orchmodels.WorkspaceGroup{
		ID:               "group-user",
		WorkspaceID:      "ws-delete",
		OwnerTaskID:      "task-2",
		MaterializedPath: "/tmp/user-owned-group",
		MaterializedKind: orchmodels.WorkspaceGroupKindPlainFolder,
		OwnedByKandev:    false,
		CleanupPolicy:    orchmodels.WorkspaceCleanupPolicyNeverDelete,
		CleanupStatus:    orchmodels.WorkspaceCleanupStatusActive,
	}); err != nil {
		t.Fatalf("create user group: %v", err)
	}
	if err := groups.CreateWorkspaceGroup(ctx, &orchmodels.WorkspaceGroup{
		ID:               "group-cleaned",
		WorkspaceID:      "ws-delete",
		OwnerTaskID:      "task-3",
		MaterializedPath: "/tmp/already-cleaned-group",
		MaterializedKind: orchmodels.WorkspaceGroupKindPlainFolder,
		OwnedByKandev:    true,
		CleanupPolicy:    orchmodels.WorkspaceCleanupPolicyDeleteWhenLastMemberArchivedOrDel,
		CleanupStatus:    orchmodels.WorkspaceCleanupStatusCleaned,
	}); err != nil {
		t.Fatalf("create cleaned group: %v", err)
	}
	cleaner := &fakeWorkspaceCleaner{}
	svc := NewHandoffService(nil, nil, nil, nil, groups, nil)
	svc.SetWorkspaceCleaner(cleaner)

	if err := svc.CleanupWorkspaceGroups(ctx, "ws-delete"); err != nil {
		t.Fatalf("cleanup workspace groups: %v", err)
	}
	if len(cleaner.plainFolders) != 1 || cleaner.plainFolders[0] != "/tmp/kandev-owned-group" {
		t.Fatalf("plain folder cleanups = %#v, want owned group path", cleaner.plainFolders)
	}
	if got := groups.cleanupStatuses["group-owned"]; got != orchmodels.WorkspaceCleanupStatusCleaned {
		t.Fatalf("owned group cleanup status = %q, want cleaned", got)
	}
	if _, ok := groups.cleanupStatuses["group-user"]; ok {
		t.Fatal("user-owned group should not be cleaned")
	}
}

func TestCleanupWorkspaceGroupsWaitsForActiveExecutions(t *testing.T) {
	groups := newCascadeWSGroupRepo()
	ctx := context.Background()
	if err := groups.CreateWorkspaceGroup(ctx, &orchmodels.WorkspaceGroup{
		ID:               "group-owned",
		WorkspaceID:      "ws-delete",
		OwnerTaskID:      "task-1",
		MaterializedPath: "/tmp/kandev-owned-group",
		MaterializedKind: orchmodels.WorkspaceGroupKindPlainFolder,
		OwnedByKandev:    true,
		CleanupPolicy:    orchmodels.WorkspaceCleanupPolicyDeleteWhenLastMemberArchivedOrDel,
		CleanupStatus:    orchmodels.WorkspaceCleanupStatusActive,
	}); err != nil {
		t.Fatalf("create owned group: %v", err)
	}
	if err := groups.AddWorkspaceGroupMember(ctx, "group-owned", "task-1", "owner"); err != nil {
		t.Fatalf("add group member: %v", err)
	}
	sessions := &flippingActiveSessionReader{taskID: "task-1", sessionID: "session-1"}
	cleaner := &fakeWorkspaceCleaner{}
	svc := NewHandoffService(nil, nil, nil, nil, groups, nil)
	svc.SetSessionReader(sessions)
	svc.SetWorkspaceCleaner(cleaner)

	cleanupCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := svc.CleanupWorkspaceGroups(cleanupCtx, "ws-delete"); err != nil {
		t.Fatalf("cleanup workspace groups: %v", err)
	}
	if len(cleaner.plainFolders) != 1 || cleaner.plainFolders[0] != "/tmp/kandev-owned-group" {
		t.Fatalf("plain folder cleanups = %#v, want owned group path", cleaner.plainFolders)
	}
	sessions.mu.Lock()
	if sessions.checks < 2 {
		t.Errorf("HasExecutorRunningRow called %d times, want >= 2", sessions.checks)
	}
	sessions.mu.Unlock()
	if got := groups.cleanupStatuses["group-owned"]; got != orchmodels.WorkspaceCleanupStatusCleaned {
		t.Fatalf("owned group cleanup status = %q, want cleaned", got)
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

type fakeWorkspaceCleaner struct {
	plainFolders   []string
	multiRepoRoots []string
}

func (f *fakeWorkspaceCleaner) ValidateManagedRoot(string) error {
	return nil
}

func (f *fakeWorkspaceCleaner) CleanupPlainFolder(_ context.Context, path string) error {
	f.plainFolders = append(f.plainFolders, path)
	return nil
}

func (f *fakeWorkspaceCleaner) CleanupSingleRepoWorktree(context.Context, string) error {
	return nil
}

func (f *fakeWorkspaceCleaner) CleanupMultiRepoRoot(_ context.Context, root string, _ []string) error {
	f.multiRepoRoots = append(f.multiRepoRoots, root)
	return nil
}

func (f *fakeWorkspaceCleaner) CleanupRemoteEnvironment(context.Context, string, string) error {
	return nil
}
func TestRunWorkspaceGroupCleanupRejectsMalformedMultiRepoConfig(t *testing.T) {
	handoff := NewHandoffService(nil, nil, nil, nil, nil, nil)
	cleaner := &fakeWorkspaceCleaner{}
	handoff.SetWorkspaceCleaner(cleaner)
	group := &orchmodels.WorkspaceGroup{
		ID:                "group-malformed-restore",
		MaterializedKind:  orchmodels.WorkspaceGroupKindMultiRepo,
		MaterializedPath:  "/managed/root",
		RestoreConfigJSON: "{malformed",
	}

	if err := handoff.runWorkspaceGroupCleanup(context.Background(), group); err == nil {
		t.Fatal("malformed multi-repo restore config was accepted")
	}
	if len(cleaner.multiRepoRoots) != 0 {
		t.Fatalf("multi-repo cleanup calls = %d, want none", len(cleaner.multiRepoRoots))
	}
}

type flippingActiveSessionReader struct {
	mu        sync.Mutex
	taskID    string
	sessionID string
	checks    int
}

func (f *flippingActiveSessionReader) ListTaskSessions(_ context.Context, taskID string) ([]*models.TaskSession, error) {
	if taskID != f.taskID {
		return nil, nil
	}
	return []*models.TaskSession{{ID: f.sessionID, TaskID: f.taskID}}, nil
}

func (f *flippingActiveSessionReader) ListTaskSessionWorktrees(context.Context, string) ([]*models.TaskEnvironmentRepo, error) {
	return nil, nil
}

func (f *flippingActiveSessionReader) GetTask(context.Context, string) (*models.Task, error) {
	return nil, nil
}

func (f *flippingActiveSessionReader) HasExecutorRunningRow(_ context.Context, sessionID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if sessionID != f.sessionID {
		return false, nil
	}
	f.checks++
	return f.checks == 1, nil
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

	if _, err := svc.ArchiveTaskTree(context.Background(), "root", true); err != nil {
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

	if _, err := svc.DeleteTaskTree(context.Background(), "root", true); err != nil {
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

	if _, err := svc.ArchiveTaskTree(context.Background(), "root", true); err != nil {
		t.Fatalf("archive with nil cleaner: %v", err)
	}
	if _, err := svc.DeleteTaskTree(context.Background(), "root", true); err != nil {
		t.Fatalf("delete with nil cleaner: %v", err)
	}
}

// TestArchiveTaskTree_NoCascade_LeavesChildrenActive pins the new
// default: archiving a parent must NOT touch its subtasks unless the
// caller explicitly opts in. The subtasks might still be in progress
// and the user just wanted the parent off the board.
func TestArchiveTaskTree_NoCascade_LeavesChildrenActive(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("c1", "root", "ws-1")
	tasks.addTask("c2", "root", "ws-1")
	groups := newCascadeWSGroupRepo()
	svc := newCascadeService(t, tasks, groups)

	out, err := svc.ArchiveTaskTree(context.Background(), "root", false)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if len(out.ArchivedTaskIDs) != 1 || out.ArchivedTaskIDs[0] != "root" {
		t.Errorf("ArchivedTaskIDs = %v, want [root]", out.ArchivedTaskIDs)
	}
	rootRow, _ := tasks.GetTask(context.Background(), "root")
	if rootRow.ArchivedAt == nil {
		t.Error("root should be archived")
	}
	for _, id := range []string{"c1", "c2"} {
		child, _ := tasks.GetTask(context.Background(), id)
		if child.ArchivedAt != nil {
			t.Errorf("%s should remain active, got archived_at=%v", id, child.ArchivedAt)
		}
	}
}

// TestDeleteTaskTree_NoCascade_PublishesUpdatedForReparentedChildren
// pins the WS-event contract for the reparent step: after children are
// reparented to root, the bus must carry one task.updated per child so
// WS-driven clients refresh their cached parent_id pointers. Without
// the publish, the kanban / sidebar would keep displaying the children
// nested under the (now-deleted) parent until a manual reload.
func TestDeleteTaskTree_NoCascade_PublishesUpdatedForReparentedChildren(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("c1", "root", "ws-1")
	tasks.addTask("c2", "root", "ws-1")
	groups := newCascadeWSGroupRepo()
	tr := &fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)}
	svc := NewHandoffService(tr, nil, nil, nil, groups, nil)
	pub := &fakeEventPublisher{}
	svc.SetTaskEventPublisher(pub)

	if _, err := svc.DeleteTaskTree(context.Background(), "root", false); err != nil {
		t.Fatalf("delete: %v", err)
	}

	pub.mu.Lock()
	defer pub.mu.Unlock()
	// We expect one update per direct child (c1, c2). The deleted root
	// goes via PublishTaskDeleted, not PublishTaskUpdated.
	want := map[string]bool{"c1": true, "c2": true}
	for _, id := range pub.updated {
		delete(want, id)
	}
	if len(want) > 0 {
		t.Errorf("missing PublishTaskUpdated for reparented children: %v", want)
	}
	if len(pub.deleted) != 1 || pub.deleted[0] != "root" {
		t.Errorf("expected exactly one task.deleted for root, got %v", pub.deleted)
	}
}

// TestDeleteTaskTree_NoCascade_ReparentFailureAborts pins the safety
// invariant: when the no-cascade reparent step fails we MUST refuse to
// delete the parent. Continuing past a reparent error would leave
// children pointing at a row we're about to remove — exactly the
// dangling pointer the no-cascade path is designed to prevent.
func TestDeleteTaskTree_NoCascade_ReparentFailureAborts(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("c1", "root", "ws-1")
	groups := newCascadeWSGroupRepo()
	tr := &fakeReparentErrRepo{fakeDeleteRepo: &fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)}}
	svc := NewHandoffService(tr, nil, nil, nil, groups, nil)

	_, err := svc.DeleteTaskTree(context.Background(), "root", false)
	if err == nil {
		t.Fatal("expected error from failed reparent, got nil")
	}
	if _, exists := tasks.tasks["root"]; !exists {
		t.Error("root should NOT be deleted when reparent fails")
	}
	if _, exists := tasks.tasks["c1"]; !exists {
		t.Error("c1 should still exist")
	}
}

// fakeReparentErrRepo overrides ReparentDirectChildren to simulate a DB
// failure so the no-cascade abort path can be exercised in tests.
type fakeReparentErrRepo struct {
	*fakeDeleteRepo
}

func (r *fakeReparentErrRepo) ReparentDirectChildren(_ context.Context, _, _ string) error {
	return errors.New("simulated DB failure")
}

func (r *fakeReparentErrRepo) ReparentDirectChildrenInWorkspace(
	_ context.Context, _, _, _ string,
) error {
	return errors.New("simulated DB failure")
}

// TestDeleteTaskTree_NoCascade_ReparentsDirectChildren verifies the
// orphaning step: when the user deletes a parent without cascade, the
// direct subtasks have their parent_id cleared so the deleted-row
// pointer doesn't dangle. The subtask rows themselves survive.
func TestDeleteTaskTree_NoCascade_ReparentsDirectChildren(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("c1", "root", "ws-1")
	tasks.addTask("c2", "root", "ws-1")
	tasks.addTask("g1", "c1", "ws-1") // grandchild — should NOT be reparented
	groups := newCascadeWSGroupRepo()
	tr := &fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)}
	svc := NewHandoffService(tr, nil, nil, nil, groups, nil)

	out, err := svc.DeleteTaskTree(context.Background(), "root", false)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(out.ArchivedTaskIDs) != 1 || out.ArchivedTaskIDs[0] != "root" {
		t.Errorf("ArchivedTaskIDs = %v, want [root]", out.ArchivedTaskIDs)
	}
	if _, exists := tasks.tasks["root"]; exists {
		t.Error("root should be removed")
	}
	for _, id := range []string{"c1", "c2"} {
		child, ok := tasks.tasks[id]
		if !ok {
			t.Errorf("%s should NOT be deleted (cascade=false)", id)
			continue
		}
		if child.ParentID != "" {
			t.Errorf("%s.parent_id = %q, want empty (reparented to root)", id, child.ParentID)
		}
	}
	g1, ok := tasks.tasks["g1"]
	if !ok {
		t.Fatal("g1 should not be deleted")
	}
	if g1.ParentID != "c1" {
		t.Errorf("g1.parent_id = %q, want c1 (only direct children of root are reparented)", g1.ParentID)
	}
}
