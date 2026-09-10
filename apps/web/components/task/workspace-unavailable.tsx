"use client";

import { IconAlertCircle, IconChevronDown, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import type { WorkspaceRestorationAttempt } from "@/lib/state/slices/session-runtime/workspace-restoration";
import { sanitizeWorkspaceRestorationDetails } from "@/lib/state/slices/session-runtime/workspace-restoration";

type WorkspaceUnavailableProps = {
  error?: string | null;
  restoration?: WorkspaceRestorationAttempt | null;
  onRetry?: () => void;
  retryDisabled?: boolean;
  compact?: boolean;
};

function getWorkspaceRestoreMessage(
  isRestoring: boolean,
  hasRestoreError: boolean,
  t: ReturnType<typeof useTranslation>["t"],
) {
  if (isRestoring) return t("task:workspaceRestorePending");
  if (hasRestoreError) return t("task:workspaceRestoreFailed");
  return t("task:thisSessionDidNotFinishSetting");
}

function getWorkspaceRestoreDetail(
  restoration: WorkspaceRestorationAttempt | null | undefined,
  error: string | null | undefined,
) {
  if (restoration?.details) return restoration.details;
  if (error) return sanitizeWorkspaceRestorationDetails(error);
  return null;
}

export function WorkspaceUnavailable({
  error,
  restoration,
  onRetry,
  retryDisabled = false,
  compact = false,
}: WorkspaceUnavailableProps) {
  const { t } = useTranslation();
  const isRestoring = restoration?.status === "pending";
  const hasRestoreError = restoration?.status === "error";
  const detail = getWorkspaceRestoreDetail(restoration, error);
  return (
    <div
      data-testid="workspace-unavailable"
      role="status"
      aria-label={t("task:workspaceUnavailable")}
      aria-busy={isRestoring || undefined}
      className={`${compact ? "w-full" : "h-full w-full"} min-w-0 p-4`}
    >
      <div className="flex min-w-0 items-start gap-2">
        <IconAlertCircle
          className="mt-0.5 h-4 w-4 flex-shrink-0 text-muted-foreground"
          aria-hidden="true"
        />
        <div className="min-w-0 flex-1">
          <div className="text-sm font-medium text-foreground">
            {t("task:workspaceUnavailable")}
          </div>
          <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
            {getWorkspaceRestoreMessage(isRestoring, hasRestoreError, t)}
          </p>
          {restoration && onRetry && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="mt-3 h-11 cursor-pointer gap-1.5 sm:h-8"
              disabled={retryDisabled}
              onClick={onRetry}
              data-testid="workspace-retry"
            >
              <IconRefresh className={isRestoring ? "h-3.5 w-3.5 animate-spin" : "h-3.5 w-3.5"} />
              {t("task:retry")}
            </Button>
          )}
          {detail && (
            <details className="mt-2 min-w-0 text-xs text-muted-foreground">
              <summary className="flex min-h-11 cursor-pointer list-none items-center gap-1.5 sm:min-h-8">
                <IconChevronDown className="h-3.5 w-3.5" aria-hidden="true" />
                {t("task:technicalDetails")}
              </summary>
              <pre className="max-h-48 max-w-full overflow-y-auto overscroll-contain whitespace-pre-wrap break-words rounded bg-muted/50 p-2 font-mono text-[11px]">
                {detail}
              </pre>
            </details>
          )}
        </div>
      </div>
    </div>
  );
}
