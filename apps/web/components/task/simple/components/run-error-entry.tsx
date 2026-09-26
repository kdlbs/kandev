"use client";

import { useSessionComposerRecovery } from "@/components/task/chat/session-recovery-context";
import { IconAlertTriangle } from "@tabler/icons-react";
import { RecoveryActions, type RecoveryChoice } from "@/components/task/recovery-actions";
import { sanitizeSessionErrorDetails } from "@/lib/session-error-details";
import { SessionErrorDetails } from "@/components/task/session-error-details";
import { useAppStore } from "@/components/state-provider";
import { selectOfficeAgentProfiles } from "@/lib/state/slices/office/selectors";
import { formatRelativeTime } from "@/lib/utils";
import { AgentAvatar } from "@/app/office/components/agent-avatar";
import { RemediationLink } from "@/components/task/remediation-link";
import { SessionRecoveryNotice } from "@/components/task/ensure-session-error";
import { useSessionRecoveryActions } from "@/hooks/domains/session/use-session-recovery-actions";
import type {
  BranchRecoveryDetails,
  SessionRecoveryAction,
} from "@/lib/services/session-recovery-service";
import type { RunError } from "@/app/office/tasks/[id]/types";
import type { TaskRepository } from "@/lib/types/http";
import { ManagedRuntimeNpmRunError } from "./managed-runtime-npm-run-error";
import { isLaunchErrorCategory, TaskLaunchErrorEntry } from "./task-launch-error-entry";
import { useTranslation } from "react-i18next";

type RunErrorEntryProps = {
  taskId: string;
  workspaceId?: string;
  repositories?: TaskRepository[];
  error: RunError;
};

function TypedRunLaunchErrorEntry({
  taskId,
  workspaceId,
  repositories,
  error,
  preview,
}: RunErrorEntryProps & { preview: string }) {
  return (
    <TaskLaunchErrorEntry
      taskId={taskId}
      workspaceId={workspaceId ?? ""}
      repositories={repositories}
      isActive={error.isActive !== false}
      error={{
        session_id: error.sessionId,
        task_repository_id: error.taskRepositoryId,
        stamp: error.errorStamp ?? "",
        occurred_at: error.failedAt,
        preview,
        details: error.failureDetails,
        category: error.failureCode,
        recovery_actions: error.recoveryActions,
      }}
    />
  );
}

type LegacyRunErrorProps = {
  agentName: string;
  error: RunError;
  isActive: boolean;
  onRecover: (action: SessionRecoveryAction) => Promise<boolean>;
  onRestore: () => void;
  onNewBranch: () => void;
  recoveryError: Error | null;
  recoveryNotice: string | null;
  branchDetails: BranchRecoveryDetails | null;
  busyAction: SessionRecoveryAction | "restore" | null;
  blocked: boolean;
  canRestore: boolean;
  failureLabel: string;
};

