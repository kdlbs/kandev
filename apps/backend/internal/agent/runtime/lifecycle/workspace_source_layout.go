package lifecycle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

// workspaceFoldersSupported reports whether an executor can expose host
// directories directly to the agent process. Remote and container executors
// have no safe host path to link into their workspace.
func workspaceFoldersSupported(executorType string) bool {
	return executorType == string(models.ExecutorTypeLocal) ||
		executorType == legacyExecutorTypeLocalPC ||
		executorType == string(models.ExecutorTypeWorktree)
}

func validateWorkspaceFolderExecutor(executorType string, folders []WorkspaceFolderSpec) error {
	if len(folders) == 0 || workspaceFoldersSupported(executorType) {
		return nil
	}
	return fmt.Errorf("workspace folders are not supported by executor %q", executorType)
}

// workspaceSourcesNeedManagedRoot identifies layouts that need a Kandev-owned
// directory containing named source siblings. A single folder keeps its direct
// host path as the working directory; every other folder-bearing layout gets a
// separate root so generated entries never appear inside a user folder.
func workspaceSourcesNeedManagedRoot(req *LaunchRequest) bool {
	if req == nil || len(req.WorkspaceFolders) == 0 {
		return false
	}
	return len(req.RepoSpecs())+len(req.WorkspaceFolders) > 1
}

// workspaceLinkOwner uses the scratch marker's task ID when a legacy or local
// executor request has no semantic task directory name. This keeps relinking
// safe across restart while preserving semantic worktree ownership when it is
// available.
func workspaceLinkOwner(taskID, taskDirName string) worktree.OwnedDirectoryLinkOwner {
	if taskDirName == "" {
		taskDirName = taskID
	}
	return ownedDirectoryLinkOwner(taskID, taskDirName)
}

func validateWorkspaceFolderTargets(root string, folders []WorkspaceFolderSpec) error {
	if len(folders) == 0 {
		return nil
	}
	if root == "" {
		return ErrSessionWorkspaceNotReady
	}
	rootInfo, err := os.Stat(root)
	if err != nil || !rootInfo.IsDir() {
		return fmt.Errorf("workspace root is missing: %s", root)
	}
	seenNames := make(map[string]struct{}, len(folders))
	seenTargets := make(map[string]struct{}, len(folders))
	for _, folder := range folders {
		if !isWorkspaceEntryName(folder.Name) || folder.LocalPath == "" {
			return fmt.Errorf("invalid durable workspace folder")
		}
		if _, exists := seenNames[folder.Name]; exists {
			return fmt.Errorf("workspace folder %q is duplicated", folder.Name)
		}
		seenNames[folder.Name] = struct{}{}

		info, err := os.Stat(folder.LocalPath)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("workspace folder %q target is missing: %s", folder.Name, folder.LocalPath)
		}
		canonical, err := filepath.EvalSymlinks(folder.LocalPath)
		if err != nil {
			return fmt.Errorf("canonicalize workspace folder %q: %w", folder.Name, err)
		}
		if _, exists := seenTargets[canonical]; exists {
			return fmt.Errorf("workspace folder %q target is duplicated", folder.Name)
		}
		seenTargets[canonical] = struct{}{}
	}
	return nil
}

// resolveWorkspaceInfoPath fills the path omitted by folder-only sessions that
// have not launched yet. It is deliberately separate from service projection:
// the lifecycle manager owns the filesystem root and can apply the same
// recovery rules to HTTP, WS, terminal, and restart entry points.
func (m *Manager) resolveWorkspaceInfoPath(ctx context.Context, info *WorkspaceInfo) {
	if info == nil || info.WorkspacePath != "" || len(info.WorkspaceFolders) == 0 {
		return
	}
	if !workspaceFoldersSupported(info.ExecutorType) {
		return
	}
	if len(info.WorkspaceRepositories) == 0 && len(info.WorkspaceFolders) == 1 {
		info.WorkspacePath = info.WorkspaceFolders[0].LocalPath
		return
	}
	if len(info.WorkspaceRepositories)+len(info.WorkspaceFolders) <= 1 {
		return
	}
	info.WorkspacePath = m.resolveScratchWorkspace(ctx, &LaunchRequest{
		TaskID:      info.TaskID,
		WorkspaceID: info.WorkspaceID,
		SessionID:   info.SessionID,
		IsEphemeral: false,
	})
}

// resolveManagedWorkspaceRoot selects the task-owned root after repository
// preparation. Local preparation starts from the selected checkout, while a
// worktree preparer returns the first named worktree. Both must be moved to the
// shared root before the agent runtime starts when folders are also present.
func (m *Manager) resolveManagedWorkspaceRoot(
	ctx context.Context,
	req *LaunchRequest,
	prepResult *EnvPrepareResult,
	resolvedPath string,
) string {
	if req == nil {
		return resolvedPath
	}
	if len(req.RepoSpecs()) == 0 {
		return resolvedPath
	}
	if req.WorkspacePath != "" && resolvedPath != "" && !sameDirectory(resolvedPath, req.RepositoryPath) {
		return resolvedPath
	}
	if req.UseWorktree && prepResult != nil && prepResult.WorkspacePath != "" {
		return filepath.Dir(prepResult.WorkspacePath)
	}
	return m.resolveScratchWorkspace(ctx, req)
}

func (m *Manager) reconcileWorkspaceSourcesForLaunch(
	ctx context.Context,
	req *LaunchRequest,
	workspacePath string,
) error {
	owner := workspaceLinkOwner(req.TaskID, req.TaskDirName)
	if err := reconcileWorkspaceSources(ctx, workspacePath, req.WorkspaceFolders, owner); err != nil {
		return err
	}
	if !workspaceRepositoryLinksSupported(req.ExecutorType) {
		return nil
	}
	return reconcileWorkspaceRepositories(
		workspacePath,
		workspaceRepositorySpecsFromLaunch(req),
		m.logger,
		owner,
	)
}

func workspaceRepositoryLinksSupported(executorType string) bool {
	return executorType == string(models.ExecutorTypeLocal) || executorType == legacyExecutorTypeLocalPC
}

func (m *Manager) prepareManagedWorkspaceSources(
	ctx context.Context,
	req *LaunchRequest,
	prepResult *EnvPrepareResult,
	workspacePath string,
) (string, error) {
	workspacePath = m.resolveManagedWorkspaceRoot(ctx, req, prepResult, workspacePath)
	if workspacePath == "" {
		return "", fmt.Errorf("workspace root is required for mixed workspace sources")
	}
	if err := m.reconcileWorkspaceSourcesForLaunch(ctx, req, workspacePath); err != nil {
		return "", err
	}
	req.WorkspacePath = workspacePath
	return workspacePath, nil
}
