import { useCallback, useEffect, useMemo, useState } from 'react';
import { getWebSocketClient } from '@/lib/ws/connection';
import { useAppStore } from '@/components/state-provider';
import type { TaskSessionState, Task, TaskSession } from '@/lib/types/http';

const EMPTY_SESSIONS: TaskSession[] = [];

interface UseTaskAgentReturn {
  isAgentRunning: boolean;
  isAgentLoading: boolean;
  taskSessionId: string | null;
  taskSessionState: TaskSessionState | null;
  worktreePath: string | null;
  worktreeBranch: string | null;
  handleStartAgent: (agentProfileId: string) => Promise<void>;
  handleStopAgent: () => Promise<void>;
}

export function useTaskAgent(task: Task | null): UseTaskAgentReturn {
  const [isAgentRunning, setIsAgentRunning] = useState(false);
  const [isAgentLoading, setIsAgentLoading] = useState(false);
  const [taskSessionId, setAgentSessionId] = useState<string | null>(null);
  const [taskSessionState, setTaskSessionState] = useState<TaskSessionState | null>(null);
  const [worktreePath, setWorktreePath] = useState<string | null>(task?.worktree_path ?? null);
  const [worktreeBranch, setWorktreeBranch] = useState<string | null>(task?.worktree_branch ?? null);
  const connectionStatus = useAppStore((state) => state.connection.status);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const activeSession = useAppStore((state) =>
    activeSessionId ? state.taskSessions.items[activeSessionId] ?? null : null
  );
  const sessionsForTask = useAppStore((state) =>
    task?.id ? state.taskSessionsByTask.itemsByTaskId[task.id] ?? EMPTY_SESSIONS : EMPTY_SESSIONS
  );
  const wsTaskSessionState = useMemo(() => {
    if (!task?.id) return undefined;
    if (activeSession && activeSession.task_id === task.id) {
      return activeSession.state;
    }
    return sessionsForTask[0]?.state;
  }, [activeSession, sessionsForTask, task?.id]);

  useEffect(() => {
    setIsAgentRunning(false);
    setAgentSessionId(null);
    setTaskSessionState(null);
    setWorktreePath(task?.worktree_path ?? null);
    setWorktreeBranch(task?.worktree_branch ?? null);
  }, [task?.id, task?.worktree_path, task?.worktree_branch]);

  useEffect(() => {
    if (!wsTaskSessionState) return;
    setTaskSessionState(wsTaskSessionState as TaskSessionState);
  }, [wsTaskSessionState]);

  // Fetch task execution status from orchestrator on mount
  useEffect(() => {
    if (!task?.id) return;
    if (connectionStatus !== 'connected') return;

    const checkExecution = async () => {
      const client = getWebSocketClient();
      if (!client) return;

      try {
        const response = await client.request<{
          has_execution: boolean;
          task_id: string;
          state?: string;
          session_id?: string;
        }>('task.execution', { task_id: task.id });

        console.log('[useTaskAgent] Task execution check:', response);
        if (response.has_execution) {
          setIsAgentRunning(true);
          if (response.state) {
            setTaskSessionState(response.state as TaskSessionState);
          }
          if (response.session_id) {
            setAgentSessionId(response.session_id);
          }
        } else {
          setIsAgentRunning(false);
          setAgentSessionId(null);
          setTaskSessionState(null);
        }
      } catch (err) {
        console.error('[useTaskAgent] Failed to check task execution:', err);
      }
    };

    checkExecution();
    const interval = setInterval(() => {
      if (connectionStatus === 'connected') {
        checkExecution();
      }
    }, 2000);

    return () => clearInterval(interval);
  }, [connectionStatus, task?.id]);

  const handleStartAgent = useCallback(async (agentProfileId: string) => {
    if (!task?.id) return;
    if (!agentProfileId) return;

    const client = getWebSocketClient();
    if (!client) return;

    setIsAgentLoading(true);
    try {
      interface StartResponse {
        success: boolean;
        task_id: string;
        agent_instance_id: string;
        session_id?: string;
        state: string;
        worktree_path?: string;
        worktree_branch?: string;
      }
      console.log('[useTaskAgent] orchestrator.start request', {
        taskId: task.id,
        agentProfileId,
      });
      const response = await client.request<StartResponse>('orchestrator.start', {
        task_id: task.id,
        agent_profile_id: agentProfileId,
      }, 15000);
      console.log('[useTaskAgent] orchestrator.start response', response);
      setIsAgentRunning(true);
      setTaskSessionState(response.state as TaskSessionState);
      if (response?.session_id) {
        setAgentSessionId(response.session_id);
      }

      // Update worktree info from response
      if (response?.worktree_path) {
        setWorktreePath(response.worktree_path);
        setWorktreeBranch(response.worktree_branch ?? null);
      }
    } catch (error) {
      console.error('Failed to start agent:', error);
    } finally {
      setIsAgentLoading(false);
    }
  }, [task?.id]);

  const handleStopAgent = useCallback(async () => {
    if (!task?.id) return;

    const client = getWebSocketClient();
    if (!client) return;

    setIsAgentLoading(true);
    try {
      await client.request('orchestrator.stop', { task_id: task.id }, 15000);
      setIsAgentRunning(false);
      setAgentSessionId(null);
      setTaskSessionState(null);
    } catch (error) {
      console.error('Failed to stop agent:', error);
    } finally {
      setIsAgentLoading(false);
    }
  }, [task?.id]);

  return {
    isAgentRunning,
    isAgentLoading,
    taskSessionId,
    taskSessionState,
    worktreePath,
    worktreeBranch,
    handleStartAgent,
    handleStopAgent,
  };
}
