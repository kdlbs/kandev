"use client";

import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { useTranslation } from "react-i18next";
import { useTaskTurnError } from "@/hooks/use-task-turn-error";

/**
 * Current failed-turn alert dismissal. It acknowledges only the stamp it was
 * rendered with; a newer failure keeps its own alert and badge.
 */
export function TaskTurnErrorDismissButton({ taskId, stamp }: { taskId: string; stamp: string }) {
  const { t } = useTranslation();
  const { dismiss, pendingStamp } = useTaskTurnError();
  const pending = pendingStamp === stamp;
  return (
    <Button
      variant="outline"
      size="default"
      className={`w-full gap-1.5 text-xs cursor-pointer sm:w-auto ${controlSizingClassName("standard", "max-md:min-h-11 [@media(pointer:coarse)]:min-h-11")}`}
      disabled={pending}
      onClick={() => void dismiss(taskId, stamp)}
      data-testid="turn-error-dismiss-button"
    >
      {t("task:dismissTurnError")}
    </Button>
  );
}
