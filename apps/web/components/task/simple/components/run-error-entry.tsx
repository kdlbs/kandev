"use client";

import { useState } from "react";
import {
  IconAlertTriangle,
  IconChevronDown,
  IconPlayerPlay,
  IconRefresh,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { useAppStore } from "@/components/state-provider";
import { selectOfficeAgentProfiles } from "@/lib/state/slices/office/selectors";
import { formatRelativeTime } from "@/lib/utils";
import { AgentAvatar } from "@/app/office/components/agent-avatar";
import { RemediationLink } from "@/components/task/remediation-link";
import {
  EnsureSessionErrorBanner,
  SessionRecoveryNotice,
} from "@/components/task/ensure-session-error";
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
      error={{
        session_id: error.sessionId,
        task_repository_id: error.taskRepositoryId,
        stamp: error.errorStamp ?? "",
        occurred_at: error.failedAt,
        preview,
        category: error.failureCode,
        recovery_actions: error.recoveryActions,
      }}
    />
  );
}

function RunErrorRecoveryFeedback({
  workspaceId,
  recoveryError,
  recoveryNotice,
  branchDetails,
  busyAction,
  onRetry,
  onRestore,
  onNewBranch,
}: {
  workspaceId: string;
  recoveryError: Error | null;
  recoveryNotice: string | null;
  branchDetails: BranchRecoveryDetails | null;
  busyAction: SessionRecoveryAction | "restore" | null;
  onRetry: () => void;
  onRestore: () => void;
  onNewBranch: () => void;
}) {
  const { t } = useTranslation();
  if (!recoveryError && !recoveryNotice) return null;
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
                  testId: "run-error-continue-new-branch-button",
                  disabled: busyAction !== null,
                }
              : {
                  label: t("task:restoreReadOnlyWorkspace"),
                  onClick: onRestore,
                  testId: "run-error-restore-workspace-button",
                  disabled: busyAction !== null,
                }
          }
          secondaryAction={
            branchDetails
              ? {
                  label: t("task:restoreReadOnlyWorkspace"),
                  onClick: onRestore,
                  testId: "run-error-restore-workspace-button",
                  disabled: busyAction !== null,
                }
              : undefined
          }
          testId="run-error-recovery-error"
        />
      ) : null}
      {recoveryNotice ? <SessionRecoveryNotice message={recoveryNotice} /> : null}
    </>
  );
}

function LegacyRunErrorEntry({
  agentName,
  error,
  onRecover,
  onRetry,
  onRestore,
  onNewBranch,
  workspaceId,
  recoveryError,
  recoveryNotice,
  branchDetails,
  busyAction,
}: {
  agentName: string;
  error: RunError;
  onRecover: (action: SessionRecoveryAction) => Promise<boolean>;
  onRetry: () => void;
  onRestore: () => void;
  onNewBranch: () => void;
  workspaceId: string;
  recoveryError: Error | null;
  recoveryNotice: string | null;
  branchDetails: BranchRecoveryDetails | null;
  busyAction: SessionRecoveryAction | "restore" | null;
}) {
  const { t } = useTranslation();
  const [showDetails, setShowDetails] = useState(false);

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
        <RunErrorRecoveryFeedback
          workspaceId={workspaceId}
          recoveryError={recoveryError}
          recoveryNotice={recoveryNotice}
          branchDetails={branchDetails}
          busyAction={busyAction}
          onRetry={onRetry}
          onRestore={onRestore}
          onNewBranch={onNewBranch}
        />
        {error.rawPayload && (
          <Collapsible open={showDetails} onOpenChange={setShowDetails} className="mt-2">
            <CollapsibleTrigger className="flex items-center gap-1 text-xs text-muted-foreground cursor-pointer hover:text-foreground transition-colors">
              <IconChevronDown
                className={`h-3.5 w-3.5 transition-transform ${showDetails ? "rotate-180" : ""}`}
              />
              {t("task:showDetails")}
            </CollapsibleTrigger>
            <CollapsibleContent>
              <pre
                className="mt-1 text-[11px] font-mono text-muted-foreground bg-muted/50 rounded p-2 overflow-auto max-h-[300px] whitespace-pre-wrap break-words"
                data-testid="run-error-raw-payload"
              >
                {error.rawPayload}
              </pre>
            </CollapsibleContent>
          </Collapsible>
        )}
        <div className="mt-2 flex items-center gap-2 flex-wrap">
          <RemediationLink url={error.remediationUrl} />
          <Button
            variant="outline"
            size="sm"
            className="h-auto min-h-11 cursor-pointer gap-1.5 text-xs sm:min-h-8"
            onClick={() => onRecover("resume")}
            disabled={busyAction !== null}
            data-testid="run-error-resume-button"
          >
            <IconRefresh className="h-3 w-3" />
            {t("task:resumeSession")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="h-auto min-h-11 cursor-pointer gap-1.5 text-xs sm:min-h-8"
            onClick={() => onRecover("fresh_start")}
            disabled={busyAction !== null}
            data-testid="run-error-fresh-button"
          >
            <IconPlayerPlay className="h-3 w-3" />
            {t("task:startFreshSession")}
          </Button>
        </div>
      </div>
    </div>
  );
}

/**
 * Top-level chat entry rendered when an office session is in FAILED
 * state. Replaces the legacy red action-message banner: shows a short
 * generic header, a Show details collapsible exposing the verbatim
 * raw payload (for bug reports), and the Resume / Start fresh
 * buttons. Click handlers wire to the existing `session.recover` WS
 * request so the recovery semantics are unchanged.
 */
export function RunErrorEntry({
  taskId,
  workspaceId = "",
  repositories,
  error,
}: RunErrorEntryProps) {
  const { t } = useTranslation();
  const agentName = useAppStore(
    (s) =>
      selectOfficeAgentProfiles(s).find((a) => a.id === error.agentProfileId)?.name ??
      t("task:agent"),
  );
  const {
    busyAction,
    recoveryError,
    branchDetails,
    recoveryNotice,
    handleRecover,
    handleRestore,
    handleRetry,
    handleNewBranch,
  } = useSessionRecoveryActions({ taskId, sessionId: error.sessionId });

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

  if (error.failureCode === "managed_runtime_npm_resolution") {
    return (
      <ManagedRuntimeNpmRunError
        error={error}
        agentName={agentName}
        onRetry={() => void handleRecover("runtime_retry")}
      />
    );
  }

  return (
    <LegacyRunErrorEntry
      agentName={agentName}
      error={error}
      onRecover={handleRecover}
      onRetry={handleRetry}
      onRestore={() => void handleRestore()}
      onNewBranch={handleNewBranch}
      workspaceId={workspaceId}
      recoveryError={recoveryError}
      recoveryNotice={recoveryNotice}
      branchDetails={branchDetails}
      busyAction={busyAction}
    />
  );
}
