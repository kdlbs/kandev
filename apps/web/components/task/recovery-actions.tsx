"use client";

import { useId } from "react";
import { useTranslation } from "react-i18next";
import {
  IconPlayerPlay,
  IconPlus,
  IconFolder,
  IconRefresh,
  IconGitBranch,
  IconLoader2,
} from "@tabler/icons-react";
import { sanitizeSessionErrorDetails } from "@/lib/session-error-details";
import { cn } from "@kandev/ui/lib/utils";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import {
  selectPrimaryRecoveryAction,
  type RecoveryActionKind,
} from "@/lib/session-recovery-actions";
import type { WorkspaceRecoveryProjection } from "@/lib/types/http";
import type { WorkspaceRecoveryStatusCheck } from "@/hooks/domains/session/use-session-recovery-actions";
import { WorkspaceRecoveryProgress } from "./workspace-recovery-progress";

export type RecoveryChoice = {
  kind: RecoveryActionKind;
  label: string;
  onClick: () => void;
  testId?: string;
  disabled?: boolean;
  tooltip?: string;
  disclosure?: string;
};

type RecoveryActionsProps = {
  actions: RecoveryChoice[];
  busy?: boolean;
  blocked?: boolean;
  preferred?: RecoveryActionKind;
  busyAction?: RecoveryActionKind | null;
  workspaceRecovery?: WorkspaceRecoveryProjection | null;
  workspaceRecoveryReadyApplies?: boolean;
  workspaceRecoveryRepositoryName?: string | null;
  workspaceRecoveryStatusCheck?: WorkspaceRecoveryStatusCheck;
  onCheckWorkspaceRecoveryStatus?: () => void;
};

type RecoveryActionsView = {
  warnings: string[];
  disclosures: string[];
  primary?: RecoveryChoice;
  ordered: RecoveryChoice[];
  controlsBlocked: boolean;
  shouldHide: boolean;
};

function buildRecoveryActionsView({
  actions,
  blocked = false,
  preferred,
  workspaceRecovery,
  workspaceRecoveryReadyApplies = true,
  workspaceRecoveryStatusCheck = "idle",
}: RecoveryActionsProps): RecoveryActionsView {
  const warnings = [
    ...new Set(
      actions.flatMap((action) =>
        action.tooltip ? [sanitizeSessionErrorDetails(action.tooltip)] : [],
      ),
    ),
  ].filter(Boolean);
  const disclosures = [
    ...new Set(actions.flatMap((action) => (action.disclosure ? [action.disclosure] : []))),
  ];
  const primaryKind = selectPrimaryRecoveryAction(
    actions.filter((action) => !action.disabled).map((action) => action.kind),
    blocked,
  );
  const primary = actions.find(
    (action) => action.kind === (preferred ?? primaryKind) && !action.disabled,
  );
  const recoveryInProgress = Boolean(workspaceRecovery?.runner_live);
  const recoveryAlreadyReady = isRelocationAlreadyReady(
    workspaceRecovery,
    workspaceRecoveryReadyApplies,
    actions,
  );
  const statusUnresolved = workspaceRecoveryStatusCheck !== "idle";
  const controlsBlocked = blocked || recoveryInProgress || statusUnresolved || recoveryAlreadyReady;
  const ordered = primary ? [primary, ...actions.filter((action) => action !== primary)] : actions;
  return {
    warnings,
    disclosures,
    primary,
    ordered,
    controlsBlocked,
    shouldHide:
      recoveryAlreadyReady ||
      ((blocked || !actions.length) && !workspaceRecovery && !statusUnresolved),
  };
}

function isRelocationAlreadyReady(
  projection: WorkspaceRecoveryProjection | null | undefined,
  readyApplies: boolean,
  actions: RecoveryChoice[],
) {
  return (
    Boolean(projection?.agent_ready && readyApplies) &&
    actions.some((action) => action.kind === "relocate_and_resume")
  );
}

