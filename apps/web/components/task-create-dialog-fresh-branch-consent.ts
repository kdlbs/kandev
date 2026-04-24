"use client";

import { useCallback, useState } from "react";
import { getLocalRepositoryStatusAction } from "@/app/actions/workspaces";
import type { useToast } from "@/components/toast-provider";

export type PendingDiscard = {
  dirtyFiles: string[];
  repoPath: string;
  resolve: (confirmed: boolean) => void;
};

type Args = {
  isFreshBranchActive: boolean;
  workspaceId: string | null;
  repositoryLocalPath: string;
  toast: ReturnType<typeof useToast>["toast"];
};

/**
 * Coordinates the destructive-checkout consent modal for the fresh-branch flow.
 *
 * Returns:
 * - `pendingDiscard`: when set, the dialog renders the confirm modal.
 * - `ensureFreshBranchConsent()`: call before submitting a task. Resolves
 *   `false` when fresh-branch is disabled or the working tree is clean
 *   (proceed without confirm), `true` after the user confirms, and `null`
 *   when the user cancels (caller should abort the submit).
 */
export function useFreshBranchConsent({
  isFreshBranchActive,
  workspaceId,
  repositoryLocalPath,
  toast,
}: Args) {
  const [pendingDiscard, setPendingDiscard] = useState<PendingDiscard | null>(null);

  const ensureFreshBranchConsent = useCallback(async (): Promise<boolean | null> => {
    if (!isFreshBranchActive || !workspaceId) return false;
    try {
      const status = await getLocalRepositoryStatusAction(workspaceId, repositoryLocalPath);
      if (status.dirty_files.length === 0) return false;
      return await new Promise<boolean | null>((resolve) => {
        setPendingDiscard({
          dirtyFiles: status.dirty_files,
          repoPath: repositoryLocalPath,
          resolve: (confirmed) => {
            setPendingDiscard(null);
            resolve(confirmed ? true : null);
          },
        });
      });
    } catch (error) {
      toast({
        title: "Failed to check local repository status",
        description: error instanceof Error ? error.message : "Request failed",
        variant: "error",
      });
      return null;
    }
  }, [isFreshBranchActive, workspaceId, repositoryLocalPath, toast]);

  return { pendingDiscard, ensureFreshBranchConsent };
}
