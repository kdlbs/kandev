package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

// RecoverySlot is one canonical repository entry selected for recovery
// admission. Slots are supplied by the selected task environment, not by a
// task-wide worktree inventory.
type RecoverySlot struct {
	WorktreeID      string
	RepositoryID    string
	BranchSlug      string
	RepositoryPath  string
	Worktree        *Worktree
	CloneRelocation *ManagedCloneRelocationProof
}

// ManagedRepositoryIdentity is the provider identity selected by the
// workspace repository record. It is checked against both clone origins
// before an old worktree can move between managed clones.
type ManagedRepositoryIdentity struct {
	Provider string
	Host     string
	Owner    string
	Name     string
}

// ManagedCloneRelocationProof contains clone paths derived from the managed
// repository service plus the source identity recorded on the environment row.
// Legacy rows may have empty RecordedSource fields and are proven from Git's
// linked-worktree registration instead.
type ManagedCloneRelocationProof struct {
	ManagedRoot               string
	ExpectedSourcePath        string
	LegacyOwnerNameSourcePath string
	ExpectedDestinationPath   string
	RecordedSourcePath        string
	RecordedSourceCommonDir   string
	Identity                  ManagedRepositoryIdentity
}

// RecoveryAdmissionRequest identifies the already-selected environment and
// its complete canonical repository inventory. An empty inventory is a
// deliberate no-op and never triggers filesystem or Git inspection.
type RecoveryAdmissionRequest struct {
	TaskID              string
	SessionID           string
	TaskEnvironmentID   string
	OwnerTaskID         string
	OwnershipGeneration int64
	ExecutorType        string
	OperationID         string
	RelocateDirty       bool
	Slots               []RecoverySlot
}

// RecoveryAdmission retains the environment authority and per-worktree locks
// until the caller crosses the external workspace-start boundary.
type RecoveryAdmission struct {
	claim       *models.TaskEnvironmentRecoveryClaim
	releaseFunc func(context.Context) error
	once        sync.Once
	releaseErr  error
}

type recoveryAdmissionContextKey struct{}

type recoveryAdmissionContextValue struct {
	admission *RecoveryAdmission
	cleared   bool
}

// Claim returns the durable authority carried into nested lifecycle calls.
func (a *RecoveryAdmission) Claim() *models.TaskEnvironmentRecoveryClaim {
	if a == nil {
		return nil
	}
	return a.claim
}

// Release releases the durable authority and local locks exactly once.
func (a *RecoveryAdmission) Release(ctx context.Context) error {
	if a == nil {
		return nil
	}
	a.once.Do(func() {
		if a.releaseFunc != nil {
			a.releaseErr = a.releaseFunc(ctx)
		}
	})
	return a.releaseErr
}

// WithRecoveryClaim carries an admission through the executor-to-lifecycle
// call. The lifecycle manager recognizes the exact claim and does not acquire
// or release a second authority.
func WithRecoveryClaim(ctx context.Context, claim *models.TaskEnvironmentRecoveryClaim) context.Context {
	return recoveryclaim.WithClaim(ctx, claim)
}

// WithRecoveryAdmission carries both the durable claim and its release owner
// through nested workspace launch calls.
func WithRecoveryAdmission(ctx context.Context, admission *RecoveryAdmission) context.Context {
	if admission == nil {
		return ctx
	}
	ctx = recoveryclaim.WithClaim(ctx, admission.Claim())
	return context.WithValue(ctx, recoveryAdmissionContextKey{}, recoveryAdmissionContextValue{admission: admission})
}

func recoveryAdmissionFromContext(ctx context.Context) *RecoveryAdmission {
	if ctx == nil {
		return nil
	}
	value, _ := ctx.Value(recoveryAdmissionContextKey{}).(recoveryAdmissionContextValue)
	if value.cleared {
		return nil
	}
	return value.admission
}

