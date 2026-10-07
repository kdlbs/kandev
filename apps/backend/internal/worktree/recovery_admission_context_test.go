package worktree

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryoperation"
)

func TestAdmitRecoveryReturnsBorrowedAdmissionFromContext(t *testing.T) {
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
	if forwarded == owner {
		t.Fatal("nested admission reused the outer owning release handle")
	}
	if got := request.Slots[0].Worktree; got == nil || got.Path != "/worktrees/relocated" {
		t.Fatalf("nested admission worktree = %#v, want the current relocated record", got)
	}
	if err := forwarded.Release(context.Background()); err != nil {
		t.Fatalf("release nested admission: %v", err)
	}
	if releaseCalls != 0 {
		t.Fatalf("release calls after nested release = %d, want 0", releaseCalls)
	}
	if err := owner.Release(context.Background()); err != nil {
		t.Fatalf("release outer admission: %v", err)
	}
	if releaseCalls != 1 {
		t.Fatalf("release calls after outer release = %d, want 1", releaseCalls)
	}
}

func TestRecoveryOperationIDReusesMatchingRetainedClaimWithoutArtifacts(t *testing.T) {
	claim := &models.TaskEnvironmentRecoveryClaim{
		TaskEnvironmentID:   "environment-retained-claim",
		OwnerTaskID:         "task-retained-claim",
		OwnershipGeneration: 3,
		SessionID:           "session-retained-claim",
		OperationID:         "operation-retained-claim",
		ExecutorType:        string(models.ExecutorTypeWorktree),
	}
	store := &managedCloneRelocationStore{claim: claim}
	manager := &Manager{store: store}
	request := RecoveryAdmissionRequest{
		TaskID: "task-retained-claim", OwnerTaskID: claim.OwnerTaskID,
		TaskEnvironmentID: claim.TaskEnvironmentID, OwnershipGeneration: claim.OwnershipGeneration,
		SessionID: claim.SessionID, ExecutorType: claim.ExecutorType,
		Slots: []RecoverySlot{{WorktreeID: "worktree-retained-claim"}},
	}

	operationID, err := manager.recoveryOperationID(context.Background(), &request, []int{0})
	if err != nil {
		t.Fatalf("read retained recovery claim: %v", err)
	}
	if operationID != claim.OperationID {
		t.Fatalf("recovered operation ID = %q, want retained claim ID %q", operationID, claim.OperationID)
	}
}

func TestRecoveryOperationIDDoesNotReuseForeignRetainedClaim(t *testing.T) {
	claim := &models.TaskEnvironmentRecoveryClaim{
		TaskEnvironmentID:   "environment-retained-claim",
		OwnerTaskID:         "task-retained-claim",
		OwnershipGeneration: 3,
		SessionID:           "another-session",
		OperationID:         "operation-retained-claim",
		ExecutorType:        string(models.ExecutorTypeWorktree),
	}
	manager := &Manager{store: &managedCloneRelocationStore{claim: claim}}
	request := RecoveryAdmissionRequest{
		TaskID: "task-retained-claim", OwnerTaskID: claim.OwnerTaskID,
		TaskEnvironmentID: claim.TaskEnvironmentID, OwnershipGeneration: claim.OwnershipGeneration,
		SessionID: "session-retained-claim", ExecutorType: claim.ExecutorType,
		Slots: []RecoverySlot{{WorktreeID: "worktree-retained-claim"}},
	}

	operationID, err := manager.recoveryOperationIDFromClaim(context.Background(), &request, []int{0})
	if err != nil {
		t.Fatalf("read foreign recovery claim: %v", err)
	}
	if operationID != "" {
		t.Fatalf("recovered operation ID = %q, want no ID from a foreign claim", operationID)
	}
}

func TestFailedProgressBeginRetainsOnlyMatchingRunningProjection(t *testing.T) {
	start := RecoveryProgressStart{
		TaskEnvironmentID: "environment-progress-begin", OwnerTaskID: "task-progress-begin",
		OwnershipGeneration: 4, SessionID: "session-progress-begin", OperationID: "operation-progress-begin",
		ErrorStamp: "error-progress-begin", Kind: recoveryoperation.KindManagedCloneRelocation,
	}
	matching := &models.TaskEnvironmentRecoveryOperation{
		TaskEnvironmentID: start.TaskEnvironmentID, OwnerTaskID: start.OwnerTaskID,
		OwnershipGeneration: start.OwnershipGeneration, SessionID: start.SessionID,
		OperationID: start.OperationID, ErrorStamp: start.ErrorStamp, Kind: start.Kind,
		State: recoveryoperation.StateRunning,
	}
	other := *matching
	other.OperationID = "another-operation"

	tests := []struct {
		name      string
		operation *models.TaskEnvironmentRecoveryOperation
		readErr   error
		want      bool
	}{
		{name: "matching live attempt", operation: matching, want: true},
		{name: "matching dead attempt awaiting reconciliation", operation: matching, want: true},
		{name: "different running operation", operation: &other, want: false},
		{name: "no persisted operation", want: false},
		{name: "projection unavailable", readErr: errors.New("read failed"), want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reporter := &projectedRecoveryProgressReporter{
				operation: test.operation,
				live:      test.name == "matching live attempt",
				err:       test.readErr,
			}
			manager := &Manager{recoveryProgressReporter: reporter}
			if got := manager.retainClaimForUnsettledProgressBegin(context.Background(), start); got != test.want {
				t.Fatalf("retain claim = %t, want %t", got, test.want)
			}
		})
	}
}

