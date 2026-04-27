"use client";

import { useCallback, useState } from "react";
import { getLocalRepositoryStatusAction } from "@/app/actions/workspaces";
import { ApiError } from "@/lib/api/client";
import { createTask } from "@/lib/api";
import type { useToast } from "@/components/toast-provider";

type CreateTaskParams = Parameters<typeof createTask>[0];
type CreateTaskResponse = Awaited<ReturnType<typeof createTask>>;

export function dirtyFilesFromApiError(err: unknown): string[] | null {
  if (!(err instanceof ApiError) || err.status !== 409) return null;
  const body = err.body as { dirty_files?: unknown } | null;
  if (!body || !Array.isArray(body.dirty_files)) return null;
  // Backend should never send an empty list with 409, but guard anyway so a
  // malformed response doesn't open the consent dialog with no files in it.
  const files = body.dirty_files.filter((p): p is string => typeof p === "string");
  return files.length > 0 ? files : null;
}

export type PendingDiscard = {
  dirtyFiles: string[];
  repoPath: string;
  resolve: (confirmed: boolean) => void;
};

/**
 * Result of the consent check:
 * - `null` when the user cancelled (caller should abort)
 * - empty array when fresh-branch is inactive or the working tree is clean
 *   (proceed without consent)
 * - non-empty array of dirty paths the user explicitly OK'd discarding
 */
export type ConsentResult = string[] | null;

type Args = {
  isFreshBranchActive: boolean;
  workspaceId: string | null;
  repositoryLocalPath: string;
  toast: ReturnType<typeof useToast>["toast"];
};

/**
 * Coordinates the destructive-checkout consent modal for the fresh-branch flow.
 *
 * `ensureFreshBranchConsent()` runs the preflight via `getLocalRepositoryStatusAction`
 * and prompts only when the working tree is dirty. `confirmForDirtyFiles(list)`
 * skips the preflight and reuses a list provided by the caller — used to
 * recover when the backend rejects with 409 because new files became dirty
 * between preflight and submit.
 */
export function useFreshBranchConsent({
  isFreshBranchActive,
  workspaceId,
  repositoryLocalPath,
  toast,
}: Args) {
  const [pendingDiscard, setPendingDiscard] = useState<PendingDiscard | null>(null);

  const promptForList = useCallback(
    (dirtyFiles: string[]): Promise<ConsentResult> =>
      new Promise<ConsentResult>((resolve) => {
        setPendingDiscard({
          dirtyFiles,
          repoPath: repositoryLocalPath,
          resolve: (confirmed) => {
            setPendingDiscard(null);
            resolve(confirmed ? dirtyFiles : null);
          },
        });
      }),
    [repositoryLocalPath],
  );

  const ensureFreshBranchConsent = useCallback(async (): Promise<ConsentResult> => {
    if (!isFreshBranchActive || !workspaceId) return [];
    try {
      const status = await getLocalRepositoryStatusAction(workspaceId, repositoryLocalPath);
      if (status.dirty_files.length === 0) return [];
      return await promptForList(status.dirty_files);
    } catch (error) {
      toast({
        title: "Failed to check local repository status",
        description: error instanceof Error ? error.message : "Request failed",
        variant: "error",
      });
      return null;
    }
  }, [isFreshBranchActive, workspaceId, repositoryLocalPath, promptForList, toast]);

  /**
   * Calls createTask once, then re-prompts the user with the updated dirty
   * list and retries once if the backend returned 409 because the working
   * tree gained new dirty files between the consent dialog and the actual
   * discard. Returns null if the user cancelled the second prompt.
   */
  const createTaskWithFreshBranchRetry = useCallback(
    async (
      build: (consented: string[]) => CreateTaskParams,
      initial: string[],
    ): Promise<CreateTaskResponse | null> => {
      try {
        return await createTask(build(initial));
      } catch (err) {
        if (!isFreshBranchActive) throw err;
        const dirty = dirtyFilesFromApiError(err);
        if (!dirty) throw err;
        const reconsented = await promptForList(dirty);
        if (reconsented === null) return null;
        return await createTask(build(reconsented));
      }
    },
    [isFreshBranchActive, promptForList],
  );

  return {
    pendingDiscard,
    ensureFreshBranchConsent,
    confirmForDirtyFiles: promptForList,
    createTaskWithFreshBranchRetry,
  };
}
