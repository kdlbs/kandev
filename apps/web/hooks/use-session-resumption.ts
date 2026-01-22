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
  const session = useAppStore((state) =>
    sessionId ? state.taskSessions.items[sessionId] ?? null : null
  );
  const setTaskSession = useAppStore((state) => state.setTaskSession);
  const hasAttemptedResume = useRef(false);

  useEffect(() => {
    hasAttemptedResume.current = false;
  }, [sessionId, taskId]);

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
        const status = await client.request<SessionStatus>('task.session.status', {
          task_id: taskId,
          session_id: sessionId,
        });
        setSessionStatus(status);

        if (status.error) {
          setResumptionState('error');
          setError(status.error);
          return;
        }

        // Update worktree info
        setWorktreePath(status.worktree_path ?? null);
        setWorktreeBranch(status.worktree_branch ?? null);
        if (taskId && sessionId && status.state) {
          setTaskSession({
            id: sessionId,
            task_id: taskId,
            state: status.state as TaskSessionState,
            started_at: session?.started_at ?? '',
            updated_at: session?.updated_at ?? '',
          });
        }

        // 2. If agent is already running, we're good
        if (status.is_agent_running) {
          setResumptionState('running');
          return;
        }

        // 3. If session needs resume and is resumable, auto-resume
        if (status.needs_resume && status.is_resumable) {
          setResumptionState('resuming');

          const resumeResp = await client.request<{
            success: boolean;
            state?: string;
            worktree_path?: string;
            worktree_branch?: string;
            error?: string;
          }>('task.session.resume', {
            task_id: taskId,
            session_id: sessionId,
          }, 30000);

          if (resumeResp.success) {
            setResumptionState('resumed');
            if (taskId && sessionId && resumeResp.state) {
              setTaskSession({
                id: sessionId,
                task_id: taskId,
                state: resumeResp.state as TaskSessionState,
                started_at: session?.started_at ?? '',
                updated_at: session?.updated_at ?? '',
              });
            }
            if (resumeResp.worktree_path) {
              setWorktreePath(resumeResp.worktree_path);
            }
            if (resumeResp.worktree_branch) {
              setWorktreeBranch(resumeResp.worktree_branch);
            }
          } else {
            setResumptionState('error');
            setError(resumeResp.error ?? 'Failed to resume session');
          }
        } else if (!status.is_resumable) {
          // Session cannot be resumed (terminal state or missing data)
          setResumptionState('idle');
        } else {
          // Session exists but doesn't need resume (already running or other state)
          setResumptionState('idle');
        }
      } catch (err) {
        setResumptionState('error');
        setError(err instanceof Error ? err.message : 'Unknown error');
      }
    };

    checkAndResume();
  }, [taskId, sessionId, connectionStatus, setTaskSession, session]);

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
        session_id: sessionId,
      }, 30000);

      if (response.success) {
        setResumptionState('resumed');
        if (taskId && sessionId && response.state) {
          setTaskSession({
            id: sessionId,
            task_id: taskId,
            state: response.state as TaskSessionState,
            started_at: session?.started_at ?? '',
            updated_at: session?.updated_at ?? '',
          });
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
  }, [taskId, sessionId, setTaskSession, session]);

  return {
    resumptionState,
    sessionStatus,
    error,
    taskSessionState: session?.state ?? null,
    worktreePath,
    worktreeBranch,
    resumeSession,
  };
}
