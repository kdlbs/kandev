"use client";

import { useState, useCallback } from "react";
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
  // Operation methods
  pull: (rebase?: boolean) => Promise<GitOperationResult>;
  push: (options?: { force?: boolean; setUpstream?: boolean }) => Promise<GitOperationResult>;
  rebase: (baseBranch: string) => Promise<GitOperationResult>;
  merge: (baseBranch: string) => Promise<GitOperationResult>;
  abort: (operation: "merge" | "rebase") => Promise<GitOperationResult>;
  commit: (message: string, stageAll?: boolean) => Promise<GitOperationResult>;
  stage: (paths?: string[]) => Promise<GitOperationResult>;
  unstage: (paths?: string[]) => Promise<GitOperationResult>;
  discard: (paths?: string[]) => Promise<GitOperationResult>;
  createPR: (
    title: string,
    body: string,
    baseBranch?: string,
    draft?: boolean,
  ) => Promise<PRCreateResult>;

  // State
  isLoading: boolean;
  error: string | null;
  lastResult: GitOperationResult | null;
}

type ExecuteOperation = <T extends GitOperationResult>(
  action: string,
  payload: Record<string, unknown>,
) => Promise<T>;

function buildGitOperationCallbacks(executeOperation: ExecuteOperation) {
  const pull = async (rebase = false) =>
    executeOperation<GitOperationResult>("worktree.pull", { rebase });

  const push = async (options?: { force?: boolean; setUpstream?: boolean }) =>
    executeOperation<GitOperationResult>("worktree.push", {
      force: options?.force ?? false,
      set_upstream: options?.setUpstream ?? false,
    });

  const rebase = async (baseBranch: string) =>
    executeOperation<GitOperationResult>("worktree.rebase", { base_branch: baseBranch });

  const merge = async (baseBranch: string) =>
    executeOperation<GitOperationResult>("worktree.merge", { base_branch: baseBranch });

  const abort = async (operation: "merge" | "rebase") =>
    executeOperation<GitOperationResult>("worktree.abort", { operation });

  const commit = async (message: string, stageAll = true) =>
    executeOperation<GitOperationResult>("worktree.commit", { message, stage_all: stageAll });

  const stage = async (paths?: string[]) =>
    executeOperation<GitOperationResult>("worktree.stage", { paths: paths ?? [] });

  const unstage = async (paths?: string[]) =>
    executeOperation<GitOperationResult>("worktree.unstage", { paths: paths ?? [] });

  const discard = async (paths?: string[]) =>
    executeOperation<GitOperationResult>("worktree.discard", { paths: paths ?? [] });

  const createPR = async (
    title: string,
    body: string,
    baseBranch?: string,
    draft?: boolean,
  ): Promise<PRCreateResult> =>
    executeOperation<PRCreateResult & GitOperationResult>("worktree.create_pr", {
      title,
      body,
      base_branch: baseBranch ?? "",
      draft: draft ?? true,
    });

  return { pull, push, rebase, merge, abort, commit, stage, unstage, discard, createPR };
}

export function useGitOperations(sessionId: string | null): UseGitOperationsReturn {
  const [isLoading, setIsLoading] = useState(false);
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
      }
    },
    [sessionId],
  );

  const ops = buildGitOperationCallbacks(executeOperation);

  return {
    ...ops,
    isLoading,
    error,
    lastResult,
  };
}