// WithoutRecoveryClaim preserves the operation context without passing a
// released recovery authority into an asynchronous runtime-start phase.
func WithoutRecoveryClaim(ctx context.Context) context.Context {
	ctx = recoveryclaim.WithoutClaim(ctx)
	return context.WithValue(ctx, recoveryAdmissionContextKey{}, recoveryAdmissionContextValue{cleared: true})
}

type dirtyCloneRelocationContextKey struct{}

// WithDirtyCloneRelocation marks an explicitly authorized recovery operation.
// The marker is set only after the orchestrator validates the current error stamp.
func WithDirtyCloneRelocation(ctx context.Context) context.Context {
	return context.WithValue(ctx, dirtyCloneRelocationContextKey{}, true)
}

func dirtyCloneRelocationAllowed(ctx context.Context) bool {
	allowed, _ := ctx.Value(dirtyCloneRelocationContextKey{}).(bool)
	return allowed
}

// RecoveryClaimFromContext returns an admission claim carried by an internal
// workspace-start operation.
func RecoveryClaimFromContext(ctx context.Context) *models.TaskEnvironmentRecoveryClaim {
	return recoveryclaim.ClaimFromContext(ctx)
}

type recoveryClaimStore interface {
	AcquireTaskEnvironmentRecoveryClaim(context.Context, models.TaskEnvironmentRecoveryClaimRequest) (*models.TaskEnvironmentRecoveryClaim, error)
	ReleaseTaskEnvironmentRecoveryClaim(context.Context, *models.TaskEnvironmentRecoveryClaim) error
}

type recoverySlotInspection struct {
	needsRecovery   bool
	needsRelocation bool
	dirty           bool
	relocation      managedCloneRelocationInspection
}

