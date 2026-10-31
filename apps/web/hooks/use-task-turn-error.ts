"use client";

import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { useToast } from "@/components/toast-provider";
import { dismissTaskTurnError } from "@/lib/api/domains/task-turn-error-api";

/**
 * Shared dismissal for the inline Chat alert and the task menu. Not optimistic:
 * the badge follows the backend projection, so a failure keeps the attention
 * visible and only surfaces localized feedback.
 */
export function useTaskTurnError() {
  const { toast } = useToast();
  const { t } = useTranslation();
  const [pendingStamp, setPendingStamp] = useState<string | null>(null);

  const dismiss = useCallback(
    async (taskId: string, stamp: string): Promise<void> => {
      if (!taskId || !stamp) return;
      setPendingStamp(stamp);
      try {
        await dismissTaskTurnError(taskId, stamp);
      } catch (error) {
        toast({
          title: t("task:failedToDismissTurnError"),
          description: error instanceof Error ? error.message : String(error),
          variant: "error",
        });
      } finally {
        setPendingStamp(null);
      }
    },
    [t, toast],
  );

  return { dismiss, pendingStamp };
}
