"use client";

import { IconAlertTriangle } from "@tabler/icons-react";
import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
import { useTranslation } from "react-i18next";
import { getTaskMoveErrorDetail } from "./task-move-error-message";

type TaskMoveErrorBannerProps = {
  error: unknown;
};

export function TaskMoveErrorBanner({ error }: TaskMoveErrorBannerProps) {
  const { t } = useTranslation();
  const title = t("task:failedToMoveTask");
  const detail = getTaskMoveErrorDetail(error, title, t);

  return (
    <div className="px-3 pt-2" data-testid="task-move-error-banner">
      <Alert variant="destructive" role="alert">
        <IconAlertTriangle />
        <AlertTitle>{title}</AlertTitle>
        {detail !== null && <AlertDescription>{detail}</AlertDescription>}
      </Alert>
    </div>
  );
}
