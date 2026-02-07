'use client';

import { useState, useCallback } from 'react';
import { getWebSocketClient } from '@/lib/ws/connection';

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
  abort: (operation: 'merge' | 'rebase') => Promise<GitOperationResult>;
  commit: (message: string, stageAll?: boolean) => Promise<GitOperationResult>;
  stage: (paths?: string[]) => Promise<GitOperationResult>;
  unstage: (paths?: string[]) => Promise<GitOperationResult>;
  discard: (paths?: string[]) => Promise<GitOperationResult>;
  createPR: (title: string, body: string, baseBranch?: string, draft?: boolean) => Promise<PRCreateResult>;

  // State
  isLoading: boolean;
  error: string | null;
  lastResult: GitOperationResult | null;
}

export function useGitOperations(sessionId: string | null): UseGitOperationsReturn {
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [lastResult, setLastResult] = useState<GitOperationResult | null>(null);

  const executeOperation = useCallback(async <T extends GitOperationResult>(
    action: string,
    payload: Record<string, unknown>
  ): Promise<T> => {
    if (!sessionId) {
      throw new Error('No session ID provided');
    }

    const client = getWebSocketClient();
    if (!client) {
      throw new Error('WebSocket not connected');
    }

    setIsLoading(true);
    setError(null);

    try {
      const result = await client.request<T>(action, {
        session_id: sessionId,
        ...payload
      }, 60000); // 60s timeout for git operations

      setLastResult(result);
      if (!result.success && result.error) {
        setError(result.error);
      }
      return result;
    } catch (e) {
      const errorMessage = e instanceof Error ? e.message : 'Operation failed';
      setError(errorMessage);
      throw e;
    } finally {
      setIsLoading(false);
    }
  }, [sessionId]);

  const pull = useCallback(async (rebase = false) => {
    return executeOperation<GitOperationResult>('worktree.pull', { rebase });
  }, [executeOperation]);

  const push = useCallback(async (options?: { force?: boolean; setUpstream?: boolean }) => {
    return executeOperation<GitOperationResult>('worktree.push', {
      force: options?.force ?? false,
      set_upstream: options?.setUpstream ?? false,
    });
  }, [executeOperation]);

  const rebase = useCallback(async (baseBranch: string) => {
    return executeOperation<GitOperationResult>('worktree.rebase', { base_branch: baseBranch });
  }, [executeOperation]);

  const merge = useCallback(async (baseBranch: string) => {
    return executeOperation<GitOperationResult>('worktree.merge', { base_branch: baseBranch });
  }, [executeOperation]);

  const abort = useCallback(async (operation: 'merge' | 'rebase') => {
    return executeOperation<GitOperationResult>('worktree.abort', { operation });
  }, [executeOperation]);

  const commit = useCallback(async (message: string, stageAll = true) => {
    return executeOperation<GitOperationResult>('worktree.commit', { message, stage_all: stageAll });
  }, [executeOperation]);

  const stage = useCallback(async (paths?: string[]) => {
    return executeOperation<GitOperationResult>('worktree.stage', { paths: paths ?? [] });
  }, [executeOperation]);

  const unstage = useCallback(async (paths?: string[]) => {
    return executeOperation<GitOperationResult>('worktree.unstage', { paths: paths ?? [] });
  }, [executeOperation]);

  const discard = useCallback(async (paths?: string[]) => {
    return executeOperation<GitOperationResult>('worktree.discard', { paths: paths ?? [] });
  }, [executeOperation]);

  const createPR = useCallback(async (title: string, body: string, baseBranch?: string, draft?: boolean): Promise<PRCreateResult> => {
    if (!sessionId) {
      throw new Error('No session ID provided');
    }

    const client = getWebSocketClient();
    if (!client) {
      throw new Error('WebSocket not connected');
    }

    setIsLoading(true);
    setError(null);

    try {
      const result = await client.request<PRCreateResult>('worktree.create_pr', {
        session_id: sessionId,
        title,
        body,
        base_branch: baseBranch ?? '',
        draft: draft ?? true,
      }, 120000); // 2 min timeout for PR creation (gh cli can be slow)

      if (!result.success && result.error) {
        setError(result.error);
      }
      return result;
    } catch (e) {
      const errorMessage = e instanceof Error ? e.message : 'Operation failed';
      setError(errorMessage);
      throw e;
    } finally {
      setIsLoading(false);
    }
  }, [sessionId]);

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
    createPR,
    isLoading,
    error,
    lastResult,
  };
}

