"use client";

import { useCallback } from "react";
import {
  IconAlertTriangle,
  IconCircleCheck,
  IconPlayerPlay,
  IconPlus,
  IconRefresh,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";
import { NewSessionDialog } from "@/components/task/new-session-dialog";
import {
  EnsureSessionErrorBanner,
  SessionRecoveryNotice,
} from "@/components/task/ensure-session-error";
import { useAppStore } from "@/components/state-provider";
import {
  useSessionRecoveryActions,
  type SessionRecoveryBusyAction,
  type SessionRecoveryActions,
} from "@/hooks/domains/session/use-session-recovery-actions";
import type { BranchRecoveryDetails } from "@/lib/services/session-recovery-service";

export type SessionStoppedBannerMode = "recoverable" | "completed";

export type SessionStoppedBannerProps = {
  mode: SessionStoppedBannerMode;
  showDialog: boolean;
  onShowDialog: (open: boolean) => void;
  taskId: string | null;
  sessionId: string | null;
  workspaceId?: string | null;
  message?: string;
  detail?: string;
  resumeLabel?: string;
  resumingLabel?: string;
  recoveryActions?: SessionRecoveryActions;
};

function StoppedRecoveryFeedback({
  workspaceId,
  recoveryError,
  recoveryNotice,
  branchDetails,
  busyAction,
  onRetry,
  onRestore,
  onNewBranch,
}: {
  workspaceId?: string | null;
  recoveryError: Error | null;
  recoveryNotice: string | null;
  branchDetails: BranchRecoveryDetails | null;
  busyAction: SessionRecoveryBusyAction;
  onRetry: () => void;
  onRestore: () => void;
  onNewBranch: () => void;
}) {
  const { t } = useTranslation();
  if (!recoveryError && !recoveryNotice) return null;
  const restoreAction = {
    label: t("task:restoreReadOnlyWorkspace"),
    onClick: onRestore,
    testId: "recovery-restore-workspace-button",
    disabled: busyAction !== null,
  };
  return (
    <>
      {recoveryError ? (
        <EnsureSessionErrorBanner
          error={recoveryError}
          onRetry={onRetry}
          retryDisabled={busyAction !== null}
          workspaceId={workspaceId}
          compact
          action={
            branchDetails
              ? {
                  label: t("task:continueOnNewBranch"),
                  onClick: onNewBranch,
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
    </>
  );
}

function RecoverableSessionActions({
  onShowDialog,
  taskId,
  sessionId,
  workspaceId,
  resumeLabel,
  resumingLabel,
  recoveryActions,
}: Pick<
  SessionStoppedBannerProps,
  | "onShowDialog"
  | "taskId"
  | "sessionId"
  | "workspaceId"
  | "resumeLabel"
  | "resumingLabel"
  | "recoveryActions"
>) {
  const { t } = useTranslation();
  const resumeText = resumeLabel ?? t("task:resume");
  const resumingText = resumingLabel ?? t("task:resuming");
  const localRecoveryActions = useSessionRecoveryActions({
    taskId: taskId ?? "",
    sessionId: sessionId ?? "",
  });
  const {
    busyAction,
    recoveryError,
    branchDetails,
    recoveryNotice,
    handleRecover,
    handleRestore,
    handleRetry,
    handleNewBranch,
  } = recoveryActions ?? localRecoveryActions;

  const profileExists = useAppStore((s) => {
    if (!sessionId) return false;
    const agentProfileId = s.taskSessions.items[sessionId]?.agent_profile_id;
    return (
      !!agentProfileId && s.agentProfiles.items.some((p: { id: string }) => p.id === agentProfileId)
    );
  });

  const handleResume = useCallback(() => {
    if (taskId && sessionId) void handleRecover("resume");
  }, [handleRecover, sessionId, taskId]);
  const handleFreshStart = useCallback(() => {
    if (!profileExists) {
      onShowDialog(true);
      return;
    }
    if (taskId && sessionId) void handleRecover("fresh_start");
  }, [profileExists, onShowDialog, handleRecover]);

  return (
    <div className="flex w-full flex-col gap-2 sm:w-auto">
      <StoppedRecoveryFeedback
        workspaceId={workspaceId}
        recoveryError={recoveryError}
        recoveryNotice={recoveryNotice}
        branchDetails={branchDetails}
        busyAction={busyAction}
        onRetry={handleRetry}
        onRestore={() => void handleRestore()}
        onNewBranch={handleNewBranch}
      />
      <RecoverableSessionButtons
        taskId={taskId}
        sessionId={sessionId}
        profileExists={profileExists}
        busyAction={busyAction}
        resumeText={resumeText}
        resumingText={resumingText}
        onResume={handleResume}
        onFreshStart={handleFreshStart}
      />
    </div>
  );
}

function RecoverableSessionButtons({
  taskId,
  sessionId,
  profileExists,
  busyAction,
  resumeText,
  resumingText,
  onResume,
  onFreshStart,
}: {
  taskId: string | null;
  sessionId: string | null;
  profileExists: boolean;
  busyAction: SessionRecoveryBusyAction;
  resumeText: string;
  resumingText: string;
  onResume: () => void;
  onFreshStart: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex w-full flex-col gap-2 sm:w-auto sm:flex-row">
      {sessionId && taskId && (
        <Tooltip>
          <TooltipTrigger asChild>
            <span
              className="inline-flex w-full sm:w-auto"
              data-testid="failed-session-resume-wrapper"
            >
              <Button
                variant="default"
                size="sm"
                data-testid="recovery-resume-button"
                className="min-h-11 w-full shrink-0 gap-1.5 cursor-pointer sm:w-auto"
                onClick={onResume}
                disabled={busyAction !== null || !profileExists}
              >
                <IconPlayerPlay className="h-3.5 w-3.5" />
                {busyAction === "resume" ? resumingText : resumeText}
              </Button>
            </span>
          </TooltipTrigger>
          {!profileExists && (
            <TooltipContent>{t("task:agentProfileNoLongerExists")}</TooltipContent>
          )}
        </Tooltip>
      )}
      <Button
        variant="outline"
        size="sm"
        className="min-h-11 w-full shrink-0 gap-1.5 cursor-pointer sm:w-auto"
        onClick={onFreshStart}
        disabled={busyAction !== null}
        data-testid="recovery-fresh-button"
      >
        <IconRefresh className="h-3.5 w-3.5" />
        {busyAction === "fresh_start" ? t("task:starting") : t("task:startFreshSession")}
      </Button>
    </div>
  );
}

export function SessionStoppedBanner({
  mode,
  showDialog,
  onShowDialog,
  taskId,
  sessionId,
  workspaceId,
  message,
  detail,
  resumeLabel,
  resumingLabel,
  recoveryActions,
}: SessionStoppedBannerProps) {
  const { t } = useTranslation();
  const isCompleted = mode === "completed";
  const bannerMessage = isCompleted
    ? t("task:sessionCompleted")
    : (message ?? t("task:agentHasStopped"));

  return (
    <>
      <div
        data-testid={isCompleted ? "completed-session-banner" : "failed-session-banner"}
        data-session-stopped-mode={mode}
        className="overflow-hidden rounded border border-border"
      >
        <div className="flex flex-col gap-3 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex min-w-0 flex-1 items-start gap-2 text-sm text-muted-foreground">
            {isCompleted ? (
              <IconCircleCheck className="mt-0.5 h-4 w-4 shrink-0 text-green-500" />
            ) : (
              <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-orange-500" />
            )}
            <span className="min-w-0 break-words">{bannerMessage}</span>
            {detail && (
              <span className="min-w-0 break-words text-xs text-muted-foreground">({detail})</span>
            )}
          </div>

          {isCompleted ? (
            <Button
              variant="default"
              size="sm"
              data-testid="completed-session-new-agent-button"
              className="min-h-11 w-full shrink-0 gap-1.5 cursor-pointer sm:w-auto"
              onClick={() => {
                if (taskId) onShowDialog(true);
              }}
              disabled={!taskId}
            >
              <IconPlus className="h-3.5 w-3.5" />
              {t("task:newAgent")}
            </Button>
          ) : (
            <RecoverableSessionActions
              onShowDialog={onShowDialog}
              taskId={taskId}
              sessionId={sessionId}
              workspaceId={workspaceId}
              resumeLabel={resumeLabel}
              resumingLabel={resumingLabel}
              recoveryActions={recoveryActions}
            />
          )}
        </div>
      </div>
      {taskId && (
        <NewSessionDialog
          open={showDialog}
          onOpenChange={onShowDialog}
          taskId={taskId}
          workspaceId={workspaceId}
        />
      )}
    </>
  );
}
