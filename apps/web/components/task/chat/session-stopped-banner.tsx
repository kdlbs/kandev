"use client";

import { useCallback, useState } from "react";
import { IconAlertTriangle, IconCircleCheck, IconPlayerStop } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { NewSessionDialog } from "@/components/task/new-session-dialog";
import { useAppStore } from "@/components/state-provider";
import { RecoveryActions, type RecoveryChoice } from "@/components/task/recovery-actions";
import { sanitizeSessionErrorDetails } from "@/lib/session-error-details";
import { SessionErrorDetails } from "@/components/task/session-error-details";
import { useSessionActions } from "@/hooks/domains/session/use-session-actions";
import {
  useSessionRecoveryActions,
  type SessionRecoveryActions,
} from "@/hooks/domains/session/use-session-recovery-actions";
import { ManagedCloneRelocationConfirmation } from "./managed-clone-relocation-confirmation";

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
  uncertainDelivery?: boolean;
  recoveryActions?: SessionRecoveryActions;
};

export function useSessionProfileExists(sessionId: string | null): boolean {
  return useAppStore((s) => {
    const profileId = sessionId ? s.taskSessions.items[sessionId]?.agent_profile_id : null;
    return Boolean(profileId && s.agentProfiles.items.some((profile) => profile.id === profileId));
  });
}

function useStoppedRecoveryChoices(
  props: SessionStoppedBannerProps & { actions: SessionRecoveryActions },
  profileExists: boolean,
  onRelocateRequested: () => void,
): RecoveryChoice[] {
  const { t } = useTranslation();

  const {
    recoveryError,
    branchDetails,
    continuationDetails,
    handleRecover,
    handleRetry,
    handleRestore,
    handleNewBranch,
    handleContinueFromHistory,
  } = props.actions;
  const completed = props.mode === "completed";

  if (props.actions.managedCloneRecoveryStamp) {
    return [
      {
        kind: "relocate_and_resume",
        label: t("task:managedCloneRelocateResume"),
        testId: "managed-clone-relocate-button",
        onClick: onRelocateRequested,
      },
    ];
  }

  const choices: RecoveryChoice[] = [];
  if (props.uncertainDelivery && !completed) {
    if (props.taskId && props.sessionId)
      choices.push({
        kind: "retry_connection",
        label: t("task:retryConnection"),
        testId: "recovery-retry-connection-button",
        onClick: () => void handleRecover("retry_connection"),
      });
    return choices;
  }
  if (props.taskId && props.sessionId)
    choices.push({
      kind: "resume",
      label: props.resumeLabel ?? t("task:resume"),
      disclosure:
        !completed && props.actions.providerRestoredResumeEligible
          ? t("task:providerRestoredResumeDisclosure")
          : undefined,
      disabled: !profileExists,
      testId: "recovery-resume-button",
      onClick: () => {
        if (recoveryError) void handleRetry();
        else void handleRecover("resume");
      },
    });
  if (props.taskId)
    choices.push({
      kind: "fresh_start",
      label: completed ? t("task:newAgent") : t("task:startFreshSession"),
      testId: completed ? "completed-session-new-agent-button" : "recovery-fresh-button",
      onClick: () => {
        if (completed || !profileExists) props.onShowDialog(true);
        else void handleRecover("fresh_start");
      },
    });
  if (recoveryError && !props.actions.guardDetails)
    choices.push({
      kind: "restore",
      label: t("task:restoreReadOnlyWorkspace"),
      testId: "recovery-restore-workspace-button",
      onClick: () => void handleRestore(),
    });
  if (branchDetails)
    choices.push({
      kind: "resume_new_branch",
      label: t("task:continueOnNewBranch"),
      testId: "recovery-new-branch-button",
      onClick: () => void handleNewBranch(),
    });
  if (continuationDetails)
    choices.push({
      kind: "continue_from_history",
      label: t("task:continueFromHistory"),
      testId: "recovery-continue-from-history-button",
      onClick: () => void handleContinueFromHistory(),
    });
  return choices;
}

function UncertainDeliveryActions({
  actions,
  busyAction,
  blocked,
  taskId,
  sessionId,
}: {
  actions: RecoveryChoice[];
  busyAction: SessionRecoveryActions["busyAction"];
  blocked: boolean;
  taskId: string | null;
  sessionId: string | null;
}) {
  const { t } = useTranslation();
  const { stop } = useSessionActions({ taskId, sessionId });
  const [stopping, setStopping] = useState(false);
  const handleStop = useCallback(async () => {
    setStopping(true);
    try {
      await stop();
    } finally {
      setStopping(false);
    }
  }, [stop]);

  return (
    <div className="mt-3 flex min-w-0 flex-col gap-2 md:flex-row md:flex-wrap md:items-center">
      <RecoveryActions
        actions={actions.map((action) => ({ ...action, disabled: stopping || action.disabled }))}
        busy={busyAction !== null}
        busyAction={busyAction}
        blocked={blocked}
      />
      <Button
        type="button"
        variant="outline"
        aria-label={t("task:stop")}
        disabled={busyAction !== null || stopping || !taskId || !sessionId}
        onClick={() => void handleStop()}
        data-testid="recovery-stop-button"
        className="h-auto min-h-11 w-full cursor-pointer gap-1.5 whitespace-normal md:min-h-7 md:w-auto"
      >
        <IconPlayerStop aria-hidden="true" className="size-3.5 shrink-0" />
        {stopping ? t("task:stopping") : t("task:stop")}
      </Button>
    </div>
  );
}

