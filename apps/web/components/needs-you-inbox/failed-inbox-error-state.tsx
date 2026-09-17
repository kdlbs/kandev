"use client";

import { IconAlertTriangle } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";

// No retry control: recovery needs no operator action here (design-01#Failure-and-recovery)
// -- the periodic and foreground-return triggers already re-read within the
// same bound the rows are, so a retry button would offer nothing.
export function FailedInboxErrorState() {
  const { t } = useTranslation();
  return (
    <div
      className="rounded-lg border border-border p-8 text-center"
      data-testid="failed-inbox-error"
      role="alert"
    >
      <IconAlertTriangle className="mx-auto h-6 w-6 text-destructive" />
      <p className="mt-2 text-sm font-medium">{t("failedInbox:errorTitle")}</p>
      <p className="mt-1 text-xs text-muted-foreground">{t("failedInbox:errorDescription")}</p>
    </div>
  );
}
