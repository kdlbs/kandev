import { useCallback, useMemo, useState } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useAppStore } from "@/components/state-provider";
import type { TaskSessionState, Task, TaskSession } from "@/lib/types/http";

const EMPTY_SESSIONS: TaskSession[] = [];

interface UseSessionAgentReturn {
  isAgentRunning: boolean;
  isAgentLoading: boolean;
  taskSessionId: string | null;
  taskSessionState: TaskSessionState | null;
  worktreePath: string | null;
  worktreeBranch: string | null;
  handleStartAgent: (agentProfileId: string, prompt?: string) => Promise<void>;
  handleStopAgent: () => Promise<void>;
}

export function useSessionAgent(task: Task | null): UseSessionAgentReturn {
  const [isAgentLoading, setIsAgentLoading] = useState(false);

  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const activeSession = useAppStore((state) =>
    activeSessionId ? (state.taskSessions.items[activeSessionId] ?? null) : null,
  );
  const sessionsForTask = useAppStore((state) =>
    task?.id ? (state.taskSessionsByTask.itemsByTaskId[task.id] ?? EMPTY_SESSIONS) : EMPTY_SESSIONS,
  );

  // Derive session from store: prefer active session if it belongs to this task, otherwise first session for task
  const currentSession = useMemo(() => {
    if (!task?.id) return null;
    if (activeSession && activeSession.task_id === task.id) {
      return activeSession;
    }
    return sessionsForTask[0] ?? null;
  }, [activeSession, sessionsForTask, task?.id]);

  const taskSessionId = currentSession?.id ?? null;
  const taskSessionState = currentSession?.state ?? null;
  const worktreePath = currentSession?.worktree_path ?? null;
  const worktreeBranch = currentSession?.worktree_branch ?? null;

  // Agent is running if session state is STARTING or RUNNING
  const isAgentRunning = taskSessionState === "STARTING" || taskSessionState === "RUNNING";

  const handleStartAgent = useCallback(
    async (agentProfileId: string, prompt?: string) => {
      if (!task?.id) return;
      if (!agentProfileId) return;

      const client = getWebSocketClient();
      if (!client) return;

      setIsAgentLoading(true);
      try {
        // Store will be updated via WebSocket notifications when session starts
        await client.request(
          "orchestrator.start",
          {
            task_id: task.id,
            agent_profile_id: agentProfileId,
            prompt: prompt ?? task.description ?? "",
          },
          15000,
        );
      } catch {
        // Failed to start agent
      } finally {
        setIsAgentLoading(false);
      }
    },
    [task?.id, task?.description],
  );

  const handleStopAgent = useCallback(async () => {
    if (!task?.id) return;

    const client = getWebSocketClient();
    if (!client) return;

    setIsAgentLoading(true);
    try {
      // Store will be updated via WebSocket notifications when session stops
      await client.request("orchestrator.stop", { task_id: task.id }, 15000);
    } catch {
      // Failed to stop agent
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