// AdmitRecovery inspects only the selected environment's slots. It returns a
// held admission when a present damaged checkout requires replacement, and a
// nil admission when there is no recovery work to perform.
//
//nolint:cyclop,gocognit,funlen // The admission boundary must fail closed at each identity and filesystem check.
func (m *Manager) AdmitRecovery(ctx context.Context, req RecoveryAdmissionRequest) (*RecoveryAdmission, error) {
	if m == nil || m.store == nil || req.TaskEnvironmentID == "" ||
		req.ExecutorType != string(models.ExecutorTypeWorktree) || len(req.Slots) == 0 {
		return nil, nil
	}
	if req.TaskID == "" || req.OwnerTaskID == "" || req.SessionID == "" || req.OwnershipGeneration <= 0 {
		return nil, recoveryAdmissionError(req, "recovery request identity is incomplete")
	}
	req.RelocateDirty = req.RelocateDirty || dirtyCloneRelocationAllowed(ctx)
	if admission := recoveryAdmissionFromContext(ctx); admission != nil {
		if !recoveryClaimMatchesRequest(admission.Claim(), req) {
			return nil, recoveryAdmissionError(req, "workspace start carries a different recovery claim")
		}
		if _, err := m.resolveRecoverySlots(ctx, &req); err != nil {
			return nil, err
		}
		return admission, nil
	}
	if claim := recoveryclaim.ClaimFromContext(ctx); claim != nil {
		if !recoveryClaimMatchesRequest(claim, req) {
			return nil, recoveryAdmissionError(req, "workspace start carries a different recovery claim")
		}
		// The outer admission owns the per-worktree locks and durable claim.
		// Nested lifecycle admission only needs to carry that authority through
		// workspace reconciliation; reacquiring a non-reentrant process mutex
		// here would deadlock the same launch.
		if _, err := m.resolveRecoverySlots(ctx, &req); err != nil {
			return nil, err
		}
		return &RecoveryAdmission{claim: claim}, nil
	}

	indices, err := m.resolveRecoverySlots(ctx, &req)
	if err != nil {
		return nil, err
	}
	locks, err := m.lockRecoverySlots(&req, indices)
	if err != nil {
		return nil, err
	}
	releaseLocks := func(context.Context) error {
		for i := len(locks) - 1; i >= 0; i-- {
			locks[i].Unlock()
		}
		return nil
	}

	inspection, err := m.inspectRecoverySlots(ctx, &req, indices)
	if err != nil {
		_ = releaseLocks(ctx)
		return nil, err
	}
	if inspection.dirty && !req.RelocateDirty {
		_ = releaseLocks(ctx)
		return nil, managedCloneRelocationRequiredError(req.TaskID)
	}
	if !inspection.needsRecovery {
		_ = releaseLocks(ctx)
		return nil, nil
	}

	claim := recoveryclaim.ClaimFromContext(ctx)
	claimOwned := false
	if claim != nil {
		// Context claims return from the fast path above.
	} else {
		claimStore, ok := m.store.(recoveryClaimStore)
		if !ok {
			_ = releaseLocks(ctx)
			return nil, recoveryAdmissionError(req, "durable recovery claim is unavailable")
		}
		operationID, operationErr := m.recoveryOperationID(&req, indices)
		if operationErr != nil {
			_ = releaseLocks(ctx)
			return nil, operationErr
		}
		claim, err = claimStore.AcquireTaskEnvironmentRecoveryClaim(ctx, models.TaskEnvironmentRecoveryClaimRequest{
			TaskEnvironmentID:   req.TaskEnvironmentID,
			OwnerTaskID:         req.OwnerTaskID,
			OwnershipGeneration: req.OwnershipGeneration,
			SessionID:           req.SessionID,
			OperationID:         operationID,
			ExecutorType:        req.ExecutorType,
			// The explicit repair owns this session's idle runtime; all other
			// environment consumers remain a hard admission barrier.
			AllowCurrentSessionRuntime: req.RelocateDirty,
		})
		if err != nil {
			_ = releaseLocks(ctx)
			return nil, recoveryAdmissionError(req, err.Error())
		}
		claimOwned = true
	}

	// Reinspect the complete selected inventory after the durable claim is
	// acquired and before any slot is changed. This closes the partial-inventory
	// window between the first inspection and claim acquisition.
	claimCtx := recoveryclaim.WithClaim(ctx, claim)
	inspection, err = m.inspectRecoverySlots(claimCtx, &req, indices)
	if err != nil {
		if claimOwned {
			_ = m.releaseRecoveryClaim(ctx, claim)
		}
		_ = releaseLocks(ctx)
		return nil, err
	}
	if inspection.dirty && !req.RelocateDirty {
		if claimOwned {
			_ = m.releaseRecoveryClaim(ctx, claim)
		}
		_ = releaseLocks(ctx)
		return nil, managedCloneRelocationRequiredError(req.TaskID)
	}
	if !inspection.needsRecovery {
		if claimOwned {
			_ = m.releaseRecoveryClaim(ctx, claim)
		}
		_ = releaseLocks(ctx)
		return nil, nil
	}

	for _, index := range indices {
		slot := &req.Slots[index]
		if slot.Worktree == nil || slot.Worktree.Path == "" {
			continue
		}
		slotInspection, inspectErr := m.inspectRecoverySlot(claimCtx, req.OwnerTaskID, slot)
		if inspectErr != nil {
			if claimOwned {
				_ = m.releaseRecoveryClaim(ctx, claim)
			}
			_ = releaseLocks(ctx)
			return nil, inspectErr
		}
		if slotInspection.needsRelocation {
			if err := m.relocateRecoverySlot(claimCtx, &req, slot, slotInspection, claim); err != nil {
				if claimOwned {
					_ = m.releaseRecoveryClaim(ctx, claim)
				}
				_ = releaseLocks(ctx)
				return nil, err
			}
			continue
		}
		linkedInspection := inspectLinkedWorktree(slot.Worktree.Path)
		if linkedInspection.class != linkedWorktreeMissingAdmin {
			continue
		}
		recovered, recoverErr := m.RecoverWorktree(claimCtx, slot.Worktree, CreateRequest{
			TaskID:              req.OwnerTaskID,
			TaskEnvironmentID:   req.TaskEnvironmentID,
			RepositoryID:        slot.Worktree.RepositoryID,
			RepositoryPath:      recoveryRepositoryPath(*slot),
			BaseBranch:          slot.Worktree.BaseBranch,
			RecoveryClaim:       claim,
			RecoveryOperationID: claim.OperationID,
		})
		if recoverErr != nil {
			if claimOwned {
				_ = m.releaseRecoveryClaim(ctx, claim)
			}
			_ = releaseLocks(ctx)
			return nil, recoverErr
		}
		if recovered == nil || !m.IsValid(recovered.Path) {
			if claimOwned {
				_ = m.releaseRecoveryClaim(ctx, claim)
			}
			_ = releaseLocks(ctx)
			return nil, recoveryAdmissionError(req, "rematerialized checkout failed integrity validation")
		}
		slot.Worktree = recovered
	}

	return &RecoveryAdmission{
		claim: claim,
		releaseFunc: func(releaseCtx context.Context) error {
			var releaseErr error
			if claimOwned {
				releaseErr = m.releaseRecoveryClaim(releaseCtx, claim)
			}
			_ = releaseLocks(releaseCtx)
			return releaseErr
		},
	}, nil
}

