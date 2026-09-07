"use client";

import { useCallback, useEffect, useMemo, useRef, useSyncExternalStore } from "react";
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
      authoritativeCommits: state.authoritativeCommits,
      providerHead: state.providerHead,
      providerCommitsComplete: state.providerCommitsComplete,
      loading: state.loading,
      error: state.error,
    };
  }
  return {
    commits: [],
    authoritativeCommits: [],
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
  const request = useMemo(
    () =>
      hasParams
        ? {
            workspaceId,
            owner,
            repo,
            prNumber,
            sourceKey,
          }
        : null,
    [hasParams, workspaceId, owner, repo, prNumber, sourceKey],
  );
  const subscribe = useCallback(
    (listener: () => void) => prCommitsResource.subscribe(request, listener),
    [request],
  );
  const getSnapshot = useCallback(() => prCommitsResource.getSnapshot(request), [request]);
  const snapshot = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
  const paramsKeyRef = useRef<string>("");

  const refresh = useCallback(async () => {
    if (!request) return null;
    return prCommitsResource.load(request, true);
  }, [request]);

  useEffect(() => {
    if (sourceKey === paramsKeyRef.current) return;
    paramsKeyRef.current = sourceKey;
    if (!request) return;
    void prCommitsResource.load(request);
  }, [request, sourceKey]);

  return { ...resolvePRCommitsView({ sourceKey, ...snapshot }, sourceKey), refresh };
}
