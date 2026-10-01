package worktree

import (
	"context"
	"path/filepath"
	"strings"
)

// A computed destination is only a layout candidate until the registered source changes.
func (m *Manager) inspectRegisteredLegacyClone(
	ctx context.Context, taskID string, wt *Worktree, proof *ManagedCloneRelocationProof,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	source := registeredLegacyClonePath(wt.RepositoryPath, proof)
	if source == "" {
		return false, nil
	}
	common, err := gitCommonDir(ctx, m, wt.Path)
	if err != nil {
		return false, legacyCloneInspectionError(ctx, taskID, "cannot verify the worktree Git identity", err)
	}
	if !sameDirectoryIdentity(common, filepath.Join(source, ".git")) {
		return false, managedCloneRelocationError(taskID, "worktree does not match the registered legacy clone")
	}
	if err := verifyRecordedManagedCloneSource(source, common, proof); err != nil {
		return false, managedCloneRelocationError(taskID, "recorded source clone identity does not match Git registration")
	}
	if err := verifyManagedCloneOrigin(ctx, m, source, proof.Identity); err != nil {
		return false, legacyCloneInspectionError(ctx, taskID, "managed clone provider identity could not be verified", err)
	}
	if _, _, err := inspectManagedCloneBranch(ctx, m, taskID, source, wt); err != nil {
		return false, legacyCloneInspectionError(ctx, taskID, "registered legacy clone branch could not be verified", err)
	}
	return true, nil
}

func registeredLegacyClonePath(repositoryPath string, proof *ManagedCloneRelocationProof) string {
	root, err := canonicalExistingPath(proof.ManagedRoot)
	if err != nil {
		return ""
	}
	registered, err := canonicalExistingPath(repositoryPath)
	if err != nil {
		return ""
	}
	for _, candidate := range []string{proof.ExpectedSourcePath, proof.LegacyOwnerNameSourcePath} {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		source, err := canonicalExistingPath(candidate)
		if err == nil && pathWithin(root, source) && sameDirectoryIdentity(registered, source) {
			return source
		}
	}
	return ""
}

func legacyCloneInspectionError(ctx context.Context, taskID, reason string, err error) error {
	if operationalErr := checkoutInspectionOperationalError(ctx, err); operationalErr != nil {
		return operationalErr
	}
	return managedCloneRelocationError(taskID, reason)
}
