import {
  useEffect,
  useRef,
  type Dispatch,
  type MutableRefObject,
  type SetStateAction,
} from "react";
import type { FileInfo } from "@/lib/state/slices/session-runtime/types";
import type { useSessionGitStatusByRepo } from "./use-session-git-status";
import { splitFilesByChangeLayer } from "./git-change-facets";

export type PendingFileOperation = "stage" | "unstage";

export type PendingFileOperationOwner = {
  operation: PendingFileOperation;
  requestId: number;
  scopeIdentity: string;
  responseSucceeded: boolean;
  targetStateObserved: boolean;
};

export function pendingKey(repo: string | undefined, path: string): string {
  return `${repo ?? ""}::${path}`;
}

export function pendingKeysForFailedRepositories(
  buckets: Map<string, string[]>,
  perRepoResults: Array<{ repository_name: string; success: boolean }> | undefined,
): string[] {
  const allKeys = Array.from(buckets).flatMap(([repositoryName, paths]) =>
    paths.map((path) => pendingKey(repositoryName, path)),
  );
  if (!perRepoResults) return allKeys;

  const failedRepositories = new Set(
    perRepoResults.filter((entry) => !entry.success).map((entry) => entry.repository_name),
  );
  if (failedRepositories.size === 0) return allKeys;

  return Array.from(buckets).flatMap(([repositoryName, paths]) =>
    failedRepositories.has(repositoryName)
      ? paths.map((path) => pendingKey(repositoryName, path))
      : [],
  );
}

export function clearPendingFileOperations(
  keys: string[],
  owner: PendingFileOperationOwner,
  pendingFileOperations: MutableRefObject<Map<string, PendingFileOperationOwner>>,
  setPendingStageFiles: Dispatch<SetStateAction<Set<string>>>,
) {
  const cleared = keys.filter((key) => pendingFileOperations.current.get(key) === owner);
  if (cleared.length === 0) return;
  for (const key of cleared) pendingFileOperations.current.delete(key);
  setPendingStageFiles((prev) => {
    const next = new Set(prev);
    for (const key of cleared) next.delete(key);
    return next;
  });
}

/** Marks the current owner's response as successful and clears keys already observed at target state. */
export function markPendingFileOperationsSucceeded(
  keys: string[],
  owner: PendingFileOperationOwner,
  pendingFileOperations: MutableRefObject<Map<string, PendingFileOperationOwner>>,
  setPendingStageFiles: Dispatch<SetStateAction<Set<string>>>,
) {
  for (const key of keys) {
    const current = pendingFileOperations.current.get(key);
    if (current === owner) current.responseSucceeded = true;
  }
  clearPendingFileOperations(
    keys.filter(
      (key) => pendingFileOperations.current.get(key) === owner && owner.targetStateObserved,
    ),
    owner,
    pendingFileOperations,
    setPendingStageFiles,
  );
}

export function usePendingFileOperationScope(
  scopeIdentity: string,
  pendingFileOperations: MutableRefObject<Map<string, PendingFileOperationOwner>>,
  setPendingStageFiles: Dispatch<SetStateAction<Set<string>>>,
): boolean {
  const activeScopeIdentity = useRef(scopeIdentity);
  const scopeMatches = activeScopeIdentity.current === scopeIdentity;

  useEffect(() => {
    if (activeScopeIdentity.current === scopeIdentity) return;
    activeScopeIdentity.current = scopeIdentity;
    pendingFileOperations.current.clear();
    setPendingStageFiles(new Set());
  }, [pendingFileOperations, scopeIdentity, setPendingStageFiles]);

  return scopeMatches;
}

