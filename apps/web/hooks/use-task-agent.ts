import { useCallback, useEffect, useState } from 'react';
import { getWebSocketClient } from '@/lib/ws/connection';
import type { Task } from '@/lib/types/http';

interface UseTaskAgentReturn {
  isAgentRunning: boolean;
  isAgentLoading: boolean;
  worktreePath: string | null;
  worktreeBranch: string | null;
  handleStartAgent: () => Promise<void>;
  handleStopAgent: () => Promise<void>;
}

export function useTaskAgent(task: Task | null): UseTaskAgentReturn {
  const [isAgentRunning, setIsAgentRunning] = useState(false);
  const [isAgentLoading, setIsAgentLoading] = useState(false);
  const [worktreePath, setWorktreePath] = useState<string | null>(task?.worktree_path ?? null);
  const [worktreeBranch, setWorktreeBranch] = useState<string | null>(task?.worktree_branch ?? null);

  // Fetch task execution status from orchestrator on mount
  useEffect(() => {
    if (!task?.id) return;

    const checkExecution = async () => {
      const client = getWebSocketClient();
      if (!client) return;

      try {
        const response = await client.request<{
          has_execution: boolean;
          task_id: string;
          status?: string;
        }>('task.execution', { task_id: task.id });

        console.log('[useTaskAgent] Task execution check:', response);
        if (response.has_execution) {
          setIsAgentRunning(true);
        }
      } catch (err) {
        console.error('[useTaskAgent] Failed to check task execution:', err);
      }
    };

    checkExecution();
  }, [task?.id]);

  const handleStartAgent = useCallback(async () => {
    if (!task?.id) return;

    const client = getWebSocketClient();
    if (!client) return;

    // Require agent_profile_id to be set on the task
    if (!task.agent_profile_id) {
      console.error('No agent profile configured for this task. Please edit the task to select an agent profile.');
      return;
    }

    setIsAgentLoading(true);
    try {
      interface StartResponse {
        success: boolean;
        task_id: string;
        agent_instance_id: string;
        status: string;
        worktree_path?: string;
        worktree_branch?: string;
      }
      const response = await client.request<StartResponse>('orchestrator.start', {
        task_id: task.id,
        agent_profile_id: task.agent_profile_id,
      }, 15000);
      setIsAgentRunning(true);

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
  }, [task?.id, task?.agent_profile_id]);

  const handleStopAgent = useCallback(async () => {
    if (!task?.id) return;

    const client = getWebSocketClient();
    if (!client) return;

    setIsAgentLoading(true);
    try {
      await client.request('orchestrator.stop', { task_id: task.id }, 15000);
      setIsAgentRunning(false);
    } catch (error) {
      console.error('Failed to stop agent:', error);
    } finally {
      setIsAgentLoading(false);
    }
  }, [task?.id]);

  return {
    isAgentRunning,
    isAgentLoading,
    worktreePath,
    worktreeBranch,
    handleStartAgent,
    handleStopAgent,
  };
}

