"use client";

import { IconAlertTriangle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";

// AC .25: an error, including a workspace the caller cannot see, renders this
// state rather than the empty state, and the History badge shows no count.
export function InboxHistoryErrorState({ onRetry }: { onRetry: () => void }) {
  const { t } = useTranslation();
  return (
    <div
      className="rounded-lg border border-border p-8 text-center"
      data-testid="inbox-history-error"
      role="alert"
    >
      <IconAlertTriangle className="mx-auto h-6 w-6 text-destructive" />
      <p className="mt-2 text-sm font-medium">{t("inboxHistory:errorTitle")}</p>
      <p className="mt-1 text-xs text-muted-foreground">{t("inboxHistory:errorDescription")}</p>
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="mt-3 cursor-pointer"
        onClick={onRetry}
      >
        {t("inboxHistory:retry")}
      </Button>
    </div>
  );
}
