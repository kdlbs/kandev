"use client";

import Link from "@/components/routing/app-link";
import { IconAlertTriangle, IconInfoCircle, IconRefresh } from "@tabler/icons-react";
import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";
import { cn } from "@/lib/utils";

// EnsureSessionErrorInfo wraps a parsed ensure error so UI can offer a targeted action for the missing-agent-profile case.
export type EnsureSessionErrorInfo = {
  title: string;
  detail: string;
  isAgentProfileMissing: boolean;
  action: { label: string; href: string } | null;
};

// Matches the backend's exact validation message in task_http_handlers.go / task_ws_handlers.go.
// i18n-exempt: matches the backend's exact validation message. See the comment above.
const AGENT_PROFILE_MISSING_HINT = "agent_profile_id is required";

export function describeEnsureError(
  error: Error | null,
  workspaceId?: string | null,
): EnsureSessionErrorInfo | null {
  if (!error) return null;
  const message = error.message ?? "";
  const isAgentProfileMissing = message.toLowerCase().includes(AGENT_PROFILE_MISSING_HINT);
  if (isAgentProfileMissing) {
    return {
      title: t("task:noAgentProfileConfigured"),
      detail: t("task:noAgentProfileConfiguredDetail"),
      isAgentProfileMissing: true,
      action: workspaceId
        ? {
            label: t("task:openWorkspaceSettings"),
            href: `/settings/workspaces/${workspaceId}`,
          }
        : null,
    };
  }
  return {
    title: t("task:couldnTStartASession"),
    detail: message || t("task:backendRejectedSessionRequest"),
    isAgentProfileMissing: false,
    action: null,
  };
}

type RecoveryAction = {
  label: string;
  onClick: () => void;
  testId: string;
  disabled?: boolean;
};

type BannerProps = {
  error: Error | null;
  onRetry: () => void;
  workspaceId?: string | null;
  action?: RecoveryAction;
  secondaryAction?: RecoveryAction;
  retryDisabled?: boolean;
  testId?: string;
  compact?: boolean;
};

/** Slim banner for the task page, rendered above the layout. */
export function EnsureSessionErrorBanner({
  error,
  onRetry,
  workspaceId,
  action,
  secondaryAction,
  retryDisabled,
  testId = "ensure-session-error-banner",
  compact = false,
}: BannerProps) {
  const { t } = useTranslation();
  const info = describeEnsureError(error, workspaceId);
  if (!info) return null;
  return (
    <div className={cn(!compact && "px-3 pt-2")} data-testid={testId}>
      <Alert variant="destructive">
        <IconAlertTriangle />
        <AlertTitle>{info.title}</AlertTitle>
        <AlertDescription>
          <span>{info.detail}</span>
          <span className="mt-1 flex flex-wrap items-center gap-2">
            {info.action ? (
              <Link
                href={info.action.href}
                className="cursor-pointer underline underline-offset-2 hover:text-foreground"
                data-testid="ensure-session-error-action"
              >
                {info.action.label}
              </Link>
            ) : null}
            {action ? (
              <Button
                variant="outline"
                size="sm"
                className="min-h-11 cursor-pointer px-2 text-xs"
                onClick={action.onClick}
                disabled={action.disabled}
                data-testid={action.testId}
              >
                {action.label}
              </Button>
            ) : null}
            {secondaryAction ? (
              <Button
                variant="outline"
                size="sm"
                className="min-h-11 cursor-pointer px-2 text-xs"
                onClick={secondaryAction.onClick}
                disabled={secondaryAction.disabled}
                data-testid={secondaryAction.testId}
              >
                {secondaryAction.label}
              </Button>
            ) : null}
            <Button
              variant="outline"
              size="sm"
              className="min-h-11 cursor-pointer px-2 text-xs"
              onClick={onRetry}
              disabled={retryDisabled}
              data-testid="ensure-session-error-retry"
            >
              <IconRefresh className="size-3" />
              {t("task:retry")}
            </Button>
          </span>
        </AlertDescription>
      </Alert>
    </div>
  );
}

/** Non-blocking result notice used when the workspace remains available read-only. */
export function SessionRecoveryNotice({ message }: { message: string }) {
  return (
    <div className="px-3 pt-2" data-testid="session-recovery-notice">
      <Alert>
        <IconInfoCircle />
        <AlertDescription>{message}</AlertDescription>
      </Alert>
    </div>
  );
}

/** Shared inline rendering for automatic resume failures and read-only notices. */
export function SessionRecoveryFeedback({
  error,
  notice,
  onRetry,
  workspaceId,
  action,
  secondaryAction,
  retryDisabled,
  testId = "session-recovery-error",
}: {
  error: string | null;
  notice: string | null;
  onRetry: () => void;
  workspaceId?: string | null;
  action?: RecoveryAction;
  secondaryAction?: RecoveryAction;
  retryDisabled?: boolean;
  testId?: string;
}) {
  return (
    <>
      <EnsureSessionErrorBanner
        error={error ? new Error(error) : null}
        onRetry={onRetry}
        workspaceId={workspaceId}
        action={action}
        secondaryAction={secondaryAction}
        retryDisabled={retryDisabled}
        testId={testId}
      />
      {notice ? <SessionRecoveryNotice message={notice} /> : null}
    </>
  );
}

/** Full-panel centered state for the kanban preview's empty-sessions slot. */
export function EnsureSessionErrorEmptyState({
  error,
  onRetry,
  workspaceId,
  retryDisabled,
}: BannerProps) {
  const { t } = useTranslation();
  const info = describeEnsureError(error, workspaceId);
  if (!info) return null;
  return (
    <div
      className="flex h-full flex-col items-center justify-center gap-3 px-4 text-center text-sm"
      data-testid="preview-ensure-error"
    >
      <span className="font-medium text-foreground">{info.title}</span>
      <span className="max-w-xs text-muted-foreground">{info.detail}</span>
      <span className="flex flex-wrap items-center justify-center gap-2">
        {info.action ? (
          <Link
            href={info.action.href}
            className="cursor-pointer underline underline-offset-2 hover:text-foreground"
            data-testid="ensure-session-error-action"
          >
            {info.action.label}
          </Link>
        ) : null}
        <Button
          variant="outline"
          size="sm"
          className="cursor-pointer"
          onClick={onRetry}
          disabled={retryDisabled}
          data-testid="ensure-session-error-retry"
        >
          {t("task:retry")}
        </Button>
      </span>
    </div>
  );
}
