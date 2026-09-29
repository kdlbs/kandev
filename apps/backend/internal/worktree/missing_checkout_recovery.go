package worktree

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
)

const missingCheckoutRecordSuffix = ".kandev-missing-checkout-recovery.json"

type missingCheckoutRecordState string

const (
	missingCheckoutRecordInProgress missingCheckoutRecordState = "in_progress"
	missingCheckoutRecordComplete   missingCheckoutRecordState = "complete"
)

type missingCheckoutSource string

const (
	missingCheckoutSourceLocal      missingCheckoutSource = "local"
	missingCheckoutSourceRemote     missingCheckoutSource = "remote"
	missingCheckoutSourceCompaction missingCheckoutSource = "compaction"
)

type missingCheckoutRecoveryRecord struct {
	Version             int                        `json:"version"`
	OperationID         string                     `json:"operation_id"`
	State               missingCheckoutRecordState `json:"state"`
	TaskID              string                     `json:"task_id"`
	TaskEnvironmentID   string                     `json:"task_environment_id"`
	OwnershipGeneration int64                      `json:"ownership_generation"`
	TaskDirName         string                     `json:"task_dir_name"`
	WorktreeID          string                     `json:"worktree_id"`
	RepositoryID        string                     `json:"repository_id"`
	RepositoryPath      string                     `json:"repository_path"`
	BranchSlug          string                     `json:"branch_slug"`
	Path                string                     `json:"path"`
	Branch              string                     `json:"branch"`
	Head                string                     `json:"head"`
	Source              missingCheckoutSource      `json:"source"`
	UpdatedAt           time.Time                  `json:"updated_at"`
}

type missingCheckoutBranchPlan struct {
	branch      string
	head        string
	source      missingCheckoutSource
	createLocal bool
	fetchRemote bool
}

type missingCheckoutInspection struct {
	needsRecovery          bool
	needsBranchReplacement bool
	pathPresent            bool
	rootMissing            bool
	pruneStale             bool
	plan                   missingCheckoutBranchPlan
	record                 *missingCheckoutRecoveryRecord
}

type missingCheckoutLocation struct {
	tasksBase string
	taskRoot  string
	name      string
}

type missingCheckoutRestoreHooks struct {
	beforeRegistrationCleanup func() error
	beforeTargetReservation   func() error
	beforeWorktreeAdd         func() error
}

func (m *Manager) inspectMissingCheckout(
	ctx context.Context,
	taskID string,
	ownershipGeneration int64,
	slot *RecoverySlot,
	pathPresent bool,
	allowBranchReplacement bool,
) (missingCheckoutInspection, error) {
	wt := slot.Worktree
	if !pathPresent && (wt.DeletedAt != nil || wt.Status != StatusActive) {
		return missingCheckoutInspection{}, missingCheckoutError(taskID, wt.Path, "automatic recovery requires an active worktree row")
	}
	location, managed, err := m.missingCheckoutLocation(wt)
	if err != nil {
		return missingCheckoutInspection{}, missingCheckoutError(taskID, wt.Path, err.Error())
	}
	if !managed {
		if pathPresent {
			return missingCheckoutInspection{}, nil
		}
		return missingCheckoutInspection{}, missingCheckoutError(taskID, wt.Path, "automatic recovery requires a persisted managed task-root identity")
	}
	parent, rootMissing, record, err := inspectMissingCheckoutTaskRoot(taskID, wt, location, pathPresent)
	if err != nil {
		return missingCheckoutInspection{}, err
	}
	if parent != nil {
		defer func() { _ = parent.Close() }()
	}
	if record != nil {
		inspection, handled, inspectErr := m.inspectMissingCheckoutRecord(
			ctx, taskID, ownershipGeneration, slot, parent, record, pathPresent,
		)
		if inspectErr != nil || handled {
			return inspection, inspectErr
		}
	}
	if pathPresent {
		return missingCheckoutInspection{}, nil
	}
	return m.inspectAbsentMissingCheckout(ctx, taskID, slot, location, parent, rootMissing, record, allowBranchReplacement)
}