func (m *Manager) relocateRecoverySlot(
	ctx context.Context,
	req *RecoveryAdmissionRequest,
	slot *RecoverySlot,
	inspection recoverySlotInspection,
	claim *models.TaskEnvironmentRecoveryClaim,
) error {
	var relocated *Worktree
	var err error
	if inspection.dirty && req.RelocateDirty {
		relocated, err = m.relocateDirtyManagedCloneWorktree(
			ctx, slot.Worktree, slot.CloneRelocation, inspection.relocation, claim,
		)
	} else {
		relocated, err = m.relocateCleanManagedCloneWorktree(
			ctx, slot.Worktree, slot.CloneRelocation, inspection.relocation, claim,
		)
	}
	if err != nil {
		return err
	}
	slot.Worktree = relocated
	return nil
}

//nolint:cyclop,gocognit // Slot resolution validates several independent durable identities.
func (m *Manager) resolveRecoverySlots(ctx context.Context, req *RecoveryAdmissionRequest) ([]int, error) {
	indices := make([]int, 0, len(req.Slots))
	for index := range req.Slots {
		slot := &req.Slots[index]
		if slot.WorktreeID != "" {
			wt, err := m.store.GetWorktreeByID(ctx, slot.WorktreeID)
			if err != nil {
				return nil, recoveryAdmissionError(*req, fmt.Sprintf("load selected worktree %q: %v", slot.WorktreeID, err))
			}
			if wt == nil {
				return nil, recoveryAdmissionError(*req, fmt.Sprintf("selected worktree %q is no longer present", slot.WorktreeID))
			}
			if slot.Worktree != nil && slot.Worktree.ID != "" && slot.Worktree.ID != wt.ID {
				return nil, recoveryAdmissionError(*req, fmt.Sprintf("selected worktree %q identity changed", slot.WorktreeID))
			}
			slot.Worktree = wt
		}
		if slot.Worktree == nil || slot.Worktree.Path == "" {
			// A row without a materialized path is created by normal workspace
			// materialization and must not cause host filesystem inspection.
			continue
		}
		if slot.Worktree.ID == "" {
			return nil, recoveryAdmissionError(*req, fmt.Sprintf("selected worktree for repository %q has no durable identity", slot.RepositoryID))
		}
		if slot.Worktree.TaskID != "" && slot.Worktree.TaskID != req.OwnerTaskID {
			return nil, recoveryAdmissionError(*req, fmt.Sprintf("selected worktree %q belongs to another task", slot.Worktree.ID))
		}
		if slot.Worktree.TaskEnvironmentID != "" && slot.Worktree.TaskEnvironmentID != req.TaskEnvironmentID {
			return nil, recoveryAdmissionError(*req, fmt.Sprintf("selected worktree %q belongs to another environment", slot.Worktree.ID))
		}
		if slot.RepositoryID != "" && slot.Worktree.RepositoryID != "" && slot.RepositoryID != slot.Worktree.RepositoryID {
			return nil, recoveryAdmissionError(*req, fmt.Sprintf("selected worktree %q repository identity changed", slot.Worktree.ID))
		}
		if slot.BranchSlug != "" && slot.Worktree.BranchSlug != "" && slot.BranchSlug != slot.Worktree.BranchSlug {
			return nil, recoveryAdmissionError(*req, fmt.Sprintf("selected worktree %q branch identity changed", slot.Worktree.ID))
		}
		if slot.RepositoryID == "" {
			slot.RepositoryID = slot.Worktree.RepositoryID
		}
		if slot.RepositoryPath != "" && slot.Worktree.RepositoryPath != "" {
			requestedPath, requestedErr := filepath.Abs(slot.RepositoryPath)
			durablePath, durableErr := filepath.Abs(slot.Worktree.RepositoryPath)
			if requestedErr != nil || durableErr != nil || filepath.Clean(requestedPath) != filepath.Clean(durablePath) {
				return nil, recoveryAdmissionError(*req, fmt.Sprintf("selected worktree %q repository path changed", slot.Worktree.ID))
			}
		}
		if slot.Worktree.RepositoryPath == "" && slot.RepositoryPath != "" {
			slot.Worktree.RepositoryPath = slot.RepositoryPath
		}
		if slot.RepositoryPath == "" {
			slot.RepositoryPath = slot.Worktree.RepositoryPath
		}
		indices = append(indices, index)
	}
	return indices, nil
}