function LegacyRunErrorEntry({
  agentName,
  error,
  isActive,
  onRecover,
  onRestore,
  onNewBranch,
  recoveryError,
  recoveryNotice,
  branchDetails,
  busyAction,
  blocked,
  canRestore,
  failureLabel,
}: LegacyRunErrorProps) {
  const { t } = useTranslation();
  const actions: RecoveryChoice[] = [
    {
      kind: "resume",
      label: t("task:resumeSession"),
      testId: "run-error-resume-button",
      onClick: () => void onRecover("resume"),
    },
    {
      kind: "fresh_start",
      label: t("task:startFreshSession"),
      testId: "run-error-fresh-button",
      onClick: () => void onRecover("fresh_start"),
    },
  ];
  if (recoveryError && canRestore)
    actions.push({
      kind: "restore",
      label: t("task:restoreReadOnlyWorkspace"),
      testId: "run-error-restore-workspace-button",
      onClick: onRestore,
    });
  if (branchDetails)
    actions.push({
      kind: "resume_new_branch",
      label: t("task:continueOnNewBranch"),
      testId: "run-error-continue-new-branch-button",
      onClick: onNewBranch,
    });

  return (
    <div className="flex gap-3 py-3 border-b border-border/50">
      <AgentAvatar name={agentName} size="md" />
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 flex-wrap">
          <span className="font-medium text-sm">{agentName}</span>
          <span className="inline-flex items-center gap-1 text-xs text-red-600 dark:text-red-400">
            <IconAlertTriangle className="h-3.5 w-3.5" />
            {t("task:stoppedWithAnError")}
          </span>
          <span className="text-xs text-muted-foreground">
            {formatRelativeTime(error.failedAt)}
          </span>
        </div>
        <p className="mt-1 text-sm text-muted-foreground">{t("task:theAgentStoppedWithAnError")}</p>
        {isActive && recoveryError && (
          <div data-testid="run-error-recovery-error">
            <p role="status">
              {branchDetails || blocked
                ? sanitizeSessionErrorDetails(recoveryError.message, 240) || failureLabel
                : failureLabel}
            </p>
            <SessionErrorDetails>{recoveryError.message}</SessionErrorDetails>
          </div>
        )}
        {isActive && recoveryNotice && <SessionRecoveryNotice message={recoveryNotice} />}
        <RemediationLink url={error.remediationUrl} />
        {isActive && (
          <RecoveryActions
            actions={actions}
            busy={busyAction !== null}
            busyAction={busyAction}
            blocked={blocked}
          />
        )}
        {error.rawPayload && (
          <SessionErrorDetails label={t("task:showDetails")} textTestId="run-error-raw-payload">
            {error.rawPayload}
          </SessionErrorDetails>
        )}
      </div>
    </div>
  );
}

export function RunErrorEntry({
  taskId,
  workspaceId = "",
  repositories,
  error,
}: RunErrorEntryProps) {
  const { t } = useTranslation();
  const composerOwner = useSessionComposerRecovery(error.sessionId);
  const agentName = useAppStore(
    (s) =>
      selectOfficeAgentProfiles(s).find((a) => a.id === error.agentProfileId)?.name ??
      t("task:agent"),
  );
  const {
    busyAction,
    recoveryError,
    branchDetails,
    guardDetails,
    recoveryNotice,
    manualRecoveryFailure,
    handleRecover,
    handleRestore,
    handleNewBranch,
  } = useSessionRecoveryActions({
    taskId,
    sessionId: error.sessionId,
    errorStamp: error.errorStamp,
  });

  if (composerOwner?.model)
    return (
      <div className="min-w-0 py-3 text-xs text-muted-foreground">
        <p>{t("task:theAgentStoppedWithAnError")}</p>
        <SessionErrorDetails>{error.failureDetails ?? error.rawPayload}</SessionErrorDetails>
      </div>
    );

  if (isLaunchErrorCategory(error.failureCode) && error.errorStamp) {
    return (
      <TypedRunLaunchErrorEntry
        taskId={taskId}
        workspaceId={workspaceId}
        repositories={repositories}
        error={error}
        preview={error.message ?? t("task:launchErrorSessionPreview")}
      />
    );
  }

  if (
    error.failureCode === "managed_runtime_npm_resolution" ||
    error.failureCode === "managed_runtime_npm_policy"
  ) {
    return (
      <ManagedRuntimeNpmRunError
        error={error}
        agentName={agentName}
        onRetry={error.isActive === false ? undefined : () => void handleRecover("runtime_retry")}
      />
    );
  }

  return (
    <LegacyRunErrorEntry
      agentName={agentName}
      error={error}
      isActive={error.isActive !== false}
      onRecover={handleRecover}
      onRestore={() => void handleRestore()}
      onNewBranch={handleNewBranch}
      recoveryError={recoveryError}
      recoveryNotice={recoveryNotice}
      branchDetails={branchDetails}
      busyAction={busyAction}
      blocked={Boolean(guardDetails && !guardDetails.retryable)}
      canRestore={!guardDetails}
      failureLabel={
        manualRecoveryFailure?.operation === "restore_workspace"
          ? t("task:failedToRestoreWorkspace")
          : t("task:failedToResumeSession")
      }
    />
  );
}