function stoppedRecoveryCause(
  actions: SessionRecoveryActions,
  t: ReturnType<typeof useTranslation>["t"],
) {
  if (actions.branchDetails) return t("task:branchIsNoLongerAvailable");
  const message = actions.guardDetails
    ? sanitizeSessionErrorDetails(actions.recoveryError?.message, 240)
    : "";
  const fallback =
    actions.manualRecoveryFailure?.operation === "restore_workspace"
      ? t("task:failedToRestoreWorkspace")
      : t("task:failedToResumeSession");
  return message || fallback;
}

function stoppedSessionTitle(
  completed: boolean,
  managedCloneRecovery: boolean,
  uncertainDelivery: boolean,
  message: string,
  t: ReturnType<typeof useTranslation>["t"],
) {
  if (managedCloneRecovery) return t("task:managedCloneRelocationTitle");
  if (completed) return t("task:sessionCompleted");
  if (uncertainDelivery) return t("task:durableDeliveryUncertain");
  return sanitizeSessionErrorDetails(message, 240) || t("task:agentHasStopped");
}

function stoppedRecoveryErrorMessage(
  actions: SessionRecoveryActions,
  managedCloneRecovery: boolean,
  cause: string,
  t: ReturnType<typeof useTranslation>["t"],
) {
  if (!actions.recoveryError) return null;
  if (managedCloneRecovery && actions.lastFailedAction !== "relocate_and_resume") return null;
  if (managedCloneRecovery) return t("task:failedToResumeSession");
  return cause;
}

function StoppedSessionContent(
  props: SessionStoppedBannerProps & { actions: SessionRecoveryActions },
) {
  const { t } = useTranslation();
  const [relocationConfirmationOpen, setRelocationConfirmationOpen] = useState(false);
  const profileExists = useSessionProfileExists(props.sessionId);
  const { busyAction, recoveryError, recoveryNotice, guardDetails } = props.actions;
  const completed = props.mode === "completed";
  const blocked = Boolean(guardDetails && !guardDetails.retryable);
  const managedCloneRecovery = Boolean(props.actions.managedCloneRecoveryStamp);
  const choices = useStoppedRecoveryChoices(props, profileExists, () =>
    setRelocationConfirmationOpen(true),
  );
  const cause = stoppedRecoveryCause(props.actions, t);
  const recoveryErrorMessage = stoppedRecoveryErrorMessage(
    props.actions,
    managedCloneRecovery,
    cause,
    t,
  );
  const title = stoppedSessionTitle(
    completed,
    managedCloneRecovery,
    Boolean(props.uncertainDelivery),
    props.message ?? "",
    t,
  );
  const Icon = completed ? IconCircleCheck : IconAlertTriangle;
  return (
    <>
      <div
        data-testid={completed ? "completed-session-banner" : "failed-session-banner"}
        data-session-stopped-mode={props.mode}
        className="min-w-0 rounded border border-border p-3"
      >
        <div className="flex min-w-0 items-start gap-2">
          <Icon className="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
          <div className="min-w-0 flex-1">
            <p className="wrap-anywhere text-sm">{title}</p>
            {managedCloneRecovery && (
              <p className="mt-1 wrap-anywhere text-sm text-muted-foreground">
                {t("task:managedCloneRelocationBody")}
              </p>
            )}
            {props.sessionId && !profileExists && (
              <p className="mt-1 text-xs text-muted-foreground">
                {t("task:agentProfileNoLongerExists")}
              </p>
            )}
            {recoveryErrorMessage && (
              <p
                role="status"
                data-testid="session-recovery-error"
                className="mt-1 text-xs text-muted-foreground"
              >
                {recoveryErrorMessage}
              </p>
            )}
            {recoveryNotice && (
              <p role="status" className="mt-1 text-xs text-muted-foreground">
                {recoveryNotice}
              </p>
            )}
            {props.uncertainDelivery && !completed ? (
              <UncertainDeliveryActions
                actions={choices}
                busyAction={busyAction}
                blocked={blocked}
                taskId={props.taskId}
                sessionId={props.sessionId}
              />
            ) : (
              <RecoveryActions
                actions={choices}
                busy={busyAction !== null}
                busyAction={busyAction}
                blocked={blocked}
              />
            )}
            {!props.uncertainDelivery && (
              <SessionErrorDetails>
                {[props.message, props.detail, recoveryError?.message].filter(Boolean).join("\n")}
              </SessionErrorDetails>
            )}
          </div>
        </div>
      </div>
      {props.taskId && (
        <NewSessionDialog
          open={props.showDialog}
          onOpenChange={props.onShowDialog}
          taskId={props.taskId}
          workspaceId={props.workspaceId}
        />
      )}
      <ManagedCloneRelocationConfirmation
        open={relocationConfirmationOpen}
        targetKey={`${props.sessionId ?? ""}:${props.actions.managedCloneRecoveryStamp ?? ""}`}
        onOpenChange={setRelocationConfirmationOpen}
        onConfirm={() => props.actions.handleManagedCloneRelocation()}
        disabled={busyAction !== null}
      />
    </>
  );
}

function LocalStoppedSession(props: SessionStoppedBannerProps) {
  const actions = useSessionRecoveryActions({
    taskId: props.taskId ?? "",
    sessionId: props.sessionId ?? "",
  });
  return <StoppedSessionContent {...props} actions={actions} />;
}

export function SessionStoppedBanner(props: SessionStoppedBannerProps) {
  return props.recoveryActions ? (
    <StoppedSessionContent {...props} actions={props.recoveryActions} />
  ) : (
    <LocalStoppedSession {...props} />
  );
}