func (m *Manager) lockRecoverySlots(req *RecoveryAdmissionRequest, indices []int) ([]*sync.Mutex, error) {
	sorted := append([]int(nil), indices...)
	sort.Slice(sorted, func(i, j int) bool {
		return recoverySlotKey(req.Slots[sorted[i]]) < recoverySlotKey(req.Slots[sorted[j]])
	})
	locks := make([]*sync.Mutex, 0, len(sorted))
	seen := make(map[string]struct{}, len(sorted))
	for _, index := range sorted {
		key := recoverySlotKey(req.Slots[index])
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		lockValue, _ := m.recoveryLocks.LoadOrStore(key, &sync.Mutex{})
		lock := lockValue.(*sync.Mutex)
		if req.RelocateDirty {
			// The stamped repair runs outside lifecycle singleflight and can wait
			// for an in-flight read-only inspection to finish.
			lock.Lock()
		} else if !lock.TryLock() {
			// Session startup is coalesced by lifecycle singleflight. Waiting here
			// can make a competing workspace request own that flight while the
			// recovery caller waits for it, so ordinary admissions never queue.
			for i := len(locks) - 1; i >= 0; i-- {
				locks[i].Unlock()
			}
			return nil, recoveryAdmissionError(*req, "selected worktree is already being recovered")
		}
		locks = append(locks, lock)
	}
	return locks, nil
}

func (m *Manager) inspectRecoverySlots(ctx context.Context, req *RecoveryAdmissionRequest, indices []int) (recoverySlotInspection, error) {
	var all recoverySlotInspection
	for _, index := range indices {
		slot := &req.Slots[index]
		inspection, err := m.inspectRecoverySlot(ctx, req.OwnerTaskID, slot)
		if err != nil {
			return recoverySlotInspection{}, err
		}
		all.needsRecovery = all.needsRecovery || inspection.needsRecovery
		all.needsRelocation = all.needsRelocation || inspection.needsRelocation
		all.dirty = all.dirty || inspection.dirty
	}
	return all, nil
}