func inspectMissingCheckoutTaskRoot(
	taskID string,
	wt *Worktree,
	location missingCheckoutLocation,
	pathPresent bool,
) (storageworkspaces.DirectoryHandle, bool, *missingCheckoutRecoveryRecord, error) {
	parent, rootMissing, err := openMissingCheckoutTaskRoot(location, false)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, false, nil, missingCheckoutError(taskID, wt.Path, fmt.Sprintf("cannot inspect managed task root: %v", err))
	}
	if errors.Is(err, os.ErrNotExist) {
		rootMissing = true
		parent = nil
	}
	if pathPresent && rootMissing {
		if parent != nil {
			_ = parent.Close()
		}
		return nil, false, nil, missingCheckoutError(taskID, wt.Path, "persisted checkout path is outside its managed task root")
	}
	if err := verifyMissingCheckoutTarget(parent, location.name, pathPresent); err != nil {
		if parent != nil {
			_ = parent.Close()
		}
		return nil, false, nil, missingCheckoutError(taskID, wt.Path, err.Error())
	}
	if parent == nil {
		return nil, rootMissing, nil, nil
	}
	record, err := readMissingCheckoutRecord(parent, location.name+missingCheckoutRecordSuffix)
	if err != nil {
		_ = parent.Close()
		return nil, false, nil, missingCheckoutError(taskID, wt.Path, fmt.Sprintf("cannot read recovery operation record: %v", err))
	}
	return parent, false, record, nil
}

func (m *Manager) inspectMissingCheckoutRecord(
	ctx context.Context,
	taskID string,
	ownershipGeneration int64,
	slot *RecoverySlot,
	parent storageworkspaces.DirectoryHandle,
	record *missingCheckoutRecoveryRecord,
	pathPresent bool,
) (missingCheckoutInspection, bool, error) {
	wt := slot.Worktree
	if err := validateMissingCheckoutRecord(*record); err != nil {
		return missingCheckoutInspection{}, true, missingCheckoutError(taskID, wt.Path, err.Error())
	}
	if record.State == missingCheckoutRecordComplete {
		return m.inspectCompletedMissingCheckout(ctx, taskID, ownershipGeneration, slot, record, pathPresent)
	}
	if err := validateMissingCheckoutRecordIdentity(*record, wt, slot.RepositoryPath, ownershipGeneration, true); err != nil {
		return missingCheckoutInspection{}, true, missingCheckoutError(taskID, wt.Path, err.Error())
	}
	if parent == nil {
		return missingCheckoutInspection{}, true, missingCheckoutError(taskID, wt.Path, "recovery record has no managed task root")
	}
	if err := validateMissingCheckoutTaskRoot(parent, wt); err != nil {
		return missingCheckoutInspection{}, true, missingCheckoutError(taskID, wt.Path, err.Error())
	}
	if pathPresent {
		if err := m.verifyMissingCheckoutResult(ctx, wt, *record); err != nil {
			return missingCheckoutInspection{}, true, missingCheckoutError(taskID, wt.Path, fmt.Sprintf("interrupted recovery produced an unexpected checkout: %v", err))
		}
		return missingCheckoutInspection{needsRecovery: true, pathPresent: true, record: record}, true, nil
	}
	if err := m.validateInterruptedMissingCheckoutCommit(ctx, taskID, slot, *record); err != nil {
		return missingCheckoutInspection{}, true, err
	}
	plan, err := m.missingCheckoutPlanFromRecord(ctx, slot.RepositoryPath, wt, *record)
	if err != nil {
		return missingCheckoutInspection{}, true, missingCheckoutError(taskID, wt.Path, err.Error())
	}
	prune, err := m.inspectMissingCheckoutRegistration(ctx, slot.RepositoryPath, wt.Path, plan)
	if err != nil {
		return missingCheckoutInspection{}, true, missingCheckoutError(taskID, wt.Path, err.Error())
	}
	return missingCheckoutInspection{needsRecovery: true, pruneStale: prune, plan: plan, record: record}, true, nil
}

