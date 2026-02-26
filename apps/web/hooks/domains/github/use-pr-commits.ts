"use client";

import { useEffect, useCallback, useState } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { PRCommitInfo } from "@/lib/types/github";

type PRCommitsState = {
  commits: PRCommitInfo[];
  loading: boolean;
  error: string | null;
};

const INITIAL_STATE: PRCommitsState = {
  commits: [],
  loading: false,
  error: null,
};

async function fetchPRCommits(
  owner: string,
  repo: string,
  prNumber: number,
  setState: (s: PRCommitsState) => void,
) {
  const client = getWebSocketClient();
  if (!client) return;

  setState({ commits: [], loading: true, error: null });
  try {
    const response = await client.request<{ commits?: PRCommitInfo[] }>("github.pr_commits.get", {
      owner,
      repo,
      number: prNumber,
    });
    setState({ commits: response?.commits ?? [], loading: false, error: null });
  } catch (err) {
    setState({
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
export function usePRCommits(owner: string | null, repo: string | null, prNumber: number | null) {
  const [state, setState] = useState<PRCommitsState>(INITIAL_STATE);

  const refresh = useCallback(() => {
    if (!owner || !repo || !prNumber) return;
    void fetchPRCommits(owner, repo, prNumber, setState);
  }, [owner, repo, prNumber]);

  useEffect(() => {
    if (!owner || !repo || !prNumber) return;
    void fetchPRCommits(owner, repo, prNumber, setState);
  }, [owner, repo, prNumber]);

  return { ...state, refresh };
}
