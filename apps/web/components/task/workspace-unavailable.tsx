"use client";

import { useEffect, useState } from "react";
import { useTaskLaunchErrorContext } from "./task-launch-error-context";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { SessionErrorDetails } from "./session-error-details";

import { IconAlertCircle, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import type { WorkspaceRestorationAttempt } from "@/lib/state/slices/session-runtime/workspace-restoration";
import { sanitizeWorkspaceRestorationDetails } from "@/lib/state/slices/session-runtime/workspace-restoration";

type WorkspaceUnavailableProps = {
  error?: string | null;
  failedSessionId?: string | null;
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

function useWorkspaceRecoveryOwner(restoration: WorkspaceRestorationAttempt | null | undefined) {
  const context = useTaskLaunchErrorContext();
  const failure = context?.automaticRecovery?.recoveryFailure;
  const ownerId =
    restoration?.attemptId &&
    restoration.taskId === context?.taskId &&
    failure?.outcome === "recovery_failed" &&
    failure.workspaceAttemptId === restoration.attemptId
      ? `session-recovery-owner-${restoration.attemptId}`
      : null;
  const [visibleOwner, setVisibleOwner] = useState<string | null>(null);
  useEffect(() => {
    const owner = ownerId ? document.getElementById(ownerId) : null;
    setVisibleOwner(owner && (!owner.checkVisibility || owner.checkVisibility()) ? ownerId : null);
  }, [ownerId, context]);
  return {
    ownerId: ownerId && visibleOwner === ownerId ? ownerId : null,
    invalidateOwner: () => setVisibleOwner(null),
  };
}

export function WorkspaceUnavailable({
  error,
  failedSessionId,
  restoration,
  onRetry,
  retryDisabled = false,
  compact = false,
}: WorkspaceUnavailableProps) {
  const { t } = useTranslation();
  const { context, sessionOwner, ownerId, invalidateOwner } = useUnavailableOwner(
    failedSessionId,
    restoration,
  );
  const hasOwner = Boolean(ownerId);

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
          {!hasOwner && restoration && onRetry && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              className={controlSizingClassName("standard", "mt-3 cursor-pointer gap-1.5")}
              disabled={retryDisabled}
              onClick={onRetry}
              data-testid="workspace-retry"
            >
              <IconRefresh className={isRestoring ? "h-3.5 w-3.5 animate-spin" : "h-3.5 w-3.5"} />
              {t("task:retry")}
            </Button>
          )}
          {hasOwner && (
            <a
              href={`#${ownerId}`}
              className="mt-2 inline-flex min-h-7 cursor-pointer items-center underline max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
              onClick={(event) => {
                event.preventDefault();
                if (sessionOwner && context?.revealSessionRecovery)
                  context.revealSessionRecovery(sessionOwner);
                else if (ownerId) focusRecoveryOwner(ownerId, invalidateOwner);
              }}
            >
              {t("task:viewRecovery")}
            </a>
          )}
          {!hasOwner && detail && <SessionErrorDetails>{detail}</SessionErrorDetails>}
        </div>
      </div>
    </div>
  );
}

function useUnavailableOwner(
  failedSessionId: string | null | undefined,
  restoration: WorkspaceRestorationAttempt | null | undefined,
) {
  const context = useTaskLaunchErrorContext();
  const candidate = context?.statusSummary?.active_error;
  const dependentSession = !restoration ? failedSessionId : null;
  const correlatedRestore = correlatedRestorationSession(context, restoration);
  const candidateSession = dependentSession ?? correlatedRestore;
  const sessionOwner =
    candidateSession &&
    candidate?.scope === "session" &&
    candidate.session_id === candidateSession &&
    candidate.stamp
      ? candidateSession
      : null;
  const { ownerId: restoreOwnerId, invalidateOwner } = useWorkspaceRecoveryOwner(restoration);
  const ownerId = sessionOwner ? `session-recovery-${sessionOwner}` : restoreOwnerId;
  return { context, sessionOwner, ownerId, invalidateOwner };
}

function correlatedRestorationSession(
  context: ReturnType<typeof useTaskLaunchErrorContext>,
  restoration: WorkspaceRestorationAttempt | null | undefined,
) {
  const failure = context?.automaticRecovery?.recoveryFailure;
  const correlatedRestore =
    restoration?.attemptId &&
    restoration.taskId === context?.taskId &&
    failure?.outcome === "recovery_failed" &&
    failure.workspaceAttemptId === restoration.attemptId
      ? restoration.sessionId
      : null;
  return correlatedRestore;
}

function focusRecoveryOwner(ownerId: string, invalidateOwner: () => void) {
  const owner = document.getElementById(ownerId);
  if (owner && (!owner.checkVisibility || owner.checkVisibility())) owner.focus();
  else invalidateOwner();
}
