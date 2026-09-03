"use client";

import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  EnsureSessionErrorBanner,
  SessionRecoveryNotice,
} from "@/components/task/ensure-session-error";
import {
  useSessionRecoveryActions,
  type SessionRecoveryBusyAction,
} from "@/hooks/domains/session/use-session-recovery-actions";
import type { SessionRecoveryAction } from "@/lib/services/session-recovery-service";
import type { MessageAction } from "@/components/task/chat/types";
import { ACTION_ICON_MAP, ActionButton } from "./action-message-actions";

export function sessionRecoveryAction(action: MessageAction): SessionRecoveryAction | null {
  if (action.type !== "ws_request" || !action.params) return null;
  if (action.params.method !== "session.recover") return null;
  const payload = action.params.payload;
  if (!payload || typeof payload !== "object") return null;
  const recoveryAction = (payload as { action?: unknown }).action;
  switch (recoveryAction) {
    case "resume":
    case "resume_new_branch":
    case "fresh_start":
    case "runtime_retry":
      return recoveryAction;
    default:
      return null;
  }
}

export function SessionRecoveryActionButtons({
  actions,
  taskId,
  sessionId,
  onRecoveryRequested,
}: {
  actions: MessageAction[];
  taskId: string;
  sessionId: string;
  onRecoveryRequested: () => void;
}) {
  const { t } = useTranslation();
  const {
    busyAction,
    recoveryError,
    branchDetails,
    recoveryNotice,
    handleRecover,
    handleRestore,
    handleRetry,
    handleNewBranch,
  } = useSessionRecoveryActions({ taskId, sessionId });

  const onRecoveryAction = useCallback(
    async (action: SessionRecoveryAction) => {
      if (await handleRecover(action)) onRecoveryRequested();
    },
    [handleRecover, onRecoveryRequested],
  );
  const onRetry = useCallback(async () => {
    if (await handleRetry()) onRecoveryRequested();
  }, [handleRetry, onRecoveryRequested]);
  const restoreAction = {
    label: t("task:restoreReadOnlyWorkspace"),
    onClick: () => void handleRestore(),
    testId: "recovery-restore-workspace-button",
    disabled: busyAction !== null,
  };

  return (
    <>
      {recoveryError ? (
        <EnsureSessionErrorBanner
          error={recoveryError}
          onRetry={() => void onRetry()}
          retryDisabled={busyAction !== null}
          compact
          action={
            branchDetails
              ? {
                  label: t("task:continueOnNewBranch"),
                  onClick: () =>
                    void handleNewBranch().then((success) => {
                      if (success) onRecoveryRequested();
                    }),
                  testId: "recovery-new-branch-button",
                  disabled: busyAction !== null,
                }
              : restoreAction
          }
          secondaryAction={branchDetails ? restoreAction : undefined}
          testId="session-recovery-error"
        />
      ) : null}
      {recoveryNotice ? <SessionRecoveryNotice message={recoveryNotice} /> : null}
      <div className="mt-2 flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center">
        {actions.map((action, i) => {
          const recoveryAction = sessionRecoveryAction(action);
          return recoveryAction ? (
            <SessionRecoveryActionButton
              key={action.test_id ?? i}
              action={action}
              busyAction={busyAction}
              onClick={() => void onRecoveryAction(recoveryAction)}
            />
          ) : (
            <ActionButton key={action.test_id ?? i} action={action} messageTaskId={taskId} />
          );
        })}
      </div>
    </>
  );
}

function SessionRecoveryActionButton({
  action,
  busyAction,
  onClick,
}: {
  action: MessageAction;
  busyAction: SessionRecoveryBusyAction;
  onClick: () => void;
}) {
  const Icon = action.icon ? ACTION_ICON_MAP[action.icon] : null;
  const button = (
    <Button
      variant="outline"
      size="sm"
      className="h-auto min-h-11 w-full gap-1.5 text-xs cursor-pointer sm:min-h-8 sm:w-auto"
      disabled={busyAction !== null}
      onClick={onClick}
      data-testid={action.test_id}
    >
      {Icon && <Icon className="h-3 w-3" />}
      {action.label}
    </Button>
  );

  if (action.tooltip) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>{button}</TooltipTrigger>
        <TooltipContent side="top">{action.tooltip}</TooltipContent>
      </Tooltip>
    );
  }
  return button;
}
