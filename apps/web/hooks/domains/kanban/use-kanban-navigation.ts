"use client";

import { useCallback } from "react";
import { useRouter } from "next/navigation";
import { getWebSocketClient } from "@/lib/ws/connection";
import { linkToSession } from "@/lib/links";
import { useAppStore } from "@/components/state-provider";
import type { Task } from "@/components/kanban-card";

type NavigationOptions = {
  enablePreviewOnClick?: boolean;
  isMobile?: boolean;
  onPreviewTask?: (task: Task) => void;
  onOpenTask?: (task: Task, sessionId: string) => void;
  setEditingTask: (task: Task) => void;
  setIsDialogOpen: (open: boolean) => void;
  setTaskSessionAvailability: React.Dispatch<React.SetStateAction<Record<string, boolean>>>;
};

async function fetchLatestSession(
  taskId: string,
  setAvailability: React.Dispatch<React.SetStateAction<Record<string, boolean>>>,
): Promise<string | null> {
  const client = getWebSocketClient();
  if (!client) return null;
  try {
    const response = await client.request<{ sessions: Array<{ id: string }> }>(
      "task.session.list",
      { task_id: taskId },
      10000,
    );
    setAvailability((prev) => ({ ...prev, [taskId]: response.sessions.length > 0 }));
    return response.sessions[0]?.id ?? null;
  } catch (error) {
    console.error("Failed to load task sessions:", error);
    return null;
  }
}

async function preparePRSession(taskId: string): Promise<string | null> {
  const client = getWebSocketClient();
  if (!client) return null;
  try {
    const response = await client.request<{ session_id: string }>(
      "task.session.prepare",
      { task_id: taskId },
      15000,
    );
    return response.session_id ?? null;
  } catch (error) {
    console.error("Failed to prepare session for PR task:", error);
    return null;
  }
}

export function useKanbanNavigation({
  enablePreviewOnClick,
  isMobile,
  onPreviewTask,
  onOpenTask,
  setEditingTask,
  setIsDialogOpen,
  setTaskSessionAvailability,
}: NavigationOptions) {
  const router = useRouter();
  const taskPRs = useAppStore((s) => s.taskPRs.byTaskId);

  const fetchLatestSessionId = useCallback(
    (taskId: string) => fetchLatestSession(taskId, setTaskSessionAvailability),
    [setTaskSessionAvailability],
  );

  const navigateToSession = useCallback(
    (task: Task, sessionId: string) => {
      if (onOpenTask) onOpenTask(task, sessionId);
      else router.push(linkToSession(sessionId));
    },
    [onOpenTask, router],
  );

  const handleNoSession = useCallback(
    async (task: Task) => {
      if (taskPRs[task.id]) {
        const sessionId = await preparePRSession(task.id);
        if (sessionId) {
          router.push(linkToSession(sessionId));
          return;
        }
      }
      setEditingTask(task);
      setIsDialogOpen(true);
    },
    [taskPRs, router, setEditingTask, setIsDialogOpen],
  );

  const handleOpenTask = useCallback(
    async (task: Task) => {
      const sessionId = await fetchLatestSessionId(task.id);
      if (!sessionId) return handleNoSession(task);
      navigateToSession(task, sessionId);
    },
    [fetchLatestSessionId, handleNoSession, navigateToSession],
  );

  const handleCardClick = useCallback(
    async (task: Task) => {
      if (isMobile || !enablePreviewOnClick) {
        const sessionId = await fetchLatestSessionId(task.id);
        if (!sessionId) return handleNoSession(task);
        navigateToSession(task, sessionId);
      } else if (onPreviewTask) {
        onPreviewTask(task);
      }
    },
    [
      isMobile,
      enablePreviewOnClick,
      onPreviewTask,
      fetchLatestSessionId,
      handleNoSession,
      navigateToSession,
    ],
  );

  return { handleOpenTask, handleCardClick, fetchLatestSessionId };
}