func (m *Manager) inspectRecoverySlot(ctx context.Context, taskID string, slot *RecoverySlot) (recoverySlotInspection, error) {
	wt := slot.Worktree
	if wt == nil || wt.Path == "" {
		return recoverySlotInspection{}, nil
	}
	if _, err := os.Lstat(wt.Path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return recoverySlotInspection{}, nil
		}
		return recoverySlotInspection{}, recoverySlotError(taskID, wt.Path, fmt.Sprintf("cannot inspect persisted checkout: %v", err))
	}
	handle, err := m.validateWorktreePathSafe(wt.Path)
	if err != nil || handle == nil {
		return recoverySlotInspection{}, recoverySlotError(taskID, wt.Path, fmt.Sprintf("cannot pin persisted checkout: %v", err))
	}
	defer func() { _ = handle.Close() }()
	if err := m.validateExistingWorktreePathOwner(wt.Path, wt); err != nil {
		return recoverySlotInspection{}, recoverySlotError(taskID, wt.Path, err.Error())
	}
	inspection := inspectLinkedWorktree(wt.Path)
	if inspection.class == linkedWorktreeHealthy {
		if err := handle.VerifyPath(filepath.Clean(wt.Path)); err != nil {
			return recoverySlotInspection{}, recoverySlotError(taskID, wt.Path, err.Error())
		}
		if slot.CloneRelocation != nil {
			relocation, relocationErr := m.inspectManagedCloneRelocation(ctx, taskID, wt, slot.CloneRelocation)
			if relocationErr != nil {
				return recoverySlotInspection{}, relocationErr
			}
			if relocation.mismatch {
				return recoverySlotInspection{
					needsRecovery: true, needsRelocation: true, dirty: relocation.dirty,
					relocation: relocation,
				}, nil
			}
		}
		return recoverySlotInspection{}, nil
	}
	if inspection.class != linkedWorktreeMissingAdmin {
		return recoverySlotInspection{}, &WorktreeRecoveryError{
			TaskID: taskID, Checkout: wt.Path, PointerTarget: inspection.adminPath,
			ExpectedBacklink: inspection.expectedBacklink, ActualBacklink: inspection.actualBacklink,
			State: string(inspection.class), Reason: inspection.reason,
		}
	}
	repositoryPath := recoveryRepositoryPath(*slot)
	if repositoryPath == "" {
		return recoverySlotInspection{}, recoverySlotError(taskID, wt.Path, "repository path is missing")
	}
	if err := validateMissingLinkedWorktreeAdmin(repositoryPath, inspection.adminPath); err != nil {
		return recoverySlotInspection{}, &WorktreeRecoveryError{
			TaskID: taskID, Checkout: wt.Path, PointerTarget: inspection.adminPath,
			State: string(linkedWorktreeAmbiguous), Reason: err.Error(),
		}
	}
	if err := handle.VerifyPath(filepath.Clean(wt.Path)); err != nil {
		return recoverySlotInspection{}, recoverySlotError(taskID, wt.Path, err.Error())
	}
	if err := m.validateRecordedRecoveryBranch(ctx, wt, repositoryPath); err != nil {
		return recoverySlotInspection{}, err
	}
	return recoverySlotInspection{needsRecovery: true}, nil
}

