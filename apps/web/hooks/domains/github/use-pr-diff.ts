"use client";

import { useEffect, useCallback, useState } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { PRDiffFile } from "@/lib/types/github";

type PRDiffState = {
  files: PRDiffFile[];
  loading: boolean;
  error: string | null;
};

const INITIAL_STATE: PRDiffState = {
  files: [],
  loading: false,
  error: null,
};

async function fetchPRFiles(
  owner: string,
  repo: string,
  prNumber: number,
  setState: (s: PRDiffState) => void,
) {
  const client = getWebSocketClient();
  if (!client) return;

  setState({ files: [], loading: true, error: null });
  try {
    const response = await client.request<{ files?: PRDiffFile[] }>("github.pr_files.get", {
      owner,
      repo,
      number: prNumber,
    });
    setState({ files: response?.files ?? [], loading: false, error: null });
  } catch (err) {
    setState({
      files: [],
      loading: false,
      error: err instanceof Error ? err.message : "Failed to fetch PR files",
    });
  }
}

/**
 * Fetches the files changed in a pull request via WebSocket.
 * Returns structured diff data from the GitHub API with full unified diff patches.
 */
export function usePRDiff(owner: string | null, repo: string | null, prNumber: number | null) {
  const [state, setState] = useState<PRDiffState>(INITIAL_STATE);

  const refresh = useCallback(() => {
    if (!owner || !repo || !prNumber) return;
    void fetchPRFiles(owner, repo, prNumber, setState);
  }, [owner, repo, prNumber]);

  useEffect(() => {
    if (!owner || !repo || !prNumber) return;
    void fetchPRFiles(owner, repo, prNumber, setState);
  }, [owner, repo, prNumber]);

  return { ...state, refresh };
}
