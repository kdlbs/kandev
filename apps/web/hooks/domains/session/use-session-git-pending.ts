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

export function pendingKey(repo: string | undefined, path: string): string {
  return `${repo ?? ""}::${path}`;
}

export function clearPendingFileOperations(
  keys: string[],
  operation: PendingFileOperation,
  pendingFileOperations: MutableRefObject<Map<string, PendingFileOperation>>,
  setPendingStageFiles: Dispatch<SetStateAction<Set<string>>>,
) {
  const cleared = keys.filter((key) => pendingFileOperations.current.get(key) === operation);
  if (cleared.length === 0) return;
  for (const key of cleared) pendingFileOperations.current.delete(key);
  setPendingStageFiles((prev) => {
    const next = new Set(prev);
    for (const key of cleared) next.delete(key);
    return next;
  });
}

function pendingFileOperationCompleted(
  key: string,
  operation: PendingFileOperation,
  allFiles: FileInfo[],
): boolean {
  const separator = key.indexOf("::");
  const repositoryName = separator === -1 ? "" : key.slice(0, separator);
  const path = separator === -1 ? key : key.slice(separator + 2);
  const matchingFiles = allFiles.filter(
    (file) => (file.repository_name ?? "") === repositoryName && file.path === path,
  );
  const { stagedFiles, unstagedFiles } = splitFilesByChangeLayer(matchingFiles);
  return operation === "stage"
    ? stagedFiles.length > 0 && unstagedFiles.length === 0
    : unstagedFiles.length > 0 && stagedFiles.length === 0;
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
  pendingFileOperations: MutableRefObject<Map<string, PendingFileOperation>>,
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
    setPendingStageFiles((prev) => {
      if (prev.size === 0) return prev;
      const out = new Set<string>();
      for (const key of prev) {
        const separator = key.indexOf("::");
        const repositoryName = separator === -1 ? "" : key.slice(0, separator);
        const operation = pendingFileOperations.current.get(key);
        const repositoryRefreshed = isLegacySingleRepo || refreshed.includes(repositoryName);
        if (
          !repositoryRefreshed ||
          !operation ||
          !pendingFileOperationCompleted(key, operation, allFiles)
        ) {
          out.add(key);
          continue;
        }
        pendingFileOperations.current.delete(key);
      }
      return out;
    });
  }, [allFiles, statusByRepo, setPendingStageFiles, pendingFileOperations]);
}
