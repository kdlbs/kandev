"use client";

import type { Ref } from "react";
import { IconAlertTriangle } from "@tabler/icons-react";
import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
import { Button } from "@kandev/ui/button";
import { cn } from "@kandev/ui/lib/utils";
import { useTranslation } from "react-i18next";

export function ForkReviewStartWarning({
  inChat = false,
  onStart,
  startRef,
}: {
  inChat?: boolean;
  onStart?: () => void;
  startRef?: Ref<HTMLButtonElement>;
}) {
  const { t } = useTranslation();
  return (
    <Alert
      className={cn(
        "flex flex-col gap-3",
        inChat
          ? "rounded-xl border-border/80 bg-muted/40 p-3 @lg:flex-row @lg:items-center @lg:gap-4"
          : "border-amber-500/35 bg-amber-500/8",
      )}
      data-testid="fork-review-start-warning"
    >
      <div className="flex min-w-0 flex-1 items-start gap-2.5">
        <IconAlertTriangle
          aria-hidden="true"
          className="mt-0.5 size-4 shrink-0 text-amber-700 dark:text-amber-400"
        />
        <div className="min-w-0 space-y-1">
          <AlertTitle className={inChat ? "text-sm leading-5" : undefined}>
            {t("task:forkReviewManualStartTitle")}
          </AlertTitle>
          <AlertDescription>{t("task:forkReviewManualStartDetail")}</AlertDescription>
        </div>
      </div>
      {onStart && (
        <Button
          ref={startRef}
          type="button"
          className="min-h-11 w-full sm:min-h-0 pointer-coarse:min-h-11 @lg:w-auto"
          onClick={onStart}
        >
          {t("task:startTask")}
        </Button>
      )}
    </Alert>
  );
}