type projectedRecoveryProgressReporter struct {
	recordingRecoveryProgressReporter
	operation *models.TaskEnvironmentRecoveryOperation
	live      bool
	err       error
}

func (r *projectedRecoveryProgressReporter) WorkspaceRecoveryProjection(
	context.Context,
	string,
) (*models.TaskEnvironmentRecoveryOperation, bool, error) {
	return r.operation, r.live, r.err
}

func TestFailClaimedRecoverySettlesAfterRequestDisconnect(t *testing.T) {
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	operationCtx, cancelOperation := acceptedRecoveryContext(requestCtx)
	defer cancelOperation()

	reporter := &requestContextRecoveryProgressReporter{}
	binding, err := reporter.BeginWorkspaceRecovery(operationCtx, RecoveryProgressStart{
		TaskEnvironmentID: "environment-disconnected-recovery", OwnerTaskID: "task-disconnected-recovery",
		OwnershipGeneration: 1, SessionID: "session-disconnected-recovery", OperationID: "operation-disconnected-recovery",
	})
	if err != nil {
		t.Fatal(err)
	}
	progress := &recoveryProgressTracker{
		reporter: reporter, binding: binding,
		update: RecoveryProgressUpdate{State: "running", Phase: "snapshotting"},
	}
	claim := &models.TaskEnvironmentRecoveryClaim{
		TaskEnvironmentID: binding.TaskEnvironmentID, OwnerTaskID: binding.OwnerTaskID,
		OwnershipGeneration: binding.OwnershipGeneration, SessionID: binding.SessionID,
		OperationID: binding.OperationID,
	}
	cancelRequest()

	_, err = (&Manager{}).failClaimedRecovery(
		requestCtx, &RecoveryAdmissionRequest{}, claim, progress, operationCtx, cancelOperation,
		nil, true, errors.New("recovery failed after disconnect"), nil, nil,
	)
	if err == nil {
		t.Fatal("failClaimedRecovery returned no operation error")
	}
	if reporter.updateContextErr != nil || reporter.endContextErr != nil {
		t.Fatalf("backend settlement inherited request cancellation: update=%v end=%v", reporter.updateContextErr, reporter.endContextErr)
	}
	if !progress.terminal() {
		t.Fatal("recovery did not store a terminal outcome after request disconnect")
	}
}

type requestContextRecoveryProgressReporter struct {
	recordingRecoveryProgressReporter
	updateContextErr error
	endContextErr    error
}

func (r *requestContextRecoveryProgressReporter) UpdateWorkspaceRecovery(
	ctx context.Context,
	binding RecoveryProgressBinding,
	update RecoveryProgressUpdate,
) (RecoveryProgressBinding, error) {
	r.updateContextErr = ctx.Err()
	if err := ctx.Err(); err != nil {
		return RecoveryProgressBinding{}, err
	}
	return r.recordingRecoveryProgressReporter.UpdateWorkspaceRecovery(ctx, binding, update)
}

func (r *requestContextRecoveryProgressReporter) EndWorkspaceRecoveryRunner(ctx context.Context, binding RecoveryProgressBinding) {
	r.endContextErr = ctx.Err()
	r.recordingRecoveryProgressReporter.EndWorkspaceRecoveryRunner(ctx, binding)
}

