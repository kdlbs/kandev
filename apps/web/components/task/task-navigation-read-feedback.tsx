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
  if (!recovery?.temporaryError) return null;
  return (
    <div
      role="status"
      aria-live="polite"
      aria-busy={recovery.retrying}
      className="absolute left-3 right-3 top-[calc(3.5rem+env(safe-area-inset-top,0px)+0.5rem)] z-30 flex min-w-0 flex-col gap-2 rounded-lg border bg-popover px-3 py-2 text-sm shadow-md md:left-auto md:top-14 md:max-w-sm md:flex-row md:items-center"
      data-testid="task-read-recovery-notice"
    >
      <span className="min-w-0 text-muted-foreground">
        {t("common:taskReadRefreshFailureNotice")}
        {recovery.retrying && <span className="block">{t("task:retrying")}</span>}
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