func (m *Manager) inspectCompletedMissingCheckout(
	ctx context.Context,
	taskID string,
	ownershipGeneration int64,
	slot *RecoverySlot,
	record *missingCheckoutRecoveryRecord,
	pathPresent bool,
) (missingCheckoutInspection, bool, error) {
	if pathPresent {
		if err := m.verifyMissingCheckoutResult(ctx, slot.Worktree, *record); err != nil {
			return missingCheckoutInspection{}, true, missingCheckoutError(taskID, slot.Worktree.Path, fmt.Sprintf("completed recovery record does not match the checkout: %v", err))
		}
		return missingCheckoutInspection{pathPresent: true, record: record}, true, nil
	}
	if err := validateMissingCheckoutRecordIdentity(*record, slot.Worktree, slot.RepositoryPath, ownershipGeneration, false); err != nil {
		return missingCheckoutInspection{}, true, missingCheckoutError(taskID, slot.Worktree.Path, "completed recovery record conflicts with the missing checkout")
	}
	return missingCheckoutInspection{record: record}, false, nil
}

func (m *Manager) validateInterruptedMissingCheckoutCommit(
	ctx context.Context,
	taskID string,
	slot *RecoverySlot,
	record missingCheckoutRecoveryRecord,
) error {
	if _, err := m.resolveCommit(ctx, slot.RepositoryPath, record.Head); err == nil {
		return nil
	}
	if record.Source != missingCheckoutSourceRemote {
		return missingCheckoutError(taskID, slot.Worktree.Path, "interrupted recovery commit is unavailable")
	}
	remoteHead, exists, err := m.missingCheckoutRemoteHead(ctx, slot.RepositoryPath, record.Branch)
	if err != nil {
		return missingCheckoutProbeError(taskID, slot.Worktree.Path, err)
	}
	if !exists || !strings.EqualFold(remoteHead, record.Head) {
		return missingCheckoutError(taskID, slot.Worktree.Path, "interrupted recovery branch head is no longer available on origin")
	}
	return nil
}

func (m *Manager) inspectAbsentMissingCheckout(
	ctx context.Context,
	taskID string,
	slot *RecoverySlot,
	location missingCheckoutLocation,
	parent storageworkspaces.DirectoryHandle,
	rootMissing bool,
	record *missingCheckoutRecoveryRecord,
	allowBranchReplacement bool,
) (missingCheckoutInspection, error) {
	wt := slot.Worktree
	if parent != nil {
		if err := validateMissingCheckoutTaskRoot(parent, wt); err != nil {
			return missingCheckoutInspection{}, missingCheckoutError(taskID, wt.Path, err.Error())
		}
		if err := verifyMissingCheckoutTarget(parent, location.name, false); err != nil {
			return missingCheckoutInspection{}, missingCheckoutError(taskID, wt.Path, err.Error())
		}
	}
	plan, err := m.resolveMissingCheckoutBranch(ctx, taskID, slot.RepositoryPath, wt)
	if err != nil {
		var branchLoss *BranchUnrecoverableError
		if allowBranchReplacement && errors.As(err, &branchLoss) {
			return missingCheckoutInspection{
				needsRecovery: true, needsBranchReplacement: true,
				record: record,
			}, nil
		}
		return missingCheckoutInspection{}, err
	}
	prune, err := m.inspectMissingCheckoutRegistration(ctx, slot.RepositoryPath, wt.Path, plan)
	if err != nil {
		return missingCheckoutInspection{}, missingCheckoutError(taskID, wt.Path, err.Error())
	}
	return missingCheckoutInspection{
		needsRecovery: true, rootMissing: rootMissing, pruneStale: prune, plan: plan, record: record,
	}, nil
}

func (m *Manager) missingCheckoutLocation(wt *Worktree) (missingCheckoutLocation, bool, error) {
	if wt == nil || wt.Path == "" || wt.TaskDirName == "" {
		return missingCheckoutLocation{}, false, nil
	}
	if !validMissingCheckoutTaskDirName(wt.TaskDirName) {
		return missingCheckoutLocation{}, true, fmt.Errorf("persisted managed task-root identity is invalid")
	}
	tasksBase, err := m.config.ExpandedTasksBasePath()
	if err != nil {
		return missingCheckoutLocation{}, true, fmt.Errorf("resolve managed tasks base: %w", err)
	}
	tasksBase, err = filepath.Abs(tasksBase)
	if err != nil {
		return missingCheckoutLocation{}, true, fmt.Errorf("resolve managed tasks base: %w", err)
	}
	path, err := filepath.Abs(wt.Path)
	if err != nil {
		return missingCheckoutLocation{}, true, fmt.Errorf("resolve persisted checkout path: %w", err)
	}
	tasksBase = filepath.Clean(tasksBase)
	path = filepath.Clean(path)
	name, managed, err := missingCheckoutNameUnderTaskRoot(tasksBase, path, wt.TaskDirName)
	if err != nil {
		return missingCheckoutLocation{}, true, err
	}
	if !managed {
		return missingCheckoutLocation{}, false, nil
	}
	return missingCheckoutLocation{tasksBase: tasksBase, taskRoot: filepath.Join(tasksBase, wt.TaskDirName), name: name}, true, nil
}

