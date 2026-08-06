"use client";

import { useEffect, useCallback, useState, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { PRCommitInfo } from "@/lib/types/github";

type PRCommitsState = {
  commits: PRCommitInfo[];
  loading: boolean;
  error: string | null;
};
export type KeyedPRCommitsState = PRCommitsState & { sourceKey: string };

const INITIAL_STATE: KeyedPRCommitsState = {
  sourceKey: "",
  commits: [],
  loading: false,
  error: null,
};

export function resolvePRCommitsView(
  state: KeyedPRCommitsState,
  requestedKey: string,
): PRCommitsState {
  if (state.sourceKey === requestedKey) {
    return { commits: state.commits, loading: state.loading, error: state.error };
  }
  return { commits: [], loading: requestedKey !== "", error: null };
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
) {
  const client = getWebSocketClient();
  setState({ sourceKey, commits: [], loading: true, error: null });
  if (!client) {
    setState({ sourceKey, commits: [], loading: false, error: null });
    return;
  }
  try {
    const response = await client.request<{ commits?: PRCommitInfo[] }>("github.pr_commits.get", {
      workspace_id: workspaceId,
      owner,
      repo,
      number: prNumber,
    });
    setState({ sourceKey, commits: response?.commits ?? [], loading: false, error: null });
  } catch (err) {
    setState({
      sourceKey,
      commits: [],
      loading: false,
      error: err instanceof Error ? err.message : "Failed to fetch PR commits",
    });
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

  const refresh = useCallback(() => {
    if (!workspaceId || !owner || !repo || !prNumber) return;
    const requestId = ++requestIdRef.current;
    void fetchPRCommits({ workspaceId, owner, repo, prNumber, sourceKey }, (next) => {
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
