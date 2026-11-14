"use client";

import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { useTranslation } from "react-i18next";
import { useContinueTurnError } from "@/hooks/use-continue-turn-error";
import { useTaskTurnError } from "@/hooks/use-task-turn-error";
import { TaskTurnErrorDismissButton } from "./turn-error-dismiss-button";

/**
 * Current failed-turn actions. Continuation submits one normal manual
 * `continue` message for the displayed error; dismissal acknowledges only the
 * rendered stamp. Both controls share readiness and pending state.
 */
export function TurnErrorActions({
  taskId,
  sessionId,
  stamp,
}: {
  taskId: string;
  sessionId: string;
  stamp: string;
}) {
  const { t } = useTranslation();
  const { available, pending, outcome, continueTurn } = useContinueTurnError({
    taskId,
    sessionId,
    stamp,
  });
  const { dismiss, pendingStamp } = useTaskTurnError();
  const dismissing = pendingStamp === stamp;
  const busy = pending || dismissing;
  return (
    <div
      data-testid="turn-error-actions"
      className="mt-2 flex w-full min-w-0 flex-col gap-2 md:flex-row md:flex-wrap md:items-center"
    >
      {available && (
        <Button
          variant="default"
          size="default"
          className={`w-full whitespace-normal break-words md:w-auto ${controlSizingClassName("standard", "max-md:min-h-12 [@media(pointer:coarse)]:min-h-12")}`}
          disabled={busy}
          onClick={() => void continueTurn()}
          data-testid="turn-error-continue-button"
        >
          {t("task:dismissAndContinue")}
        </Button>
      )}
      <TaskTurnErrorDismissButton disabled={busy} onClick={() => void dismiss(taskId, stamp)} />
      {outcome === "pending" && (
        <span role="status" aria-live="polite" className="text-xs text-muted-foreground">
          {t("task:continuingTurnError")}
        </span>
      )}
      {outcome === "failed" && (
        <span role="status" aria-live="polite" className="text-xs text-destructive">
          {t("task:failedToContinueTurnError")}
        </span>
      )}
    </div>
  );
}
