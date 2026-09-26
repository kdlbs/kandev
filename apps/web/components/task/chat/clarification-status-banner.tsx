"use client";

import { IconInfoCircle, IconAlertTriangle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";

// Surfaces a failed or expired submit/skip so it is never indistinguishable
// from a dead button. "error" keeps the recorded answers and offers retry()
// (the hook replays whichever of submit/skip last failed); "expired" means
// the bundle itself is no longer active on the backend, so retrying would
// only fail again -- no retry affordance is offered for it.
export function ClarificationStatusBanner({
  state,
  onRetry,
}: {
  state: "error" | "expired";
  onRetry: () => void;
}) {
  const { t } = useTranslation();
  if (state === "error") {
    return (
      <div
        data-testid="clarification-submit-error"
        role="alert"
        className="mx-4 mt-3 mb-2 flex items-center justify-between gap-3 rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive"
      >
        <span className="flex items-center gap-1.5">
          <IconAlertTriangle className="h-3.5 w-3.5 flex-shrink-0" />
          {t("task:clarificationResponseFailed")}
        </span>
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="min-h-11 min-w-11 cursor-pointer md:min-h-0 md:min-w-0"
          onClick={onRetry}
          data-testid="clarification-retry"
        >
          {t("task:retry")}
        </Button>
      </div>
    );
  }
  return (
    <div
      data-testid="clarification-expired"
      role="status"
      className="mx-4 mt-3 mb-2 flex items-center gap-1.5 rounded-md border border-slate-300 bg-slate-50 px-3 py-2 text-xs text-slate-600 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-400"
    >
      <IconInfoCircle className="h-3.5 w-3.5 flex-shrink-0" />
      {t("task:clarificationExpired")}
    </div>
  );
}
