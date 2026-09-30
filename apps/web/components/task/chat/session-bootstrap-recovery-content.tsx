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
  needsManagedCloneRelocation,
  providerRestoredResumeEligible,
  onResume,
  onRestore,
  onFreshStart,
  onNewBranch,
  onRelocate,
}: {
  profileExists: boolean;
  busyAction: SessionRecoveryBusyAction;
  hasBranchRecovery: boolean;
  blocked: boolean;
  canRestore: boolean;
  needsManagedCloneRelocation: boolean;
  providerRestoredResumeEligible: boolean;
  onResume: () => void;
  onRestore: () => void;
  onFreshStart: () => void;
  onNewBranch: () => void;
  onRelocate: () => void;
}) {
  const { t } = useTranslation();
  const actions: RecoveryChoice[] = needsManagedCloneRelocation
    ? [
        {
          kind: "relocate_and_resume",
          label: t("task:managedCloneRelocateResume"),
          onClick: onRelocate,
          testId: "managed-clone-relocate-button",
        },
      ]
    : [
        {
          kind: "resume",
          label: t("task:resume"),
          disclosure: providerRestoredResumeEligible
            ? t("task:providerRestoredResumeDisclosure")
            : undefined,
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
  if (!needsManagedCloneRelocation && hasBranchRecovery)
    actions.push({
      kind: "resume_new_branch",
      label: t("task:continueOnNewBranch"),
      onClick: onNewBranch,
      testId: "recovery-new-branch-button",
    });
  const preferred = preferredBootstrapRecoveryAction(needsManagedCloneRelocation, profileExists);
  return (
    <RecoveryActions
      actions={canRestore ? actions : actions.filter((action) => action.kind !== "restore")}
      busy={busyAction !== null}
      busyAction={busyAction}
      blocked={blocked}
      preferred={preferred}
    />
  );
}

function preferredBootstrapRecoveryAction(
  needsManagedCloneRelocation: boolean,
  profileExists: boolean,
) {
  if (needsManagedCloneRelocation) return "relocate_and_resume";
  if (!profileExists) return "fresh_start";
  return undefined;
}

export function RecoveryCardContent({
  model,
  error,
  profileExists,
  providerRestoredResumeEligible,
  busyAction,
  hasBranchRecovery,
  blocked,
  canRestore,
  needsManagedCloneRelocation,
  onResume,
  onRestore,
  onFreshStart,
  onNewBranch,
  onRelocate,
  copy,
  translate,
}: {
  model: RecoveryCardModel;
  error: TaskStatusSummaryActiveError;
  profileExists: boolean;
  providerRestoredResumeEligible: boolean;
  busyAction: SessionRecoveryBusyAction;
  hasBranchRecovery: boolean;
  blocked: boolean;
  canRestore: boolean;
  needsManagedCloneRelocation: boolean;
  onResume: () => void;
  onRestore: () => void;
  onFreshStart: () => void;
  onNewBranch: () => void;
  onRelocate: () => void;
  copy: RecoveryCardCopy;
  translate: (key: string) => string;
}) {
  const profileMissing = translate("task:agentProfileNoLongerExists");
  const title = needsManagedCloneRelocation
    ? translate("task:managedCloneRelocationTitle")
    : translate(model.titleKey);
  const summary = needsManagedCloneRelocation
    ? translate("task:managedCloneRelocationBody")
    : model.summary;
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
        <span className="text-sm font-medium">{title}</span>
        {!model.isReadOnly ? (
          <span className="text-xs text-muted-foreground">{copy.launchNeedsAttention}</span>
        ) : null}
      </div>
      {summary && (model.showSummary || needsManagedCloneRelocation) ? (
        <p className="mt-1 max-w-prose break-words text-sm text-muted-foreground">{summary}</p>
      ) : null}
      <p className="mt-2 text-xs text-muted-foreground" data-testid="session-bootstrap-no-change">
        {copy.launchErrorNoChanges}
      </p>
      {!profileExists && <p className="mt-1 text-xs text-muted-foreground">{profileMissing}</p>}
      <BootstrapRecoveryActions
        profileExists={profileExists}
        providerRestoredResumeEligible={providerRestoredResumeEligible}
        busyAction={busyAction}
        hasBranchRecovery={hasBranchRecovery}
        blocked={blocked}
        canRestore={canRestore}
        needsManagedCloneRelocation={needsManagedCloneRelocation}
        onResume={onResume}
        onRestore={onRestore}
        onFreshStart={onFreshStart}
        onNewBranch={onNewBranch}
        onRelocate={onRelocate}
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
