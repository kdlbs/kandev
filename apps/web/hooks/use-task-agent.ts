import { useCallback, useEffect, useState } from 'react';
import { getWebSocketClient } from '@/lib/ws/connection';
import { useAppStore } from '@/components/state-provider';
import type { AgentSessionState, Task } from '@/lib/types/http';

interface UseTaskAgentReturn {
  isAgentRunning: boolean;
  isAgentLoading: boolean;
  agentSessionId: string | null;
  agentSessionState: AgentSessionState | null;
  worktreePath: string | null;
  worktreeBranch: string | null;
  handleStartAgent: (agentProfileId: string) => Promise<void>;
  handleStopAgent: () => Promise<void>;
}

export function useTaskAgent(task: Task | null): UseTaskAgentReturn {
  const [isAgentRunning, setIsAgentRunning] = useState(false);
  const [isAgentLoading, setIsAgentLoading] = useState(false);
  const [agentSessionId, setAgentSessionId] = useState<string | null>(null);
  const [agentSessionState, setAgentSessionState] = useState<AgentSessionState | null>(null);
  const [worktreePath, setWorktreePath] = useState<string | null>(task?.worktree_path ?? null);
  const [worktreeBranch, setWorktreeBranch] = useState<string | null>(task?.worktree_branch ?? null);
  const connectionStatus = useAppStore((state) => state.connection.status);
  const wsAgentSessionState = useAppStore((state) =>
    task?.id ? state.agentSessionStatesByTaskId[task.id] : undefined
  );

  useEffect(() => {
    setIsAgentRunning(false);
    setAgentSessionId(null);
    setAgentSessionState(null);
    setWorktreePath(task?.worktree_path ?? null);
    setWorktreeBranch(task?.worktree_branch ?? null);
  }, [task?.id, task?.worktree_path, task?.worktree_branch]);

  useEffect(() => {
    if (!wsAgentSessionState) return;
    setAgentSessionState(wsAgentSessionState as AgentSessionState);
  }, [wsAgentSessionState]);

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
          agent_session_id?: string;
        }>('task.execution', { task_id: task.id });

        console.log('[useTaskAgent] Task execution check:', response);
        if (response.has_execution) {
          setIsAgentRunning(true);
          if (response.state) {
            setAgentSessionState(response.state as AgentSessionState);
          }
          if (response.agent_session_id) {
            setAgentSessionId(response.agent_session_id);
          }
        } else {
          setIsAgentRunning(false);
          setAgentSessionId(null);
          setAgentSessionState(null);
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
        agent_session_id?: string;
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
      setAgentSessionState(response.state as AgentSessionState);
      if (response?.agent_session_id) {
        setAgentSessionId(response.agent_session_id);
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
      setAgentSessionState(null);
    } catch (error) {
      console.error('Failed to stop agent:', error);
    } finally {
      setIsAgentLoading(false);
    }
  }, [task?.id]);

  return {
    isAgentRunning,
    isAgentLoading,
    agentSessionId,
    agentSessionState,
    worktreePath,
    worktreeBranch,
    handleStartAgent,
    handleStopAgent,
  };
}