func (m *Manager) recoveryOperationID(req *RecoveryAdmissionRequest, indices []int) (string, error) {
	if strings.TrimSpace(req.OperationID) != "" {
		operationID := strings.TrimSpace(req.OperationID)
		if _, err := uuid.Parse(operationID); err != nil {
			return "", recoveryAdmissionError(*req, "recovery operation ID is invalid")
		}
		return operationID, nil
	}
	operationID := ""
	for _, index := range indices {
		slot := req.Slots[index]
		if slot.Worktree == nil || slot.Worktree.Path == "" {
			continue
		}
		relocationOperationID, relocationErr := pendingManagedCloneRelocationOperationID(
			slot.Worktree.Path + ".kandev-clone-relocation.json",
		)
		if relocationErr != nil {
			return "", recoveryAdmissionError(*req, "managed-clone relocation record is unreadable")
		}
		if relocationOperationID != "" {
			if operationID != "" && operationID != relocationOperationID {
				return "", recoveryAdmissionError(*req, "selected recovery records use different operation identities")
			}
			operationID = relocationOperationID
		}
		record, err := readRecoveryRecord(slot.Worktree.Path + ".kandev-recovery.json")
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", recoveryAdmissionError(*req, fmt.Sprintf("read recovery record for %q: %v", slot.Worktree.Path, err))
		}
		if record.State != RecoveryStateSnapshotting && record.State != RecoveryStateRematerializing {
			return "", recoveryAdmissionError(*req, fmt.Sprintf("recovery record for %q is already %s", slot.Worktree.Path, record.State))
		}
		if _, err := uuid.Parse(record.OperationID); err != nil {
			return "", recoveryAdmissionError(*req, fmt.Sprintf("recovery record for %q has an invalid operation identity", slot.Worktree.Path))
		}
		if operationID == "" {
			operationID = record.OperationID
		} else if operationID != record.OperationID {
			return "", recoveryAdmissionError(*req, "selected recovery records use different operation identities")
		}
	}
	if operationID == "" {
		return uuid.NewString(), nil
	}
	return operationID, nil
}

func pendingManagedCloneRelocationOperationID(path string) (string, error) {
	record, err := readManagedCloneRelocationRecord(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if record.State != managedCloneRelocationStatePrepared && record.State != managedCloneRelocationStateMaterialized {
		return "", nil
	}
	return record.OperationID, nil
}

func (m *Manager) releaseRecoveryClaim(ctx context.Context, claim *models.TaskEnvironmentRecoveryClaim) error {
	claimStore, ok := m.store.(recoveryClaimStore)
	if !ok {
		return recoveryAdmissionError(RecoveryAdmissionRequest{TaskID: claim.OwnerTaskID, TaskEnvironmentID: claim.TaskEnvironmentID}, "durable recovery claim is unavailable")
	}
	return claimStore.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim)
}

func recoveryRepositoryPath(slot RecoverySlot) string {
	if strings.TrimSpace(slot.RepositoryPath) != "" {
		return slot.RepositoryPath
	}
	if slot.Worktree != nil {
		return slot.Worktree.RepositoryPath
	}
	return ""
}

func recoverySlotKey(slot RecoverySlot) string {
	if slot.WorktreeID != "" {
		return "id:" + slot.WorktreeID
	}
	if slot.Worktree != nil && slot.Worktree.ID != "" {
		return "id:" + slot.Worktree.ID
	}
	return "path:" + filepath.Clean(recoveryRepositoryPath(slot)) + "\x00" + slot.WorktreePath()
}

func (slot RecoverySlot) WorktreePath() string {
	if slot.Worktree == nil {
		return ""
	}
	return filepath.Clean(slot.Worktree.Path)
}

func recoveryClaimMatchesRequest(claim *models.TaskEnvironmentRecoveryClaim, req RecoveryAdmissionRequest) bool {
	return claim != nil && claim.TaskEnvironmentID == req.TaskEnvironmentID && claim.OwnerTaskID == req.OwnerTaskID &&
		claim.OwnershipGeneration == req.OwnershipGeneration && claim.SessionID == req.SessionID &&
		claim.ExecutorType == req.ExecutorType && (req.OperationID == "" || claim.OperationID == req.OperationID)
}

func recoveryAdmissionError(req RecoveryAdmissionRequest, reason string) error {
	return &WorktreeRecoveryError{TaskID: req.TaskID, State: "admission", Reason: reason}
}

func recoverySlotError(taskID, checkout, reason string) error {
	return &WorktreeRecoveryError{TaskID: taskID, Checkout: checkout, State: string(linkedWorktreeAmbiguous), Reason: reason}
}
