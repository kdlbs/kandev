"use client";

import { IconAlertTriangle } from "@tabler/icons-react";
import { RecoveryActions, type RecoveryChoice } from "@/components/task/recovery-actions";
import { sanitizeSessionErrorDetails } from "@/lib/session-error-details";
import { formatRelativeTime } from "@/lib/utils";
import { AgentAvatar } from "@/app/office/components/agent-avatar";
import { RemediationLink } from "@/components/task/remediation-link";
import { SessionRecoveryNotice } from "@/components/task/ensure-session-error";
import type {
  BranchRecoveryDetails,
  SessionRecoveryAction,
} from "@/lib/services/session-recovery-service";
import { SessionErrorDetails } from "@/components/task/session-error-details";
import type { RunError } from "@/app/office/tasks/[id]/types";
import { useTranslation } from "react-i18next";

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

export function LegacyRunErrorEntry({
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
