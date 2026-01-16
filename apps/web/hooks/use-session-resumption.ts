import { useCallback, useEffect, useState, useRef } from 'react';
import { getWebSocketClient } from '@/lib/ws/connection';
import { useAppStore } from '@/components/state-provider';
import type { TaskSessionState } from '@/lib/types/http';

export type SessionStatus = {
  session_id: string;
  task_id: string;
  state: string;
  agent_profile_id?: string;
  is_agent_running: boolean;
  is_resumable: boolean;
  needs_resume: boolean;
  resume_reason?: string;
  acp_session_id?: string;
  worktree_path?: string;
  worktree_branch?: string;
  error?: string;
};

export type ResumptionState = 'idle' | 'checking' | 'resuming' | 'resumed' | 'running' | 'error';

interface UseSessionResumptionReturn {
  resumptionState: ResumptionState;
  sessionStatus: SessionStatus | null;
  error: string | null;
  taskSessionState: TaskSessionState | null;
  worktreePath: string | null;
  worktreeBranch: string | null;
  resumeSession: () => Promise<boolean>;
  startNewSession: (agentProfileId: string) => Promise<boolean>;
}

/**
 * Hook for handling session resumption on page reload.
 * When a sessionId is provided (from URL), it checks the session status
 * and automatically resumes if needed.
 */
export function useSessionResumption(
  taskId: string | null,
  sessionId: string | null
): UseSessionResumptionReturn {
  const [resumptionState, setResumptionState] = useState<ResumptionState>('idle');
  const [sessionStatus, setSessionStatus] = useState<SessionStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [worktreePath, setWorktreePath] = useState<string | null>(null);
  const [worktreeBranch, setWorktreeBranch] = useState<string | null>(null);
  const connectionStatus = useAppStore((state) => state.connection.status);
  // Read task session state from store so WebSocket updates are reflected
  const taskSessionState = useAppStore((state) => taskId ? state.taskSessionStatesByTaskId[taskId] ?? null : null);
  const setStoreTaskSessionState = useAppStore((state) => state.setTaskSessionState);
  const hasAttemptedResume = useRef(false);

  // Check session status and auto-resume if needed
  useEffect(() => {
    if (!taskId || !sessionId) return;
    if (connectionStatus !== 'connected') return;
    if (hasAttemptedResume.current) return;

    const checkAndResume = async () => {
      const client = getWebSocketClient();
      if (!client) return;

      hasAttemptedResume.current = true;
      setResumptionState('checking');
      setError(null);

      try {
        // 1. Check session status
        console.log('[useSessionResumption] Checking session status:', { taskId, sessionId });
        const status = await client.request<SessionStatus>('task.session.status', {
          task_id: taskId,
          task_session_id: sessionId,
        });
        console.log('[useSessionResumption] Session status:', status);
        setSessionStatus(status);

        if (status.error) {
          setResumptionState('error');
          setError(status.error);
          return;
        }

        // Update worktree info
        setWorktreePath(status.worktree_path ?? null);
        setWorktreeBranch(status.worktree_branch ?? null);
        if (taskId && status.state) {
          setStoreTaskSessionState(taskId, status.state as TaskSessionState);
        }

        // 2. If agent is already running, we're good
        if (status.is_agent_running) {
          console.log('[useSessionResumption] Agent already running');
          setResumptionState('running');
          return;
        }

        // 3. If session needs resume and is resumable, auto-resume
        if (status.needs_resume && status.is_resumable) {
          console.log('[useSessionResumption] Auto-resuming session');
          setResumptionState('resuming');

          const resumeResp = await client.request<{
            success: boolean;
            state?: string;
            worktree_path?: string;
            worktree_branch?: string;
            error?: string;
          }>('task.session.resume', {
            task_id: taskId,
            task_session_id: sessionId,
          }, 30000);

          if (resumeResp.success) {
            console.log('[useSessionResumption] Session resumed successfully');
            setResumptionState('resumed');
            if (taskId && resumeResp.state) {
              setStoreTaskSessionState(taskId, resumeResp.state as TaskSessionState);
            }
            if (resumeResp.worktree_path) {
              setWorktreePath(resumeResp.worktree_path);
            }
            if (resumeResp.worktree_branch) {
              setWorktreeBranch(resumeResp.worktree_branch);
            }
          } else {
            console.error('[useSessionResumption] Resume failed:', resumeResp.error);
            setResumptionState('error');
            setError(resumeResp.error ?? 'Failed to resume session');
          }
        } else if (!status.is_resumable) {
          // Session cannot be resumed (terminal state or missing data)
          console.log('[useSessionResumption] Session not resumable');
          setResumptionState('idle');
        } else {
          // Session exists but doesn't need resume (already running or other state)
          setResumptionState('idle');
        }
      } catch (err) {
        console.error('[useSessionResumption] Error checking/resuming session:', err);
        setResumptionState('error');
        setError(err instanceof Error ? err.message : 'Unknown error');
      }
    };

    checkAndResume();
  }, [taskId, sessionId, connectionStatus, setStoreTaskSessionState]);

  // Manual resume function
  const resumeSession = useCallback(async (): Promise<boolean> => {
    if (!taskId || !sessionId) return false;

    const client = getWebSocketClient();
    if (!client) return false;

    setResumptionState('resuming');
    setError(null);

    try {
      const response = await client.request<{
        success: boolean;
        state?: string;
        worktree_path?: string;
        worktree_branch?: string;
        error?: string;
      }>('task.session.resume', {
        task_id: taskId,
        task_session_id: sessionId,
      }, 30000);

      if (response.success) {
        setResumptionState('resumed');
        if (taskId && response.state) {
          setStoreTaskSessionState(taskId, response.state as TaskSessionState);
        }
        if (response.worktree_path) {
          setWorktreePath(response.worktree_path);
        }
        if (response.worktree_branch) {
          setWorktreeBranch(response.worktree_branch);
        }
        return true;
      } else {
        setResumptionState('error');
        setError(response.error ?? 'Failed to resume session');
        return false;
      }
    } catch (err) {
      setResumptionState('error');
      setError(err instanceof Error ? err.message : 'Unknown error');
      return false;
    }
  }, [taskId, sessionId, setStoreTaskSessionState]);

  // Start new session function (fallback when resume fails)
  const startNewSession = useCallback(async (agentProfileId: string): Promise<boolean> => {
    if (!taskId) return false;

    const client = getWebSocketClient();
    if (!client) return false;

    setResumptionState('resuming');
    setError(null);

    try {
      const response = await client.request<{
        success: boolean;
        task_session_id?: string;
        state?: string;
        worktree_path?: string;
        worktree_branch?: string;
      }>('orchestrator.start', {
        task_id: taskId,
        agent_profile_id: agentProfileId,
      }, 15000);

      if (response.success) {
        setResumptionState('resumed');
        if (taskId && response.state) {
          setStoreTaskSessionState(taskId, response.state as TaskSessionState);
        }
        if (response.worktree_path) {
          setWorktreePath(response.worktree_path);
        }
        if (response.worktree_branch) {
          setWorktreeBranch(response.worktree_branch);
        }
        return true;
      }
      return false;
    } catch (err) {
      setResumptionState('error');
      setError(err instanceof Error ? err.message : 'Unknown error');
      return false;
    }
  }, [taskId, setStoreTaskSessionState]);

  return {
    resumptionState,
    sessionStatus,
    error,
    taskSessionState,
    worktreePath,
    worktreeBranch,
    resumeSession,
    startNewSession,
  };
}

