"use client";

import { useState, useCallback, useMemo } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";

// GitOperationResult matches the backend response
export interface GitOperationResult {
  success: boolean;
  operation: string;
  output: string;
  error?: string;
  conflict_files?: string[];
}

// PRCreateResult matches the backend PR creation response
export interface PRCreateResult {
  success: boolean;
  pr_url?: string;
  output?: string;
  error?: string;
}

interface UseGitOperationsReturn {
  // Operation methods. The optional `repo` parameter is the multi-repo subpath
  // (e.g. "kandev"); pass empty/undefined for single-repo workspaces. Multi-repo
  // workspaces MUST scope each call to one repo — bulk callers fan out themselves.
  pull: (rebase?: boolean, repo?: string) => Promise<GitOperationResult>;
  push: (
    options?: { force?: boolean; setUpstream?: boolean },
    repo?: string,
  ) => Promise<GitOperationResult>;
  rebase: (baseBranch: string, repo?: string) => Promise<GitOperationResult>;
  merge: (baseBranch: string, repo?: string) => Promise<GitOperationResult>;
  abort: (operation: "merge" | "rebase", repo?: string) => Promise<GitOperationResult>;
  commit: (
    message: string,
    stageAll?: boolean,
    amend?: boolean,
    repo?: string,
  ) => Promise<GitOperationResult>;
  stage: (paths?: string[], repo?: string) => Promise<GitOperationResult>;
  unstage: (paths?: string[], repo?: string) => Promise<GitOperationResult>;
  discard: (paths?: string[], repo?: string) => Promise<GitOperationResult>;
  revertCommit: (commitSHA: string, repo?: string) => Promise<GitOperationResult>;
  renameBranch: (newName: string) => Promise<GitOperationResult>;
  reset: (commitSHA: string, mode: "soft" | "hard", repo?: string) => Promise<GitOperationResult>;
  createPR: (
    title: string,
    body: string,
    baseBranch?: string,
    draft?: boolean,
    repo?: string,
  ) => Promise<PRCreateResult>;

  // State
  isLoading: boolean;
  loadingOperation: string | null;
  error: string | null;
  lastResult: GitOperationResult | null;
}

type ExecuteOperation = <T extends GitOperationResult>(
  action: string,
  payload: Record<string, unknown>,
) => Promise<T>;

function buildGitOperationCallbacks(executeOperation: ExecuteOperation) {
  const pull = async (rebase = false, repo?: string) =>
    executeOperation<GitOperationResult>("worktree.pull", {
      rebase,
      ...(repo ? { repo } : {}),
    });

  const push = async (options?: { force?: boolean; setUpstream?: boolean }, repo?: string) =>
    executeOperation<GitOperationResult>("worktree.push", {
      force: options?.force ?? false,
      set_upstream: options?.setUpstream ?? false,
      ...(repo ? { repo } : {}),
    });

  const rebase = async (baseBranch: string, repo?: string) =>
    executeOperation<GitOperationResult>("worktree.rebase", {
      base_branch: baseBranch,
      ...(repo ? { repo } : {}),
    });

  const merge = async (baseBranch: string, repo?: string) =>
    executeOperation<GitOperationResult>("worktree.merge", {
      base_branch: baseBranch,
      ...(repo ? { repo } : {}),
    });

  const abort = async (operation: "merge" | "rebase", repo?: string) =>
    executeOperation<GitOperationResult>("worktree.abort", {
      operation,
      ...(repo ? { repo } : {}),
    });

  const commit = async (message: string, stageAll = true, amend = false, repo?: string) =>
    executeOperation<GitOperationResult>("worktree.commit", {
      message,
      stage_all: stageAll,
      amend,
      ...(repo ? { repo } : {}),
    });

  const stage = async (paths?: string[], repo?: string) =>
    executeOperation<GitOperationResult>("worktree.stage", {
      paths: paths ?? [],
      ...(repo ? { repo } : {}),
    });

  const unstage = async (paths?: string[], repo?: string) =>
    executeOperation<GitOperationResult>("worktree.unstage", {
      paths: paths ?? [],
      ...(repo ? { repo } : {}),
    });

  const discard = async (paths?: string[], repo?: string) =>
    executeOperation<GitOperationResult>("worktree.discard", {
      paths: paths ?? [],
      ...(repo ? { repo } : {}),
    });

  const revertCommit = async (commitSHA: string, repo?: string) =>
    executeOperation<GitOperationResult>("worktree.revert_commit", {
      commit_sha: commitSHA,
      ...(repo ? { repo } : {}),
    });

  const renameBranch = async (newName: string) =>
    executeOperation<GitOperationResult>("worktree.rename_branch", { new_name: newName });

  const reset = async (commitSHA: string, mode: "soft" | "hard", repo?: string) =>
    executeOperation<GitOperationResult>("worktree.reset", {
      commit_sha: commitSHA,
      mode,
      ...(repo ? { repo } : {}),
    });

  const createPR = async (
    title: string,
    body: string,
    baseBranch?: string,
    draft?: boolean,
    repo?: string,
  ): Promise<PRCreateResult> =>
    executeOperation<PRCreateResult & GitOperationResult>("worktree.create_pr", {
      title,
      body,
      base_branch: baseBranch ?? "",
      draft: draft ?? true,
      ...(repo ? { repo } : {}),
    });

  return {
    pull,
    push,
    rebase,
    merge,
    abort,
    commit,
    stage,
    unstage,
    discard,
    revertCommit,
    renameBranch,
    reset,
    createPR,
  };
}

export function useGitOperations(sessionId: string | null): UseGitOperationsReturn {
  const [isLoading, setIsLoading] = useState(false);
  const [loadingOperation, setLoadingOperation] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [lastResult, setLastResult] = useState<GitOperationResult | null>(null);

  const executeOperation = useCallback(
    async <T extends GitOperationResult>(
      action: string,
      payload: Record<string, unknown>,
    ): Promise<T> => {
      if (!sessionId) throw new Error("No session ID provided");
      const client = getWebSocketClient();
      if (!client) throw new Error("WebSocket not connected");

      setIsLoading(true);
      setLoadingOperation(action.replace("worktree.", ""));
      setError(null);

      const timeout = action === "worktree.create_pr" ? 120000 : 60000;
      try {
        const result = await client.request<T>(
          action,
          { session_id: sessionId, ...payload },
          timeout,
        );
        setLastResult(result);
        if (!result.success && result.error) setError(result.error);
        return result;
      } catch (e) {
        const errorMessage = e instanceof Error ? e.message : "Operation failed";
        setError(errorMessage);
        throw e;
      } finally {
        setIsLoading(false);
        setLoadingOperation(null);
      }
    },
    [sessionId],
  );

  const ops = useMemo(() => buildGitOperationCallbacks(executeOperation), [executeOperation]);

  return {
    ...ops,
    isLoading,
    loadingOperation,
    error,
    lastResult,
  };
}
