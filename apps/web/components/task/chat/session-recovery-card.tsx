"use client";

import { useRef, useLayoutEffect } from "react";
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

export function SessionRecoveryCard({
  model,
  actions,
  onNewSession,
}: {
  model: ActiveSessionRecovery;
  actions: SessionRecoveryActions;
  onNewSession: () => void;
}) {
  const { t } = useTranslation();
  const ref = useRef<HTMLDivElement>(null);
  const profileExists = useSessionProfileExists(model.sessionId);
  const choices = useRecoveryChoices(model, actions, profileExists, onNewSession);
  const context = useTaskLaunchErrorContext();
  const { copy, busy, busyAction, details, failure } = useRecoveryPresentation(
    model,
    actions,
    context,
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
  return (
    <div
      ref={ref}
      id={`session-recovery-${model.sessionId}`}
      tabIndex={-1}
      data-testid="session-recovery-card"
      data-recovering={busyAction !== null}
      className="max-h-[50dvh] min-h-0 min-w-0 overflow-y-auto overscroll-contain rounded-md border border-amber-500/35 bg-card"
    >
      <div className="flex min-w-0 gap-3 border-b border-amber-500/20 bg-amber-500/5 p-3 dark:bg-amber-500/10 md:p-4">
        <IconAlertTriangle
          className="mt-0.5 size-4 shrink-0 text-amber-600 dark:text-amber-400"
          aria-hidden="true"
        />
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-medium">{copy.title}</h3>
          <p
            data-failed={Boolean(actions.recoveryError)}
            data-testid={actions.recoveryError ? "session-recovery-error" : undefined}
            role={actions.recoveryError || actions.recoveryNotice ? "status" : undefined}
            className="mt-1 wrap-anywhere text-sm text-muted-foreground data-[failed=true]:text-destructive"
          >
            {actions.recoveryError ? failure : (actions.recoveryNotice ?? copy.summary)}
          </p>
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
      <div className="min-w-0 bg-card px-3 pb-3 md:px-4 md:pb-4">
        <RecoveryActions
          actions={choices}
          busy={busy}
          busyAction={busyAction}
          preferred={preferredRecoveryAction(model, profileExists)}
          blocked={Boolean(actions.guardDetails && !actions.guardDetails.retryable)}
        />
        <AdditionalActions model={model} taskId={context?.taskId} />
        <div className="mt-3 min-w-0 border-t border-border pt-1 [&_pre]:bg-muted">
          <SessionErrorDetails>{details}</SessionErrorDetails>
        </div>
        <ActionMessageDetails
          metadata={model.metadata ? { ...model.metadata, error_output: undefined } : undefined}
        />
      </div>
    </div>
  );
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
