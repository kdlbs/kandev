"use client";

import { useTranslation } from "react-i18next";
import { RecoveryActions, type RecoveryChoice } from "@/components/task/recovery-actions";
import { SessionErrorDetails } from "@/components/task/session-error-details";
import type { SessionRecoveryBusyAction } from "@/hooks/domains/session/use-session-recovery-actions";
import type { TaskStatusSummaryActiveError } from "@/lib/types/task-status-summary";
import {
  causeLabel,
  operationLabel,
  type RecoveryCardModel,
} from "./session-bootstrap-recovery-model";

type RecoveryCardCopy = {
  launchNeedsAttention: string;
  launchErrorNoChanges: string;
  sessionRecoveryDetails: string;
};

function BootstrapRecoveryActions({
  profileExists,
  busyAction,
  hasBranchRecovery,
  blocked,
  canRestore,
  onResume,
  onRestore,
  onFreshStart,
  onNewBranch,
}: {
  profileExists: boolean;
  busyAction: SessionRecoveryBusyAction;
  hasBranchRecovery: boolean;
  blocked: boolean;
  canRestore: boolean;
  onResume: () => void;
  onRestore: () => void;
  onFreshStart: () => void;
  onNewBranch: () => void;
}) {
  const { t } = useTranslation();
  const actions: RecoveryChoice[] = [
    {
      kind: "resume",
      label: t("task:resume"),
      onClick: onResume,
      disabled: !profileExists,
      testId: "recovery-resume-button",
    },
    {
      kind: "restore",
      label: t("task:restoreReadOnlyWorkspace"),
      onClick: onRestore,
      testId: "recovery-restore-workspace-button",
    },
    {
      kind: "fresh_start",
      label: t("task:startFreshSession"),
      onClick: onFreshStart,
      testId: "recovery-fresh-button",
    },
  ];
  if (hasBranchRecovery)
    actions.push({
      kind: "resume_new_branch",
      label: t("task:continueOnNewBranch"),
      onClick: onNewBranch,
      testId: "recovery-new-branch-button",
    });
  return (
    <RecoveryActions
      actions={canRestore ? actions : actions.filter((action) => action.kind !== "restore")}
      busy={busyAction !== null}
      busyAction={busyAction}
      blocked={blocked}
      preferred={!profileExists ? "fresh_start" : undefined}
    />
  );
}

export function RecoveryCardContent({
  model,
  error,
  profileExists,
  busyAction,
  hasBranchRecovery,
  blocked,
  canRestore,
  onResume,
  onRestore,
  onFreshStart,
  onNewBranch,
  copy,
  translate,
}: {
  model: RecoveryCardModel;
  error: TaskStatusSummaryActiveError;
  profileExists: boolean;
  busyAction: SessionRecoveryBusyAction;
  hasBranchRecovery: boolean;
  blocked: boolean;
  canRestore: boolean;
  onResume: () => void;
  onRestore: () => void;
  onFreshStart: () => void;
  onNewBranch: () => void;
  copy: RecoveryCardCopy;
  translate: (key: string) => string;
}) {
  const profileMissing = translate("task:agentProfileNoLongerExists");
  const details = [
    ...model.causes.map((cause) =>
      [operationLabel(cause.operation, translate), causeLabel(cause.code, translate), cause.detail]
        .filter(Boolean)
        .join("\n"),
    ),
    error.details,
  ]
    .filter(Boolean)
    .join("\n\n");
  return (
    <div className="min-w-0 flex-1">
      <div className="flex min-w-0 flex-wrap items-center gap-2">
        <span className="text-sm font-medium">{translate(model.titleKey)}</span>
        {!model.isReadOnly ? (
          <span className="text-xs text-muted-foreground">{copy.launchNeedsAttention}</span>
        ) : null}
      </div>
      <p className="mt-1 max-w-prose break-words text-sm text-muted-foreground">{model.summary}</p>
      <p className="mt-2 text-xs text-muted-foreground" data-testid="session-bootstrap-no-change">
        {copy.launchErrorNoChanges}
      </p>
      {!profileExists && <p className="mt-1 text-xs text-muted-foreground">{profileMissing}</p>}
      <BootstrapRecoveryActions
        profileExists={profileExists}
        busyAction={busyAction}
        hasBranchRecovery={hasBranchRecovery}
        blocked={blocked}
        canRestore={canRestore}
        onResume={onResume}
        onRestore={onRestore}
        onFreshStart={onFreshStart}
        onNewBranch={onNewBranch}
      />
      {model.hasDetails ? (
        <SessionErrorDetails
          testId="session-bootstrap-recovery-details"
          textTestId="session-bootstrap-cause-details"
          label={copy.sessionRecoveryDetails}
        >
          {details}
        </SessionErrorDetails>
      ) : null}
    </div>
  );
}