/** Clears pending operations only for repositories whose checkout generation changed. */
export function usePendingFileOperationRepositoryScope(
  generations: Record<string, number>,
  pendingFileOperations: MutableRefObject<Map<string, PendingFileOperationOwner>>,
  setPendingStageFiles: Dispatch<SetStateAction<Set<string>>>,
) {
  const previousGenerations = useRef(generations);

  useEffect(() => {
    const changedScopes = new Set<string>();
    const scopes = new Set([
      ...Object.keys(previousGenerations.current),
      ...Object.keys(generations),
    ]);
    for (const scope of scopes) {
      if (previousGenerations.current[scope] !== generations[scope]) changedScopes.add(scope);
    }
    previousGenerations.current = generations;
    if (changedScopes.size === 0) return;

    setPendingStageFiles((prev) => {
      if (prev.size === 0) return prev;
      const next = new Set(prev);
      for (const key of prev) {
        const separator = key.indexOf("::");
        const repositoryName = separator === -1 ? "" : key.slice(0, separator);
        if (!changedScopes.has("") && !changedScopes.has(repositoryName)) continue;
        next.delete(key);
        pendingFileOperations.current.delete(key);
      }
      return next;
    });
  }, [generations, pendingFileOperations, setPendingStageFiles]);
}

type PendingFileState = {
  hasStaged: boolean;
  hasUnstaged: boolean;
};

/** Indexes the current staged/unstaged facets once so each pending key is O(1) to reconcile. */
export function buildPendingFileStateIndex(allFiles: FileInfo[]): Map<string, PendingFileState> {
  const index = new Map<string, PendingFileState>();
  const { stagedFiles, unstagedFiles } = splitFilesByChangeLayer(allFiles);
  const mark = (files: FileInfo[], layer: keyof PendingFileState) => {
    for (const file of files) {
      const key = pendingKey(file.repository_name, file.path);
      const current = index.get(key) ?? { hasStaged: false, hasUnstaged: false };
      current[layer] = true;
      index.set(key, current);
    }
  };
  mark(stagedFiles, "hasStaged");
  mark(unstagedFiles, "hasUnstaged");
  return index;
}

function pendingFileOperationCompleted(
  key: string,
  operation: PendingFileOperation,
  fileStateIndex: Map<string, PendingFileState>,
): boolean {
  const state = fileStateIndex.get(key);
  // Unstaging a staged-only file can make it clean, so the refreshed status
  // legitimately omits it entirely.
  if (!state) return operation === "unstage";
  return operation === "stage"
    ? state.hasStaged && !state.hasUnstaged
    : state.hasUnstaged && !state.hasStaged;
}

/**
 * Reconciles pending markers only when a refreshed repository status reaches
 * the requested staged or unstaged state. Repository polling can publish the
 * previous state after the next action starts, so refresh identity alone is
 * not a completion signal.
 */
export function usePerRepoPendingClear(
  statusByRepo: ReturnType<typeof useSessionGitStatusByRepo>,
  allFiles: FileInfo[],
  setPendingStageFiles: Dispatch<SetStateAction<Set<string>>>,
  pendingFileOperations: MutableRefObject<Map<string, PendingFileOperationOwner>>,
) {
  const prevStatusRef = useRef<Map<string, unknown>>(new Map());
  useEffect(() => {
    const next = new Map<string, unknown>();
    const refreshed: string[] = [];
    for (const { repository_name, status } of statusByRepo) {
      next.set(repository_name, status);
      if (prevStatusRef.current.get(repository_name) !== status) {
        refreshed.push(repository_name);
      }
    }
    const isLegacySingleRepo = statusByRepo.length === 0;
    prevStatusRef.current = next;
    if (refreshed.length === 0 && !isLegacySingleRepo) return;
    const fileStateIndex = buildPendingFileStateIndex(allFiles);
    setPendingStageFiles((prev) => {
      if (prev.size === 0) return prev;
      const out = new Set<string>();
      for (const key of prev) {
        const separator = key.indexOf("::");
        const repositoryName = separator === -1 ? "" : key.slice(0, separator);
        const owner = pendingFileOperations.current.get(key);
        const repositoryRefreshed = isLegacySingleRepo || refreshed.includes(repositoryName);
        if (
          !repositoryRefreshed ||
          !owner ||
          !pendingFileOperationCompleted(key, owner.operation, fileStateIndex)
        ) {
          out.add(key);
          continue;
        }
        owner.targetStateObserved = true;
        if (owner.responseSucceeded) {
          pendingFileOperations.current.delete(key);
          continue;
        }
        out.add(key);
      }
      return out;
    });
  }, [allFiles, statusByRepo, setPendingStageFiles, pendingFileOperations]);
}
