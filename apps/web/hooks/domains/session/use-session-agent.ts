import { useCallback, useMemo, useState } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { launchSession } from "@/lib/services/session-launch-service";
import { buildStartRequest } from "@/lib/services/session-launch-helpers";
import { useAppStore } from "@/components/state-provider";
import {
  useTaskSessionById,
  useTaskSessionsByTask,
} from "@/hooks/domains/session/use-task-session-by-id";
import type { TaskSessionState, Task, TaskSession } from "@/lib/types/http";

/** Prefer the active session if it belongs to this task; else the first session for the task. */
function pickCurrentSession(
  taskId: string | undefined,
  activeSession: TaskSession | null,
  sessionsForTask: TaskSession[],
): TaskSession | null {
  if (!taskId) return null;
  if (activeSession && activeSession.task_id === taskId) return activeSession;
  return sessionsForTask[0] ?? null;
}

/** Start/stop controls for a task's agent, extracted to keep useSessionAgent simple. */
function useAgentControls(task: Task | null, setIsAgentLoading: (v: boolean) => void) {
  const handleStartAgent = useCallback(
    async (agentProfileId: string, opts?: { prompt?: string; autoStart?: boolean }) => {
      if (!task?.id || !agentProfileId) return;
      setIsAgentLoading(true);
      try {
        const { request } = buildStartRequest(task.id, agentProfileId, {
          prompt: opts?.prompt ?? task.description ?? "",
          autoStart: opts?.autoStart,
        });
        await launchSession(request);
      } catch {
        // Failed to start agent
      } finally {
        setIsAgentLoading(false);
      }
    },
    [task?.id, task?.description, setIsAgentLoading],
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
  }, [task?.id, setIsAgentLoading]);

  return { handleStartAgent, handleStopAgent };
}

interface UseSessionAgentReturn {
  isAgentRunning: boolean;
  isAgentLoading: boolean;
  taskSessionId: string | null;
  taskSessionState: TaskSessionState | null;
  worktreePath: string | null;
  worktreeBranch: string | null;
  handleStartAgent: (
    agentProfileId: string,
    opts?: { prompt?: string; autoStart?: boolean },
  ) => Promise<void>;
  handleStopAgent: () => Promise<void>;
}

export function useSessionAgent(task: Task | null): UseSessionAgentReturn {
  const [isAgentLoading, setIsAgentLoading] = useState(false);

  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const activeSession = useTaskSessionById(activeSessionId);
  const { sessions: sessionsForTask } = useTaskSessionsByTask(task?.id ?? null);

  // Derive session: prefer active session if it belongs to this task, otherwise first session for task
  const currentSession = useMemo(
    () => pickCurrentSession(task?.id, activeSession, sessionsForTask),
    [activeSession, sessionsForTask, task?.id],
  );

  const taskSessionId = currentSession?.id ?? null;
  const taskSessionState = currentSession?.state ?? null;
  const worktreePath = currentSession?.worktree_path ?? null;
  const worktreeBranch = currentSession?.worktree_branch ?? null;

  // Agent is running if session state is STARTING or RUNNING
  const isAgentRunning = taskSessionState === "STARTING" || taskSessionState === "RUNNING";

  const { handleStartAgent, handleStopAgent } = useAgentControls(task, setIsAgentLoading);

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
