"use client";

import { useCallback } from "react";
import { useRouter } from "next/navigation";
import { linkToTask } from "@/lib/links";
import type { Task } from "@/components/kanban-card";

type NavigationOptions = {
  enablePreviewOnClick?: boolean;
  isMobile?: boolean;
  onPreviewTask?: (task: Task) => void;
  onOpenTask?: (task: Task) => void;
};

export function useKanbanNavigation({
  enablePreviewOnClick,
  isMobile,
  onPreviewTask,
  onOpenTask,
}: NavigationOptions) {
  const router = useRouter();

  const navigateToTask = useCallback(
    (task: Task) => {
      if (onOpenTask) onOpenTask(task);
      else router.push(linkToTask(task.id));
    },
    [onOpenTask, router],
  );

  const handleOpenTask = useCallback(
    (task: Task) => {
      navigateToTask(task);
    },
    [navigateToTask],
  );

  const handleCardClick = useCallback(
    (task: Task) => {
      if (isMobile || !enablePreviewOnClick) {
        navigateToTask(task);
      } else if (onPreviewTask) {
        onPreviewTask(task);
      } else {
        navigateToTask(task);
      }
    },
    [isMobile, enablePreviewOnClick, onPreviewTask, navigateToTask],
  );

  return { handleOpenTask, handleCardClick };
}
