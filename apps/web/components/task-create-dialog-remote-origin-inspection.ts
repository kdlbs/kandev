"use client";

import { useEffect, useMemo } from "react";
import type { Branch, ExecutorType } from "@/lib/types/http";
import type {
  DialogFormState,
  TaskRepositorySelection,
} from "@/components/task-create-dialog-types";
import { resolveRepositorySelections } from "@/components/task-create-dialog-repositories-state";
import { getWorkspaceSourceCapabilities } from "@/components/workspace-source-picker/executor-capabilities";
import {
  useRepositoryCloneSource,
  type RepositoryCloneSourceCandidate,
  type RepositoryCloneSourceState,
} from "@/hooks/domains/repositories/use-repository-clone-source";

export type RemoteOriginInspection = {
  candidates: RepositoryCloneSourceCandidate[];
  states: Record<string, RepositoryCloneSourceState>;
  enabled: boolean;
  refresh: () => void;
};

/**
 * Inspects every selected host repository while a clone-capable executor is
 * active. A row may have been selected before the executor changed, so the
 * candidate list deliberately does not depend on checkoutSource. Ready
 * results enrich the same row in place and retain its key and branch intent.
 */
export function useSelectedRemoteOriginInspection(
  fs: DialogFormState,
  workspaceId: string | null,
  executorType: ExecutorType | string | null | undefined,
): RemoteOriginInspection {
  const candidates = useMemo<RepositoryCloneSourceCandidate[]>(
    () =>
      resolveRepositorySelections(fs)
        .filter(isInspectableLocalSelection)
        .map((selection) => ({
          key: selection.key,
          repositoryId: selection.repositoryId,
          localPath: selection.localPath,
        })),
    [
      fs.repositorySelections,
      fs.repositories,
      fs.remoteRepos,
      fs.remoteProviderReadiness,
      fs.useRemote,
    ],
  );
  const enabled =
    getWorkspaceSourceCapabilities(executorType).requiresCloneableLocalRepository &&
    Boolean(workspaceId);
  const inspection = useRepositoryCloneSource(workspaceId, candidates, enabled);

  useEffect(() => {
    if (!enabled) return;
    const selections = resolveRepositorySelections(fs);
    for (const candidate of candidates) {
      const selection = selections.find(
        (item): item is Extract<TaskRepositorySelection, { kind: "local" }> =>
          item.kind === "local" && item.key === candidate.key,
      );
      const state = inspection.states[candidate.key];
      if (!selection || state?.status !== "ready" || !state.result.origin) continue;
      if (
        selection.checkoutSource === "remote_origin" &&
        selection.expectedOrigin &&
        selection.expectedOrigin !== state.result.origin
      ) {
        continue;
      }
      const patch: Partial<Extract<TaskRepositorySelection, { kind: "local" }>> = {};
      if (selection.checkoutSource !== "remote_origin") {
        patch.checkoutSource = "remote_origin";
      }
      if (selection.expectedOrigin !== state.result.origin) {
        patch.expectedOrigin = state.result.origin;
      }
      if (!sameBranches(selection.remoteBranches, state.result.branches)) {
        patch.remoteBranches = state.result.branches;
      }
      if (Object.keys(patch).length > 0) fs.updateRepository(selection.key, patch);
    }
  }, [candidates, enabled, fs, inspection.states]);

  return { candidates, states: inspection.states, enabled, refresh: inspection.refresh };
}

export function effectiveRemoteOriginBranch(
  selection: Extract<TaskRepositorySelection, { kind: "local" }>,
): string {
  return (selection.baseBranch || selection.branch).trim();
}

export function remoteOriginBranchesForState(
  remoteOriginMode: boolean | undefined,
  state: RepositoryCloneSourceState | undefined,
  fallback?: Branch[],
): Branch[] | undefined {
  if (remoteOriginMode !== true) return fallback;
  if (state?.status === "ready") return state.result.branches;
  if (state?.status === "unavailable" || state?.status === "error") return [];
  return undefined;
}

/** A selected row is ready only when its current origin and selected ref agree. */
export function remoteOriginSelectionIsCompatible(
  selection: Extract<TaskRepositorySelection, { kind: "local" }>,
  state: RepositoryCloneSourceState | undefined,
): boolean {
  if (selection.checkoutSource !== "remote_origin" || state?.status !== "ready") return false;
  if (!state.result.origin || selection.expectedOrigin !== state.result.origin) return false;
  const availableBranches = new Set(state.result.branches.map((candidate) => candidate.name));
  return [selection.baseBranch, selection.branch].every((branch) => {
    const normalized = branch?.trim().replace(/^origin\//, "") ?? "";
    return normalized === "" || availableBranches.has(normalized);
  });
}

/**
 * Identifies a saved remote-origin row that needs an explicit recheck before
 * it can be submitted. Unavailable and checking states have their own copy;
 * this covers an origin or branch set that changed after a successful check.
 */
export function remoteOriginSelectionNeedsRecovery(
  remoteOriginMode: boolean | undefined,
  selection: Extract<TaskRepositorySelection, { kind: "local" }>,
  state: RepositoryCloneSourceState | undefined,
): boolean {
  return (
    remoteOriginMode === true &&
    selection.checkoutSource === "remote_origin" &&
    state?.status === "ready" &&
    !remoteOriginSelectionIsCompatible(selection, state)
  );
}

function isInspectableLocalSelection(
  selection: TaskRepositorySelection,
): selection is Extract<TaskRepositorySelection, { kind: "local" }> {
  return selection.kind === "local" && Boolean(selection.repositoryId || selection.localPath);
}

function sameBranches(
  left: Extract<TaskRepositorySelection, { kind: "local" }>["remoteBranches"],
  right: NonNullable<Extract<TaskRepositorySelection, { kind: "local" }>["remoteBranches"]>,
): boolean {
  if (!left || left.length !== right.length) return false;
  return left.every(
    (branch, index) =>
      branch.name === right[index]?.name &&
      branch.type === right[index]?.type &&
      branch.remote === right[index]?.remote,
  );
}