export function RecoveryActions(props: RecoveryActionsProps) {
  const warningId = useId();
  const view = buildRecoveryActionsView(props);
  if (view.shouldHide) return null;
  return (
    <div>
      <WorkspaceRecoveryProgress
        projection={props.workspaceRecovery}
        repositoryName={props.workspaceRecoveryRepositoryName}
        statusCheck={props.workspaceRecoveryStatusCheck}
        onCheckStatus={props.onCheckWorkspaceRecoveryStatus}
      />
      <RecoveryDisclosures disclosures={view.disclosures} />
      <RecoveryChoiceButtons
        actions={view.ordered}
        primary={view.primary}
        busy={props.busy}
        controlsBlocked={view.controlsBlocked}
        warningId={warningId}
      />
      <RecoveryWarnings id={warningId} warnings={view.warnings} />
      <RecoveryBusyStatus busy={props.busy ?? false} busyAction={props.busyAction} />
    </div>
  );
}

function RecoveryDisclosures({ disclosures }: { disclosures: string[] }) {
  return disclosures.map((disclosure) => (
    <p
      key={disclosure}
      data-testid="provider-restored-resume-disclosure"
      className="mt-2 max-w-prose break-words text-xs text-muted-foreground"
    >
      {disclosure}
    </p>
  ));
}

function RecoveryChoiceButtons({
  actions,
  primary,
  busy,
  controlsBlocked,
  warningId,
}: {
  actions: RecoveryChoice[];
  primary?: RecoveryChoice;
  busy?: boolean;
  controlsBlocked: boolean;
  warningId: string;
}) {
  return (
    <div
      className={cn(
        "mt-3 flex min-w-0 flex-col gap-2 md:flex-row md:flex-wrap md:items-center",
        controlsBlocked && "hidden",
      )}
      aria-busy={busy || undefined}
    >
      {actions.map((action) => (
        <Button
          key={action.kind}
          type="button"
          variant="outline"
          aria-label={action.label}
          aria-describedby={action.tooltip ? warningId : undefined}
          title={action.tooltip ? sanitizeSessionErrorDetails(action.tooltip) : undefined}
          data-recommended={action === primary}
          disabled={busy || action.disabled || controlsBlocked}
          onClick={action.onClick}
          data-testid={action.testId}
          className={controlSizingClassName(
            "standard",
            "h-auto min-h-7 w-full cursor-pointer gap-1.5 whitespace-normal py-0.5 md:w-auto",
          )}
        >
          <RecoveryActionIcon kind={action.kind} />
          {action.label}
        </Button>
      ))}
    </div>
  );
}

function RecoveryWarnings({ id, warnings }: { id: string; warnings: string[] }) {
  if (!warnings.length) return null;
  return (
    <p id={id} className="mt-2 wrap-anywhere text-xs text-muted-foreground">
      {warnings.join(" ")}
    </p>
  );
}

function RecoveryBusyStatus({
  busy,
  busyAction,
}: {
  busy: boolean;
  busyAction?: RecoveryActionKind | null;
}) {
  const { t } = useTranslation();
  return (
    <div
      role="status"
      className={cn(
        "flex items-center gap-2 text-xs text-muted-foreground",
        busy && "mt-2 min-h-5",
      )}
    >
      {busy && (
        <>
          <IconLoader2
            aria-hidden="true"
            className="size-3.5 animate-spin motion-reduce:animate-none"
          />
          {busyAction ? pendingLabel(busyAction, t) : t("task:workflowMovePreviewChecking")}
        </>
      )}
    </div>
  );
}

function RecoveryActionIcon({ kind }: { kind: RecoveryActionKind }) {
  const icons = {
    fresh_start: IconPlus,
    restore: IconFolder,
    runtime_retry: IconRefresh,
    resume_new_branch: IconGitBranch,
    relocate_and_resume: IconFolder,
    continue_from_history: IconPlayerPlay,
    retry_connection: IconRefresh,
    resume: IconPlayerPlay,
  };
  const Icon = icons[kind];
  return <Icon aria-hidden="true" className="size-3.5 shrink-0" />;
}

function pendingLabel(action: RecoveryActionKind, t: ReturnType<typeof useTranslation>["t"]) {
  if (action === "restore") return t("task:restoring");
  if (action === "fresh_start") return t("task:starting");
  if (action === "retry_connection") return t("task:retryingConnection");
  if (action === "runtime_retry") return t("task:retrying");
  return t("task:resuming");
}
