"use client";

import { IconAlertTriangle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import type { PlanCommentMigrationStatus } from "@/lib/state/slices/session/types";

type PlanCommentMigrationNoticeProps = {
  status: PlanCommentMigrationStatus;
  needsAttention: boolean;
  retry: () => void;
};

export function PlanCommentMigrationNotice({
  status,
  needsAttention,
  retry,
}: PlanCommentMigrationNoticeProps) {
  const { t } = useTranslation("task");
  if (!needsAttention) return null;

  const isFailed = status === "failed";
  const message = isFailed ? t("planCommentMigrationFailed") : t("planCommentMigrationNeedsPlan");

  return (
    <div
      className="mx-1 mb-1 flex min-h-9 flex-wrap items-center gap-2 rounded-md border border-border bg-muted/50 px-2 py-1.5 text-xs text-muted-foreground [@media(pointer:coarse)]:min-h-11"
      role={isFailed ? "alert" : "status"}
      data-testid="plan-comment-migration-notice"
    >
      <IconAlertTriangle className="h-4 w-4 shrink-0" />
      <span className="min-w-0 flex-1">{message}</span>
      <Button
        type="button"
        size="sm"
        variant="outline"
        className="h-7 shrink-0 text-xs [@media(pointer:coarse)]:h-11"
        onClick={retry}
      >
        {t("retry")}
      </Button>
    </div>
  );
}
