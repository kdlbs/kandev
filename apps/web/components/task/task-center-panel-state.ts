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

export function useCenterPanelGitChanges(sessionId: string | null): boolean | undefined {
  const gitStatus = useSessionGitStatus(sessionId);
  const { commits, loaded, loading } = useSessionCommits(sessionId);
  const evidence = useAppStore(
    useShallow((state) => {
      const environment = sessionId
        ? (state.environmentIdBySessionId[sessionId] ?? sessionId)
        : null;
      return {
        scopes: environment
          ? state.gitStatus.inventoryByEnvironment?.[environment]?.scopes
          : undefined,
        repositories: environment ? state.gitStatus.byEnvironmentRepo[environment] : undefined,
        refresh: environment ? state.gitStatus.refreshByEnvironmentId?.[environment] : undefined,
        repositoryRefresh: environment
          ? state.gitStatus.refreshByEnvironmentRepo?.[environment]
          : undefined,
      };
    }),
  );
  const legacyScope = gitStatus?.repository_name ?? "";
  return useMemo(() => {
    const { scopes, repositories, refresh, repositoryRefresh } = evidence;
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
