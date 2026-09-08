"use client";

import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { useToast } from "@/components/toast-provider";
import { updateTask } from "@/lib/api/domains/kanban-api";
import type { TaskPriority } from "@/lib/types/http";

/**
 * Persists a priority change from a task menu. Not optimistic: each surface
 * follows the stored value from the resulting `task.updated` event, so a
 * failure here needs only a toast — the card keeps showing whatever it was
 * already showing.
 */
export function useUpdateTaskPriority() {
  const { toast } = useToast();
  const { t } = useTranslation("task");

  return useCallback(
    async (taskId: string, priority: TaskPriority): Promise<void> => {
      try {
        await updateTask(taskId, { priority });
      } catch (error) {
        toast({
          title: t("task:failedToUpdateTask"),
          description: error instanceof Error ? error.message : String(error),
          variant: "error",
        });
      }
    },
    [t, toast],
  );
}
