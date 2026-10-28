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
      className="flex w-full min-w-0 flex-col gap-2 border-b bg-muted/50 px-3 py-2 text-sm md:flex-row md:items-center md:justify-between"
      data-testid="task-read-recovery-notice"
    >
      <span className="flex min-w-0 flex-col gap-0.5 text-muted-foreground">
        <span>{t("common:taskReadRefreshFailureNotice")}</span>
        {recovery.retrying ? <span>{t("task:retrying")}</span> : null}
      </span>
      <Button
        size="default"
        variant="outline"
        className="max-md:h-11 max-md:w-full pointer-coarse:h-11"
        disabled={recovery.retrying}
        onClick={recovery.onRetry}
        data-testid="task-read-retry"
      >
        {t("task:retry")}
      </Button>
    </div>
  );
}
