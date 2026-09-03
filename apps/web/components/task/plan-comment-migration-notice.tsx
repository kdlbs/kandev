"use client";

import { IconAlertTriangle, IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import type { PlanCommentMigrationStatus } from "@/lib/state/slices/session/types";

type PlanCommentMigrationNoticeProps = {
  status: PlanCommentMigrationStatus;
  retry: () => void;
};

export function PlanCommentMigrationNotice({ status, retry }: PlanCommentMigrationNoticeProps) {
  const { t } = useTranslation("task");
  if (status === "complete") return null;

  const isFailed = status === "failed";
  const isWaitingForPlan = status === "waiting_for_plan";
  let message = t("restoringSavedPlanComments");
  if (isFailed) message = t("planCommentMigrationFailed");
  else if (isWaitingForPlan) message = t("planCommentMigrationNeedsPlan");

  return (
    <div
      className="mx-1 mb-1 flex min-h-9 items-center gap-2 rounded-md border border-border bg-muted/50 px-2 py-1.5 text-xs text-muted-foreground [@media(pointer:coarse)]:min-h-11"
      role={isFailed ? "alert" : "status"}
      data-testid="plan-comment-migration-notice"
    >
      {isFailed || isWaitingForPlan ? (
        <IconAlertTriangle className="h-4 w-4 shrink-0" />
      ) : (
        <IconLoader2 className="h-4 w-4 shrink-0 animate-spin" />
      )}
      <span className="min-w-0 flex-1">{message}</span>
      {isFailed && (
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="h-7 shrink-0 text-xs [@media(pointer:coarse)]:h-11"
          onClick={retry}
        >
          {t("retry")}
        </Button>
      )}
    </div>
  );
}
