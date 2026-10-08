"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { Skeleton } from "@kandev/ui/skeleton";
import AppLink from "@/components/routing/app-link";
import type { ControlAction, ControlSetting } from "@/lib/api/domains/coordinator-api";
import { linkToCoordinatorActivityClass } from "@/lib/coordinator/links";
import { CONTROL_ACTIONS } from "@/lib/coordinators/control-draft";
import {
  useActionSummary,
  type ActionCounts,
} from "@/hooks/domains/coordinator/use-action-summary";
import type { useControlDraft } from "@/hooks/domains/coordinator/use-control-draft";

type Control = ReturnType<typeof useControlDraft>;

const ACTION_LABEL: Record<ControlAction, string> = {
  create_task: "coordinator:mayDoCreateTask",
  start_agent: "coordinator:mayDoStartAgent",
  message: "coordinator:mayDoMessage",
  move: "coordinator:mayDoMove",
  resume: "coordinator:mayDoResume",
  stop: "coordinator:mayDoStop",
};

type MayDoSectionProps = {
  workspaceId: string;
  coordinatorId: string;
  canManage: boolean;
  control: Control;
};

function Counts({
  action,
  counts,
  status,
}: {
  action: ControlAction;
  counts: ActionCounts | null;
  status: string;
}) {
  const { t } = useTranslation();
  if (status === "loading")
    return <Skeleton className="h-4 w-40" data-testid="may-do-counts-loading" />;
  const entry = counts?.[action];
  if (status === "error" || !entry) return null;
  if (entry.approved === 0 && entry.rejected === 0) {
    return (
      <span className="text-xs text-muted-foreground">{t("coordinator:mayDoNothingYet")}</span>
    );
  }
  return (
    <span className="text-xs text-muted-foreground" data-testid={`may-do-counts-${action}`}>
      {t("coordinator:mayDoCounts", { approved: entry.approved, rejected: entry.rejected })}
    </span>
  );
}

export type MayDoActivity = {
  workspaceId: string;
  coordinatorId: string;
  counts: ActionCounts | null;
  status: string;
};

type RowProps = {
  action: ControlAction;
  value: ControlSetting;
  disabled: boolean;
  onChange: (value: ControlSetting) => void;
  activity?: MayDoActivity;
  errorMessage?: string;
};

function ActionRow({ action, value, disabled, onChange, activity, errorMessage }: RowProps) {
  const { t } = useTranslation();
  const stopLocked = action === "stop";
  const name = `may-do-${action}`;
  return (
    <div className="space-y-2 py-3" data-testid={`may-do-row-${action}`}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="text-sm font-medium">{t(ACTION_LABEL[action])}</span>
        {activity && (
          <AppLink
            href={linkToCoordinatorActivityClass(
              activity.workspaceId,
              activity.coordinatorId,
              action,
            )}
            className="cursor-pointer text-xs underline"
            data-testid={`may-do-review-${action}`}
          >
            {t("coordinator:mayDoReviewLast30")}
          </AppLink>
        )}
      </div>
      <RadioGroup
        value={value === "automatic" ? "denied" : value}
        onValueChange={(next) => onChange(next as ControlSetting)}
        disabled={disabled}
        className="flex flex-wrap gap-4"
        aria-label={t(ACTION_LABEL[action])}
      >
        <label className="flex cursor-pointer items-center gap-2 text-sm">
          <RadioGroupItem value="denied" id={`${name}-denied`} />
          {t("coordinator:mayDoDenied")}
        </label>
        <label className="flex cursor-pointer items-center gap-2 text-sm">
          <RadioGroupItem value="requires_approval" id={`${name}-approval`} disabled={stopLocked} />
          {t("coordinator:mayDoRequiresApproval")}
        </label>
        <label className="flex items-center gap-2 text-sm text-muted-foreground">
          <RadioGroupItem value="automatic" id={`${name}-automatic`} disabled />
          {t("coordinator:mayDoAutomatic")}
          <span className="text-xs">{t("coordinator:mayDoAutomaticNote")}</span>
        </label>
      </RadioGroup>
      {stopLocked && (
        <p className="text-xs text-muted-foreground">{t("coordinator:mayDoStopUnavailable")}</p>
      )}
      {action === "start_agent" && value !== "denied" && (
        <p className="text-xs text-muted-foreground" data-testid="may-do-start-agent-note">
          {t("coordinator:mayDoStartAgentNote")}
        </p>
      )}
      {errorMessage && (
        <p role="alert" className="text-xs text-destructive" data-testid={`may-do-error-${action}`}>
          {errorMessage}
        </p>
      )}
      {activity && <Counts action={action} counts={activity.counts} status={activity.status} />}
    </div>
  );
}

export type MayDoRowsProps = {
  actions: Record<ControlAction, ControlSetting>;
  canManage: boolean;
  onChange: (action: ControlAction, value: ControlSetting) => void;
  activity?: MayDoActivity;
  rowErrors?: Partial<Record<ControlAction, string>>;
  errorMessage?: string | null;
  freshConversationNote?: boolean;
};

export function MayDoRows({
  actions,
  canManage,
  onChange,
  activity,
  rowErrors,
  errorMessage,
  freshConversationNote = false,
}: MayDoRowsProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2" data-testid="may-do-section">
      {errorMessage && (
        <p role="alert" className="text-sm text-destructive" data-testid="may-do-error">
          {errorMessage}
        </p>
      )}
      <div className="divide-y">
        {CONTROL_ACTIONS.map((action) => (
          <ActionRow
            key={action}
            action={action}
            value={actions[action]}
            disabled={!canManage}
            onChange={(next) => onChange(action, next)}
            activity={activity}
            errorMessage={rowErrors?.[action]}
          />
        ))}
      </div>
      <div className="space-y-1 pt-3 text-sm" data-testid="may-do-always-human">
        <p className="font-medium">{t("coordinator:mayDoAlwaysHuman")}</p>
        <p className="text-muted-foreground">{t("coordinator:mayDoMergePr")}</p>
        <p className="text-muted-foreground">{t("coordinator:mayDoMoveToDone")}</p>
      </div>
      {freshConversationNote && (
        <p className="text-xs text-muted-foreground">{t("coordinator:mayDoFreshConversation")}</p>
      )}
    </div>
  );
}

export function MayDoSection({
  workspaceId,
  coordinatorId,
  canManage,
  control,
}: MayDoSectionProps) {
  const { t } = useTranslation();
  const summary = useActionSummary(workspaceId, coordinatorId, 30);
  const { draft, status, retry } = control;

  if (status === "loading" || !draft) {
    if (status === "error") {
      return (
        <div className="flex items-center gap-2 text-sm" data-testid="may-do-load-failed">
          {t("coordinator:mayDoLoadFailed")}
          <Button variant="outline" size="sm" className="cursor-pointer" onClick={retry}>
            {t("coordinator:tryAgain")}
          </Button>
        </div>
      );
    }
    return <Skeleton className="h-48 w-full" data-testid="may-do-loading" />;
  }

  return (
    <div className="space-y-2">
      {summary.status === "error" && (
        <div className="flex items-center gap-2 text-sm" data-testid="may-do-summary-failed">
          {t("coordinator:mayDoSummaryFailed")}
          <Button variant="outline" size="sm" className="cursor-pointer" onClick={summary.retry}>
            {t("coordinator:tryAgain")}
          </Button>
        </div>
      )}
      <MayDoRows
        actions={draft.actions}
        canManage={canManage}
        onChange={control.setAction}
        activity={{ workspaceId, coordinatorId, counts: summary.counts, status: summary.status }}
        freshConversationNote
      />
    </div>
  );
}
