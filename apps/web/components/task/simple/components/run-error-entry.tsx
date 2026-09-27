"use client";

import { useSessionComposerRecovery } from "@/components/task/chat/session-recovery-context";
import { SessionErrorDetails } from "@/components/task/session-error-details";
import { useAppStore } from "@/components/state-provider";
import { selectOfficeAgentProfiles } from "@/lib/state/slices/office/selectors";
import { useSessionRecoveryActions } from "@/hooks/domains/session/use-session-recovery-actions";
import type { RunError } from "@/app/office/tasks/[id]/types";
import type { TaskRepository } from "@/lib/types/http";
import { ManagedRuntimeNpmRunError } from "./managed-runtime-npm-run-error";
import { isLaunchErrorCategory, TaskLaunchErrorEntry } from "./task-launch-error-entry";
import { useTranslation } from "react-i18next";

import { LegacyRunErrorEntry } from "./legacy-run-error-entry";

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

function composerOwnsRunError(error: RunError, stamp: string | undefined) {
  return error.isActive !== false && Boolean(error.errorStamp) && error.errorStamp === stamp;
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

  if (composerOwnsRunError(error, composerOwner?.model?.stamp))
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
