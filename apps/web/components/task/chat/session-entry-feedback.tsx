"use client";

import { IconInfoCircle, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { GridSpinner } from "@/components/grid-spinner";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { MessageHistoryStatus } from "@/hooks/domains/session/use-message-fetch-state";
import { cn } from "@/lib/utils";

function errorDetail(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message) return error.message;
  if (typeof error === "string" && error) return error;
  return fallback;
}

export function SessionHistoryFeedback({
  status,
  error,
  onRetry,
}: {
  status: MessageHistoryStatus;
  error: unknown;
  onRetry: () => void;
}) {
  const { t } = useTranslation();
  const { isFinePointer } = useResponsiveBreakpoint();

  if (status === "ready") return null;

  if (status === "loading" || status === "retrying") {
    return (
      <div
        className="flex items-center justify-center gap-2 py-8 text-muted-foreground"
        data-testid={status === "retrying" ? "session-history-retrying" : "session-history-loading"}
        role="status"
        aria-live="polite"
      >
        <span aria-hidden="true">
          <GridSpinner className="text-primary" />
        </span>
        <span>
          {status === "retrying" ? t("task:sessionHistoryRetrying") : t("task:loadingConversation")}
        </span>
      </div>
    );
  }

  return (
    <div
      className="mb-3 min-w-0 rounded-md border border-border bg-muted/20 px-3 py-2 text-sm"
      data-testid="session-history-unavailable"
      role="status"
      aria-live="polite"
    >
      <div className="flex min-w-0 items-start gap-2">
        <IconInfoCircle
          className="mt-0.5 size-4 shrink-0 text-muted-foreground"
          aria-hidden="true"
        />
        <div className="min-w-0 flex-1">
          <div className="font-medium">{t("task:sessionHistoryUnavailable")}</div>
          <div
            className={cn(
              "mt-2 flex gap-2",
              isFinePointer ? "flex-wrap items-center" : "flex-col items-stretch",
            )}
          >
            <Button
              type="button"
              variant="outline"
              size="sm"
              className={cn("cursor-pointer px-2 text-xs", !isFinePointer && "min-h-11 w-full")}
              data-testid="session-history-retry"
              onClick={onRetry}
            >
              <IconRefresh className="size-3" aria-hidden="true" />
              {t("task:retry")}
            </Button>
            <details className="min-w-0 text-xs" data-testid="session-history-details">
              <summary
                className={cn(
                  "block cursor-pointer select-none rounded-sm underline underline-offset-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                  isFinePointer ? "min-h-7 py-1" : "min-h-11 py-3",
                )}
                data-testid="session-history-details-summary"
              >
                {t("task:details")}
              </summary>
              <p className="mt-2 break-words text-muted-foreground">
                {errorDetail(error, t("task:sessionHistoryUnknownError"))}
              </p>
            </details>
          </div>
        </div>
      </div>
    </div>
  );
}
