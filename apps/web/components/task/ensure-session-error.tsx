"use client";

import Link from "next/link";
import { IconAlertTriangle, IconRefresh } from "@tabler/icons-react";
import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
import { Button } from "@kandev/ui/button";

// EnsureSessionErrorInfo wraps a parsed ensure error so UI can offer a targeted action for the missing-agent-profile case.
export type EnsureSessionErrorInfo = {
  title: string;
  detail: string;
  isAgentProfileMissing: boolean;
  action: { label: string; href: string } | null;
};

// Matches the backend's exact validation message in task_http_handlers.go / task_ws_handlers.go.
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
      title: "No agent profile configured",
      detail:
        "This task has no agent profile, and the workspace, workflow, and workflow step have no default set. Pick a default agent profile so new tasks can start a session.",
      isAgentProfileMissing: true,
      action: workspaceId
        ? { label: "Open workspace settings", href: `/settings/workspace/${workspaceId}` }
        : null,
    };
  }
  return {
    title: "Couldn't start a session",
    detail: message || "The backend rejected the session request.",
    isAgentProfileMissing: false,
    action: null,
  };
}

type BannerProps = {
  error: Error | null;
  onRetry: () => void;
  workspaceId?: string | null;
};

/** Slim banner for the task page, rendered above the layout. */
export function EnsureSessionErrorBanner({ error, onRetry, workspaceId }: BannerProps) {
  const info = describeEnsureError(error, workspaceId);
  if (!info) return null;
  return (
    <div className="px-3 pt-2" data-testid="ensure-session-error-banner">
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
            <Button
              variant="outline"
              size="sm"
              className="h-6 cursor-pointer px-2 text-xs"
              onClick={onRetry}
              data-testid="ensure-session-error-retry"
            >
              <IconRefresh className="size-3" />
              Retry
            </Button>
          </span>
        </AlertDescription>
      </Alert>
    </div>
  );
}

/** Full-panel centered state for the kanban preview's empty-sessions slot. */
export function EnsureSessionErrorEmptyState({ error, onRetry, workspaceId }: BannerProps) {
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
          data-testid="ensure-session-error-retry"
        >
          Retry
        </Button>
      </span>
    </div>
  );
}
