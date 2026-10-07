"use client";

import { useRef, useLayoutEffect, useState } from "react";
import { IconAlertTriangle } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import type { ActiveSessionRecovery } from "@/lib/active-session-recovery";
import type { SessionRecoveryActions } from "@/hooks/domains/session/use-session-recovery-actions";
import { RecoveryActions } from "../recovery-actions";
import { SessionErrorDetails } from "../session-error-details";
import { useSessionProfileExists } from "./session-stopped-banner";
import { ActionButtons } from "./messages/action-message-actions";
import { ActionMessageDetails } from "./messages/action-message-details";
import { useTaskLaunchErrorContext } from "../task-launch-error-context";
import { useRecoveryChoices, useRecoveryPresentation } from "./session-recovery-model";
import { sessionRecoveryAction } from "./messages/action-message-recovery";
import { ManagedCloneRelocationConfirmation } from "./managed-clone-relocation-confirmation";

export function SessionRecoveryCard({
  model,
  actions,
  onNewSession,
}: {
  model: ActiveSessionRecovery;
  actions: SessionRecoveryActions;
  onNewSession: () => void;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [relocationConfirmationOpen, setRelocationConfirmationOpen] = useState(false);
  const profileExists = useSessionProfileExists(model.sessionId);
  const projectionMatchesModel = workspaceRecoveryMatchesModel(model, actions.workspaceRecovery);
  const recoveryActions = actionsForRecoveryModel(actions, projectionMatchesModel);
  const choices = useRecoveryChoices(model, recoveryActions, profileExists, onNewSession, () =>
    setRelocationConfirmationOpen(true),
  );
  const context = useTaskLaunchErrorContext();
  const { copy, busy, busyAction, details, detailFields, failure } = useRecoveryPresentation(
    model,
    recoveryActions,
    context,
  );
  const workspaceRecoveryRelevant = isWorkspaceRecoveryRelevant(
    model,
    actions,
    projectionMatchesModel,
    choices,
  );
  const workspaceRecoveryForActions = workspaceRecoveryForProgress(
    actions.workspaceRecovery,
    projectionMatchesModel,
  );
  useLayoutEffect(() => {
    const node = ref.current;
    return () => {
      if (!node?.contains(document.activeElement)) return;
      const panel = node.closest('[data-testid="session-chat"]');
      requestAnimationFrame(() =>
        panel?.querySelector<HTMLElement>('[contenteditable="true"]')?.focus(),
      );
    };
  }, []);
  if (projectionMatchesModel && actions.workspaceRecovery?.agent_ready) return null;
  return (
    <div
      ref={ref}
      id={`session-recovery-${model.sessionId}`}
      tabIndex={-1}
      data-testid="session-recovery-card"
      data-recovering={busyAction !== null}
      className="max-h-[50dvh] min-h-0 min-w-0 overflow-y-auto overscroll-contain rounded-md border border-amber-500/35 bg-card"
    >
      <RecoveryCardHeader
        model={model}
        actions={actions}
        choices={choices}
        workspaceRecoveryRelevant={workspaceRecoveryRelevant}
        profileExists={profileExists}
        copy={copy}
        failure={failure}
      />
      <div className="min-w-0 bg-card px-3 pb-3 md:px-4 md:pb-4">
        <RecoveryActions
          actions={choices}
          busy={busy}
          busyAction={busyAction}
          preferred={preferredRecoveryAction(model, profileExists)}
          blocked={Boolean(actions.guardDetails && !actions.guardDetails.retryable)}
          workspaceRecovery={workspaceRecoveryForActions}
          workspaceRecoveryReadyApplies={projectionMatchesModel}
          workspaceRecoveryRepositoryName={actions.workspaceRecoveryRepositoryName}
          workspaceRecoveryStatusCheck={
            workspaceRecoveryRelevant ? actions.workspaceRecoveryStatusCheck : "idle"
          }
          onCheckWorkspaceRecoveryStatus={() => void actions.checkWorkspaceRecoveryStatus()}
        />
        <AdditionalActions model={model} taskId={context?.taskId} />
        <div className="mt-3 min-w-0 border-t border-border pt-1 [&_pre]:bg-muted">
          <SessionErrorDetails structuredFields={detailFields}>{details}</SessionErrorDetails>
        </div>
        <ActionMessageDetails
          metadata={model.metadata ? { ...model.metadata, error_output: undefined } : undefined}
        />
      </div>
      <ManagedCloneRelocationConfirmation
        open={relocationConfirmationOpen}
        targetKey={`${model.sessionId}:${actions.managedCloneRecoveryStamp ?? model.stamp ?? ""}`}
        onOpenChange={setRelocationConfirmationOpen}
        onConfirm={() => actions.handleManagedCloneRelocation()}
        disabled={actions.busyAction !== null}
      />
    </div>
  );
}

function workspaceRecoveryMatchesModel(
  model: ActiveSessionRecovery,
  projection: SessionRecoveryActions["workspaceRecovery"],
) {
  return Boolean(
    model.stamp &&
    projection?.session_id === model.sessionId &&
    projection.error_stamp === model.stamp,
  );
}

function actionsForRecoveryModel(
  actions: SessionRecoveryActions,
  projectionMatchesModel: boolean,
): SessionRecoveryActions {
  return projectionMatchesModel ? actions : { ...actions, workspaceRecovery: null };
}

function isWorkspaceRecoveryRelevant(
  model: ActiveSessionRecovery,
  actions: SessionRecoveryActions,
  projectionMatchesModel: boolean,
  choices: ReturnType<typeof useRecoveryChoices>,
) {
  return (
    model.kind === "managed_clone_relocation_required" ||
    Boolean(actions.managedCloneRecoveryStamp) ||
    projectionMatchesModel ||
    choices.some((choice) => choice.kind === "relocate_and_resume")
  );
}

function workspaceRecoveryForProgress(
  projection: SessionRecoveryActions["workspaceRecovery"],
  projectionMatchesModel: boolean,
) {
  return projectionMatchesModel || projection?.runner_live ? projection : null;
}

function RecoveryCardHeader({
  model,
  actions,
  choices,
  workspaceRecoveryRelevant,
  profileExists,
  copy,
  failure,
}: {
  model: ActiveSessionRecovery;
  actions: SessionRecoveryActions;
  choices: ReturnType<typeof useRecoveryChoices>;
  workspaceRecoveryRelevant: boolean;
  profileExists: boolean;
  copy: ReturnType<typeof useRecoveryPresentation>["copy"];
  failure: string;
}) {
  const { t } = useTranslation();
  const { summary, suppressSummary } = recoveryHeaderCopy(
    copy,
    actions,
    failure,
    workspaceRecoveryRelevant,
  );
  return (
    <div className="flex min-w-0 gap-3 border-b border-amber-500/20 bg-amber-500/5 p-3 dark:bg-amber-500/10 md:p-4">
      <IconAlertTriangle
        className="mt-0.5 size-4 shrink-0 text-amber-600 dark:text-amber-400"
        aria-hidden="true"
      />
      <div className="min-w-0 flex-1">
        <h3
          className="text-sm font-medium"
          aria-live={suppressSummary ? "polite" : undefined}
          aria-atomic={suppressSummary ? "true" : undefined}
        >
          {copy.title}
        </h3>
        {!suppressSummary && (
          <p
            data-failed={Boolean(actions.recoveryError)}
            data-testid={actions.recoveryError ? "session-recovery-error" : undefined}
            role={actions.recoveryError || actions.recoveryNotice ? "status" : undefined}
            className="mt-1 wrap-anywhere text-sm text-muted-foreground data-[failed=true]:text-destructive"
          >
            {summary}
          </p>
        )}
        <RecoveryStatusNotices copy={copy} />
        {model.kind === "provider_quota_limited" && model.metadata?.model_id && (
          <p className="mt-1 text-xs text-muted-foreground">
            {t("chat:providerQuotaModel", { model: model.metadata.model_id })}
          </p>
        )}
        {!profileExists && choices.length > 0 && (
          <p className="mt-1 text-xs text-muted-foreground">
            {t("task:agentProfileNoLongerExists")}
          </p>
        )}
      </div>
    </div>
  );
}

function RecoveryStatusNotices({
  copy,
}: {
  copy: ReturnType<typeof useRecoveryPresentation>["copy"];
}) {
  const { t } = useTranslation();
  return (
    <>
      {copy.noPromptSent ? (
        <p
          className="mt-1 break-words text-xs text-muted-foreground"
          data-testid="session-bootstrap-no-prompt"
        >
          {t("task:sessionBootstrapNoPromptSent")}
        </p>
      ) : null}
      {copy.workspaceStatus ? (
        <p
          className="mt-1 break-words text-xs text-muted-foreground"
          data-testid="session-recovery-workspace-status"
        >
          {copy.workspaceStatus}
        </p>
      ) : null}
      {copy.freshStartWarning ? (
        <p
          className="mt-1 break-words text-xs text-muted-foreground"
          data-testid="session-recovery-fresh-start-warning"
        >
          {t("task:sessionRecoveryFreshStartWarning")}
        </p>
      ) : null}
    </>
  );
}

function recoveryHeaderCopy(
  copy: ReturnType<typeof useRecoveryPresentation>["copy"],
  actions: SessionRecoveryActions,
  failure: string,
  workspaceRecoveryRelevant: boolean,
) {
  if ((actions.workspaceRecoveryStatusCheck ?? "idle") !== "idle") {
    return { summary: "", suppressSummary: true };
  }
  if (actions.workspaceRecovery && workspaceRecoveryRelevant) {
    return { summary: "", suppressSummary: true };
  }
  const hasDistinctGuidance = Boolean(
    actions.recoveryError || actions.branchDetails || actions.guardDetails,
  );
  let summary = actions.recoveryNotice ?? copy.summary;
  if (hasDistinctGuidance) summary = failure;
  if (copy.hasTypedSelectionCause) summary = copy.summary;
  return {
    summary,
    suppressSummary: !copy.showSummary && !hasDistinctGuidance,
  };
}

function AdditionalActions({ model, taskId }: { model: ActiveSessionRecovery; taskId?: string }) {
  if (model.kind !== "missing_pr_branch") return null;
  const actions =
    model.metadata?.actions?.filter(
      (action) =>
        !sessionRecoveryAction(action) &&
        action.type !== "archive_task" &&
        action.type !== "delete_task",
    ) ?? [];
  return <ActionButtons taskId={taskId} actions={actions} />;
}

function preferredRecoveryAction(model: ActiveSessionRecovery, profileExists: boolean) {
  if (!profileExists) return "fresh_start";
  return model.metadata?.actions?.map(sessionRecoveryAction).find((kind) => kind !== null);
}
