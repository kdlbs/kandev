"use client";

import { Button } from "@kandev/ui/button";
import Link from "@/components/routing/app-link";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { linkToTaskOverview } from "@/lib/links";
import { GridSpinner } from "@/components/grid-spinner";

/**
 * Ordinary progress for a bounded automatic recovery with no eligible content.
 * Retry stays present with its stable name but disabled until recovery ends.
 */
export function TaskLoadProgressState({ onRetry }: { onRetry?: () => void }) {
  const { t } = useTranslation();
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);

  return (
    <div
      className="flex h-full min-h-0 w-full items-center justify-center bg-background px-4"
      data-testid="task-read-progress-state"
    >
      <div
        className="flex min-h-24 max-w-sm min-w-0 flex-col items-center justify-center gap-3 text-center text-sm text-muted-foreground"
        role="status"
        aria-live="polite"
        aria-busy
      >
        <span aria-hidden="true">
          <GridSpinner className="text-primary" />
        </span>
        <span>{t("common:loadingTask")}</span>
        <Button
          size="default"
          className="w-full max-w-xs"
          disabled
          onClick={onRetry}
          data-testid="task-read-retry"
        >
          {t("task:retry")}
        </Button>
        <Button
          asChild
          size="default"
          className="w-full max-w-xs px-3"
          data-testid="task-unavailable-overview-link"
        >
          <Link href={linkToTaskOverview({ workspaceId: activeWorkspaceId ?? undefined })}>
            {t("common:backToTaskOverview")}
          </Link>
        </Button>
      </div>
    </div>
  );
}