func TestNestedRecoveryReleaseDoesNotFinishOuterProgress(t *testing.T) {
	claim := &models.TaskEnvironmentRecoveryClaim{
		TaskEnvironmentID:   "environment-progress-borrow",
		OwnerTaskID:         "task-progress-borrow",
		OwnershipGeneration: 1,
		SessionID:           "session-progress-borrow",
		OperationID:         "operation-progress-borrow",
		ExecutorType:        string(models.ExecutorTypeWorktree),
	}
	reporter := &recordingRecoveryProgressReporter{}
	binding, err := reporter.BeginWorkspaceRecovery(context.Background(), RecoveryProgressStart{
		TaskEnvironmentID: claim.TaskEnvironmentID, OwnerTaskID: claim.OwnerTaskID,
		OwnershipGeneration: claim.OwnershipGeneration, SessionID: claim.SessionID,
		OperationID: claim.OperationID,
	})
	if err != nil {
		t.Fatalf("begin progress: %v", err)
	}
	progress := &recoveryProgressTracker{
		reporter: reporter,
		binding:  binding,
		update: RecoveryProgressUpdate{
			State: "running", Phase: "resuming", WorkspaceComplete: true,
		},
	}
	operationCtx, cancelOperation := context.WithCancel(context.Background())
	releaseCalls := 0
	owner := &RecoveryAdmission{
		claim: claim, progress: progress, operationCtx: operationCtx,
		operationCancel: cancelOperation,
		releaseFunc: func(context.Context) error {
			releaseCalls++
			return nil
		},
	}
	request := &RecoveryAdmissionRequest{
		TaskID: claim.OwnerTaskID, SessionID: claim.SessionID,
		TaskEnvironmentID: claim.TaskEnvironmentID, OwnerTaskID: claim.OwnerTaskID,
		OwnershipGeneration: claim.OwnershipGeneration, ExecutorType: claim.ExecutorType,
		OperationID: claim.OperationID,
	}
	borrowed, handled, err := (&Manager{}).admitWithContextAuthority(
		WithRecoveryAdmission(context.Background(), owner), request,
	)
	if err != nil || !handled || borrowed == nil {
		t.Fatalf("nested admission = (%v, %t, %v), want borrowed admission", borrowed, handled, err)
	}
	if err := borrowed.Release(context.Background()); err != nil {
		t.Fatalf("release nested admission: %v", err)
	}
	if progress.terminal() {
		t.Fatal("nested release finished the outer recovery progress")
	}
	if operationCtx.Err() != nil {
		t.Fatalf("nested release canceled the outer operation: %v", operationCtx.Err())
	}
	if releaseCalls != 0 {
		t.Fatalf("nested release calls = %d, want 0", releaseCalls)
	}
	if err := owner.CompleteRecoveryResume(context.Background(), true, ""); err != nil {
		t.Fatalf("complete outer resume: %v", err)
	}
	if err := owner.Release(context.Background()); err != nil {
		t.Fatalf("release outer admission: %v", err)
	}
	if releaseCalls != 1 {
		t.Fatalf("outer release calls = %d, want 1", releaseCalls)
	}
	if !errors.Is(operationCtx.Err(), context.Canceled) {
		t.Fatalf("outer release did not cancel its operation context: %v", operationCtx.Err())
	}
	final := reporter.updates[len(reporter.updates)-1]
	if final.State != "completed" || !final.AgentReady {
		t.Fatalf("outer recovery result = %+v, want completed and agent ready", final)
	}
}

func TestWithRecoveryAdmissionPreservesNestedLaunchContextValues(t *testing.T) {
	type resumeAttemptContextKey struct{}

	operationCtx, cancelOperation := context.WithCancel(context.Background())
	defer cancelOperation()
	owner := &RecoveryAdmission{
		claim:        &models.TaskEnvironmentRecoveryClaim{OperationID: "operation-context-values"},
		operationCtx: operationCtx,
	}
	launchCtx := context.WithValue(context.Background(), resumeAttemptContextKey{}, "resume-attempt-1")
	admittedCtx := WithRecoveryAdmission(launchCtx, owner)

	if got := admittedCtx.Value(resumeAttemptContextKey{}); got != "resume-attempt-1" {
		t.Fatalf("resume attempt context value = %v, want preserved value", got)
	}
	if recoveryAdmissionFromContext(admittedCtx) != owner {
		t.Fatal("recovery admission context did not carry the owner")
	}
	if err := admittedCtx.Err(); err != nil {
		t.Fatalf("admitted context unexpectedly canceled: %v", err)
	}
	cancelOperation()
	select {
	case <-admittedCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("admitted context did not follow operation cancellation")
	}
	if !errors.Is(admittedCtx.Err(), context.Canceled) {
		t.Fatalf("admitted context error = %v, want context.Canceled", admittedCtx.Err())
	}
}

func TestWithBorrowedRecoveryAdmissionPreservesResumeCancellation(t *testing.T) {
	operationCtx, cancelOperation := context.WithCancel(context.Background())
	defer cancelOperation()
	resumeCtx, cancelResume := context.WithCancel(context.Background())
	defer cancelResume()
	borrowed := &RecoveryAdmission{
		claim:        &models.TaskEnvironmentRecoveryClaim{OperationID: "operation-resume-cancel"},
		operationCtx: operationCtx,
		borrowed:     true,
	}
	launchCtx := WithRecoveryAdmission(resumeCtx, borrowed)

	cancelResume()
	select {
	case <-launchCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("borrowed admission dropped resume cancellation")
	}
	if !errors.Is(launchCtx.Err(), context.Canceled) {
		t.Fatalf("launch context error = %v, want context.Canceled", launchCtx.Err())
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
