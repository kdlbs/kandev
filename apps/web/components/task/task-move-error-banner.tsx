"use client";

import { IconAlertTriangle } from "@tabler/icons-react";
import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
import { useTranslation } from "react-i18next";

export function getTaskMoveErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message.trim()) return error.message;
  if (typeof error === "string" && error.trim()) return error;
  return fallback;
}

type TaskMoveErrorBannerProps = {
  error: unknown;
};

export function TaskMoveErrorBanner({ error }: TaskMoveErrorBannerProps) {
  const { t } = useTranslation();
  const title = t("task:failedToMoveTask");
  const detail = getTaskMoveErrorMessage(error, title);

  return (
    <div className="px-3 pt-2" data-testid="task-move-error-banner">
      <Alert variant="destructive" role="alert">
        <IconAlertTriangle />
        <AlertTitle>{title}</AlertTitle>
        <AlertDescription>{detail}</AlertDescription>
      </Alert>
    </div>
  );
}
