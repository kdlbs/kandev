package worktree

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type observedDoneContext struct {
	context.Context
	observed chan struct{}
	once     sync.Once
}

func (c *observedDoneContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.observed) })
	return c.Context.Done()
}

func TestRecoveryAdmissionWaitPolicyAllowsManualInspectionWait(t *testing.T) {
	manager := &Manager{}
	request := RecoveryAdmissionRequest{
		InspectionWait: 250 * time.Millisecond,
		Slots:          []RecoverySlot{{WorktreeID: "worktree-manual-wait"}},
	}
	owner, err := manager.lockRecoverySlots(context.Background(), &request, []int{0})
	if err != nil {
		t.Fatalf("inspection lock: %v", err)
	}
	unlockRecoveryLocks := func(locks []*sync.Mutex) {
		for i := len(locks) - 1; i >= 0; i-- {
			locks[i].Unlock()
		}
	}
	releaseOwner := func() {
		unlockRecoveryLocks(owner)
		owner = nil
	}
	t.Cleanup(releaseOwner)

	ctx := &observedDoneContext{Context: context.Background(), observed: make(chan struct{})}
	type result struct {
		locks []*sync.Mutex
		err   error
	}
	resultCh := make(chan result, 1)
	go func() {
		locks, lockErr := manager.lockRecoverySlots(ctx, &request, []int{0})
		resultCh <- result{locks: locks, err: lockErr}
	}()

	select {
	case <-ctx.observed:
	case got := <-resultCh:
		unlockRecoveryLocks(got.locks)
		releaseOwner()
		t.Fatalf("manual admission returned before the held inspection lock was released: %v", got.err)
	case <-time.After(2 * time.Second):
		releaseOwner()
		t.Fatal("manual admission did not begin its bounded inspection wait")
	}

	releaseOwner()
	select {
	case got := <-resultCh:
		if got.err != nil {
			t.Fatalf("manual admission after inspection release: %v", got.err)
		}
		unlockRecoveryLocks(got.locks)
	case <-time.After(2 * time.Second):
		t.Fatal("manual admission did not acquire the released inspection lock")
	}
}

