"use client";

import { useEffect, useCallback, useState, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { createDebugLogger } from "@/lib/debug/log";
import type { PRDiffFile } from "@/lib/types/github";

const debug = createDebugLogger("review:pr-diff");

type PRDiffState = {
  files: PRDiffFile[];
  loading: boolean;
  error: string | null;
};
type WorkspacePRDiffState = { workspaceId: string | null; state: PRDiffState };

const INITIAL_STATE: PRDiffState = {
  files: [],
  loading: false,
  error: null,
};

async function fetchPRFiles(
  workspaceId: string,
  owner: string,
  repo: string,
  prNumber: number,
  setState: (s: PRDiffState) => void,
) {
  const client = getWebSocketClient();
  if (!client) return;

  setState({ files: [], loading: true, error: null });
  debug("fetch.start", { owner, repo, prNumber });
  try {
    const response = await client.request<{ files?: PRDiffFile[] }>("github.pr_files.get", {
      workspace_id: workspaceId,
      owner,
      repo,
      number: prNumber,
    });
    const files = response?.files ?? [];
    setState({ files, loading: false, error: null });
    debug("fetch.success", { owner, repo, prNumber, fileCount: files.length });
  } catch (err) {
    const message = err instanceof Error ? err.message : "Failed to fetch PR files";
    setState({ files: [], loading: false, error: message });
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
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const [result, setResult] = useState<WorkspacePRDiffState>({
    workspaceId: null,
    state: INITIAL_STATE,
  });
  const hasParams = !!workspaceId && !!owner && !!repo && !!prNumber;
  const paramsKeyRef = useRef<string>("");
  const requestIdRef = useRef(0);

  const refresh = useCallback(() => {
    if (!workspaceId || !owner || !repo || !prNumber) return;
    const requestId = ++requestIdRef.current;
    void fetchPRFiles(workspaceId, owner, repo, prNumber, (next) => {
      if (requestId !== requestIdRef.current) return;
      setResult({ workspaceId, state: next });
    });
  }, [workspaceId, owner, repo, prNumber]);

  useEffect(() => {
    const key = hasParams ? `${workspaceId}/${owner}/${repo}/${prNumber}/${refreshKey ?? ""}` : "";
    if (key === paramsKeyRef.current) return;
    paramsKeyRef.current = key;
    if (!workspaceId || !owner || !repo || !prNumber) {
      requestIdRef.current++; // invalidate in-flight responses
      return;
    }
    const requestId = ++requestIdRef.current;
    void fetchPRFiles(workspaceId, owner, repo, prNumber, (next) => {
      if (requestId !== requestIdRef.current) return;
      setResult({ workspaceId, state: next });
    });
  }, [workspaceId, owner, repo, prNumber, hasParams, refreshKey]);

  // Return initial state when params are null to clear stale data
  if (!hasParams || result.workspaceId !== workspaceId) {
    return { ...INITIAL_STATE, refresh };
  }
  return { ...result.state, refresh };
}
