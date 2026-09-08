"use client";

import { IconAlertTriangle } from "@tabler/icons-react";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { useTranslation } from "react-i18next";
import { formatDistinctByteSizes } from "@/lib/utils/format-bytes";
import type { PlanSaveError } from "@/hooks/domains/session/use-task-plan";

type TaskPlanSaveErrorBannerProps = {
  saveError: PlanSaveError;
};

/** Renders a rejected plan save. Both variants render localized copy only — never `err.message`. */
export function TaskPlanSaveErrorBanner({ saveError }: TaskPlanSaveErrorBannerProps) {
  const { t } = useTranslation("task");
  const message =
    saveError.kind === "content-too-large"
      ? (() => {
          const [limit, submitted] = formatDistinctByteSizes(saveError.limit, saveError.submitted);
          return t("task:planContentTooLarge", { limit, submitted });
        })()
      : t("task:failedToSavePlan");

  return (
    <div className="px-3 pt-2" data-testid="plan-save-error-banner">
      <Alert variant="destructive" role="alert">
        <IconAlertTriangle />
        <AlertDescription>{message}</AlertDescription>
      </Alert>
    </div>
  );
}
