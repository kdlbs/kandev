"use client";

import { useMemo } from "react";
import { useSession } from "@/hooks/domains/session/use-session";
import { useWorkspaceRestoration } from "@/hooks/domains/session/use-workspace-restoration";
import { useRepository } from "@/hooks/domains/workspace/use-repository";
import { useSessionGitStatus } from "@/hooks/domains/session/use-session-git-status";
import { useAppStore } from "@/components/state-provider";
import { useTaskFolderAction } from "@/hooks/use-task-folder-action";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { useFileBrowserSearch, useFileBrowserTree } from "./file-browser-hooks";
import { useFileTreeCacheBinding } from "./file-browser-tree-state";
import { getFileBrowserSessionWorkspacePath, resolveFileBrowserPaths } from "./file-browser-path";
import {
  buildFileBrowserRepositoryLabels,
  workspaceInventoryRevision,
} from "./file-browser-repository-labels";

export function getFileBrowserResetKey({
  sessionId,
  environmentId,
  worktreeCount,
  inventoryRevision,
  workspaceFilesRefresh,
}: {
  sessionId: string;
  environmentId?: string | null;
  worktreeCount: number;
  inventoryRevision?: string;
  workspaceFilesRefresh: number;
}) {
  return `${environmentId ?? sessionId}:${worktreeCount}:${inventoryRevision ?? ""}:${workspaceFilesRefresh}`;
}

function useFileBrowserResetKey(
  sessionId: string,
  environmentId?: string | null,
  inventoryRevision = "",
) {
  const worktreeCount = useAppStore(
    (state) => state.sessionWorktreesBySessionId.itemsBySessionId[sessionId]?.length ?? 0,
  );
  const workspaceFilesRefresh = useAppStore(
    (state) => state.workspaceFilesRefresh.bySessionId[sessionId] ?? 0,
  );
  return getFileBrowserResetKey({
    sessionId,
    environmentId,
    worktreeCount,
    inventoryRevision,
    workspaceFilesRefresh,
  });
}

export function useFileBrowserData(sessionId: string, environmentId: string | null | undefined) {
  const { session, isFailed: isSessionFailed, errorMessage: sessionError } = useSession(sessionId);
  const workspaceRestoration = useWorkspaceRestoration(session?.task_id, sessionId, environmentId);
  const repository = useRepository(session?.repository_id ?? null);
  const repositoriesByWorkspaceId = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const repositories = useMemo(
    () => Object.values(repositoriesByWorkspaceId).flat(),
    [repositoriesByWorkspaceId],
  );
  const repositoryDisplayLabels = useMemo(
    () =>
      buildFileBrowserRepositoryLabels(session?.workspace_path, session?.worktrees, repositories),
    [repositories, session?.worktrees, session?.workspace_path],
  );
  const inventoryRevision = workspaceInventoryRevision(session?.worktrees);
  const gitStatus = useSessionGitStatus(sessionId);
  const folderAction = useTaskFolderAction(sessionId);
  const { copied, copy: copyPath } = useCopyToClipboard(1000);
  const resetKey = useFileBrowserResetKey(sessionId, environmentId, inventoryRevision);
  const cacheBinding = useFileTreeCacheBinding(environmentId ?? sessionId, resetKey);
  const search = useFileBrowserSearch(sessionId, cacheBinding, resetKey);
  const treeState = useFileBrowserTree(sessionId, resetKey, cacheBinding);
  const isTreeLoaded = !treeState.isLoadingTree && treeState.tree !== null;
  const fileStatuses = useMemo(
    () =>
      new Map(Object.entries(gitStatus?.files ?? {}).map(([path, info]) => [path, info.status])),
    [gitStatus?.files],
  );
  const paths = resolveFileBrowserPaths({
    sessionWorktreePath: getFileBrowserSessionWorkspacePath(session),
    repositoryLocalPath: repository?.local_path,
    treePath: treeState.tree?.path,
    treeLoaded: isTreeLoaded,
  });
  return {
    sessionId,
    isSessionFailed,
    sessionError,
    folderAction,
    copied,
    copyPath,
    search,
    treeState,
    workspaceRestoration,
    repositoryDisplayLabels,
    inventoryRevision,
    isTreeLoaded,
    fileStatuses,
    ...paths,
  };
}