func validMissingCheckoutTaskDirName(name string) bool {
	return filepath.Base(name) == name && name != "." && name != ".." && !strings.ContainsAny(name, "/\\\x00")
}

func missingCheckoutNameUnderTaskRoot(tasksBase, path, taskDirName string) (string, bool, error) {
	relative, err := filepath.Rel(tasksBase, path)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false, nil
	}
	parts := strings.Split(relative, string(filepath.Separator))
	wantRoot := filepath.Join(tasksBase, taskDirName)
	if len(parts) != 2 || parts[0] != taskDirName || filepath.Dir(path) != wantRoot {
		return "", true, fmt.Errorf("persisted checkout path does not match its managed task-root identity")
	}
	return parts[1], true, nil
}

func openMissingCheckoutTaskRoot(location missingCheckoutLocation, create bool) (storageworkspaces.DirectoryHandle, bool, error) {
	if create {
		handle, err := storageworkspaces.CreateDirectoryNoFollow(location.tasksBase, location.taskRoot, 0o755)
		return handle, false, err
	}
	handle, err := storageworkspaces.OpenDirectoryNoFollow(location.tasksBase, location.taskRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil, true, nil
	}
	return handle, false, err
}

func validateMissingCheckoutTaskRoot(parent storageworkspaces.DirectoryHandle, wt *Worktree) error {
	if parent == nil || wt == nil || wt.TaskDirName == "" {
		return fmt.Errorf("managed task-root identity is unavailable")
	}
	mode, err := parent.LstatEntry(storageworkspaces.OwnershipMarkerFilename)
	if err != nil {
		return fmt.Errorf("inspect managed task-root ownership marker: %w", err)
	}
	if !mode.IsRegular() {
		return fmt.Errorf("managed task-root ownership marker is not a regular file")
	}
	data, err := parent.ReadFile(storageworkspaces.OwnershipMarkerFilename)
	if err != nil {
		return fmt.Errorf("read managed task-root ownership marker: %w", err)
	}
	var marker storageworkspaces.OwnershipMarker
	// Environment ownership can transfer while the physical task root keeps its
	// original task ID. The stable task-directory identity fences that root.
	if err := json.Unmarshal(data, &marker); err != nil || marker.TaskID == "" || marker.TaskDirName != wt.TaskDirName ||
		(marker.LayoutVersion != storageworkspaces.LayoutVersionSemantic && marker.LayoutVersion != storageworkspaces.LayoutVersionScratch) {
		return fmt.Errorf("managed task-root ownership marker does not match the persisted identity")
	}
	return nil
}

