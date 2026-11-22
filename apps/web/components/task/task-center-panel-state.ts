import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import { useSessionGitStatus } from "@/hooks/domains/session/use-session-git-status";
import { useSessionCommits } from "@/hooks/domains/session/use-session-commits";
import { canAdmitTabletFile, finishTabletFileOpen } from "@/lib/state/tablet-file-authority";
import type { OpenFileTab } from "@/lib/types/backend";
import { calculateHash } from "@/lib/utils/file-diff";
import { useAppStore } from "@/components/state-provider";
import { useShallow } from "zustand/react/shallow";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";
import { resolveLatestTaskProjection } from "./task-page-content-helpers";
import type { AppState } from "@/lib/state/store";
import type { FileEditorTab } from "./task-center-panel-file-tabs";

function hasScopedFiles(
  repositories: Record<string, GitStatusEntry> | undefined,
  scopes: readonly string[] | null | undefined,
  legacy: GitStatusEntry | undefined,
): boolean {
  for (const repository in repositories) {
    if (
      (!scopes || scopes.includes(repository)) &&
      Object.keys(repositories[repository].files ?? {}).length > 0
    )
      return true;
  }
  return (
    (!scopes || scopes.includes(legacy?.repository_name ?? "")) &&
    Object.keys(legacy?.files ?? {}).length > 0
  );
}

function readyGitStatus(status: GitStatusEntry | undefined): boolean {
  return (
    !!status &&
    status.status_state !== "loading" &&
    status.status_state !== "unavailable" &&
    status.files_complete !== false &&
    status.detail_state !== "pending" &&
    status.detail_state !== "unavailable"
  );
}

/**
 * Only a validated inventory, or an unnamed status in a task with at most one
 * registered repository and no named status/refresh, defines scopes.
 */
function knownScopes(
  inventory: readonly string[] | null | undefined,
  repositories: Record<string, unknown> | undefined,
  repositoryRefresh: Record<string, unknown> | undefined,
  legacy: GitStatusEntry | undefined,
  taskRepositoryCount: number,
): readonly string[] | undefined {
  if (inventory) return inventory;
  const named =
    taskRepositoryCount > 1 ||
    [...Object.entries(repositories ?? {}), ...Object.entries(repositoryRefresh ?? {})].some(
      ([scope, value]) => scope !== "" && !!value,
    );
  return legacy && !legacy.repository_name && !named ? [""] : undefined;
}

type GitEvidence = {
  taskRepositoryCount: number;
  scopes?: readonly string[] | null;
  repositories?: Record<string, GitStatusEntry>;
  refresh?: unknown;
  repositoryRefresh?: Record<string, unknown>;
};

/** Git evidence plus the task page's own projection of the session's task (overview, then newest board row). */
function selectGitEvidence(state: AppState, sessionId: string | null): GitEvidence {
  if (!sessionId) return { taskRepositoryCount: 0 };
  const environment = state.environmentIdBySessionId[sessionId] ?? sessionId;
  const taskId = state.taskSessions.items[sessionId]?.task_id ?? state.tasks.activeTaskId;
  const gitStatus = state.gitStatus;
  return {
    taskRepositoryCount: taskId
      ? ((
          state.taskOverview.byId[taskId] ??
          resolveLatestTaskProjection(taskId, state.kanban.tasks, state.kanbanMulti.snapshots)
        )?.repositories?.length ?? 0)
      : 0,
    scopes: gitStatus.inventoryByEnvironment?.[environment]?.scopes,
    repositories: gitStatus.byEnvironmentRepo[environment],
    refresh: gitStatus.refreshByEnvironmentId?.[environment],
    repositoryRefresh: gitStatus.refreshByEnvironmentRepo?.[environment],
  };
}

export function useCenterPanelGitChanges(sessionId: string | null): boolean | undefined {
  const gitStatus = useSessionGitStatus(sessionId);
  const { commits, loaded, loading } = useSessionCommits(sessionId);
  const evidence = useAppStore(useShallow((state) => selectGitEvidence(state, sessionId)));
  const legacyScope = gitStatus?.repository_name ?? "";
  return useMemo(() => {
    const { repositories, refresh, repositoryRefresh } = evidence;
    const scopes = knownScopes(
      evidence.scopes,
      repositories,
      repositoryRefresh,
      gitStatus,
      evidence.taskRepositoryCount,
    );
    if (commits.length > 0 || hasScopedFiles(repositories, scopes, gitStatus)) return true;
    if (!scopes || refresh || !loaded || loading) return undefined;
    for (const repository of scopes) {
      const status =
        repositories?.[repository] ?? (repository === legacyScope ? gitStatus : undefined);
      if (repositoryRefresh?.[repository] || !readyGitStatus(status)) return undefined;
    }
    return false;
  }, [gitStatus, legacyScope, commits, loaded, loading, evidence]);
}

/** Ref and rendered tabs publish the same admission, including aliases restored between renders. */
export function useLiveFileTabs() {
  const current = useRef<FileEditorTab[]>([]);
  const [tabs, publish] = useState(current.current);
  const setTabs = useCallback<Dispatch<SetStateAction<FileEditorTab[]>>>((next) => {
    const admitted = typeof next === "function" ? next(current.current) : next;
    current.current = admitted;
    publish(admitted);
  }, []);
  return [tabs, setTabs, current] as const;
}

export function useTabletFileRequest(
  request: OpenFileTab | null,
  acknowledge: () => void,
  addFile: (file: OpenFileTab) => void,
) {
  useEffect(() => {
    if (!request) return;
    let current = true;
    queueMicrotask(async () => {
      if (!current) return;
      if (!canAdmitTabletFile(request)) {
        finishTabletFileOpen(request);
        acknowledge();
        return;
      }
      const hash = request.originalHash || (await calculateHash(request.content));
      if (!current) return;
      if (canAdmitTabletFile(request))
        addFile({
          ...request,
          originalContent: request.originalContent || request.content,
          originalHash: hash,
          isDirty: request.isDirty ?? false,
        });
      acknowledge();
      finishTabletFileOpen(request);
    });
    return () => {
      current = false;
    };
  }, [request, acknowledge, addFile]);
}
