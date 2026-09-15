"use client";

import { IconAlertTriangle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";

export function NeedsYouInboxErrorState({ onRetry }: { onRetry: () => void }) {
  const { t } = useTranslation();
  return (
    <div
      className="rounded-lg border border-border p-8 text-center"
      data-testid="needs-you-inbox-error"
      role="alert"
    >
      <IconAlertTriangle className="mx-auto h-6 w-6 text-destructive" />
      <p className="mt-2 text-sm font-medium">{t("needsYouInbox:errorTitle")}</p>
      <p className="mt-1 text-xs text-muted-foreground">{t("needsYouInbox:errorDescription")}</p>
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="mt-3 cursor-pointer"
        onClick={onRetry}
      >
        {t("needsYouInbox:retry")}
      </Button>
    </div>
  );
}
