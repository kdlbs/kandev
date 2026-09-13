"use client";

import { useMemo } from "react";
import { useSession } from "@/hooks/domains/session/use-session";
import { useWorkspaceRestoration } from "@/hooks/domains/session/use-workspace-restoration";
import { useRepository } from "@/hooks/domains/workspace/use-repository";
import { useSessionGitStatus } from "@/hooks/domains/session/use-session-git-status";
import { useAppStore } from "@/components/state-provider";
import { useOpenSessionFolder } from "@/hooks/use-open-session-folder";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { useFileBrowserSearch, useFileBrowserTree } from "./file-browser-hooks";
import { getFileBrowserSessionWorkspacePath, resolveFileBrowserPaths } from "./file-browser-path";

export function getFileBrowserResetKey({
  sessionId,
  environmentId,
  worktreeCount,
  workspaceFilesRefresh,
}: {
  sessionId: string;
  environmentId?: string | null;
  worktreeCount: number;
  workspaceFilesRefresh: number;
}) {
  return `${environmentId ?? sessionId}:${worktreeCount}:${workspaceFilesRefresh}`;
}

function useFileBrowserResetKey(sessionId: string, environmentId?: string | null) {
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
    workspaceFilesRefresh,
  });
}

export function useFileBrowserData(sessionId: string, environmentId: string | null | undefined) {
  const { session, isFailed: isSessionFailed, errorMessage: sessionError } = useSession(sessionId);
  const workspaceRestoration = useWorkspaceRestoration(session?.task_id, sessionId, environmentId);
  const repository = useRepository(session?.repository_id ?? null);
  const gitStatus = useSessionGitStatus(sessionId);
  const { open: openFolder } = useOpenSessionFolder(sessionId);
  const { copied, copy: copyPath } = useCopyToClipboard(1000);
  const search = useFileBrowserSearch(sessionId);
  const resetKey = useFileBrowserResetKey(sessionId, environmentId);
  const treeState = useFileBrowserTree(sessionId, resetKey);
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
    isSessionFailed,
    sessionError,
    openFolder,
    copied,
    copyPath,
    search,
    treeState,
    workspaceRestoration,
    isTreeLoaded,
    fileStatuses,
    ...paths,
  };
}
