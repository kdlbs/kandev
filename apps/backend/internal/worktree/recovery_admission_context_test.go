package worktree

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestAdmitRecoveryReusesOwningAdmissionFromContext(t *testing.T) {
	claim := &models.TaskEnvironmentRecoveryClaim{
		TaskEnvironmentID:   "environment-admission-context",
		OwnerTaskID:         "task-admission-context",
		OwnershipGeneration: 1,
		SessionID:           "session-admission-context",
		OperationID:         "operation-admission-context",
		ExecutorType:        string(models.ExecutorTypeWorktree),
	}
	releaseCalls := 0
	owner := &RecoveryAdmission{
		claim: claim,
		releaseFunc: func(context.Context) error {
			releaseCalls++
			return nil
		},
	}
	store := newMockStore()
	store.worktrees["worktree-admission-context"] = &Worktree{
		ID: "worktree-admission-context", TaskID: "task-admission-context",
		TaskEnvironmentID: "environment-admission-context", RepositoryID: "repository-admission-context",
		BranchSlug: "branch-admission-context", RepositoryPath: "/repos/current",
		Path: "/worktrees/relocated", Branch: "task/recovery",
	}
	manager := &Manager{store: store}
	request := RecoveryAdmissionRequest{
		TaskID: "task-admission-context", SessionID: "session-admission-context",
		TaskEnvironmentID: "environment-admission-context", OwnerTaskID: "task-admission-context",
		OwnershipGeneration: 1, ExecutorType: string(models.ExecutorTypeWorktree),
		OperationID: "operation-admission-context", Slots: []RecoverySlot{{
			WorktreeID: "worktree-admission-context", RepositoryID: "repository-admission-context",
			BranchSlug: "branch-admission-context", RepositoryPath: "/repos/current",
		}},
	}

	forwarded, err := manager.AdmitRecovery(WithRecoveryAdmission(context.Background(), owner), request)
	if err != nil {
		t.Fatalf("nested admission: %v", err)
	}
	if forwarded != owner {
		t.Fatal("nested admission did not preserve the owning release handle")
	}
	if got := request.Slots[0].Worktree; got == nil || got.Path != "/worktrees/relocated" {
		t.Fatalf("nested admission worktree = %#v, want the current relocated record", got)
	}
	if err := forwarded.Release(context.Background()); err != nil {
		t.Fatalf("release nested admission: %v", err)
	}
	if releaseCalls != 1 {
		t.Fatalf("release calls after nested release = %d, want 1", releaseCalls)
	}
	if err := owner.Release(context.Background()); err != nil {
		t.Fatalf("release outer admission: %v", err)
	}
	if releaseCalls != 1 {
		t.Fatalf("release calls after outer release = %d, want 1", releaseCalls)
	}
}

func TestDirtyRecoverySlotLockWaitHonorsCancellation(t *testing.T) {
	manager := &Manager{}
	request := RecoveryAdmissionRequest{
		TaskID: "task-lock-cancel", RelocateDirty: true,
		Slots: []RecoverySlot{
			{Worktree: &Worktree{ID: "a", Path: "/tasks/a", RepositoryPath: "/repos/a"}},
			{Worktree: &Worktree{ID: "b", Path: "/tasks/b", RepositoryPath: "/repos/b"}},
		},
	}
	firstLock := &sync.Mutex{}
	secondLock := &sync.Mutex{}
	manager.recoveryLocks.Store(recoverySlotKey(request.Slots[0]), firstLock)
	manager.recoveryLocks.Store(recoverySlotKey(request.Slots[1]), secondLock)
	secondLock.Lock()
	defer secondLock.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if locks, err := manager.lockRecoverySlots(ctx, &request, []int{0, 1}); err == nil || locks != nil {
		t.Fatalf("lockRecoverySlots() = (%v, %v), want cancellation error", locks, err)
	}
	if !firstLock.TryLock() {
		t.Fatal("cancellation left an earlier worktree lock held")
	}
	firstLock.Unlock()
}

