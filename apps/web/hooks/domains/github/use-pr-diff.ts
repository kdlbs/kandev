"use client";

import { useEffect, useCallback, useState, useRef } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { createDebugLogger } from "@/lib/debug/log";
import type { PRDiffFile } from "@/lib/types/github";

const debug = createDebugLogger("review:pr-diff");

type PRDiffView = {
  files: PRDiffFile[];
  loading: boolean;
  error: string | null;
};

export type KeyedPRDiffState = PRDiffView & {
  sourceKey: string;
};

const INITIAL_STATE: KeyedPRDiffState = {
  sourceKey: "",
  files: [],
  loading: false,
  error: null,
};

export function resolvePRDiffView(state: KeyedPRDiffState, requestedKey: string): PRDiffView {
  if (state.sourceKey === requestedKey) {
    return { files: state.files, loading: state.loading, error: state.error };
  }
  return { files: [], loading: requestedKey !== "", error: null };
}

async function fetchPRFiles(
  owner: string,
  repo: string,
  prNumber: number,
  sourceKey: string,
  setState: (s: KeyedPRDiffState) => void,
) {
  const client = getWebSocketClient();
  setState({ sourceKey, files: [], loading: true, error: null });
  if (!client) {
    setState({ sourceKey, files: [], loading: false, error: null });
    return;
  }
  debug("fetch.start", { owner, repo, prNumber });
  try {
    const response = await client.request<{ files?: PRDiffFile[] }>("github.pr_files.get", {
      owner,
      repo,
      number: prNumber,
    });
    const files = response?.files ?? [];
    setState({ sourceKey, files, loading: false, error: null });
    debug("fetch.success", { owner, repo, prNumber, fileCount: files.length });
  } catch (err) {
    const message = err instanceof Error ? err.message : "Failed to fetch PR files";
    setState({ sourceKey, files: [], loading: false, error: message });
    debug("fetch.error", { owner, repo, prNumber, error: message });
  }
}

/**
 * Fetches the files changed in a pull request via WebSocket.
 * Returns structured diff data from the GitHub API with full unified diff patches.
 */
export function usePRDiff(
  owner: string | null,
  repo: string | null,
  prNumber: number | null,
  refreshKey?: string | null,
) {
  const [state, setState] = useState<KeyedPRDiffState>(INITIAL_STATE);
  const hasParams = !!owner && !!repo && !!prNumber;
  const sourceKey = hasParams ? `${owner}/${repo}/${prNumber}/${refreshKey ?? ""}` : "";
  const paramsKeyRef = useRef<string>("");
  const requestIdRef = useRef(0);

  const refresh = useCallback(() => {
    if (!owner || !repo || !prNumber) return;
    const requestId = ++requestIdRef.current;
    void fetchPRFiles(owner, repo, prNumber, sourceKey, (next) => {
      if (requestId !== requestIdRef.current) return;
      setState(next);
    });
  }, [owner, repo, prNumber, sourceKey]);

  useEffect(() => {
    if (sourceKey === paramsKeyRef.current) return;
    paramsKeyRef.current = sourceKey;
    if (!owner || !repo || !prNumber) {
      requestIdRef.current++; // invalidate in-flight responses
      return;
    }
    const requestId = ++requestIdRef.current;
    void fetchPRFiles(owner, repo, prNumber, sourceKey, (next) => {
      if (requestId !== requestIdRef.current) return;
      setState(next);
    });
  }, [owner, repo, prNumber, sourceKey]);

  return { ...resolvePRDiffView(state, sourceKey), refresh };
}
