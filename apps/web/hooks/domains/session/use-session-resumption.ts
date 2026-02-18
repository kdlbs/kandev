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

type ResumeResponse = {
  success: boolean;
  state?: string;
  worktree_path?: string;
  worktree_branch?: string;
  error?: string;
};

type ResumeStateSetter = {
  setResumptionState: (s: ResumptionState) => void;
  setError: (e: string | null) => void;
  setWorktreePath: (p: string | null) => void;
  setWorktreeBranch: (b: string | null) => void;
  setTaskSession: (s: { id: string; task_id: string; state: TaskSessionState; started_at: string; updated_at: string }) => void;
};

type SessionLike = { started_at?: string; updated_at?: string } | null;

/** Apply a successful resume response to local state. */
function applyResumeResponse(
  resp: ResumeResponse,
  taskId: string,
  sessionId: string,
  session: SessionLike,
  setters: ResumeStateSetter
): boolean {
  if (resp.success) {
    setters.setResumptionState('resumed');
    if (resp.state) {
      setters.setTaskSession({
        id: sessionId, task_id: taskId, state: resp.state as TaskSessionState,
        started_at: session?.started_at ?? '', updated_at: session?.updated_at ?? '',
      });
    }
    if (resp.worktree_path) setters.setWorktreePath(resp.worktree_path);
    if (resp.worktree_branch) setters.setWorktreeBranch(resp.worktree_branch);
    return true;
  }
  setters.setResumptionState('error');
  setters.setError(resp.error ?? 'Failed to resume session');
  return false;
}

type CheckAndResumeParams = {
  taskId: string;
  sessionId: string;
  session: SessionLike;
  setSessionStatus: (s: SessionStatus) => void;
  setters: ResumeStateSetter;
};

async function checkAndResume({ taskId, sessionId, session, setSessionStatus, setters }: CheckAndResumeParams): Promise<void> {
  const client = getWebSocketClient();
  if (!client) return;
  setters.setResumptionState('checking');
  setters.setError(null);
  try {
    const status = await client.request<SessionStatus>('task.session.status', { task_id: taskId, session_id: sessionId });
    setSessionStatus(status);
    if (status.error) { setters.setResumptionState('error'); setters.setError(status.error); return; }
    setters.setWorktreePath(status.worktree_path ?? null);
    setters.setWorktreeBranch(status.worktree_branch ?? null);
    if (status.state) {
      setters.setTaskSession({ id: sessionId, task_id: taskId, state: status.state as TaskSessionState,
        started_at: session?.started_at ?? '', updated_at: session?.updated_at ?? '' });
    }
    if (status.is_agent_running) { setters.setResumptionState('running'); return; }
    if (status.needs_resume && status.is_resumable) {
      setters.setResumptionState('resuming');
      const resumeResp = await client.request<ResumeResponse>('task.session.resume', { task_id: taskId, session_id: sessionId }, 30000);
      applyResumeResponse(resumeResp, taskId, sessionId, session, setters);
    } else {
      setters.setResumptionState('idle');
    }
  } catch (err) {
    setters.setResumptionState('error');
    setters.setError(err instanceof Error ? err.message : 'Unknown error');
  }
}

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

  const setters: ResumeStateSetter = {
    setResumptionState, setError, setWorktreePath, setWorktreeBranch, setTaskSession,
  };

  useEffect(() => { hasAttemptedResume.current = false; }, [sessionId, taskId]);

  // Check session status and auto-resume if needed
  useEffect(() => {
    if (!taskId || !sessionId || connectionStatus !== 'connected' || hasAttemptedResume.current) return;
    hasAttemptedResume.current = true;
    checkAndResume({ taskId, sessionId, session, setSessionStatus, setters });
  }, [taskId, sessionId, connectionStatus, setTaskSession, session]); // eslint-disable-line react-hooks/exhaustive-deps

  // Manual resume function
  const resumeSession = useCallback(async (): Promise<boolean> => {
    if (!taskId || !sessionId) return false;
    const client = getWebSocketClient();
    if (!client) return false;
    setResumptionState('resuming');
    setError(null);
    try {
      const response = await client.request<ResumeResponse>(
        'task.session.resume', { task_id: taskId, session_id: sessionId }, 30000
      );
      if (response.success) {
        setResumptionState('resumed');
        if (response.state) {
          setTaskSession({
            id: sessionId, task_id: taskId, state: response.state as TaskSessionState,
            started_at: session?.started_at ?? '', updated_at: session?.updated_at ?? '',
          });
        }
        if (response.worktree_path) setWorktreePath(response.worktree_path);
        if (response.worktree_branch) setWorktreeBranch(response.worktree_branch);
        return true;
      }
      setResumptionState('error');
      setError(response.error ?? 'Failed to resume session');
      return false;
    } catch (err) {
      setResumptionState('error');
      setError(err instanceof Error ? err.message : 'Unknown error');
      return false;
    }
  }, [taskId, sessionId, session, setTaskSession, setWorktreePath, setWorktreeBranch]);

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