func verifyMissingCheckoutTarget(parent storageworkspaces.DirectoryHandle, name string, present bool) error {
	if parent == nil {
		if present {
			return fmt.Errorf("managed checkout parent is unavailable")
		}
		return nil
	}
	mode, err := parent.LstatEntry(name)
	if errors.Is(err, os.ErrNotExist) {
		if present {
			return fmt.Errorf("checkout disappeared during recovery inspection")
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect checkout path without following links: %w", err)
	}
	if !present {
		return fmt.Errorf("checkout path appeared during recovery inspection")
	}
	if !mode.IsDir() || mode&os.ModeSymlink != 0 {
		return fmt.Errorf("persisted checkout path is not a real directory")
	}
	return nil
}

func readMissingCheckoutRecord(parent storageworkspaces.DirectoryHandle, name string) (*missingCheckoutRecoveryRecord, error) {
	mode, err := parent.LstatEntry(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !mode.IsRegular() {
		return nil, fmt.Errorf("recovery operation record is not a regular file")
	}
	data, err := parent.ReadFile(name)
	if err != nil {
		return nil, err
	}
	var record missingCheckoutRecoveryRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("decode recovery operation record: %w", err)
	}
	return &record, nil
}

func validateMissingCheckoutRecord(record missingCheckoutRecoveryRecord) error {
	if !missingCheckoutRecordFieldsComplete(record) {
		return fmt.Errorf("recovery operation record is incomplete")
	}
	if _, err := uuid.Parse(record.OperationID); err != nil {
		return fmt.Errorf("recovery operation record has an invalid operation identity")
	}
	if !validMissingCheckoutRecordState(record.State) {
		return fmt.Errorf("recovery operation record has an unsupported state")
	}
	if !validMissingCheckoutSource(record.Source) {
		return fmt.Errorf("recovery operation record has an unsupported branch source")
	}
	return nil
}

func missingCheckoutRecordFieldsComplete(record missingCheckoutRecoveryRecord) bool {
	return record.Version == 1 && record.TaskID != "" && record.TaskEnvironmentID != "" &&
		record.OwnershipGeneration > 0 && record.TaskDirName != "" && record.WorktreeID != "" &&
		record.RepositoryID != "" && record.RepositoryPath != "" && record.Path != "" &&
		record.Branch != "" && commitSHA.MatchString(record.Head) && !record.UpdatedAt.IsZero()
}

func validMissingCheckoutRecordState(state missingCheckoutRecordState) bool {
	return state == missingCheckoutRecordInProgress || state == missingCheckoutRecordComplete
}

func validMissingCheckoutSource(source missingCheckoutSource) bool {
	return source == missingCheckoutSourceLocal || source == missingCheckoutSourceRemote || source == missingCheckoutSourceCompaction
}

func validateMissingCheckoutRecordIdentity(
	record missingCheckoutRecoveryRecord,
	wt *Worktree,
	repositoryPath string,
	ownershipGeneration int64,
	includeGeneration bool,
) error {
	if wt == nil || !sameAbsoluteCleanPath(wt.Path, record.Path) || !sameAbsoluteCleanPath(repositoryPath, record.RepositoryPath) ||
		record.TaskID != wt.TaskID || record.TaskEnvironmentID != wt.TaskEnvironmentID ||
		record.WorktreeID != wt.ID || record.RepositoryID != wt.RepositoryID ||
		record.TaskDirName != wt.TaskDirName || record.BranchSlug != wt.BranchSlug ||
		record.Branch != wt.Branch || record.TaskDirName == "" {
		return fmt.Errorf("recovery operation record does not match the selected worktree identity")
	}
	if includeGeneration && record.OwnershipGeneration != ownershipGeneration {
		return fmt.Errorf("recovery operation record belongs to a different ownership generation")
	}
	return nil
}

func sameAbsoluteCleanPath(left, right string) bool {
	leftPath, leftErr := filepath.Abs(left)
	rightPath, rightErr := filepath.Abs(right)
	return leftErr == nil && rightErr == nil && filepath.Clean(leftPath) == filepath.Clean(rightPath)
}

func (m *Manager) resolveMissingCheckoutBranch(
	ctx context.Context,
	taskID, repositoryPath string,
	wt *Worktree,
) (missingCheckoutBranchPlan, error) {
	branch := strings.TrimSpace(wt.Branch)
	if branch == "" || branch != wt.Branch || strings.HasPrefix(branch, "refs/") {
		return missingCheckoutBranchPlan{}, missingCheckoutError(taskID, wt.Path, "recorded branch identity is missing or invalid")
	}
	if _, err := m.runBoundedGitInspect(ctx, repositoryPath, "check-ref-format", "--branch", branch); err != nil {
		return missingCheckoutBranchPlan{}, missingCheckoutError(taskID, wt.Path, "recorded branch name is invalid")
	}
	plan, found, err := m.resolveLocalMissingCheckoutBranch(ctx, taskID, repositoryPath, wt, branch)
	if err != nil {
		return missingCheckoutBranchPlan{}, err
	}
	if found {
		return plan, nil
	}
	plan, found, err = m.resolveCompactedMissingCheckoutBranch(ctx, taskID, repositoryPath, wt, branch)
	if err != nil {
		return missingCheckoutBranchPlan{}, err
	}
	if found {
		return plan, nil
	}
	return m.resolveRemoteMissingCheckoutBranch(ctx, taskID, repositoryPath, wt, branch)
}

func (m *Manager) resolveLocalMissingCheckoutBranch(
	ctx context.Context,
	taskID string,
	repositoryPath string,
	wt *Worktree,
	branch string,
) (missingCheckoutBranchPlan, bool, error) {
	localRef := "refs/heads/" + branch
	exists, err := m.branchExists(ctx, repositoryPath, localRef)
	if err != nil {
		return missingCheckoutBranchPlan{}, false, missingCheckoutError(taskID, wt.Path, "cannot inspect the recorded local branch")
	}
	if !exists {
		return missingCheckoutBranchPlan{}, false, nil
	}
	head, err := m.resolveCommit(ctx, repositoryPath, localRef)
	if err != nil {
		return missingCheckoutBranchPlan{}, false, missingCheckoutError(taskID, wt.Path, "cannot resolve the recorded local branch head")
	}
	return missingCheckoutBranchPlan{branch: branch, head: head, source: missingCheckoutSourceLocal}, true, nil
}

func (m *Manager) resolveCompactedMissingCheckoutBranch(
	ctx context.Context,
	taskID string,
	repositoryPath string,
	wt *Worktree,
	branch string,
) (missingCheckoutBranchPlan, bool, error) {
	if wt.BranchCompactedAt == nil || wt.BranchOwner != BranchOwnerManaged || wt.RecoveryHeadSHA == "" {
		return missingCheckoutBranchPlan{}, false, nil
	}
	head, err := m.resolveCommit(ctx, repositoryPath, wt.RecoveryHeadSHA)
	if err != nil || head != strings.ToLower(wt.RecoveryHeadSHA) {
		return missingCheckoutBranchPlan{}, false, missingCheckoutError(taskID, wt.Path, "recorded compacted branch head is unavailable")
	}
	return missingCheckoutBranchPlan{branch: branch, head: head, source: missingCheckoutSourceCompaction, createLocal: true}, true, nil
}

func (m *Manager) resolveRemoteMissingCheckoutBranch(
	ctx context.Context,
	taskID, repositoryPath string,
	wt *Worktree,
	branch string,
) (missingCheckoutBranchPlan, error) {
	remoteName := normalizeOriginBranchName(branch)
	if remoteName == "" {
		return missingCheckoutBranchPlan{}, missingCheckoutError(taskID, wt.Path, "recorded branch is unavailable")
	}
	remoteRef := "refs/remotes/origin/" + remoteName
	remoteExists, err := m.branchExists(ctx, repositoryPath, remoteRef)
	if err != nil {
		return missingCheckoutBranchPlan{}, missingCheckoutError(taskID, wt.Path, "cannot inspect the recorded remote branch")
	}
	if !remoteExists {
		return m.resolveUntrackedRemoteMissingCheckoutBranch(ctx, taskID, repositoryPath, wt, branch)
	}
	head, err := m.resolveCommit(ctx, repositoryPath, remoteRef)
	if err != nil {
		return missingCheckoutBranchPlan{}, missingCheckoutError(taskID, wt.Path, "cannot resolve the recorded remote branch head")
	}
	return missingCheckoutBranchPlan{branch: branch, head: head, source: missingCheckoutSourceRemote, createLocal: true}, nil
}

func (m *Manager) resolveUntrackedRemoteMissingCheckoutBranch(
	ctx context.Context,
	taskID, repositoryPath string,
	wt *Worktree,
	branch string,
) (missingCheckoutBranchPlan, error) {
	remoteHead, exists, err := m.missingCheckoutRemoteHead(ctx, repositoryPath, branch)
	if err != nil {
		return missingCheckoutBranchPlan{}, missingCheckoutProbeError(taskID, wt.Path, err)
	}
	if !exists {
		return missingCheckoutBranchPlan{}, fmt.Errorf("%w: %w", ErrReuseWorktreeUnavailable, &BranchUnrecoverableError{Branch: branch})
	}
	return missingCheckoutBranchPlan{
		branch: branch, head: remoteHead, source: missingCheckoutSourceRemote,
		createLocal: true, fetchRemote: true,
	}, nil
}

func (m *Manager) missingCheckoutRemoteHead(ctx context.Context, repositoryPath, branch string) (string, bool, error) {
	remoteName := normalizeOriginBranchName(branch)
	if remoteName == "" {
		return "", false, fmt.Errorf("remote branch name is empty: %w", ErrGitCommandFailed)
	}
	wantRef := "refs/heads/" + remoteName
	output, err := m.runBoundedGitInspect(
		ctx, repositoryPath, "ls-remote", "--exit-code", "--heads", "origin", wantRef,
	)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 2 {
			return "", false, nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", false, err
		}
		if containsAuthFailure(strings.ToLower(output)) {
			return "", false, ErrAuthFailed
		}
		return "", false, fmt.Errorf("%w: authoritative origin branch probe failed: %v", ErrGitCommandFailed, err)
	}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[1] != wantRef {
			continue
		}
		if !commitSHA.MatchString(fields[0]) {
			return "", false, fmt.Errorf("%w: origin returned an invalid commit for %q", ErrGitCommandFailed, wantRef)
		}
		return strings.ToLower(fields[0]), true, nil
	}
	return "", false, fmt.Errorf("%w: origin probe returned no exact ref for %q", ErrGitCommandFailed, wantRef)
}

