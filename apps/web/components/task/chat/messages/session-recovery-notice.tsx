"use client";

import { IconInfoCircle } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";

export function SessionRecoveryNotice() {
  const { t } = useTranslation();
  return (
    <div
      className="flex w-full items-start gap-2 rounded border border-border/60 bg-muted/20 px-3 py-2 text-sm text-muted-foreground"
      data-testid="session-recovery-notice"
      role="note"
      aria-live="polite"
    >
      <IconInfoCircle aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0" />
      <p className="min-w-0">{t("task:sessionRestoredAfterInterruption")}</p>
    </div>
  );
}