func TestLockRecoverySlotsFailsWhenAnotherAdmissionOwnsWorktree(t *testing.T) {
	manager := &Manager{}
	request := RecoveryAdmissionRequest{Slots: []RecoverySlot{{WorktreeID: "worktree-contended"}}}
	first, err := manager.lockRecoverySlots(context.Background(), &request, []int{0})
	if err != nil {
		t.Fatalf("first admission lock: %v", err)
	}

	secondResult := make(chan error, 1)
	go func() {
		locks, lockErr := manager.lockRecoverySlots(context.Background(), &request, []int{0})
		for i := len(locks) - 1; i >= 0; i-- {
			locks[i].Unlock()
		}
		secondResult <- lockErr
	}()

	select {
	case err := <-secondResult:
		if err == nil {
			t.Fatal("competing admission acquired a worktree lock already held by another admission")
		}
		var contention *RecoveryInspectionContentionError
		if !errors.As(err, &contention) {
			t.Fatalf("competing admission error = %v, want typed inspection contention", err)
		}
	case <-time.After(100 * time.Millisecond):
		for i := len(first) - 1; i >= 0; i-- {
			first[i].Unlock()
		}
		if err := <-secondResult; err == nil {
			t.Fatal("competing admission blocked instead of failing while another admission owned the worktree")
		} else {
			t.Fatalf("competing admission blocked instead of failing quickly: %v", err)
		}
	}

	for i := len(first) - 1; i >= 0; i-- {
		first[i].Unlock()
	}
}

func TestLockRecoverySlotsAllowsWaitPolicyWithoutDirtyAuthorization(t *testing.T) {
	manager := &Manager{}
	request := RecoveryAdmissionRequest{Slots: []RecoverySlot{{WorktreeID: "worktree-explicit"}}}
	owner, err := manager.lockRecoverySlots(context.Background(), &request, []int{0})
	if err != nil {
		t.Fatalf("inspection lock: %v", err)
	}

	explicit := request
	explicit.InspectionWait = 250 * time.Millisecond
	if explicit.RelocateDirty {
		t.Fatal("inspection wait policy unexpectedly authorizes dirty relocation")
	}
	started := make(chan struct{})
	type result struct {
		locks []*sync.Mutex
		err   error
	}
	resultCh := make(chan result, 1)
	go func() {
		close(started)
		locks, lockErr := manager.lockRecoverySlots(context.Background(), &explicit, []int{0})
		resultCh <- result{locks: locks, err: lockErr}
	}()
	<-started

	select {
	case got := <-resultCh:
		for i := len(got.locks) - 1; i >= 0; i-- {
			got.locks[i].Unlock()
		}
		for i := len(owner) - 1; i >= 0; i-- {
			owner[i].Unlock()
		}
		if got.err != nil {
			t.Fatalf("explicit recovery failed instead of waiting for inspection: %v", got.err)
		}
		t.Fatal("explicit recovery acquired the slot before the inspection released it")
	case <-time.After(25 * time.Millisecond):
	}
	for i := len(owner) - 1; i >= 0; i-- {
		owner[i].Unlock()
	}

	select {
	case got := <-resultCh:
		if got.err != nil {
			t.Fatalf("explicit recovery after inspection: %v", got.err)
		}
		for i := len(got.locks) - 1; i >= 0; i-- {
			got.locks[i].Unlock()
		}
	case <-time.After(time.Second):
		t.Fatal("explicit recovery did not acquire the slot after inspection released it")
	}
}

func TestLockRecoverySlotsDoesNotUseDirtyAuthorizationAsWaitPolicy(t *testing.T) {
	manager := &Manager{}
	request := RecoveryAdmissionRequest{Slots: []RecoverySlot{{WorktreeID: "worktree-dirty-no-wait"}}}
	owner, err := manager.lockRecoverySlots(context.Background(), &request, []int{0})
	if err != nil {
		t.Fatalf("inspection lock: %v", err)
	}
	t.Cleanup(func() {
		for i := len(owner) - 1; i >= 0; i-- {
			owner[i].Unlock()
		}
	})

	dirty := request
	dirty.RelocateDirty = true
	locks, err := manager.lockRecoverySlots(context.Background(), &dirty, []int{0})
	if err == nil {
		for i := len(locks) - 1; i >= 0; i-- {
			locks[i].Unlock()
		}
		t.Fatal("dirty relocation authorization alone allowed waiting for the inspection lock")
	}
	var contention *RecoveryInspectionContentionError
	if !errors.As(err, &contention) {
		t.Fatalf("lock error = %v, want typed inspection contention", err)
	}
}

func TestWithoutRecoveryClaimClearsAdmissionOwner(t *testing.T) {
	owner := &RecoveryAdmission{claim: &models.TaskEnvironmentRecoveryClaim{OperationID: "operation"}}
	ctx := WithRecoveryAdmission(context.Background(), owner)
	ctx = WithoutRecoveryClaim(ctx)
	if recoveryAdmissionFromContext(ctx) != nil {
		t.Fatal("asynchronous startup retained the recovery admission owner")
	}
}