func missingCheckoutProbeError(taskID, checkout string, cause error) error {
	return fmt.Errorf("%w: %w", missingCheckoutError(taskID, checkout, "cannot determine whether the recorded origin branch survives"), cause)
}

func (m *Manager) missingCheckoutPlanFromRecord(
	ctx context.Context,
	repositoryPath string,
	wt *Worktree,
	record missingCheckoutRecoveryRecord,
) (missingCheckoutBranchPlan, error) {
	localRef := "refs/heads/" + record.Branch
	localExists, err := m.branchExists(ctx, repositoryPath, localRef)
	if err != nil {
		return missingCheckoutBranchPlan{}, fmt.Errorf("cannot inspect interrupted recovery branch")
	}
	if localExists {
		head, err := m.resolveCommit(ctx, repositoryPath, localRef)
		if err != nil || head != strings.ToLower(record.Head) {
			return missingCheckoutBranchPlan{}, fmt.Errorf("interrupted recovery branch changed from its recorded head")
		}
		return missingCheckoutBranchPlan{branch: record.Branch, head: strings.ToLower(record.Head), source: record.Source}, nil
	}
	if record.Source == missingCheckoutSourceLocal {
		return missingCheckoutBranchPlan{}, fmt.Errorf("interrupted recovery local branch is no longer available")
	}
	_, commitErr := m.resolveCommit(ctx, repositoryPath, record.Head)
	return missingCheckoutBranchPlan{
		branch: record.Branch, head: strings.ToLower(record.Head), source: record.Source,
		createLocal: true, fetchRemote: record.Source == missingCheckoutSourceRemote && commitErr != nil,
	}, nil
}

func (m *Manager) inspectMissingCheckoutRegistration(
	ctx context.Context,
	repositoryPath, worktreePath string,
	plan missingCheckoutBranchPlan,
) (bool, error) {
	output, err := m.runBoundedGitInspect(ctx, repositoryPath, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return false, fmt.Errorf("cannot inspect linked-worktree registrations")
	}
	wantPath, err := normalizedWorktreeTargetPath(worktreePath)
	if err != nil {
		return false, err
	}
	wantBranch := "refs/heads/" + plan.branch
	staleRegistration := false
	for _, registration := range parseWorktreeRegistrations(output) {
		registeredPath, err := normalizedWorktreeTargetPath(registration.path)
		if err != nil {
			return false, fmt.Errorf("linked-worktree registration path is invalid")
		}
		if registeredPath == wantPath {
			if staleRegistration || !registration.prunable || registration.locked ||
				registration.branch != wantBranch || !strings.EqualFold(registration.head, plan.head) {
				return false, fmt.Errorf("recorded checkout has a conflicting Git registration")
			}
			staleRegistration = true
			continue
		}
		if registration.branch == wantBranch {
			return false, fmt.Errorf("recorded branch is checked out in another worktree")
		}
	}
	return staleRegistration, nil
}
