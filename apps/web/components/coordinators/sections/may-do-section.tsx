"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { Skeleton } from "@kandev/ui/skeleton";
import AppLink from "@/components/routing/app-link";
import type { ControlAction, ControlSetting } from "@/lib/api/domains/coordinator-api";
import type { ClassEligibility } from "@/lib/api/domains/coordinator-automatic-api";
import { linkToCoordinatorActivityClass } from "@/lib/coordinator/links";
import { CONTROL_ACTIONS } from "@/lib/coordinators/control-draft";
import {
  useActionSummary,
  type ActionCounts,
} from "@/hooks/domains/coordinator/use-action-summary";
import type { useControlDraft } from "@/hooks/domains/coordinator/use-control-draft";
import {
  useClassEligibility,
  type ClassEligibilityStatus,
} from "@/hooks/domains/coordinator/use-class-eligibility";
import { useReviewOpened } from "@/hooks/domains/coordinator/use-review-opened";
import { AutomaticEligibility, RaisedRecord } from "./automatic-eligibility";

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
  phase3?: boolean;
};

/** The phase 3 raise surface: eligibility of create_task and the review flow. */
export type MayDoAutomatic = {
  eligibility: ClassEligibility | null;
  status: ClassEligibilityStatus;
  reviewOpened: boolean;
  reviewFailed: boolean;
  onReviewOpened: () => void;
  onMarkReviewed: () => void;
  onRetry: () => void;
};

function automaticRadioState(
  action: ControlAction,
  value: ControlSetting,
  canManage: boolean,
  automatic: MayDoAutomatic | undefined,
): { disabled: boolean; noteKey: string } {
  if (!automatic) return { disabled: true, noteKey: "coordinator:mayDoAutomaticNote" };
  if (action !== "create_task") {
    return { disabled: true, noteKey: "coordinator:mayDoAutomaticCannot" };
  }
  const allowed = canManage && (value === "automatic" || automatic.eligibility?.eligible === true);
  return { disabled: !allowed, noteKey: "coordinator:mayDoAutomaticNotEligible" };
}

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
  automatic?: MayDoAutomatic;
};

function AutomaticOption({
  action,
  radio,
}: {
  action: ControlAction;
  radio: { disabled: boolean; noteKey: string };
}) {
  const { t } = useTranslation();
  return (
    <label
      className={`flex items-center gap-2 text-sm ${radio.disabled ? "text-muted-foreground" : "cursor-pointer"}`}
    >
      <RadioGroupItem
        value="automatic"
        id={`may-do-${action}-automatic`}
        disabled={radio.disabled}
      />
      {t("coordinator:mayDoAutomatic")}
      {radio.disabled && (
        <span className="text-xs" data-testid={`may-do-automatic-note-${action}`}>
          {t(radio.noteKey)}
        </span>
      )}
    </label>
  );
}

function ActionRow({
  action,
  value,
  disabled,
  onChange,
  activity,
  errorMessage,
  automatic,
}: RowProps) {
  const { t } = useTranslation();
  const stopLocked = action === "stop";
  const name = `may-do-${action}`;
  const radio = automaticRadioState(action, value, !disabled, automatic);
  const radioValue = value === "automatic" && !automatic ? "denied" : value;
  const onReviewClick = action === "create_task" ? automatic?.onReviewOpened : undefined;
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
            onClick={onReviewClick}
            data-testid={`may-do-review-${action}`}
          >
            {t("coordinator:mayDoReviewLast30")}
          </AppLink>
        )}
      </div>
      <RadioGroup
        value={radioValue}
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
        <AutomaticOption action={action} radio={radio} />
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
      {action === "create_task" && automatic && (
        <>
          {automatic.eligibility && <RaisedRecord eligibility={automatic.eligibility} />}
          <AutomaticEligibility
            eligibility={automatic.eligibility}
            status={automatic.status}
            canManage={!disabled}
            reviewOpened={automatic.reviewOpened}
            reviewFailed={automatic.reviewFailed}
            onMarkReviewed={automatic.onMarkReviewed}
            onRetry={automatic.onRetry}
          />
        </>
      )}
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
  automatic?: MayDoAutomatic;
};

export function MayDoRows({
  actions,
  canManage,
  onChange,
  activity,
  rowErrors,
  errorMessage,
  freshConversationNote = false,
  automatic,
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
            automatic={automatic}
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
  phase3 = false,
}: MayDoSectionProps) {
  const { t } = useTranslation();
  const summary = useActionSummary(workspaceId, coordinatorId, 30);
  const eligibility = useClassEligibility(workspaceId, coordinatorId, "create_task", phase3);
  const [reviewOpened, markReviewOpened] = useReviewOpened(coordinatorId, "create_task");
  const { draft, status, retry } = control;
  const automatic: MayDoAutomatic | undefined = phase3
    ? {
        eligibility: eligibility.eligibility,
        status: eligibility.status,
        reviewOpened,
        reviewFailed: eligibility.reviewFailed,
        onReviewOpened: markReviewOpened,
        onMarkReviewed: eligibility.markReviewed,
        onRetry: eligibility.reload,
      }
    : undefined;

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
        automatic={automatic}
      />
    </div>
  );
}
