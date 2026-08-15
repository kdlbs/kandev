"use client";

import { useCallback, useEffect, useRef, useSyncExternalStore } from "react";
import { useAppStore } from "@/components/state-provider";
import { prCommitsResource, type PRCommitsState } from "./pr-commits-resource";

export type KeyedPRCommitsState = PRCommitsState & { sourceKey: string };

export function resolvePRCommitsView(
  state: KeyedPRCommitsState,
  requestedKey: string,
): PRCommitsState {
  if (state.sourceKey === requestedKey) {
    return {
      commits: state.commits,
      providerHead: state.providerHead,
      providerCommitsComplete: state.providerCommitsComplete,
      loading: state.loading,
      error: state.error,
    };
  }
  return {
    commits: [],
    providerHead: null,
    providerCommitsComplete: false,
    loading: requestedKey !== "",
    error: null,
  };
}

/**
 * Fetches the commits in a pull request via WebSocket.
 * Returns commit metadata from the GitHub API.
 */
export function usePRCommits(
  owner: string | null,
  repo: string | null,
  prNumber: number | null,
  refreshKey?: string | null,
) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const hasParams = !!workspaceId && !!owner && !!repo && !!prNumber;
  const sourceKey = hasParams
    ? `${workspaceId}/${owner}/${repo}/${prNumber}/${refreshKey ?? ""}`
    : "";
  const subscribe = useCallback(
    (listener: () => void) => prCommitsResource.subscribe(sourceKey, listener),
    [sourceKey],
  );
  const getSnapshot = useCallback(() => prCommitsResource.getSnapshot(sourceKey), [sourceKey]);
  const snapshot = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
  const paramsKeyRef = useRef<string>("");

  const refresh = useCallback(async () => {
    if (!workspaceId || !owner || !repo || !prNumber) return null;
    return prCommitsResource.load({ workspaceId, owner, repo, prNumber, sourceKey }, true);
  }, [workspaceId, owner, repo, prNumber, sourceKey]);

  useEffect(() => {
    if (sourceKey === paramsKeyRef.current) return;
    paramsKeyRef.current = sourceKey;
    if (!workspaceId || !owner || !repo || !prNumber) return;
    void prCommitsResource.load({ workspaceId, owner, repo, prNumber, sourceKey });
  }, [workspaceId, owner, repo, prNumber, sourceKey]);

  return { ...resolvePRCommitsView({ sourceKey, ...snapshot }, sourceKey), refresh };
}