func TestRecoveryAdmissionWaitPolicyHonorsCancellationAndDeadline(t *testing.T) {
	t.Run("cancellation releases earlier slots", func(t *testing.T) {
		manager := &Manager{}
		request := RecoveryAdmissionRequest{Slots: []RecoverySlot{{WorktreeID: "a"}, {WorktreeID: "b"}}}
		blockedOwner, err := manager.lockRecoverySlots(context.Background(), &request, []int{1})
		if err != nil {
			t.Fatalf("block second slot: %v", err)
		}
		t.Cleanup(func() { unlockRecoveryAdmissionTestLocks(blockedOwner) })

		ctxBase, cancel := context.WithCancel(context.Background())
		defer cancel()
		ctx := &observedDoneContext{Context: ctxBase, observed: make(chan struct{})}
		request.InspectionWait = time.Second
		resultCh := make(chan error, 1)
		go func() {
			locks, lockErr := manager.lockRecoverySlots(ctx, &request, []int{0, 1})
			unlockRecoveryAdmissionTestLocks(locks)
			resultCh <- lockErr
		}()
		select {
		case <-ctx.observed:
		case <-time.After(2 * time.Second):
			t.Fatal("manual admission did not reach the contended slot")
		}
		cancel()
		select {
		case err := <-resultCh:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled wait error = %v, want context.Canceled", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("manual admission did not stop after cancellation")
		}

		first, err := manager.lockRecoverySlots(context.Background(), &request, []int{0})
		if err != nil {
			t.Fatalf("acquire first slot after canceled wait: %v", err)
		}
		unlockRecoveryAdmissionTestLocks(first)
	})

	t.Run("caller deadline bounds manual wait", func(t *testing.T) {
		manager := &Manager{}
		request := RecoveryAdmissionRequest{
			InspectionWait: time.Second,
			Slots:          []RecoverySlot{{WorktreeID: "deadline"}},
		}
		owner, err := manager.lockRecoverySlots(context.Background(), &request, []int{0})
		if err != nil {
			t.Fatalf("inspection lock: %v", err)
		}
		t.Cleanup(func() { unlockRecoveryAdmissionTestLocks(owner) })

		deadlineCtx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
		defer cancel()
		ctx := &observedDoneContext{Context: deadlineCtx, observed: make(chan struct{})}
		resultCh := make(chan error, 1)
		go func() {
			locks, lockErr := manager.lockRecoverySlots(ctx, &request, []int{0})
			unlockRecoveryAdmissionTestLocks(locks)
			resultCh <- lockErr
		}()
		select {
		case <-ctx.observed:
		case <-time.After(2 * time.Second):
			t.Fatal("manual admission did not reach the contended slot")
		}
		select {
		case err := <-resultCh:
			var busy *RecoveryInspectionContentionError
			if !errors.As(err, &busy) {
				t.Fatalf("deadline wait error = %v, want typed inspection contention", err)
			}
			if got := err.Error(); got != "workspace recovery inspection is busy" {
				t.Fatalf("deadline error exposed unexpected details: %q", got)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("manual admission did not stop at the caller deadline")
		}
	})
}

func TestRecoveryAdmissionWaitPolicyRejectsChangedInventory(t *testing.T) {
	store := newMockStore()
	store.worktrees["worktree-changed"] = &Worktree{
		ID: "worktree-changed", TaskID: "task-changed", TaskEnvironmentID: "environment-changed",
		RepositoryID: "repository-changed", BranchSlug: "main", RepositoryPath: "/repos/original",
		Path: "/tasks/original", Branch: "feature/original", Status: StatusActive,
	}
	manager := &Manager{store: store}
	request := RecoveryAdmissionRequest{
		TaskID: "task-changed", OwnerTaskID: "task-changed", TaskEnvironmentID: "environment-changed",
		Slots: []RecoverySlot{{
			WorktreeID: "worktree-changed", RepositoryID: "repository-changed", BranchSlug: "main",
			RepositoryPath: "/repos/original", Worktree: cloneRecoveryAdmissionTestWorktree(store.worktrees["worktree-changed"]),
		}},
	}
	owner, err := manager.lockRecoverySlots(context.Background(), &request, []int{0})
	if err != nil {
		t.Fatalf("inspection lock: %v", err)
	}
	releaseOwner := func() {
		unlockRecoveryAdmissionTestLocks(owner)
		owner = nil
	}
	t.Cleanup(releaseOwner)

	waitRequest := request
	waitRequest.InspectionWait = time.Second
	ctx := &observedDoneContext{Context: context.Background(), observed: make(chan struct{})}
	resultCh := make(chan error, 1)
	go func() {
		locks, lockErr := manager.lockRecoverySlots(ctx, &waitRequest, []int{0})
		unlockRecoveryAdmissionTestLocks(locks)
		resultCh <- lockErr
	}()
	select {
	case <-ctx.observed:
	case <-time.After(2 * time.Second):
		t.Fatal("manual admission did not reach the contended slot")
	}

	replacement := cloneRecoveryAdmissionTestWorktree(store.worktrees["worktree-changed"])
	replacement.Path = "/tasks/replacement"
	replacement.Branch = "feature/replacement"
	store.worktrees[replacement.ID] = replacement
	releaseOwner()
	select {
	case err := <-resultCh:
		var recoveryErr *WorktreeRecoveryError
		if !errors.As(err, &recoveryErr) {
			t.Fatalf("changed inventory error = %v, want WorktreeRecoveryError", err)
		}
		if !strings.Contains(recoveryErr.Reason, "changed") {
			t.Fatalf("changed inventory reason = %q", recoveryErr.Reason)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("manual admission did not revalidate after lock acquisition")
	}
	lock, err := manager.lockRecoverySlots(context.Background(), &request, []int{0})
	if err != nil {
		t.Fatalf("acquire slot after changed-inventory refusal: %v", err)
	}
	unlockRecoveryAdmissionTestLocks(lock)
}

func unlockRecoveryAdmissionTestLocks(locks []*sync.Mutex) {
	for i := len(locks) - 1; i >= 0; i-- {
		locks[i].Unlock()
	}
}

func cloneRecoveryAdmissionTestWorktree(worktree *Worktree) *Worktree {
	if worktree == nil {
		return nil
	}
	clone := *worktree
	return &clone
}

type recoverySelectionSnapshotWaitStore struct {
	Store
	mu            sync.Mutex
	snapshot      models.WorkspaceRecoverySelectionSnapshot
	worktreeReads int
}

func (s *recoverySelectionSnapshotWaitStore) GetWorktreeByID(ctx context.Context, id string) (*Worktree, error) {
	s.mu.Lock()
	s.worktreeReads++
	s.mu.Unlock()
	return s.Store.GetWorktreeByID(ctx, id)
}

func (s *recoverySelectionSnapshotWaitStore) ReadRecoverySelectionSnapshot(
	_ context.Context,
	_ models.WorkspaceRecoverySelectionSnapshot,
) (models.WorkspaceRecoverySelectionSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot.Canonical(), nil
}

func (s *recoverySelectionSnapshotWaitStore) mutateSnapshot(snapshot models.WorkspaceRecoverySelectionSnapshot) {
	s.mu.Lock()
	s.snapshot = snapshot.Canonical()
	s.mu.Unlock()
}

func (s *recoverySelectionSnapshotWaitStore) readCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.worktreeReads
}

// @covers AC-TASKS-MANAGED-CLONE-RELOCATION-001.3
func TestRecoveryAdmissionRejectsSelectionDriftAfterInspectionWait(t *testing.T) {
	for _, drift := range []string{"added sibling slot", "selected environment", "owner generation"} {
		t.Run(drift, func(t *testing.T) {
			store := newMockStore()
			worktreePath := t.TempDir()
			store.worktrees["worktree-selected"] = &Worktree{
				ID: "worktree-selected", SessionID: "session-selected", TaskID: "task-selected",
				TaskEnvironmentID: "environment-selected", RepositoryID: "repository-selected",
				BranchSlug: "main", RepositoryPath: "/repos/widget", Path: worktreePath,
				Branch: "feature/selected", Status: StatusActive,
			}
			storeWithSnapshot := &recoverySelectionSnapshotWaitStore{
				Store: store,
				snapshot: models.NewWorkspaceRecoverySelectionSnapshot(
					&models.TaskSession{ID: "session-selected", TaskID: "task-selected", TaskEnvironmentID: "environment-selected"},
					&models.TaskEnvironment{
						ID: "environment-selected", TaskID: "task-selected", OwnershipGeneration: 4,
						ExecutorType: string(models.ExecutorTypeWorktree), Status: models.TaskEnvironmentStatusReady,
						WorkspacePath: "/tasks/selected", TaskDirName: "selected",
						Repos: []*models.TaskEnvironmentRepo{{
							ID: "slot-selected", TaskEnvironmentID: "environment-selected", RepositoryID: "repository-selected",
							BranchSlug: "main", WorktreeID: "worktree-selected", WorktreePath: worktreePath,
							WorktreeBranch: "feature/selected", Status: "active",
						}},
					},
					map[string]*models.Repository{"repository-selected": {
						ID: "repository-selected", WorkspaceID: "workspace-selected", SourceType: "github",
						LocalPath: "/repos/widget", Provider: "github", ProviderHost: "github.com",
						ProviderOwner: "acme", ProviderName: "widget",
					}},
				),
			}
			request := RecoveryAdmissionRequest{
				TaskID: "task-selected", SessionID: "session-selected", TaskEnvironmentID: "environment-selected",
				OwnerTaskID: "task-selected", OwnershipGeneration: 4,
				ExecutorType: string(models.ExecutorTypeWorktree), InspectionWait: time.Second,
				SelectionSnapshot: storeWithSnapshot.snapshot,
				Slots: []RecoverySlot{{
					WorktreeID: "worktree-selected", RepositoryID: "repository-selected", BranchSlug: "main",
					RepositoryPath: "/repos/widget",
				}},
			}
			manager := &Manager{store: storeWithSnapshot}
			key := recoverySlotKey(request.Slots[0])
			lockValue, _ := manager.recoveryLocks.LoadOrStore(key, &sync.Mutex{})
			owner := lockValue.(*sync.Mutex)
			owner.Lock()
			defer func() {
				if owner.TryLock() {
					owner.Unlock()
					return
				}
				owner.Unlock()
			}()

			ctx := &observedDoneContext{Context: context.Background(), observed: make(chan struct{})}
			type admissionResult struct {
				admission *RecoveryAdmission
				err       error
			}
			resultCh := make(chan admissionResult, 1)
			go func() {
				admission, err := manager.AdmitRecovery(ctx, request)
				resultCh <- admissionResult{admission: admission, err: err}
			}()
			select {
			case <-ctx.observed:
			case got := <-resultCh:
				if got.admission != nil {
					_ = got.admission.Release(context.Background())
				}
				owner.Unlock()
				t.Fatalf("admission returned before the held inspection lock was released: %v", got.err)
			case <-time.After(2 * time.Second):
				owner.Unlock()
				t.Fatal("manual admission did not begin waiting for inspection")
			}

			changed := request.SelectionSnapshot.Canonical()
			switch drift {
			case "added sibling slot":
				changed.Slots = append(changed.Slots, models.WorkspaceRecoveryInventorySlot{
					EnvironmentRepoID: "slot-sibling", RepositoryID: "repository-sibling",
					BranchSlug: "main", Status: "active", RepositoryPresent: true,
				})
			case "selected environment":
				changed.TaskEnvironmentID = "environment-replacement"
			case "owner generation":
				changed.OwnershipGeneration++
			}
			storeWithSnapshot.mutateSnapshot(changed)
			owner.Unlock()

			select {
			case got := <-resultCh:
				var recoveryErr *WorktreeRecoveryError
				require.ErrorAs(t, got.err, &recoveryErr)
				require.Contains(t, recoveryErr.Reason, "inventory changed")
				require.Nil(t, got.admission)
				require.Equal(t, 1, storeWithSnapshot.readCount(), "stale preflight must stop before reloading or inspecting worktrees")
				lockValue, ok := manager.recoveryLocks.Load(key)
				require.True(t, ok)
				require.True(t, lockValue.(*sync.Mutex).TryLock(), "stale admission must release every inspection lock")
				lockValue.(*sync.Mutex).Unlock()
			case <-time.After(2 * time.Second):
				t.Fatal("manual admission did not finish after releasing inspection lock")
			}
		})
	}
}
