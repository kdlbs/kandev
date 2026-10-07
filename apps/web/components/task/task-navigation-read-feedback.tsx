"use client";

import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";

export type TaskNavigationReadRecovery = {
  temporaryError: boolean;
  retrying: boolean;
  onRetry: () => void;
};

export function TaskNavigationReadFeedback({
  recovery,
}: {
  recovery?: TaskNavigationReadRecovery;
}) {
  const { t } = useTranslation();
  if (!recovery || (!recovery.temporaryError && !recovery.retrying)) return null;
  return (
    <div
      role="status"
      aria-live="polite"
      aria-busy={recovery.retrying}
      className="flex min-w-0 shrink-0 flex-col gap-2 border-b px-3 py-2 text-sm md:flex-row md:items-center md:justify-between md:px-4"
      data-testid="task-read-recovery-notice"
    >
      <span className="min-w-0 text-muted-foreground">
        {t(recovery.retrying ? "task:retrying" : "common:taskReadRefreshFailureNotice")}
      </span>
      <Button
        size="default"
        className="max-md:w-full"
        disabled={recovery.retrying}
        onClick={recovery.onRetry}
        data-testid="task-read-retry"
      >
        {t("task:retry")}
      </Button>
    </div>
  );
}
