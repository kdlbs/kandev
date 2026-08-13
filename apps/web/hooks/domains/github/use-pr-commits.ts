"use client";

import { useEffect, useCallback, useState, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { PRCommitInfo } from "@/lib/types/github";

type PRCommitsState = {
  commits: PRCommitInfo[];
  providerHead: string | null;
  providerCommitsComplete: boolean;
  loading: boolean;
  error: string | null;
};
export type KeyedPRCommitsState = PRCommitsState & { sourceKey: string };

const INITIAL_STATE: KeyedPRCommitsState = {
  sourceKey: "",
  commits: [],
  providerHead: null,
  providerCommitsComplete: false,
  loading: false,
  error: null,
};

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

type PRCommitsRequest = {
  workspaceId: string;
  owner: string;
  repo: string;
  prNumber: number;
  sourceKey: string;
};

async function fetchPRCommits(
  { workspaceId, owner, repo, prNumber, sourceKey }: PRCommitsRequest,
  setState: (s: KeyedPRCommitsState) => void,
): Promise<PRCommitsState> {
  const client = getWebSocketClient();
  setState({
    sourceKey,
    commits: [],
    providerHead: null,
    providerCommitsComplete: false,
    loading: true,
    error: null,
  });
  if (!client) {
    const next = {
      sourceKey,
      commits: [],
      providerHead: null,
      providerCommitsComplete: false,
      loading: false,
      error: null,
    } satisfies KeyedPRCommitsState;
    setState(next);
    return next;
  }
  try {
    const response = await client.request<{
      commits?: PRCommitInfo[];
      head_sha?: string;
      complete?: boolean;
    }>("github.pr_commits.get", {
      workspace_id: workspaceId,
      owner,
      repo,
      number: prNumber,
    });
    const next = {
      sourceKey,
      commits: response?.commits ?? [],
      providerHead: response?.head_sha ?? null,
      providerCommitsComplete: response?.complete === true,
      loading: false,
      error: null,
    } satisfies KeyedPRCommitsState;
    setState(next);
    return next;
  } catch (err) {
    const next = {
      sourceKey,
      commits: [],
      providerHead: null,
      providerCommitsComplete: false,
      loading: false,
      error: err instanceof Error ? err.message : "Failed to fetch PR commits",
    } satisfies KeyedPRCommitsState;
    setState(next);
    return next;
  }
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
  const [state, setState] = useState<KeyedPRCommitsState>(INITIAL_STATE);
  const paramsKeyRef = useRef<string>("");
  const requestIdRef = useRef(0);

  const refresh = useCallback(async () => {
    if (!workspaceId || !owner || !repo || !prNumber) return null;
    const requestId = ++requestIdRef.current;
    return fetchPRCommits({ workspaceId, owner, repo, prNumber, sourceKey }, (next) => {
      if (requestId !== requestIdRef.current) return;
      setState(next);
    });
  }, [workspaceId, owner, repo, prNumber, sourceKey]);

  useEffect(() => {
    if (sourceKey === paramsKeyRef.current) return;
    paramsKeyRef.current = sourceKey;
    if (!workspaceId || !owner || !repo || !prNumber) {
      requestIdRef.current++; // invalidate in-flight responses
      return;
    }
    const requestId = ++requestIdRef.current;
    void fetchPRCommits({ workspaceId, owner, repo, prNumber, sourceKey }, (next) => {
      if (requestId !== requestIdRef.current) return;
      setState(next);
    });
  }, [workspaceId, owner, repo, prNumber, sourceKey]);

  return { ...resolvePRCommitsView(state, sourceKey), refresh };
}
