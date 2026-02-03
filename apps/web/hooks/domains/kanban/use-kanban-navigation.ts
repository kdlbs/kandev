'use client';

import { useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { getWebSocketClient } from '@/lib/ws/connection';
import { linkToSession } from '@/lib/links';
import type { Task } from '@/components/kanban-card';

type NavigationOptions = {
  enablePreviewOnClick?: boolean;
  isMobile?: boolean;
  onPreviewTask?: (task: Task) => void;
  onOpenTask?: (task: Task, sessionId: string) => void;
  setEditingTask: (task: Task) => void;
  setIsDialogOpen: (open: boolean) => void;
  setTaskSessionAvailability: React.Dispatch<React.SetStateAction<Record<string, boolean>>>;
};

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

  // Fetch latest session ID for a task
  const fetchLatestSessionId = useCallback(
    async (taskId: string) => {
      const client = getWebSocketClient();
      if (!client) return null;
      try {
        const response = await client.request<{ sessions: Array<{ id: string }> }>(
          'task.session.list',
          { task_id: taskId },
          10000
        );
        setTaskSessionAvailability((prev) => ({
          ...prev,
          [taskId]: response.sessions.length > 0,
        }));
        return response.sessions[0]?.id ?? null;
      } catch (error) {
        console.error('Failed to load task sessions:', error);
        return null;
      }
    },
    [setTaskSessionAvailability]
  );

  // Open task (always navigates, opens dialog if no session)
  const handleOpenTask = useCallback(
    async (task: Task) => {
      const latestSessionId = await fetchLatestSessionId(task.id);
      if (!latestSessionId) {
        setEditingTask(task);
        setIsDialogOpen(true);
        return;
      }
      if (onOpenTask) {
        onOpenTask(task, latestSessionId);
      } else {
        router.push(linkToSession(latestSessionId));
      }
    },
    [fetchLatestSessionId, onOpenTask, router, setEditingTask, setIsDialogOpen]
  );

  // Handle card click (preview or navigate based on settings)
  const handleCardClick = useCallback(
    async (task: Task) => {
      // On mobile, always navigate to task (no preview panel)
      if (isMobile) {
        const latestSessionId = await fetchLatestSessionId(task.id);
        if (!latestSessionId) {
          setEditingTask(task);
          setIsDialogOpen(true);
          return;
        }

        if (onOpenTask) {
          onOpenTask(task, latestSessionId);
        } else {
          router.push(linkToSession(latestSessionId));
        }
        return;
      }

      // Desktop/tablet: preview or navigate based on settings
      const shouldOpenPreview = enablePreviewOnClick;

      if (shouldOpenPreview) {
        // Preview mode - just call the preview handler without fetching session
        if (onPreviewTask) {
          onPreviewTask(task);
        }
      } else {
        // Navigate mode - fetch session and navigate
        const latestSessionId = await fetchLatestSessionId(task.id);
        if (!latestSessionId) {
          setEditingTask(task);
          setIsDialogOpen(true);
          return;
        }

        if (onOpenTask) {
          onOpenTask(task, latestSessionId);
        } else {
          router.push(linkToSession(latestSessionId));
        }
      }
    },
    [isMobile, enablePreviewOnClick, onPreviewTask, fetchLatestSessionId, onOpenTask, router, setEditingTask, setIsDialogOpen]
  );

  return {
    handleOpenTask,
    handleCardClick,
    fetchLatestSessionId,
  };
}
