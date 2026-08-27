"use client";

import {
  useEffect,
  useCallback,
  useState,
  useRef,
  type Dispatch,
  type SetStateAction,
} from "react";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { createDebugLogger } from "@/lib/debug/log";
import type { PRDiffFile } from "@/lib/types/github";
import { t } from "@/lib/i18n";

const debug = createDebugLogger("review:pr-diff");

type PRDiffView = {
  files: PRDiffFile[];
  loading: boolean;
  error: string | null;
};

export type KeyedPRDiffState = PRDiffView & {
  identityKey: string;
  hasResolved: boolean;
};

type PRDiffRequest = {
  workspaceId: string;
  owner: string;
  repo: string;
  prNumber: number;
  identityKey: string;
};

const INITIAL_STATE: KeyedPRDiffState = {
  identityKey: "",
  hasResolved: false,
  files: [],
  loading: false,
  error: null,
};

export function resolvePRDiffView(state: KeyedPRDiffState, requestedIdentity: string): PRDiffView {
  if (state.identityKey === requestedIdentity) {
    return { files: state.files, loading: state.loading, error: state.error };
  }
  return { files: [], loading: requestedIdentity !== "", error: null };
}

async function fetchPRFiles(
  { workspaceId, owner, repo, prNumber, identityKey }: PRDiffRequest,
  setState: Dispatch<SetStateAction<KeyedPRDiffState>>,
) {
  const client = getWebSocketClient();
  setState((previous) => {
    const retainResolved = previous.identityKey === identityKey && previous.hasResolved;
    return {
      identityKey,
      hasResolved: retainResolved,
      files: retainResolved ? previous.files : [],
      loading: !retainResolved,
      error: null,
    };
  });
  if (!client) {
    setState((previous) => ({ ...previous, loading: false }));
    return;
  }
  debug("fetch.start", { owner, repo, prNumber });
  try {
    const response = await client.request<{ files?: PRDiffFile[] }>("github.pr_files.get", {
      workspace_id: workspaceId,
      owner,
      repo,
      number: prNumber,
    });
    const files = response?.files ?? [];
    setState({
      identityKey,
      hasResolved: true,
      files,
      loading: false,
      error: null,
    });
    debug("fetch.success", { owner, repo, prNumber, fileCount: files.length });
  } catch (err) {
    const message = err instanceof Error ? err.message : t("github:failedToFetchPrFiles");
    setState((previous) => {
      if (previous.identityKey === identityKey && previous.hasResolved) {
        return { ...previous, loading: false, error: null };
      }
      return {
        identityKey,
        hasResolved: false,
        files: [],
        loading: false,
        error: message,
      };
    });
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
  const [state, setState] = useState<KeyedPRDiffState>(INITIAL_STATE);
  const hasParams = !!workspaceId && !!owner && !!repo && !!prNumber;
  const identityKey = hasParams ? `${workspaceId}/${owner}/${repo}/${prNumber}` : "";
  const requestKey = hasParams ? `${identityKey}/${refreshKey ?? ""}` : "";
  const requestKeyRef = useRef<string>("");
  const requestIdRef = useRef(0);

  const refresh = useCallback(() => {
    if (!workspaceId || !owner || !repo || !prNumber) return;
    const requestId = ++requestIdRef.current;
    void fetchPRFiles({ workspaceId, owner, repo, prNumber, identityKey }, (next) => {
      if (requestId !== requestIdRef.current) return;
      setState(next);
    });
  }, [workspaceId, owner, repo, prNumber, identityKey]);

  useEffect(() => {
    if (requestKey === requestKeyRef.current) return;
    requestKeyRef.current = requestKey;
    if (!workspaceId || !owner || !repo || !prNumber) {
      requestIdRef.current++; // invalidate in-flight responses
      return;
    }
    const requestId = ++requestIdRef.current;
    void fetchPRFiles({ workspaceId, owner, repo, prNumber, identityKey }, (next) => {
      if (requestId !== requestIdRef.current) return;
      setState(next);
    });
  }, [workspaceId, owner, repo, prNumber, identityKey, requestKey]);

  return { ...resolvePRDiffView(state, identityKey), refresh };
}
